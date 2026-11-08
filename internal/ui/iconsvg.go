package ui

import (
	"fmt"
	"strings"
)

// iconVariant selects the colouring of the pad icon.
type iconVariant int

const (
	iconVariantApp       iconVariant = iota // full colour, window / about / README
	iconVariantIdle                         // tray: neutral, readable on dark and light panels
	iconVariantBusy                         // tray: blue knob
	iconVariantAttention                    // tray: orange body plus a badge
)

// padIconSVG draws the agentpad icon: a 3-key + knob pad seen from above,
// flat shapes on a transparent 64x64 canvas so it stays legible at 16-22 px.
func padIconSVG(v iconVariant) []byte {
	type style struct {
		body, bodyStroke, key, knob, knobMark string
		gradient, badge                       bool
	}
	s := style{body: "#30343f", bodyStroke: "#aab0bc", key: "#e3e6ec", knob: "#e3e6ec", knobMark: "#30343f"}
	switch v {
	case iconVariantApp:
		s = style{body: "url(#g)", bodyStroke: "#4b33c9", key: "#ffffff", knob: "#1c1440", knobMark: "#ffffff", gradient: true}
	case iconVariantBusy:
		s.knob, s.knobMark = "#4c9aff", "#ffffff"
	case iconVariantAttention:
		s = style{body: "#ff8a3d", bodyStroke: "#b8521a", key: "#ffffff", knob: "#3a1c08", knobMark: "#ffffff", badge: true}
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">`)
	if s.gradient {
		b.WriteString(`<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1">` +
			`<stop offset="0" stop-color="#8f74ff"/><stop offset="1" stop-color="#6342f0"/></linearGradient></defs>`)
	}
	fmt.Fprintf(&b, `<rect x="2" y="17" width="60" height="30" rx="9" fill="%s" stroke="%s" stroke-width="2"/>`, s.body, s.bodyStroke)
	for i := range 3 {
		fmt.Fprintf(&b, `<rect x="%d" y="25" width="11" height="14" rx="3.5" fill="%s"/>`, 7+i*13, s.key)
	}
	fmt.Fprintf(&b, `<circle cx="53" cy="32" r="6.5" fill="%s"/>`, s.knob)
	fmt.Fprintf(&b, `<rect x="52" y="26.5" width="2" height="4.5" rx="1" fill="%s"/>`, s.knobMark)
	if s.badge {
		b.WriteString(`<circle cx="54" cy="12" r="8" fill="#ff5c5c" stroke="#ffffff" stroke-width="2.5"/>`)
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}
