package ui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/settings"
)

// newFileSettings opens a store in a temp dir; posted work is queued on the
// returned channel so the test goroutine runs it (like the UI goroutine).
func newFileSettings(t *testing.T) (*FileSettings, chan func()) {
	t.Helper()
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fs := NewFileSettings(st, nil)
	q := make(chan func(), 64)
	fs.post = func(f func()) { q <- f }
	return fs, q
}

// drain runs queued UI work until cond holds or the timeout passes.
func drain(t *testing.T, q chan func(), timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.After(timeout)
	for !cond() {
		select {
		case f := <-q:
			f()
		case <-deadline:
			return false
		}
	}
	return true
}

func TestFileSettingsRoundTrip(t *testing.T) {
	fs, _ := newFileSettings(t)
	fs.SetOrientation(profile.OrientClockwise)
	fs.SetLEDFollow(false)
	p8890 := LEDPolicy{Attention: pad.LEDSetting{Mode: 1}, Otherwise: pad.LEDSetting{Mode: 2}}
	p884x := LEDPolicy{Attention: pad.LEDSetting{Layer: 1, Effect: "shock", Color: "cyan"}, Otherwise: pad.LEDSetting{Effect: "off"}}
	fs.SetLEDPolicy(0x8890, p8890)
	fs.SetLEDPolicy(0x8842, p884x)
	fs.SetLEDLabel(3, " Rainbow ")
	fs.SetActiveProfile("codex")
	fs.SetMode(ModeGesture)
	fs.SetNotifyAttention(false)

	// a fresh store on the same dir sees everything
	st2, err := settings.Open(fs.Store().Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []*FileSettings{fs, NewFileSettings(st2, nil)} {
		if f.Orientation() != profile.OrientClockwise || f.LEDFollow() || f.ActiveProfile() != "codex" ||
			f.Mode() != ModeGesture || f.NotifyAttention() || f.LEDLabel(3) != "Rainbow" {
			t.Fatalf("values = %+v", f.Store().Get())
		}
		if got, ok := f.LEDPolicy(0x8890); !ok || got != p8890 {
			t.Errorf("8890 policy = %+v", got)
		}
		if got, _ := f.LEDPolicy(0x8840); got != p884x {
			t.Errorf("884x policy = %+v (shared by 8840 and 8842)", got)
		}
	}
	fs.SetLEDLabel(3, "")
	if fs.LEDLabel(3) != "" {
		t.Error("empty label not removed")
	}

	// gesture timings come from the file
	data, _ := os.ReadFile(fs.Store().Path())
	data = []byte(strings.Replace(string(data), "hold_ms: 450", "hold_ms: 500", 1))
	data = []byte(strings.Replace(string(data), "chord_window_ms: 80", "chord_window_ms: 120", 1))
	if err := os.WriteFile(fs.Store().Path(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	fs.SetMode(ModePlain) // Update adopts the external edit first
	g := fs.GestureConfig()
	if g.Hold != 500*time.Millisecond || g.LongHold != 1500*time.Millisecond || g.DoubleTap != 300*time.Millisecond ||
		g.ChordWindow != 120*time.Millisecond {
		t.Errorf("gestures = %+v", g)
	}
	if c := g.config(); c.ChordWindow != 120*time.Millisecond || c.Hold != 500*time.Millisecond {
		t.Errorf("config = %+v", c)
	}
}

func TestFileSettingsOnChange(t *testing.T) {
	fs, q := newFileSettings(t)
	n := 0
	fs.OnChange(func() { n++ })
	var invalid []error
	fs.OnInvalid(func(err error) { invalid = append(invalid, err) })

	fs.SetOrientation(profile.OrientUpsideDown)
	if !drain(t, q, time.Second, func() bool { return n == 1 }) {
		t.Fatal("own setter did not fire OnChange")
	}

	ctx := t.Context()
	fs.Watch(ctx)
	writeSetting(t, fs, "orientation: upsidedown", "orientation: counterclockwise")
	if !drain(t, q, 5*time.Second, func() bool { return n == 2 }) {
		t.Fatal("external edit did not fire OnChange")
	}
	if fs.Orientation() != profile.OrientCounterClockwise {
		t.Fatalf("orientation = %s", fs.Orientation())
	}

	// invalid edit: old values stay, the error names field and line
	writeSetting(t, fs, "orientation: counterclockwise", "orientation: sideways")
	if !drain(t, q, 5*time.Second, func() bool { return len(invalid) == 1 }) {
		t.Fatal("invalid edit not reported")
	}
	var ve *settings.ValidationError
	if !errors.As(invalid[0], &ve) || ve.Errors[0].Path != "pad.orientation" || ve.Errors[0].Line == 0 {
		t.Fatalf("error = %v", invalid[0])
	}
	if fs.Orientation() != profile.OrientCounterClockwise || n != 2 {
		t.Fatalf("orientation = %s, changes = %d", fs.Orientation(), n)
	}
	if got := configPillText(invalid[0]); !strings.Contains(got, "line ") {
		t.Errorf("pill = %q", got)
	}

	// fixed again: listeners hear nil, then the change
	writeSetting(t, fs, "orientation: sideways", "orientation: clockwise")
	if !drain(t, q, 5*time.Second, func() bool { return n == 3 }) {
		t.Fatal("fix not picked up")
	}
	if len(invalid) != 2 || invalid[1] != nil || fs.Orientation() != profile.OrientClockwise {
		t.Fatalf("invalid = %v, orientation = %s", invalid, fs.Orientation())
	}
}

func TestFileSettingsInitialError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settings.FileName), []byte("schema: 1\npad:\n  mode: turbo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := settings.Open(dir)
	if st == nil || err == nil {
		t.Fatalf("Open = %v, %v", st, err)
	}
	fs := NewFileSettings(st, err)
	fs.post = func(f func()) { f() }
	var got error
	fs.OnInvalid(func(e error) { got = e })
	if got == nil || fs.Mode() != ModePlain {
		t.Fatalf("got %v, mode %s", got, fs.Mode())
	}
}

func writeSetting(t *testing.T, fs *FileSettings, old, new string) {
	t.Helper()
	data, err := os.ReadFile(fs.Store().Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%q not in settings.yaml", old)
	}
	if err := os.WriteFile(fs.Store().Path(), []byte(strings.Replace(string(data), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPrefsMigration(t *testing.T) {
	a := test.NewTempApp(t)
	p := a.Preferences()
	p.SetString(prefOrientation, "normal")
	p.SetString(prefActiveProfile, "claude-code")
	p.SetString(prefMode, "gesture")
	p.SetBool(prefLEDFollow, false)
	p.SetBool(prefNotify, false)
	p.SetString(ledPolicyKey(0x8890), encodeLEDPolicy(LEDPolicy{Attention: pad.LEDSetting{Mode: 1}}))
	p.SetString(ledPolicyKey(0x8842), encodeLEDPolicy(LEDPolicy{
		Attention: pad.LEDSetting{Effect: "shock", Color: "red"}, Otherwise: pad.LEDSetting{Effect: "off"},
	}))
	p.SetString(ledLabelKey(2), "Breathing")
	p.SetInt(prefGestureHold, 600)
	p.SetInt(prefGestureLongHold, 0) // 0 = default

	prefs := LegacyPrefs(p)
	if _, ok := prefs[prefAttentionMode]; ok {
		t.Error("unset int reported")
	}
	if prefs[prefLEDFollow] != "false" || prefs[prefGestureHold] != "600" {
		t.Errorf("prefs = %v", prefs)
	}

	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := st.Migrate(prefs); !ok || err != nil {
		t.Fatalf("Migrate = %v, %v (warnings %v)", ok, err, st.Warnings())
	}
	s := st.Get()
	if s.Pad.Orientation != profile.OrientNormal || s.Pad.ActiveProfile != "claude-code" || s.Pad.Mode != settings.ModeGesture ||
		s.LED.FollowAgents || s.Notifications.Attention || s.LED.Basic.Attention.Mode != 1 ||
		s.LED.PerLayer.Attention != (settings.LED{Effect: "shock", Color: "red"}) || s.LED.Label(2) != "Breathing" ||
		s.Gestures.HoldMs != 600 || s.Gestures.LongHoldMs != 1500 {
		t.Fatalf("migrated = %+v", s)
	}
	if w := st.Warnings(); len(w) > 0 {
		t.Errorf("warnings = %v", w)
	}
}

// Every preference key the Fyne implementation writes is understood by Migrate.
func TestLegacyKeysCoverFynePrefs(t *testing.T) {
	known := settings.LegacyKeys()
	keys := []string{prefOrientation, prefActiveProfile, prefMode, prefLEDFollow, prefNotify, prefAttentionMode,
		prefGestureHold, prefGestureLongHold, prefGestureDoubleTap, ledPolicyKey(0x8890), ledPolicyKey(0x8840), "ledLabel.N"}
	for _, k := range keys {
		if !slices.Contains(known, k) {
			t.Errorf("settings.LegacyKeys() lacks %q", k)
		}
	}
}

func TestPlanPreview(t *testing.T) {
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mine := profile.Builtins()[0]
	mine.Name = "mine"
	if err := st.Profiles().Save(mine); err != nil {
		t.Fatal(err)
	}
	codex, _ := st.Profiles().Get("codex")
	codex.Description = "changed"
	if err := st.Profiles().Save(codex); err != nil {
		t.Fatal(err)
	}
	bundle := `kind: agentpad-bundle
schema: 1
settings:
  pad: {orientation: clockwise}
  experimental: {foo: 1}
profiles:
  - name: zellij-claude
    description: Claude in zellij
    keys: ["1", "2", escape]
    knob: {ccw: up, press: enter, cw: down}
`
	b, err := settings.ParseBundle(strings.NewReader(bundle))
	if err != nil {
		t.Fatal(err)
	}
	merge, err := st.PlanImport(b, settings.Merge)
	if err != nil {
		t.Fatal(err)
	}
	groups := planPreview(merge)
	titles := []string{}
	for _, g := range groups {
		titles = append(titles, g.Title)
	}
	if !slices.Equal(titles, []string{"Settings", "Profiles", "Warnings"}) {
		t.Fatalf("groups = %v", titles)
	}
	if r := groups[0].Rows[0]; r.Kind != rowChange || r.Text != "pad.orientation" || r.Detail != "normal → clockwise" {
		t.Errorf("settings row = %+v", r)
	}
	if r := groups[1].Rows; len(r) != 1 || r[0].Kind != rowAdd || r[0].Text != "zellij-claude" {
		t.Errorf("profile rows = %+v", r)
	}
	if r := groups[2].Rows[0]; r.Kind != rowWarning || !strings.Contains(r.Text, "experimental") {
		t.Errorf("warning = %+v", r)
	}
	if planSummary(merge) != "1 added · 1 changed · 0 removed" {
		t.Errorf("summary = %q", planSummary(merge))
	}

	replace, err := st.PlanImport(b, settings.Replace)
	if err != nil {
		t.Fatal(err)
	}
	var prof previewGroup
	for _, g := range planPreview(replace) {
		if g.Title == "Profiles" {
			prof = g
		}
	}
	// adds, changes, then removals; a dropped built-in override is a change
	// (back to the built-in), a dropped custom profile a removal
	kinds := map[string]previewRow{}
	for i, r := range prof.Rows {
		kinds[r.Text] = r
		if i > 0 && r.Kind < prof.Rows[i-1].Kind {
			t.Errorf("rows out of order: %+v", prof.Rows)
		}
	}
	if kinds["mine"].Kind != rowRemove || kinds["mine"].Detail != "profile is deleted" ||
		kinds["codex"].Kind != rowChange {
		t.Errorf("replace rows = %+v", prof.Rows)
	}
	if planPreview(nil) != nil || planSummary(&settings.Plan{}) != "Nothing would change." {
		t.Error("empty plan")
	}
}

func TestImportErrorText(t *testing.T) {
	st, _ := settings.Open(t.TempDir())
	b, err := settings.ParseBundle(strings.NewReader(`kind: agentpad-bundle
schema: 1
profiles:
  - name: bad
    keys: ["1", foo, "3"]
    knob: {ccw: up, press: enter, cw: down}
`))
	if err == nil {
		_, err = st.PlanImport(b, settings.Merge)
	}
	if err == nil {
		t.Fatal("bad profile accepted")
	}
	got := errorText(err)
	if !strings.Contains(got, "bad") || !strings.Contains(got, "foo") || !strings.Contains(got, "• ") {
		t.Errorf("text = %q", got)
	}
}

func TestShareImportRename(t *testing.T) {
	store := profile.NewFileStore(t.TempDir())
	codex, _ := store.Get("codex")
	codex.Description = "mine"
	if err := store.Save(codex); err != nil {
		t.Fatal(err)
	}
	other := codex
	other.Description = "theirs"
	data, err := settings.ExportProfile(other)
	if err != nil {
		t.Fatal(err)
	}
	msg, saved, err := importShared(data, store)
	if err != nil || saved != "codex-2" || !strings.Contains(msg, "codex-2") || !strings.Contains(msg, "different codex") {
		t.Fatalf("import = %q, %q, %v", msg, saved, err)
	}
	// same content again: nothing new
	mine, _ := settings.ExportProfile(codex)
	msg, saved, err = importShared(mine, store)
	if err != nil || saved != "codex" || !strings.Contains(msg, "nothing changed") {
		t.Fatalf("identical = %q, %q, %v", msg, saved, err)
	}
	fresh := codex
	fresh.Name = "brand-new"
	data, _ = settings.ExportProfile(fresh)
	if msg, saved, _ = importShared(data, store); saved != "brand-new" || msg != "Imported brand-new." {
		t.Fatalf("new = %q, %q", msg, saved)
	}
}

func TestConfigFileNames(t *testing.T) {
	day := time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC)
	if got := exportFileName(day); got != "agentpad-2026-11-09.agentpad.yaml" {
		t.Errorf("export name = %q", got)
	}
	dir := t.TempDir()
	if dirSignature(dir) != "" || dirSignature(filepath.Join(dir, "missing")) != "" {
		t.Error("empty dir signature")
	}
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0o644)
	s1 := dirSignature(dir)
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("xy"), 0o644)
	if s2 := dirSignature(dir); s1 == "" || s1 == s2 {
		t.Errorf("signature did not change: %q %q", s1, s2)
	}
}

func TestAppUsesFileSettings(t *testing.T) {
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
	u.ledFollow.Store(false)
	t.Cleanup(u.shutdown)
	if u.files != fs.Store() || u.deps.ProfilesDir != filepath.Join(fs.Store().Dir(), "profiles") {
		t.Fatalf("files = %v, profiles dir = %q", u.files, u.deps.ProfilesDir)
	}

	// rotating in the UI writes settings.yaml
	u.profiles.rotate(profile.OrientClockwise)
	drain(t, q, 100*time.Millisecond, func() bool { return false })
	if data, _ := os.ReadFile(fs.Store().Path()); !strings.Contains(string(data), "orientation: clockwise") {
		t.Fatalf("settings.yaml:\n%s", data)
	}

	fs.Watch(t.Context())
	writeSetting(t, fs, "\n  attention: true", "\n  attention: false")
	if !drain(t, q, 5*time.Second, func() bool { return !u.notifyOn.Load() }) {
		t.Fatal("external notifications edit not applied")
	}
	if u.pad.notifyCheck.Checked {
		t.Error("notify checkbox still on")
	}
	writeSetting(t, fs, "orientation: clockwise", "orientation: sideways")
	if !drain(t, q, 5*time.Second, func() bool { return u.pillConfig.Visible() }) {
		t.Fatal("error pill not shown")
	}
	if u.profiles.orient != profile.OrientClockwise || !strings.Contains(u.pillConfig.text.Text, "line") {
		t.Errorf("orient = %s, pill = %q", u.profiles.orient, u.pillConfig.text.Text)
	}
	writeSetting(t, fs, "orientation: sideways", "orientation: upsidedown")
	if !drain(t, q, 5*time.Second, func() bool { return u.profiles.orient == profile.OrientUpsideDown }) {
		t.Fatal("fixed edit not applied")
	}
	if u.pillConfig.Visible() {
		t.Error("error pill still shown")
	}

	// a profile file dropped into profiles/ shows up on the next poll
	p := profile.Builtins()[0]
	p.Name = "dropped"
	if err := profile.NewFileStore(u.deps.ProfilesDir).Save(p); err != nil {
		t.Fatal(err)
	}
	if sig := dirSignature(u.deps.ProfilesDir); sig == u.profileSig {
		t.Fatal("signature unchanged")
	}
	u.profiles.externalReload()
	found := slices.ContainsFunc(u.profiles.profiles, func(p profile.Profile) bool { return p.Name == "dropped" })
	if !found || dirSignature(u.deps.ProfilesDir) != u.profileSig {
		t.Errorf("dropped profile not listed")
	}

	// import from a bundle applies settings and profiles and keeps a backup
	b, _ := settings.ParseBundle(strings.NewReader("kind: agentpad-bundle\nschema: 1\nsettings:\n  pad: {orientation: normal}\n"))
	plan, err := fs.Store().PlanImport(b, settings.Merge)
	if err != nil {
		t.Fatal(err)
	}
	s := &settingsUI{u: u, status: wrapLabel(""), restoreBtn: newCopyButton(a, "x", nil)}
	s.apply(plan)
	drain(t, q, 100*time.Millisecond, func() bool { return false })
	if u.profiles.orient != profile.OrientNormal || s.lastBackup == "" || !strings.Contains(s.status.Text, s.lastBackup) {
		t.Errorf("after import: orient %s, status %q", u.profiles.orient, s.status.Text)
	}
}
