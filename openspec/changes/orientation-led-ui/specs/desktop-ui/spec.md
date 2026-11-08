## ADDED Requirements

### Requirement: Responsive layout
The window SHALL remain fully usable from 480 px to full-screen widths, switching to a compact layout below 720 px, without clipped controls or horizontal page scrolling.

#### Scenario: Narrow window
- **WHEN** the window is resized to 500 px wide
- **THEN** navigation and the pad editor reflow into a single column and every control stays reachable

### Requirement: Non-blocking device access
The UI SHALL stay interactive while device writes run, and agent event handling SHALL NOT wait for device writes.

#### Scenario: Slow device
- **WHEN** a pad write takes several seconds
- **THEN** the window keeps responding and hook requests are answered immediately

### Requirement: Rotatable pad editor
The profile editor SHALL draw the pad in the configured orientation and let the user change orientation from the editor.

#### Scenario: Rotate
- **WHEN** the user rotates the pad clockwise in the editor
- **THEN** the drawing shows the keys stacked vertically with the knob below and the orientation is saved
