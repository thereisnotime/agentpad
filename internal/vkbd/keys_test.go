package vkbd

import "testing"

func TestKeyName(t *testing.T) {
	for code, want := range map[uint16]string{30: "A", 28: "Enter", 1: "Esc", 29: "Ctrl", 115: "Volumeup", 9999: "keycode 9999"} {
		if got := KeyName(code); got != want {
			t.Errorf("KeyName(%d) = %q, want %q", code, got, want)
		}
	}
}
