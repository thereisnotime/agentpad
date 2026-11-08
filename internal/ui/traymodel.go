package ui

import (
	"fmt"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/profile"
)

// trayAction is what a tray menu item does when clicked.
type trayAction int

const (
	trayNone trayAction = iota
	trayProfile
	trayOrientation
	trayFollow
	trayMode
	trayJump
	trayShow
	trayQuit
)

// trayItem is one entry of the tray menu, independent of Fyne.
type trayItem struct {
	Label     string
	Action    trayAction
	Arg       string // profile name or orientation
	Checked   bool
	Disabled  bool
	Separator bool
	Quit      bool
	Children  []trayItem
}

// trayState is everything the tray menu shows.
type trayState struct {
	Agents   events.State
	Profiles []string
	Active   string
	Orient   profile.Orientation
	Follow   bool
	Mode     Mode
	Waiting  int // sessions waiting for the user
}

// trayModel builds the tray menu: status · Jump to waiting agent · separator ·
// Mode ▸ · Profile ▸ · Orientation ▸ · LED follows agents · separator ·
// Show window · Quit.
func trayModel(s trayState) []trayItem {
	profiles := make([]trayItem, 0, len(s.Profiles))
	for _, name := range s.Profiles {
		profiles = append(profiles, trayItem{Label: name, Action: trayProfile, Arg: name, Checked: name == s.Active})
	}
	if len(profiles) == 0 {
		profiles = append(profiles, trayItem{Label: "No profiles", Disabled: true})
	}
	var orients []trayItem
	for _, o := range profile.Orientations() {
		orients = append(orients, trayItem{Label: o.Label(), Action: trayOrientation, Arg: string(o), Checked: o == s.Orient})
	}
	modes := []trayItem{
		{Label: "Plain", Action: trayMode, Arg: string(ModePlain), Checked: s.Mode != ModeGesture},
		{Label: "Gesture", Action: trayMode, Arg: string(ModeGesture), Checked: s.Mode == ModeGesture},
	}
	return []trayItem{
		{Label: trayStatus(s.Agents), Disabled: true},
		{Label: jumpLabel(s.Waiting), Action: trayJump, Disabled: s.Waiting == 0},
		{Separator: true},
		{Label: "Mode", Children: modes},
		{Label: "Profile", Children: profiles},
		{Label: "Orientation", Children: orients},
		{Label: "LED follows agents", Action: trayFollow, Checked: s.Follow},
		{Separator: true},
		{Label: "Show window", Action: trayShow},
		{Label: "Quit", Action: trayQuit, Quit: true},
	}
}

func trayStatus(s events.State) string {
	switch s {
	case events.StateAttention:
		return "Agents: an agent needs you"
	case events.StateBusy:
		return "Agents: working"
	}
	return "Agents: idle"
}

// jumpLabel is the jump action label, with a count when several sessions wait.
func jumpLabel(waiting int) string {
	if waiting > 1 {
		return fmt.Sprintf("Jump to waiting agent (%d)", waiting)
	}
	return "Jump to waiting agent"
}
