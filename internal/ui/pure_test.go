package ui

import (
	"encoding/xml"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestBreakpoints(t *testing.T) {
	if isWide(719) || !isWide(720) || !isWide(1600) || isWide(480) {
		t.Error("isWide breakpoint should be 720")
	}
	if sideBySide(639) || !sideBySide(640) {
		t.Error("sideBySide breakpoint should be 640")
	}
}

func TestColumnsFor(t *testing.T) {
	cases := []struct {
		w, min, gap float32
		want        int
	}{
		{300, 340, 12, 1},
		{340, 340, 12, 1},
		{692, 340, 12, 2},
		{691, 340, 12, 1},
		{1100, 340, 12, 3},
		{500, 0, 12, 1},
	}
	for _, c := range cases {
		if got := columnsFor(c.w, c.min, c.gap); got != c.want {
			t.Errorf("columnsFor(%v, %v) = %d, want %d", c.w, c.min, got, c.want)
		}
	}
}

func TestParseWindowSize(t *testing.T) {
	if w, h, ok := parseWindowSize("500x800"); !ok || w != 500 || h != 800 {
		t.Errorf("got %v %v %v", w, h, ok)
	}
	for _, bad := range []string{"", "500", "ax800", "10x10", "500x"} {
		if _, _, ok := parseWindowSize(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestScreenIndex(t *testing.T) {
	if screenIndex("About") != screenAbout || screenIndex(" pad ") != screenPad || screenIndex("nope") != -1 {
		t.Error("screenIndex")
	}
}

func inside(r rect, w, h float32) bool {
	const eps = 0.01
	return r.X >= -eps && r.Y >= -eps && r.right() <= w+eps && r.bottom() <= h+eps
}

func TestPadGeometryOrientations(t *testing.T) {
	const w, h = 900, 600
	for _, o := range profile.Orientations() {
		g := padGeometry(w, h, o)
		if g.Unit <= 0 {
			t.Fatalf("%s: unit %v", o, g.Unit)
		}
		all := []rect{g.Keys[0], g.Keys[1], g.Keys[2], g.Knob, g.CCW, g.CW}
		for i, r := range all {
			if !inside(r, g.Body.right(), g.Body.bottom()) || r.X < g.Body.X || r.Y < g.Body.Y {
				t.Errorf("%s: part %d %+v outside body %+v", o, i, r, g.Body)
			}
			for j := i + 1; j < len(all); j++ {
				if r.overlaps(all[j]) {
					t.Errorf("%s: parts %d and %d overlap", o, i, j)
				}
			}
		}
		if !inside(g.Body, w, h) {
			t.Errorf("%s: body %+v outside %vx%v", o, g.Body, w, h)
		}
		k := g.Keys
		knobX, knobY := g.Knob.X, g.Knob.Y
		switch o {
		case profile.OrientNormal:
			if k[0].Y != k[2].Y || k[0].X >= k[1].X || k[1].X >= k[2].X || knobX <= k[2].right() {
				t.Errorf("normal: keys should run left->right with the knob on the right")
			}
		case profile.OrientUpsideDown:
			if k[0].Y != k[2].Y || k[0].X >= k[2].X || g.Knob.right() >= k[0].X {
				t.Errorf("upsidedown: knob should be left of the keys")
			}
		case profile.OrientClockwise:
			if k[0].X != k[2].X || k[0].Y >= k[1].Y || k[1].Y >= k[2].Y || knobY <= k[2].bottom() {
				t.Errorf("clockwise: keys top->bottom, knob below")
			}
		case profile.OrientCounterClockwise:
			if k[0].X != k[2].X || g.CW.bottom() >= k[0].Y {
				t.Errorf("counterclockwise: knob should be above the keys")
			}
		}
	}
}

func TestPadGeometryScales(t *testing.T) {
	small := padGeometry(300, 200, profile.OrientNormal)
	big := padGeometry(700, 400, profile.OrientNormal)
	if big.Unit <= small.Unit {
		t.Errorf("unit should grow with space: %v -> %v", small.Unit, big.Unit)
	}
	huge := padGeometry(5000, 3000, profile.OrientNormal)
	if huge.Unit > geoMaxUnit {
		t.Errorf("unit should be capped, got %v", huge.Unit)
	}
	// centred
	g := padGeometry(1000, 300, profile.OrientNormal)
	if d := (1000 - g.Body.right()) - g.Body.X; d > 0.01 || d < -0.01 {
		t.Errorf("body not centred: %+v", g.Body)
	}
}

func TestLEDPolicy(t *testing.T) {
	p := LEDPolicy{Attention: pad.LEDSetting{Mode: 2}, Otherwise: pad.LEDSetting{Mode: 0}}
	cases := map[events.State]pad.LEDSetting{
		events.StateAttention: {Mode: 2},
		events.StateBusy:      {Mode: 0},
		events.StateIdle:      {Mode: 0},
		"":                    {Mode: 0},
	}
	for s, want := range cases {
		if got := ledPolicy(s, p); got != want {
			t.Errorf("ledPolicy(%q) = %+v, want %+v", s, got, want)
		}
	}
}

func TestDefaultLEDPolicy(t *testing.T) {
	d := defaultLEDPolicy(0x8890)
	if d.Attention != (pad.LEDSetting{Mode: 1}) || d.Otherwise != (pad.LEDSetting{}) {
		t.Errorf("8890 defaults = %+v", d)
	}
	for _, pid := range []uint16{0x8840, 0x8842} {
		d := defaultLEDPolicy(pid)
		if d.Attention != (pad.LEDSetting{Effect: "backlight", Color: "orange"}) || d.Otherwise.Effect != "off" {
			t.Errorf("%04x defaults = %+v", pid, d)
		}
	}
	if ledPolicyKey(0x8840) != ledPolicyKey(0x8842) || ledPolicyKey(0x8840) == ledPolicyKey(0x8890) {
		t.Error("policy keys should be per LED family")
	}
	if ledLabelKey(3) != "ledLabel.3" {
		t.Error("label key")
	}
}

func TestDecodeLEDPolicy(t *testing.T) {
	p := LEDPolicy{Attention: pad.LEDSetting{Effect: "shock", Color: "red"}, Otherwise: pad.LEDSetting{Effect: "off"}}
	if got := decodeLEDPolicy(encodeLEDPolicy(p), 0x8842, nil); got != p {
		t.Errorf("round trip = %+v", got)
	}
	if got := decodeLEDPolicy("{broken", 0x8890, nil); got != defaultLEDPolicy(0x8890) {
		t.Errorf("broken JSON should give defaults, got %+v", got)
	}
	reject := func(pad.LEDSetting) error { return errors.New("no") }
	if got := decodeLEDPolicy(encodeLEDPolicy(p), 0x8890, reject); got != defaultLEDPolicy(0x8890) {
		t.Errorf("invalid policy should give defaults, got %+v", got)
	}
}

// fakeWriter records calls; Upload blocks until release is closed when set.
type fakeWriter struct {
	mu      sync.Mutex
	uploads int
	leds    []pad.LEDSetting
	release chan struct{}
	started chan struct{}
}

func (f *fakeWriter) Available() error { return nil }
func (f *fakeWriter) Upload([]byte) error {
	if f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		<-f.release
	}
	f.mu.Lock()
	f.uploads++
	f.mu.Unlock()
	return nil
}
func (f *fakeWriter) SetLED(s pad.LEDSetting) error {
	f.mu.Lock()
	f.leds = append(f.leds, s)
	f.mu.Unlock()
	return nil
}

type fakeLED struct {
	mu      sync.Mutex
	sets    []pad.LEDSetting
	forgets int
}

func (f *fakeLED) Set(s pad.LEDSetting) error {
	f.mu.Lock()
	f.sets = append(f.sets, s)
	f.mu.Unlock()
	return nil
}
func (f *fakeLED) Forget() { f.mu.Lock(); f.forgets++; f.mu.Unlock() }

func TestWorkerCoalescesFollow(t *testing.T) {
	w := &fakeWriter{release: make(chan struct{}), started: make(chan struct{})}
	led := &fakeLED{}
	d := newDeviceWorker(w, led, 4)
	d.start()
	defer func() { d.Close(); d.wait() }()

	done := make(chan error, 1)
	d.Upload([]byte("x"), func(err error) { done <- err })
	<-w.started // worker is now busy inside Upload
	for i := 1; i <= 5; i++ {
		d.Follow(pad.LEDSetting{Mode: i}, nil)
	}
	last := make(chan struct{})
	d.Follow(pad.LEDSetting{Mode: 9}, func(error) { close(last) })
	close(w.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-last:
	case <-time.After(2 * time.Second):
		t.Fatal("follow job never ran")
	}
	led.mu.Lock()
	defer led.mu.Unlock()
	if len(led.sets) != 1 || led.sets[0].Mode != 9 {
		t.Errorf("follow should coalesce to the latest, got %+v", led.sets)
	}
}

func TestWorkerSetLEDForgets(t *testing.T) {
	w := &fakeWriter{}
	led := &fakeLED{}
	d := newDeviceWorker(w, led, 4)
	d.start()
	defer func() { d.Close(); d.wait() }()
	done := make(chan error)
	d.SetLED(pad.LEDSetting{Mode: 3}, func(err error) { done <- err })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(w.leds) != 1 || w.leds[0].Mode != 3 || led.forgets != 1 {
		t.Errorf("leds=%+v forgets=%d", w.leds, led.forgets)
	}
}

func TestWorkerQueueFull(t *testing.T) {
	d := newDeviceWorker(&fakeWriter{}, &fakeLED{}, 1) // not started: nothing drains
	d.Upload(nil, nil)
	var got error
	d.Upload(nil, func(err error) { got = err })
	if !errors.Is(got, errDeviceBusy) {
		t.Errorf("want errDeviceBusy, got %v", got)
	}
}

func TestControlActions(t *testing.T) {
	var p profile.Profile
	for i, c := range allControls {
		p = setAction(p, c, slotAction, " a"+string(rune('0'+i))+" ")
	}
	if p.Keys != [3]string{"a0", "a1", "a2"} || p.Knob != (profile.Knob{CCW: "a3", Press: "a4", CW: "a5"}) {
		t.Errorf("got %+v", p)
	}
	for i, c := range allControls {
		if got := getAction(p, c, slotAction); got != "a"+string(rune('0'+i)) {
			t.Errorf("getAction(%d) = %q", c, got)
		}
		if c.title() == "" || len(slotsFor(c, p, false)) != 1 {
			t.Errorf("control %d needs a title and a slot", c)
		}
	}
	if !reflect.DeepEqual(setAction(p, ctlKey1, "hold", "x"), p) || getAction(p, ctlKey1, "hold") != "" {
		t.Error("unknown slots are ignored")
	}
}

func TestCapLabel(t *testing.T) {
	if capLabel("", 5) != "—" || capLabel("enter", 5) != "enter" || capLabel("volumeup", 5) != "volu…" {
		t.Error("capLabel")
	}
}

func TestBugReportURL(t *testing.T) {
	d := diagnostics{Version: "v1.2 beta", OS: osString("linux", "amd64", "6.19.8\n", "wayland"), Pad: "1189:8890"}
	raw := bugReportURL(d)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "github.com" || u.Path != "/thereisnotime/agentpad/issues/new" {
		t.Errorf("url = %s", raw)
	}
	q := u.Query()
	want := map[string]string{
		"template": "bug_report.yml", "version": "v1.2 beta",
		"os": "linux/amd64 · 6.19.8 · wayland", "pad": "1189:8890", "title": "[bug] ",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if !strings.Contains(raw, "title=%5Bbug%5D+") {
		t.Errorf("title not escaped as expected: %s", raw)
	}
	if fu, _ := url.Parse(featureReqURL); fu.Query().Get("template") != "feature_request.yml" {
		t.Error("feature template")
	}
	if !strings.Contains(d.text(), "Pad: 1189:8890") {
		t.Errorf("text = %q", d.text())
	}
	if padString(pad.Device{}, false) != "not connected" ||
		padString(pad.Device{VendorID: 0x1189, ProductID: 0x8842}, true) != "1189:8842" {
		t.Error("padString")
	}
	if osString("linux", "arm64", "", "") != "linux/arm64" {
		t.Error("osString without extras")
	}
}

func TestPadIconSVG(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range []iconVariant{iconVariantApp, iconVariantIdle, iconVariantBusy, iconVariantAttention} {
		s := padIconSVG(v)
		if err := xml.Unmarshal(s, new(struct{})); err != nil {
			t.Errorf("variant %d: invalid XML: %v", v, err)
		}
		if seen[string(s)] {
			t.Errorf("variant %d duplicates another", v)
		}
		seen[string(s)] = true
	}
	if !strings.Contains(string(padIconSVG(iconVariantAttention)), "#ff5c5c") {
		t.Error("attention needs the badge")
	}
}

func TestKindAndAgentColours(t *testing.T) {
	if kindColor(events.KindToolPending) != colorAttention || kindColor(events.KindPrompt) != colorBusy {
		t.Error("kind colours")
	}
	if l, _ := agentBadge("claude"); l != "C" {
		t.Error("claude badge")
	}
	if l, _ := agentBadge("zed"); l != "z" {
		t.Error("fallback badge")
	}
	if ledColor("white") == ledColor("red") {
		t.Error("led colours")
	}
}

func TestInputControlMapping(t *testing.T) {
	want := map[string]controlID{
		InputKey1: ctlKey1, InputKey2: ctlKey2, InputKey3: ctlKey3,
		InputKnobCCW: ctlKnobCCW, InputKnobPress: ctlKnobPress, InputKnobCW: ctlKnobCW,
	}
	for name, c := range want {
		if got, ok := inputControl(name); !ok || got != c {
			t.Errorf("inputControl(%q) = %v %v", name, got, ok)
		}
		if inputCaption(c) == "" {
			t.Errorf("no caption for %q", name)
		}
	}
	if _, ok := inputControl("key4"); ok {
		t.Error("unknown control should not map")
	}
	if inputCaption(ctlKey2) != "key 2 · tap" || inputCaption(ctlKnobCW) != "knob ↻" {
		t.Error("captions")
	}
}

func TestDetentAngle(t *testing.T) {
	if detentAngle(ctlKnobCW) != 30 || detentAngle(ctlKnobCCW) != -30 || detentAngle(ctlKey1) != 0 || detentAngle(ctlKnobPress) != 0 {
		t.Error("detentAngle")
	}
	if tickAngle(30, 0) != 0 || tickAngle(30, 1) != 0 {
		t.Error("tick should start and end at rest")
	}
	if a := tickAngle(30, 0.4); a < 29.9 || a > 30.1 {
		t.Errorf("tick peak = %v", a)
	}
	if a := tickAngle(-30, 0.7); a >= 0 || a <= -30 {
		t.Errorf("ccw easing back = %v", a)
	}
}

func TestFakeInput(t *testing.T) {
	in := NewFakeInput(time.Millisecond).(*fakeInput)
	e := <-in.Events()
	if _, ok := inputControl(e.Control); !ok || !e.Down {
		t.Errorf("first event = %+v", e)
	}
	in.Close()
	for range in.Events() { //nolint:revive // drain until closed
	}
}
