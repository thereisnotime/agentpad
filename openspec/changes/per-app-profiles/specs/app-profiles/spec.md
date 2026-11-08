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

### Requirement: Agent origin capture
The hook command SHALL record where the calling agent runs: terminal identifiers (Konsole D-Bus service/window/session, `WINDOWID`), multiplexer identifiers (zellij session and pane, tmux pane), and the agent process ID, and attach them to the event without delaying the agent.

#### Scenario: Claude in zellij inside Konsole
- **WHEN** Claude Code runs in a zellij pane inside a Konsole tab and triggers a permission prompt
- **THEN** the received event carries the Konsole service, window and session, the zellij session name and pane ID, and the agent PID

### Requirement: Jump to waiting agent
On a jump request the system SHALL bring the waiting agent into focus: raise its terminal window, select its terminal tab, and focus its multiplexer pane; with several waiting sessions, successive jumps SHALL cycle through them oldest first.

#### Scenario: Jump from the browser
- **WHEN** a Claude session is waiting for permission in Konsole tab 3 / zellij pane 2 and the user, focused on a browser, triggers jump
- **THEN** the Konsole window is raised, tab 3 is selected and zellij pane 2 is focused

#### Scenario: Partial information
- **WHEN** the event has no multiplexer information
- **THEN** the jump still raises the window and selects the tab, and reports which steps were skipped

### Requirement: Attention notification
The system SHALL optionally (on by default) show a desktop notification when a session starts waiting, including agent, tool and directory, with a Jump action, and SHALL dismiss it when the session stops waiting.

#### Scenario: Notification jump
- **WHEN** the user clicks Jump on the notification
- **THEN** the same jump as the pad trigger is performed

#### Scenario: Session answered
- **WHEN** a waiting session continues (the prompt was answered in the terminal)
- **THEN** its notification is dismissed
