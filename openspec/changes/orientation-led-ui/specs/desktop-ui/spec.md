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

### Requirement: Live input feedback
When a live pad input source is configured and the window is visible, the profile editor SHALL animate the drawn control that was pressed or turned and show the last input below the pad.

#### Scenario: Knob turn
- **WHEN** the user turns the knob clockwise by one detent
- **THEN** the knob indicator ticks clockwise, the turn chip flashes and the caption reads "knob ↻"

### Requirement: Bug reports from the app
The About screen SHALL open a prefilled GitHub bug report with the app version, OS and pad, and SHALL offer the same diagnostics as copyable text.

#### Scenario: Report a bug
- **WHEN** the user presses "Report a bug"
- **THEN** the browser opens the bug template with version, OS and pad filled in

### Requirement: Open snippet target files
The Setup screen SHALL offer an "Edit file" action per snippet that opens the snippet's target file in the user's default editor. A missing agent config file SHALL only be created empty after confirmation and never overwritten; the udev rule SHALL only be opened when it exists. agentpad SHALL NOT write snippet content into these files.

#### Scenario: Missing Claude settings
- **WHEN** the user clicks Edit file for Claude Code and `~/.claude/settings.json` does not exist
- **THEN** agentpad asks before creating it empty, then opens it in the default editor
