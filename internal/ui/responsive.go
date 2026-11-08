package ui

import (
	"strconv"
	"strings"
)

// Breakpoints (device independent pixels).
const (
	// wideBreakpoint switches the shell between sidebar (>=) and top tabs (<).
	wideBreakpoint = 720
	// paneBreakpoint switches two-pane screens between side by side and stacked.
	paneBreakpoint = 640
	sidebarWidth   = 188
	inspectorWidth = 330
)

// isWide reports whether a window of width w gets the sidebar layout.
func isWide(w float32) bool { return w >= wideBreakpoint }

// sideBySide reports whether a two-pane area of width w shows both panes in a row.
func sideBySide(w float32) bool { return w >= paneBreakpoint }

// columnsFor returns how many columns of at least minCol fit in width w with
// gap between them; always at least 1.
func columnsFor(w, minCol, gap float32) int {
	if minCol <= 0 || w <= minCol {
		return 1
	}
	n := int((w + gap) / (minCol + gap))
	return max(n, 1)
}

// parseWindowSize parses "WIDTHxHEIGHT" (e.g. "500x800"). Used by the
// AGENTPAD_WINDOW dev variable to open the window at a fixed size.
func parseWindowSize(s string) (w, h float32, ok bool) {
	a, b, found := strings.Cut(strings.ToLower(strings.TrimSpace(s)), "x")
	if !found {
		return 0, 0, false
	}
	wi, err1 := strconv.Atoi(a)
	hi, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || wi < 200 || hi < 200 {
		return 0, 0, false
	}
	return float32(wi), float32(hi), true
}

// screenNames are the navigation screens in order (lower case).
var screenNames = []string{"pad", "profiles", "sessions", "log", "setup", "about"}

// screenIndex returns the index of a screen name, or -1.
func screenIndex(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, n := range screenNames {
		if n == name {
			return i
		}
	}
	return -1
}
