package ui

import (
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
}

// trayModel builds the tray menu: status · jump (placeholder) · separator ·
// Profile ▸ · Orientation ▸ · LED follows agents · separator · Show window · Quit.
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
	return []trayItem{
		{Label: trayStatus(s.Agents), Disabled: true},
		// Placeholder: the jump-to-session backend is built separately.
		{Label: "Jump to waiting agent", Disabled: true},
		{Separator: true},
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
