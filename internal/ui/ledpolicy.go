package ui

import (
	"encoding/json"
	"fmt"

	"github.com/thereisnotime/agentpad/internal/events"
	"github.com/thereisnotime/agentpad/internal/pad"
)

// LEDPolicy is what the LED shows while following agents.
type LEDPolicy struct {
	Attention pad.LEDSetting `json:"attention"` // an agent needs you
	Otherwise pad.LEDSetting `json:"otherwise"`
}

// ledPolicy maps the aggregate agent state to an LED setting.
func ledPolicy(s events.State, p LEDPolicy) pad.LEDSetting {
	if s == events.StateAttention {
		return p.Attention
	}
	return p.Otherwise
}

// perLayerLED reports whether a product uses the 884x style per-layer LEDs.
func perLayerLED(pid uint16) bool { return pid == 0x8840 || pid == 0x8842 }

// ledFamily names the preference bucket for a product's LED settings.
func ledFamily(pid uint16) string {
	if perLayerLED(pid) {
		return "884x"
	}
	return "8890"
}

// ledPolicyKey is the preference key holding the policy JSON for pid.
func ledPolicyKey(pid uint16) string { return "led.policy." + ledFamily(pid) }

// ledLabelKey is the preference key of the user label for 8890 mode n.
func ledLabelKey(n int) string { return fmt.Sprintf("ledLabel.%d", n) }

// defaultLEDPolicy: 8890 mode 1 / mode 0; 884x orange backlight / off (layer 1).
func defaultLEDPolicy(pid uint16) LEDPolicy {
	if perLayerLED(pid) {
		return LEDPolicy{
			Attention: pad.LEDSetting{Effect: "backlight", Color: "orange"},
			Otherwise: pad.LEDSetting{Effect: "off"},
		}
	}
	return LEDPolicy{Attention: pad.LEDSetting{Mode: 1}, Otherwise: pad.LEDSetting{Mode: 0}}
}

// decodeLEDPolicy parses stored JSON, falling back to the defaults for pid
// when it is empty, broken, or invalid for the model (validate may be nil).
func decodeLEDPolicy(raw string, pid uint16, validate func(pad.LEDSetting) error) LEDPolicy {
	def := defaultLEDPolicy(pid)
	if raw == "" {
		return def
	}
	var p LEDPolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return def
	}
	if validate != nil && (validate(p.Attention) != nil || validate(p.Otherwise) != nil) {
		return def
	}
	return p
}

func encodeLEDPolicy(p LEDPolicy) string {
	b, _ := json.Marshal(p) // plain struct, cannot fail
	return string(b)
}
