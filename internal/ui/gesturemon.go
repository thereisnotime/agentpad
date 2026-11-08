package ui

// Live gesture monitor and key map warnings, without Fyne.

import (
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/gesture"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// monitorSize is how many recognised gestures the monitor keeps.
const monitorSize = 8

// monitorFade is how long a monitor line stays fully visible before fading.
const monitorFade = 8 * time.Second

// monitorItem is one line of the live gesture monitor.
type monitorItem struct {
	Text string
	At   time.Time
	Err  bool
}

// gestureLog keeps the latest recognised gestures, newest first.
type gestureLog struct{ items []monitorItem }

func (l *gestureLog) add(it monitorItem) {
	l.items = append([]monitorItem{it}, l.items...)
	if len(l.items) > monitorSize {
		l.items = l.items[:monitorSize]
	}
}

// gestureLine describes a recognised gesture: "key 1 · hold → ctrl-c".
func gestureLine(ev gesture.Event) monitorItem {
	g := ev.Gesture
	s := controlLabel(string(g.Control)) + " · " + strings.ToLower(kindLabel(string(g.Kind)))
	if g.Kind == gesture.Chord {
		s = controlLabel(string(g.Control)) + " · chord"
	}
	switch {
	case ev.Action == "":
		s += " (not used)"
	case ev.Err != nil:
		s += " → " + ev.Action + " failed: " + ev.Err.Error()
	default:
		s += " → " + ev.Action
	}
	return monitorItem{Text: s, At: ev.Time, Err: ev.Err != nil}
}

// monitorAlpha is the opacity of line i (0 = newest) at age: older lines
// are dimmer, and every line fades out after monitorFade.
func monitorAlpha(i int, age time.Duration) uint8 {
	a := 255 - i*22
	if age > monitorFade {
		over := float64(age-monitorFade) / float64(2*time.Second)
		if over >= 1 {
			return 0
		}
		a = int(float64(a) * (1 - over))
	}
	return uint8(max(a, 0))
}

// collisionWarning explains which controls live input cannot tell apart,
// e.g. "Key 1 and knob press both send Enter, so live input can't tell
// them apart". Empty when the mapping is unambiguous.
func collisionWarning(p profile.Profile, o profile.Orientation) string {
	_, rep, err := gesture.PlainKeyMap(p, o)
	if err != nil || len(rep.Collisions) == 0 {
		return ""
	}
	var lines []string
	for _, c := range rep.Collisions {
		ctls := append([]gesture.Control{c.Winner}, c.Others...)
		names := make([]string, len(ctls))
		actions := map[string]bool{}
		var action string
		for i, ctl := range ctls {
			names[i] = controlLabel(string(ctl))
			action, _ = p.GestureAction(string(ctl), plainKind(ctl))
			actions[action] = true
		}
		who := joinAnd(names)
		who = strings.ToUpper(who[:1]) + who[1:]
		both := "both"
		if len(ctls) > 2 {
			both = "all"
		}
		if len(actions) == 1 {
			lines = append(lines, who+" "+both+" send "+actionLabel(action)+", so live input can't tell them apart")
		} else {
			lines = append(lines, who+" send the same key, so live input can't tell them apart")
		}
	}
	return strings.Join(lines, ". ")
}

// plainKind is the gesture kind whose fallback is the plain action of c.
func plainKind(c gesture.Control) string {
	if c == gesture.KnobCCW || c == gesture.KnobCW {
		return profile.KindRotate
	}
	return profile.KindTap
}

// actionLabel capitalises a single-word action for prose ("enter" -> "Enter").
func actionLabel(a string) string {
	if len(a) > 1 && !strings.ContainsAny(a, "-,<") {
		return strings.ToUpper(a[:1]) + a[1:]
	}
	return a
}

func joinAnd(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	}
	return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
}
