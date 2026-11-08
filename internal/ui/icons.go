package ui

import (
	"fyne.io/fyne/v2"

	"github.com/thereisnotime/agentpad/internal/events"
)

var (
	iconApp       = fyne.NewStaticResource("agentpad.svg", padIconSVG(iconVariantApp))
	iconIdle      = fyne.NewStaticResource("agentpad-idle.svg", padIconSVG(iconVariantIdle))
	iconBusy      = fyne.NewStaticResource("agentpad-busy.svg", padIconSVG(iconVariantBusy))
	iconAttention = fyne.NewStaticResource("agentpad-attention.svg", padIconSVG(iconVariantAttention))
)

// stateIcon returns the tray icon for an aggregate state.
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
