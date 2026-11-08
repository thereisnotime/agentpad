package ui

// Colour palette shared by the theme and the custom drawn widgets. No Fyne
// imports here so the colour helpers stay unit-testable.

import (
	"image/color"

	"github.com/thereisnotime/agentpad/internal/events"
)

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

// withAlpha returns c with alpha a.
func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

var (
	colBackground = rgb(0x121318)
	colSurface    = rgb(0x1a1c23) // cards
	colSurface2   = rgb(0x22252e) // raised elements inside cards
	colSurface3   = rgb(0x2b2f3a) // hover / keycap face
	colBorder     = rgb(0x2c303b)
	colText       = rgb(0xeceef3)
	colText2      = rgb(0xb4b9c6) // secondary text, still readable on dark
	colText3      = rgb(0x858b99) // captions / placeholders
	colAccent     = rgb(0x7c5cff)
	colAccentSoft = withAlpha(rgb(0x7c5cff), 0x38)

	colorAttention = rgb(0xff8a3d)
	colorBusy      = rgb(0x4c9aff)
	colorIdle      = rgb(0x8a8f98)
	colorSuccess   = rgb(0x3ddc84)
	colorError     = rgb(0xff5c5c)
)

// stateColor returns the indicator colour of a session or aggregate state.
func stateColor(s events.State) color.NRGBA {
	switch s {
	case events.StateAttention:
		return colorAttention
	case events.StateBusy:
		return colorBusy
	default:
		return colorIdle
	}
}

// kindColor returns the badge colour of an event kind in the log.
func kindColor(k events.Kind) color.NRGBA {
	switch k {
	case events.KindToolPending:
		return colorAttention
	case events.KindPrompt, events.KindToolDone:
		return colorBusy
	case events.KindSessionStart, events.KindSessionEnd:
		return colAccent
	case events.KindWaitingInput:
		return colorSuccess
	default:
		return colorIdle
	}
}

// agentBadge returns the letter and colour used for an agent's badge.
func agentBadge(agent string) (string, color.NRGBA) {
	switch agent {
	case events.AgentClaude:
		return "C", rgb(0xd97757)
	case events.AgentCodex:
		return "X", rgb(0x5b6475)
	case events.AgentOpenCode:
		return "O", rgb(0x2fbf71)
	}
	if agent == "" {
		return "?", colorIdle
	}
	return string([]rune(agent)[0:1]), colorIdle
}

// ledColor maps a pad LED colour name to a swatch colour.
func ledColor(name string) color.NRGBA {
	switch name {
	case "red":
		return rgb(0xff4d4d)
	case "orange":
		return rgb(0xff8a3d)
	case "yellow":
		return rgb(0xffd23f)
	case "green":
		return rgb(0x3ddc84)
	case "cyan":
		return rgb(0x2fd4e6)
	case "blue":
		return rgb(0x4c7dff)
	case "purple":
		return rgb(0xa45cff)
	case "white":
		return rgb(0xf4f4f4)
	}
	return colorIdle
}

// mix blends a towards b by t (0..1).
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	t = min(max(t, 0), 1)
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}
