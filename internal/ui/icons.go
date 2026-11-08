package ui

import (
	"fyne.io/fyne/v2"

	"github.com/thereisnotime/agentpad/internal/events"
)

var (
	iconIdle      = fyne.NewStaticResource("agentpad-idle.svg", circleSVG(colorIdle))
	iconBusy      = fyne.NewStaticResource("agentpad-busy.svg", circleSVG(colorBusy))
	iconAttention = fyne.NewStaticResource("agentpad-attention.svg", circleSVG(colorAttention))
)

// stateIcon returns the tray / window icon for an aggregate state.
func stateIcon(s events.State) fyne.Resource {
	switch s {
	case events.StateAttention:
		return iconAttention
	case events.StateBusy:
		return iconBusy
	default:
		return iconIdle
	}
}
