package jump

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
)

// fakeRunner answers commands by their joined argv.
type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	reply func(cmd string) (string, error)
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, cmd)
	f.mu.Unlock()
	if f.reply == nil {
		return nil, nil
	}
	out, err := f.reply(cmd)
	return []byte(out), err
}

type busCall struct {
	dest, path, method string
	args               []any
}

type fakeBus struct {
	mu      sync.Mutex
	calls   []busCall
	replies map[string][]any // by method
	errs    map[string]error
	script  string
	result  []any // sent on the callback channel when the script starts
	cbErr   error
	ch      chan []any
	closed  bool
}

func (b *fakeBus) Call(_ context.Context, dest, path, method string, args ...any) ([]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, busCall{dest, path, method, args})
	if method == kwinLoad {
		data, err := os.ReadFile(args[0].(string))
		if err != nil {
			return nil, err
		}
		b.script = string(data)
	}
	if method == kwinStart && b.result != nil && b.ch != nil {
		b.ch <- b.result
	}
	if err := b.errs[method]; err != nil {
		return nil, err
	}
	return b.replies[method], nil
}

func (b *fakeBus) Callback(string) (string, string, <-chan []any, func(), error) {
	if b.cbErr != nil {
		return "", "", nil, nil, b.cbErr
	}
	b.ch = make(chan []any, 1)
	return ":1.99", callbackPath, b.ch, func() {}, nil
}

func (b *fakeBus) Close() error { b.closed = true; return nil }

func (b *fakeBus) methods() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, c := range b.calls {
		out = append(out, c.method)
	}
	return out
}

func (b *fakeBus) find(method string) (busCall, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.calls {
		if c.method == method {
			return c, true
		}
	}
	return busCall{}, false
}

type fakeX11 struct {
	win  uint32
	pids []int
	err  error
}

func (x *fakeX11) Activate(win uint32, pids []int) (uint32, error) {
	x.win, x.pids = win, pids
	if x.err != nil {
		return 0, x.err
	}
	if win == 0 {
		return 77, nil
	}
	return win, nil
}

func env(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

var kdeWayland = env(map[string]string{"XDG_SESSION_TYPE": "wayland", "XDG_CURRENT_DESKTOP": "KDE", "WAYLAND_DISPLAY": "wayland-0"})

func newTestJumper(t *testing.T, r *fakeRunner, b *fakeBus) *Jumper {
	t.Helper()
	return &Jumper{
		Run:        r.run,
		Bus:        b,
		Getenv:     kdeWayland,
		Ancestors:  func(pid int) []int { return []int{pid, pid - 1, 1} },
		ScriptDir:  t.TempDir(),
		ScriptWait: 50 * time.Millisecond,
	}
}

func session(o events.Origin) events.Session {
	return events.Session{Agent: "claude", SessionID: "s1", State: events.StateAttention, Origin: o}
}

const zellijPanes = `[
 {"id":0,"is_plugin":true,"tab_id":0},
 {"id":0,"is_plugin":false,"tab_id":0},
 {"id":2,"is_plugin":false,"tab_id":1},
 {"id":3,"is_plugin":false,"tab_id":1},
 {"id":3,"is_plugin":true,"tab_id":1}
]`

func TestJumpFullKonsoleZellijKWin(t *testing.T) {
	clientPane := "terminal_0"
	r := &fakeRunner{reply: func(cmd string) (string, error) {
		switch {
		case strings.HasSuffix(cmd, "list-panes --json --tab"):
			return zellijPanes, nil
		case strings.HasSuffix(cmd, "go-to-tab-by-id 1"):
			clientPane = "terminal_2"
		case strings.HasSuffix(cmd, "focus-next-pane"):
			clientPane = "terminal_3"
		case strings.HasSuffix(cmd, "list-clients"):
			return "CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n1         " + clientPane + "     N/A\n", nil
		}
		return "", nil
	}}
	b := &fakeBus{
		replies: map[string][]any{dbusPID: {uint32(7222)}, konsoleTitle: {"agentpad"}, kwinLoad: {int32(3)}},
		result:  []any{"tok", true, "agentpad — Konsole"},
	}
	j := newTestJumper(t, r, b)
	rep := j.Jump(context.Background(), session(events.Origin{
		AgentPID: 400, KonsoleService: ":1.80", KonsoleWindow: "/Windows/1", KonsoleSession: "/Sessions/26",
		ZellijSession: "main", ZellijPane: "3",
	}))
	if !rep.OK() || len(rep.Steps) != 3 {
		t.Fatalf("report:\n%s", rep)
	}
	wantCmds := []string{
		"zellij --session main action list-panes --json --tab",
		"zellij --session main action go-to-tab-by-id 1",
		"zellij --session main action focus-pane-id terminal_3",
		"zellij --session main action list-clients",
		"zellij --session main action focus-next-pane",
		"zellij --session main action list-clients",
	}
	if strings.Join(r.calls, "\n") != strings.Join(wantCmds, "\n") {
		t.Fatalf("commands:\n%s", strings.Join(r.calls, "\n"))
	}
	if s, _ := rep.Step(StepMultiplexer); s.Backend != "zellij" || !strings.Contains(s.Reason, "terminal_3 in tab 1") {
		t.Fatalf("zellij step %+v", s)
	}

	set, ok := b.find(konsoleSetCurrent)
	if !ok || set.dest != ":1.80" || set.path != "/Windows/1" || len(set.args) != 1 || set.args[0] != int32(26) {
		t.Fatalf("setCurrentSession call %+v", set)
	}
	if c, _ := b.find(dbusPID); c.args[0] != ":1.80" {
		t.Fatalf("pid lookup %+v", c)
	}
	wantOrder := []string{dbusPID, konsoleSetCurrent, konsoleTitle, kwinLoad, kwinStart, kwinUnload}
	if strings.Join(b.methods(), ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("bus calls %v", b.methods())
	}
	for _, want := range []string{`var pids = [7222,400,399], hint = "agentpad"`, `callDBus(":1.99", "/dev/thereisnotime/agentpad/Jump", "dev.thereisnotime.agentpad.Jump", "Result"`, "workspace.activeWindow = best"} {
		if !strings.Contains(b.script, want) {
			t.Errorf("script missing %q:\n%s", want, b.script)
		}
	}
	load, _ := b.find(kwinLoad)
	unload, _ := b.find(kwinUnload)
	if load.args[1] != unload.args[0] || load.dest != kwinService || load.path != kwinPath {
		t.Fatalf("load %+v unload %+v", load, unload)
	}
	if _, err := os.Stat(load.args[0].(string)); !os.IsNotExist(err) {
		t.Fatal("script file left behind")
	}
	if s, _ := rep.Step(StepWindow); s.Status != StatusDone || !strings.Contains(s.Reason, "agentpad — Konsole") {
		t.Fatalf("window step %+v", s)
	}
}

func TestJumpPartialInformation(t *testing.T) {
	r := &fakeRunner{}
	b := &fakeBus{replies: map[string][]any{dbusPID: {uint32(50)}, kwinLoad: {int32(0)}}, result: []any{"t", true, "x"}}
	j := newTestJumper(t, r, b)
	rep := j.Jump(context.Background(), session(events.Origin{KonsoleService: ":1.5", KonsoleWindow: "/Windows/2", KonsoleSession: "/Sessions/3"}))
	if len(r.calls) != 0 {
		t.Fatalf("unexpected commands %v", r.calls)
	}
	mux, _ := rep.Step(StepMultiplexer)
	tab, _ := rep.Step(StepTerminalTab)
	win, _ := rep.Step(StepWindow)
	if mux.Status != StatusSkipped || !strings.Contains(mux.Reason, "no zellij or tmux") {
		t.Fatalf("mux %+v", mux)
	}
	if tab.Status != StatusDone || win.Status != StatusDone || !rep.OK() {
		t.Fatalf("report:\n%s", rep)
	}
	if !strings.Contains(b.script, "var pids = [50]") {
		t.Fatalf("script %s", b.script)
	}
}

func TestJumpNothingKnown(t *testing.T) {
	r := &fakeRunner{}
	b := &fakeBus{}
	j := newTestJumper(t, r, b)
	rep := j.Jump(context.Background(), session(events.Origin{}))
	for _, s := range rep.Steps {
		if s.Status != StatusSkipped {
			t.Fatalf("step %+v", s)
		}
	}
	if rep.OK() || len(b.calls) != 0 || len(r.calls) != 0 {
		t.Fatalf("report %s calls %v %v", rep, b.calls, r.calls)
	}
	if !strings.Contains(rep.String(), "not a Konsole terminal") || !strings.Contains(rep.String(), "no terminal or agent pid") {
		t.Fatalf("reasons:\n%s", rep)
	}
}

func TestKonsoleSkipReasons(t *testing.T) {
	tests := []struct {
		o    events.Origin
		want string
	}{
		{events.Origin{KonsoleSession: "/Sessions/1"}, "lacks KONSOLE_DBUS_SERVICE"},
		{events.Origin{KonsoleService: ":1.2", KonsoleWindow: "/Windows/1", KonsoleSession: "bogus"}, "unusable KONSOLE_DBUS_SESSION"},
	}
	for _, tc := range tests {
		b := &fakeBus{}
		j := newTestJumper(t, &fakeRunner{}, b)
		step, _, _ := j.konsoleTab(context.Background(), tc.o)
		if step.Status != StatusSkipped || !strings.Contains(step.Reason, tc.want) || len(b.calls) != 0 {
			t.Errorf("%+v -> %+v", tc.o, step)
		}
	}
}

func TestKonsoleFailure(t *testing.T) {
	b := &fakeBus{errs: map[string]error{konsoleSetCurrent: errors.New("no such object")}}
	j := newTestJumper(t, &fakeRunner{}, b)
	step, _, _ := j.konsoleTab(context.Background(), events.Origin{KonsoleService: ":1.2", KonsoleWindow: "/Windows/1", KonsoleSession: "/Sessions/9"})
	if step.Status != StatusFailed || !strings.Contains(step.Reason, "no such object") {
		t.Fatalf("%+v", step)
	}
}

func TestBusDialFailure(t *testing.T) {
	j := newTestJumper(t, &fakeRunner{}, nil)
	j.Bus = nil
	j.DialBus = func() (Bus, error) { return nil, errors.New("no bus") }
	rep := j.Jump(context.Background(), session(events.Origin{AgentPID: 10, KonsoleService: ":1.2", KonsoleWindow: "/Windows/1", KonsoleSession: "/Sessions/9"}))
	tab, _ := rep.Step(StepTerminalTab)
	win, _ := rep.Step(StepWindow)
	if tab.Status != StatusFailed || win.Status != StatusFailed || !strings.Contains(win.Reason, "no bus") {
		t.Fatalf("report:\n%s", rep)
	}
}

func TestBusLazyDialAndClose(t *testing.T) {
	b := &fakeBus{replies: map[string][]any{kwinLoad: {int32(1)}}}
	j := newTestJumper(t, &fakeRunner{}, nil)
	j.Bus = nil
	dials := 0
	j.DialBus = func() (Bus, error) { dials++; return b, nil }
	for range 2 {
		j.Jump(context.Background(), session(events.Origin{AgentPID: 10}))
	}
	if dials != 1 {
		t.Fatalf("dials = %d", dials)
	}
	if err := j.Close(); err != nil || !b.closed {
		t.Fatal("bus not closed")
	}
}

func TestZellij(t *testing.T) {
	tests := []struct {
		name    string
		o       events.Origin
		reply   func(string) (string, error)
		status  Status
		reason  string
		maxCmds int
	}{
		{"no session", events.Origin{ZellijPane: "3"}, nil, StatusSkipped, "no zellij session name", 0},
		{"bad pane", events.Origin{ZellijSession: "m", ZellijPane: "x"}, nil, StatusSkipped, "no usable zellij pane id", 0},
		{"list fails", events.Origin{ZellijSession: "m", ZellijPane: "3"}, func(string) (string, error) {
			return "", errors.New("session not found")
		}, StatusFailed, "session not found", 1},
		{"pane gone", events.Origin{ZellijSession: "m", ZellijPane: "9"}, func(string) (string, error) { return zellijPanes, nil }, StatusFailed, "not found", 1},
		{"no client", events.Origin{ZellijSession: "m", ZellijPane: "terminal_2"}, func(c string) (string, error) {
			if strings.HasSuffix(c, "list-panes --json --tab") {
				return zellijPanes, nil
			}
			if strings.HasSuffix(c, "list-clients") {
				return "CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n", nil
			}
			return "", nil
		}, StatusDone, "no client attached", 4},
		{"never reaches pane", events.Origin{ZellijSession: "m", ZellijPane: "2"}, func(c string) (string, error) {
			if strings.HasSuffix(c, "list-panes --json --tab") {
				return zellijPanes, nil
			}
			if strings.HasSuffix(c, "list-clients") {
				return "CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n1 terminal_7 N/A\n", nil
			}
			return "", nil
		}, StatusFailed, "could not focus terminal_2", 8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{reply: tc.reply}
			j := newTestJumper(t, r, &fakeBus{})
			s := j.zellij(context.Background(), tc.o)
			if s.Status != tc.status || !strings.Contains(s.Reason, tc.reason) || len(r.calls) > tc.maxCmds {
				t.Fatalf("%+v calls=%v", s, r.calls)
			}
		})
	}
}

func TestTmux(t *testing.T) {
	r := &fakeRunner{}
	j := newTestJumper(t, r, &fakeBus{})
	rep := j.Jump(context.Background(), session(events.Origin{Tmux: "/tmp/tmux-1000/work,1234,0", TmuxPane: "%5"}))
	want := "tmux -S /tmp/tmux-1000/work select-window -t %5\ntmux -S /tmp/tmux-1000/work select-pane -t %5"
	if strings.Join(r.calls, "\n") != want {
		t.Fatalf("calls:\n%s", strings.Join(r.calls, "\n"))
	}
	if s, _ := rep.Step(StepMultiplexer); s.Status != StatusDone || s.Backend != "tmux" {
		t.Fatalf("%+v", s)
	}

	r = &fakeRunner{reply: func(string) (string, error) { return "", errors.New("can't find pane: %5") }}
	j = newTestJumper(t, r, &fakeBus{})
	if s := j.tmux(context.Background(), events.Origin{TmuxPane: "%5"}); s.Status != StatusFailed || len(r.calls) != 1 || r.calls[0] != "tmux select-window -t %5" {
		t.Fatalf("%+v %v", s, r.calls)
	}
	if s := j.tmux(context.Background(), events.Origin{Tmux: "/s,1,0"}); s.Status != StatusSkipped {
		t.Fatalf("%+v", s)
	}
}

func TestNestedMultiplexersBothRun(t *testing.T) {
	r := &fakeRunner{reply: func(string) (string, error) { return "", errors.New("down") }}
	j := newTestJumper(t, r, &fakeBus{})
	steps := j.multiplexer(context.Background(), events.Origin{ZellijSession: "z", ZellijPane: "1", TmuxPane: "%1"})
	if len(steps) != 2 || steps[0].Backend != "zellij" || steps[1].Backend != "tmux" {
		t.Fatalf("%+v", steps)
	}
}

func TestKWinResults(t *testing.T) {
	tests := []struct {
		name   string
		bus    *fakeBus
		status Status
		reason string
	}{
		{"not found", &fakeBus{result: []any{"t", false, ""}}, StatusFailed, "no window found"},
		{"no confirmation", &fakeBus{}, StatusDone, "no confirmation"},
		{"no callback", &fakeBus{cbErr: errors.New("export"), result: []any{"t", true, "x"}}, StatusDone, "no feedback channel"},
		{"load fails", &fakeBus{errs: map[string]error{kwinLoad: errors.New("denied")}}, StatusFailed, "loadScript: denied"},
		{"load negative", &fakeBus{replies: map[string][]any{kwinLoad: {int32(-1)}}}, StatusFailed, "returned -1"},
		{"start fails", &fakeBus{errs: map[string]error{kwinStart: errors.New("nope")}}, StatusFailed, "start script"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := newTestJumper(t, &fakeRunner{}, tc.bus)
			s := j.kwinActivate(context.Background(), []int{5}, "")
			if s.Status != tc.status || !strings.Contains(s.Reason, tc.reason) {
				t.Fatalf("%+v", s)
			}
			if tc.name != "load fails" && tc.name != "load negative" {
				if _, ok := tc.bus.find(kwinUnload); !ok {
					t.Fatal("script not unloaded")
				}
			}
			if tc.name == "no callback" && strings.Contains(tc.bus.script, "callDBus") {
				t.Fatal("script calls back without a receiver")
			}
		})
	}
}

func TestX11(t *testing.T) {
	x11env := env(map[string]string{"XDG_SESSION_TYPE": "x11", "DISPLAY": ":0"})
	tests := []struct {
		name    string
		o       events.Origin
		status  Status
		wantWin uint32
	}{
		{"windowid", events.Origin{WindowID: "48234567"}, StatusDone, 48234567},
		{"bogus windowid uses pid lookup", events.Origin{WindowID: "94187130383728", AgentPID: 30}, StatusDone, 0},
		{"nothing", events.Origin{}, StatusSkipped, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := &fakeX11{}
			j := newTestJumper(t, &fakeRunner{}, &fakeBus{})
			j.Getenv, j.X11 = x11env, x
			s := j.raise(context.Background(), tc.o, "", 0)
			if s.Status != tc.status || s.Backend != "x11" || x.win != tc.wantWin {
				t.Fatalf("%+v win=%d", s, x.win)
			}
			if tc.name == "bogus windowid uses pid lookup" && fmt.Sprint(x.pids) != "[30 29]" {
				t.Fatalf("pids %v", x.pids)
			}
		})
	}
	j := newTestJumper(t, &fakeRunner{}, &fakeBus{})
	j.Getenv, j.X11 = x11env, nil
	j.DialX11 = func() (X11, error) { return nil, errors.New("no display") }
	if s := j.raise(context.Background(), events.Origin{WindowID: "5"}, "", 0); s.Status != StatusFailed || !strings.Contains(s.Reason, "no display") {
		t.Fatalf("%+v", s)
	}
	j.X11 = &fakeX11{err: errors.New("BadWindow")}
	if s := j.raise(context.Background(), events.Origin{WindowID: "5"}, "", 0); s.Status != StatusFailed {
		t.Fatalf("%+v", s)
	}
}

func TestDetectWindowBackend(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want WindowBackend
	}{
		{map[string]string{"XDG_SESSION_TYPE": "x11", "XDG_CURRENT_DESKTOP": "KDE"}, WindowX11},
		{map[string]string{"XDG_SESSION_TYPE": "wayland", "XDG_CURRENT_DESKTOP": "KDE"}, WindowKWin},
		{map[string]string{"XDG_SESSION_TYPE": "wayland", "XDG_CURRENT_DESKTOP": "GNOME"}, WindowNone},
		{map[string]string{"WAYLAND_DISPLAY": "wayland-0", "XDG_CURRENT_DESKTOP": "kde"}, WindowKWin},
		{map[string]string{"DISPLAY": ":0"}, WindowX11},
		{map[string]string{"DISPLAY": ":0", "WAYLAND_DISPLAY": "w"}, WindowNone},
		{map[string]string{}, WindowNone},
	}
	for _, tc := range tests {
		if got := DetectWindowBackend(env(tc.env)); got != tc.want {
			t.Errorf("%v -> %s want %s", tc.env, got, tc.want)
		}
	}
	if DetectWindowBackend(nil) != WindowNone {
		t.Error("nil getenv")
	}
	j := newTestJumper(t, &fakeRunner{}, &fakeBus{})
	j.Backend = WindowNone
	if s := j.raise(context.Background(), events.Origin{AgentPID: 3}, "", 0); s.Status != StatusSkipped {
		t.Fatalf("%+v", s)
	}
}

func TestKWinScriptQuoting(t *testing.T) {
	s := kwinScript([]int{1, 2}, `a"b</script>\`, "tok", "", "")
	if !strings.Contains(s, `hint = "a\"b\u003c/script\u003e\\"`) || strings.Contains(s, "callDBus") {
		t.Fatalf("%s", s)
	}
}

func TestReport(t *testing.T) {
	r := Report{Session: "claude/x", Steps: []Step{
		{Name: StepMultiplexer, Status: StatusSkipped, Reason: "none"},
		{Name: StepWindow, Backend: "kwin", Status: StatusDone, Reason: "ok"},
	}}
	if !r.OK() || !strings.Contains(r.String(), "window (kwin): done: ok") {
		t.Fatalf("%s", r)
	}
	r.Steps = append(r.Steps, Step{Name: StepTerminalTab, Status: StatusFailed})
	if r.OK() {
		t.Fatal("failed step should make report not OK")
	}
	if _, ok := r.Step("nope"); ok {
		t.Fatal("unknown step found")
	}
}

func TestZellijClientPanes(t *testing.T) {
	out := []byte("CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND\n1         terminal_2     N/A            \n2 plugin_1 x\n\n")
	if got := zellijClientPanes(out); fmt.Sprint(got) != "[terminal_2 plugin_1]" {
		t.Fatalf("%v", got)
	}
}

func TestExecRunner(t *testing.T) {
	out, err := execRunner(context.Background(), "sh", "-c", "echo out; echo boom >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "boom") || strings.TrimSpace(string(out)) != "out" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, err := execRunner(context.Background(), "sh", "-c", "exit 0"); err != nil {
		t.Fatal(err)
	}
}
