package ui

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/scripts"
	"github.com/thereisnotime/agentpad/internal/settings"
)

// newScriptsUI is an app UI on a temp settings.yaml; UI work queues on q.
func newScriptsUI(t *testing.T) (*appUI, chan func()) {
	t.Helper()
	fs, q := newFileSettings(t)
	a := test.NewTempApp(t)
	dir := t.TempDir()
	u := newAppUI(a, Deps{
		Writer:   &pad.ToolWriter{Path: filepath.Join(dir, "no-such-tool")},
		Settings: fs,
		Version:  "test",
		LogPath:  filepath.Join(dir, "events.jsonl"),
		Addr:     "127.0.0.1:0",
		Live:     noLive(),
		Jumper:   &fakeJumper{},
		Notifier: func(func(string)) (NotifyBackend, error) { return &fakeNotifier{}, nil },
	})
	u.post = func(f func()) { q <- f }
	u.ledFollow.Store(false)
	t.Cleanup(u.shutdown)
	return u, q
}

func echoScript(s string) scripts.Script {
	return scripts.Script{Run: []string{"/bin/sh", "-c", "echo " + s + " $AGENTPAD_EVENT $AGENTPAD_AGENT $AGENTPAD_TOOL"}}
}

func TestHooksRunFromAgentEvents(t *testing.T) {
	u, q := newScriptsUI(t)
	err := u.updateScripts(func(s *settings.Settings) {
		s.Scripts = map[string]scripts.Script{"att": echoScript("att"), "busy": echoScript("busy"),
			"start": echoScript("start"), "end": echoScript("end")}
		s.Hooks = []scripts.Hook{
			{On: scripts.EventAttention, Agent: "claude", Script: "att"},
			{On: scripts.EventBusy, Script: "busy"},
			{On: scripts.EventSessionStart, Script: "start"},
			{On: scripts.EventSessionEnd, Script: "end"},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, e := range []events.Event{
		{Time: now, Agent: "claude", SessionID: "s1", Kind: events.KindSessionStart},
		{Time: now, Agent: "claude", SessionID: "s1", Kind: events.KindToolPending, Tool: "Bash"},
		{Time: now, Agent: "claude", SessionID: "s1", Kind: events.KindToolPending, Tool: "Bash"}, // no transition
		{Time: now, Agent: "codex", SessionID: "c1", Kind: events.KindToolPending, Tool: "shell"}, // filtered out
	} {
		u.onEvent(e)
	}
	u.runner.Wait()
	u.onEvent(events.Event{Time: now, Agent: "claude", SessionID: "s1", Kind: events.KindPrompt})
	u.runner.Wait()
	u.onEvent(events.Event{Time: now, Agent: "claude", SessionID: "s1", Kind: events.KindSessionEnd})
	u.runner.Wait()
	drain(t, q, 50*time.Millisecond, func() bool { return false })

	var got []string
	for _, r := range slices.Backward(u.runner.Runs()) {
		if r.Status != scripts.StatusOK {
			t.Errorf("run %+v", r)
		}
		got = append(got, strings.TrimSpace(r.Stdout))
	}
	want := []string{"start session_start claude", "att attention claude Bash", "busy busy claude Bash", "end session_end claude Bash"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("runs:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if u.scripts.runsBox.Objects == nil || len(u.scripts.runsBox.Objects) != 5 { // header + 4
		t.Errorf("last runs rows = %d", len(u.scripts.runsBox.Objects))
	}
}

func TestRunNowToastAndScriptsScreen(t *testing.T) {
	u, q := newScriptsUI(t)
	if err := u.updateScripts(func(s *settings.Settings) {
		s.Scripts = map[string]scripts.Script{"hello": {Run: []string{"/bin/echo", "hi there"}}}
	}); err != nil {
		t.Fatal(err)
	}
	if len(u.scripts.list.Objects) != 1 || !slices.Contains(u.scripts.hookScrpt.Options, "hello") {
		t.Fatalf("script list %d, hook options %v", len(u.scripts.list.Objects), u.scripts.hookScrpt.Options)
	}
	if !slices.Contains(u.tray.state().Scripts, "hello") {
		t.Error("tray does not list the script")
	}
	u.runScriptNow("hello")
	if !drain(t, q, 5*time.Second, func() bool { return len(u.runNow) == 0 }) {
		t.Fatal("run now never finished")
	}
	if got := u.toasts.label.Text; got != "hello finished (exit 0, "+durationText(u.runner.Runs()[0].Duration)+"). hi there" {
		t.Errorf("toast %q", got)
	}
	// editor: rename keeps hooks pointing at the script
	if err := u.updateScripts(func(s *settings.Settings) {
		s.Hooks = []scripts.Hook{{On: scripts.EventJump, Script: "hello"}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := u.scripts.save("hello", scriptForm{Name: "greet", Command: "/bin/echo 'hi there'"}); err != nil {
		t.Fatal(err)
	}
	s := u.files.Get()
	if s.HasScript("hello") || !slices.Equal(s.Scripts["greet"].Run, []string{"/bin/echo", "hi there"}) || s.Hooks[0].Script != "greet" {
		t.Errorf("after rename: %+v", s)
	}
	if err := u.scripts.save("", scriptForm{Name: "greet", Command: "x"}); err == nil {
		t.Error("duplicate name accepted")
	}
}

func TestRunActionNeedsDefinedScript(t *testing.T) {
	u, _ := newScriptsUI(t)
	p := profile.Profile{Name: "x", Keys: [3]string{"run:deploy", "a", "b"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"}}
	if err := profile.Validate(p); err == nil || !strings.Contains(err.Error(), `unknown script "deploy"`) {
		t.Fatalf("err = %v", err)
	}
	if err := u.updateScripts(func(s *settings.Settings) {
		s.Scripts = map[string]scripts.Script{"deploy": {Shell: "true"}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Validate(p); err != nil {
		t.Fatal(err)
	}
	if n := len(u.profiles.pickRow.Objects); n != len(quickPicks)+1 {
		t.Errorf("quick picks = %d, want a run:deploy chip", n)
	}
	if u.profiles.runBtn.Disabled() {
		t.Error("Run script ▸ disabled with a script defined")
	}
}

type scriptCalls struct {
	mu    sync.Mutex
	names []string
	trigs []scriptTrigger
}

func (s *scriptCalls) run(name string, t scriptTrigger) {
	s.mu.Lock()
	s.names = append(s.names, name)
	s.trigs = append(s.trigs, t)
	s.mu.Unlock()
}

func (s *scriptCalls) n() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.names) }

func TestPlainModeRunsScriptOnRightLayer(t *testing.T) {
	fp := newFakePad()
	calls := &scriptCalls{}
	var inputs []string
	var mu sync.Mutex
	c := newLiveController(fp.devices(), (&uploads{}).upload, liveCallbacks{Script: calls.run,
		Input: func(ev PadInputEvent) { mu.Lock(); inputs = append(inputs, ev.Control); mu.Unlock() }})
	defer func() { _ = c.Close(nil) }()
	l1 := profile.Profile{Name: "one", Keys: [3]string{"run:a", "x", "y"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"}}
	l2 := profile.Profile{Name: "two", Keys: [3]string{"run:b", "x", "z"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"}}
	c.SetLayers([]profile.Profile{l1, l2})
	if _, err := c.SetProfile(l1, profile.OrientNormal); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureObserver(); err != nil {
		t.Fatal(err)
	}
	const f19, f20 = 189, 190
	fp.src.ch <- rawKey{f19, true}
	fp.src.ch <- rawKey{f19, false}
	waitFor(t, "layer 1 script", func() bool { return calls.n() == 1 })
	fp.src.ch <- rawKey{f20, true} // only layer 2 sends F20
	fp.src.ch <- rawKey{f20, false}
	waitFor(t, "layer 2 script", func() bool { return calls.n() == 2 })
	calls.mu.Lock()
	defer calls.mu.Unlock()
	if !slices.Equal(calls.names, []string{"a", "b"}) || calls.trigs[0] != (scriptTrigger{Control: "key1", Layer: 1}) || calls.trigs[1].Layer != 2 {
		t.Errorf("calls %v %+v", calls.names, calls.trigs)
	}
	if padTrigger(calls.trigs[1]) != "key 1 · layer 2" {
		t.Errorf("trigger text %q", padTrigger(calls.trigs[1]))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(inputs) != 4 {
		t.Errorf("presses still animate the pad: %v", inputs)
	}
}

func TestGestureModeRunsScript(t *testing.T) {
	fp := newFakePad()
	calls := &scriptCalls{}
	c := newLiveController(fp.devices(), (&uploads{}).upload, liveCallbacks{Script: calls.run})
	p := profile.Profile{Name: "g", Keys: [3]string{"1", "2", "3"}, Knob: profile.Knob{CCW: "up", Press: "enter", CW: "down"},
		Gestures: map[string]map[string]string{"key2": {"hold": "run:mic-off"}}}
	_, _ = c.SetProfile(p, profile.OrientNormal)
	cfg := gesture.DefaultConfig()
	cfg.Hold, cfg.LongHold = 10*time.Millisecond, time.Second
	if _, err := c.EnterGesture(cfg); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(nil) }()
	fp.src.ch <- rawKey{gesture.KeyF14, true}
	time.Sleep(40 * time.Millisecond)
	fp.src.ch <- rawKey{gesture.KeyF14, false}
	waitFor(t, "hold script", func() bool { return calls.n() == 1 })
	if calls.names[0] != "mic-off" || calls.trigs[0] != (scriptTrigger{Control: "key2", Gesture: "hold"}) {
		t.Errorf("calls %v %+v", calls.names, calls.trigs)
	}
	time.Sleep(20 * time.Millisecond)
	if typed := fp.emitters[0].typed; len(typed) != 0 {
		t.Errorf("run: action was typed: %q", typed)
	}
}

func TestScriptForm(t *testing.T) {
	argv, err := splitArgs(`curl -d "Claude \"waits\"" 'https://ntfy.sh/my topic' a\ b`)
	if err != nil || !slices.Equal(argv, []string{"curl", "-d", `Claude "waits"`, "https://ntfy.sh/my topic", "a b"}) {
		t.Fatalf("argv %q %v", argv, err)
	}
	if _, err := splitArgs(`echo "open`); err == nil {
		t.Error("unterminated quote accepted")
	}
	s := scripts.Script{Run: []string{"notify-send", "agent waits", "it's $HOME"}, Cwd: "~/x",
		Env: map[string]string{"B": "2", "A": "1=1"}, Timeout: 90 * time.Second}
	f := formFromScript("ping", s)
	if f.Env != "A=1=1\nB=2" || f.Timeout != "1m30s" || f.Shell {
		t.Errorf("form %+v", f)
	}
	name, back, err := f.script()
	if err != nil || name != "ping" || !slices.Equal(back.Run, s.Run) || back.Env["A"] != "1=1" || back.Timeout != s.Timeout || back.Cwd != "~/x" {
		t.Errorf("round trip %q %+v %v", name, back, err)
	}
	for _, bad := range []scriptForm{
		{Name: "Bad", Command: "x"},
		{Name: "x", Command: ""},
		{Name: "x", Command: "a", Env: "novalue"},
		{Name: "x", Command: "a", Timeout: "soon"},
		{Name: "x", Command: "a", Timeout: "1h"},
	} {
		if _, _, err := bad.script(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, sh, err := (scriptForm{Name: "m", Command: "pactl set-source-mute @DEFAULT_SOURCE@ toggle | cat", Shell: true}).script(); err != nil || sh.Shell == "" || sh.Run != nil {
		t.Errorf("shell %+v %v", sh, err)
	}
}

func TestScriptModels(t *testing.T) {
	m := trayModel(trayState{Orient: profile.OrientNormal, Scripts: []string{"deploy", "mic"}})
	var run *trayItem
	for i := range m {
		if m[i].Label == "Run script" {
			run = &m[i]
		}
	}
	if run == nil || len(run.Children) != 2 || run.Children[1].Arg != "mic" || run.Children[1].Action != trayScript {
		t.Fatalf("tray %+v", run)
	}
	for _, it := range trayModel(trayState{Orient: profile.OrientNormal}) {
		if it.Label == "Run script" {
			t.Error("Run script shown without scripts")
		}
	}
	picks := inspectorPicks([]string{"deploy"})
	if picks[len(picks)-1] != "run:deploy" || len(picks) != len(quickPicks)+1 {
		t.Errorf("picks %v", picks)
	}
	for _, tc := range []struct {
		r    scripts.Run
		want string
	}{
		{scripts.Run{Status: scripts.StatusOK}, "exit 0"},
		{scripts.Run{Status: scripts.StatusFailed, ExitCode: 2}, "exit 2"},
		{scripts.Run{Status: scripts.StatusTimeout, Err: "killed after 30s"}, "timeout"},
		{scripts.Run{Status: scripts.StatusSkipped}, "skipped"},
	} {
		if got := runResult(tc.r); got != tc.want {
			t.Errorf("%+v: %q", tc.r, got)
		}
	}
	if durationText(1500*time.Millisecond) != "1.5 s" || durationText(12*time.Millisecond) != "12 ms" {
		t.Error("durationText")
	}
	for _, ev := range scripts.Events() {
		if eventFromLabel(eventLabel(ev)) != ev {
			t.Errorf("event label round trip %s", ev)
		}
	}
}

func TestImportConsentModel(t *testing.T) {
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	doc := "kind: agentpad-bundle\nschema: 1\nsettings:\n  ui: {theme: dark}\n  scripts:\n    a: {shell: \"true\"}\n    b: {run: [echo, b]}\n  hooks:\n    - {on: idle, script: b}\n"
	b, err := settings.ParseBundle(bytes.NewReader([]byte(doc)))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.PlanImport(b, settings.Merge)
	if err != nil {
		t.Fatal(err)
	}
	rows := consentRows(plan)
	if len(rows) != 3 {
		t.Fatalf("consent rows %+v", rows)
	}
	for _, g := range planPreview(plan) {
		for _, r := range g.Rows {
			if strings.Contains(r.Text, "script") || strings.Contains(r.Text, "hook") {
				t.Errorf("dangerous change outside the consent list: %+v", r)
			}
		}
	}
	// nothing ticked: theme only
	if _, err := st.Apply(consentPlan(plan, map[string]bool{})); err != nil {
		t.Fatal(err)
	}
	if got := st.Get(); got.UI.Theme != settings.ThemeDark || len(got.Scripts) != 0 || len(got.Hooks) != 0 {
		t.Errorf("after import: %+v", got)
	}
	// tick b and its hook
	plan, _ = st.PlanImport(b, settings.Merge)
	ticked := map[string]bool{}
	for _, r := range consentRows(plan) {
		if strings.Contains(r.Change.Path, "b") {
			ticked[consentKey(r.Change)] = true
		}
	}
	if _, err := st.Apply(consentPlan(plan, ticked)); err != nil {
		t.Fatal(err)
	}
	if got := st.Get(); !got.HasScript("b") || got.HasScript("a") || len(got.Hooks) != 1 {
		t.Errorf("after ticking b: %+v", got)
	}
}
