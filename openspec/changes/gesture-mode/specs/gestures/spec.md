## Purpose

Recognise tap, hold, double-tap, chord and knob press+rotate gestures on the pad and emit a configurable action for each, while agentpad runs.

## ADDED Requirements

### Requirement: Gesture mode programming
In gesture mode the system SHALL program each pad control to a distinct otherwise-unused key and SHALL restore the plain mapping when gesture mode is turned off.

#### Scenario: Enable gesture mode
- **WHEN** the user enables gesture mode
- **THEN** the pad is programmed with distinct keys per control and agentpad starts reading the pad

#### Scenario: Quit in gesture mode
- **WHEN** agentpad quits (window, tray, SIGINT or SIGTERM) while gesture mode is on
- **THEN** gesture mode stops and the active profile's plain mapping is uploaded again

#### Scenario: Missing permission
- **WHEN** the user enables gesture mode without access to the pad's input devices or to a virtual keyboard
- **THEN** agentpad stays in plain mode and shows the fix (`just udev-install`, then replug the pad)

### Requirement: Exclusive input
While gesture mode is active the system SHALL grab the pad's input devices so raw gesture keys do not reach other applications, and SHALL release them on exit.

#### Scenario: Raw keys hidden
- **WHEN** a key is tapped in gesture mode
- **THEN** only the mapped action is delivered to the focused application

### Requirement: Key gestures
The system SHALL distinguish tap, hold, long hold and double-tap per key using configurable thresholds.

#### Scenario: Hold
- **WHEN** key 1 is pressed for longer than the hold threshold
- **THEN** the hold action of key 1 is emitted once and the tap action is not

#### Scenario: Double-tap
- **WHEN** key 3 is tapped twice within the double-tap window and has a double-tap action
- **THEN** only the double-tap action is emitted

### Requirement: Knob press and rotate
The system SHALL emit press+rotate actions when the knob is rotated while pressed, detected by a knob-press release coinciding with a rotation detent.

#### Scenario: Press and rotate clockwise
- **WHEN** the knob is held and rotated two detents clockwise
- **THEN** the press+rotate clockwise action is emitted twice and no plain rotate or press action is emitted

### Requirement: Chords
The system SHALL emit a chord action when two keys are pressed within the chord window.

#### Scenario: Key 1 + key 3
- **WHEN** keys 1 and 3 are pressed within 80 ms
- **THEN** the chord action for 1+3 is emitted and neither key's own action is emitted

### Requirement: Live input observation
The system SHALL be able to read the pad's key events without grabbing them, so keys still reach the focused application, and SHALL map them to controls using the active profile, reporting controls that cannot be told apart.

#### Scenario: Ambiguous plain mapping
- **WHEN** key 1 and the knob press both send Enter in plain mode
- **THEN** an Enter press is shown as key 1 and the mapping is reported as ambiguous

### Requirement: Reserved agentpad actions
Gesture actions named `agentpad:<name>` SHALL be handled by agentpad instead of being typed; the known names are `jump` (jump to the waiting agent) and `next-profile` (switch to the next profile). Unknown names SHALL be rejected by profile validation, and agentpad actions SHALL NOT be allowed in the plain mapping.

#### Scenario: Jump from the pad
- **WHEN** the knob is long-held in gesture mode with the claude-code profile
- **THEN** agentpad jumps to the waiting agent and no key is typed

### Requirement: Gesture editing and monitor
In gesture mode the profile editor SHALL show one action slot per gesture kind of the selected control and the chord slots, with tap and rotate falling back to the plain action, and SHALL list the most recently recognised gestures.

#### Scenario: Recognised gesture
- **WHEN** key 1 is held in gesture mode and its hold action is ctrl-c
- **THEN** the monitor shows "key 1 · hold → ctrl-c"
