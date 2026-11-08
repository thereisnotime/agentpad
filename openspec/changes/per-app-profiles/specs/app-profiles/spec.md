## Purpose

Select the active key profile automatically from the focused window on X11 and KDE Plasma Wayland, using user-defined rules.

## ADDED Requirements

### Requirement: Focused window tracking on X11
On an X11 session the system SHALL track the focused window's class, title and process ID without polling.

#### Scenario: Focus change on X11
- **WHEN** the user focuses a different window in an X11 session
- **THEN** the system receives the new window's class, title and PID within 200 ms

### Requirement: Focused window tracking on KDE Wayland
On a KDE Plasma Wayland session the system SHALL track the focused window via a KWin script and SHALL unload that script when agentpad exits.

#### Scenario: Clean exit
- **WHEN** agentpad quits
- **THEN** the KWin script is unloaded and no agentpad script remains loaded in KWin

#### Scenario: Unsupported compositor
- **WHEN** the session is Wayland but not KDE Plasma
- **THEN** automatic switching is reported as unavailable and manual switching keeps working

### Requirement: Rules
The system SHALL evaluate ordered rules on window class, title and running agent, select the first matching rule's profile, and fall back to a default profile.

#### Scenario: Agent in terminal
- **WHEN** a terminal window running `claude` is focused and a rule maps agent `claude` to `claude-code`
- **THEN** `claude-code` becomes the active profile

#### Scenario: No match
- **WHEN** no rule matches the focused window
- **THEN** the default profile becomes active

### Requirement: Applying the selected profile
In gesture mode the system SHALL switch the active profile immediately; in plain mode it SHALL only rewrite the pad when enabled by the user and after the focus has been stable for the configured dwell time.

#### Scenario: Rapid alt-tab in plain mode
- **WHEN** the user switches focus several times within the dwell time
- **THEN** at most one pad write happens, for the window focused at the end
