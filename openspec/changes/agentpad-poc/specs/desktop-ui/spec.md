## Purpose

Native desktop window and system tray that expose pad status, profile editing, live agent sessions, the event log and hook setup snippets.

## ADDED Requirements

### Requirement: Main window
The system SHALL provide a native desktop window with sections for the pad, profiles, sessions, log and setup.

#### Scenario: Launch
- **WHEN** the user starts `agentpad` without arguments
- **THEN** the main window opens showing pad connection status

### Requirement: Profile editor
The window SHALL show the selected profile as a visual 3-key + knob layout where each control can be edited, saved, reset and applied to the pad.

#### Scenario: Apply from UI
- **WHEN** the user clicks Apply with a connected pad
- **THEN** the profile is written and the result is shown

### Requirement: Live sessions and log
The window SHALL list active agent sessions with agent, directory, state and last tool, and show the most recent log events, updating without user action.

#### Scenario: New event
- **WHEN** an event arrives
- **THEN** the session list and log update within one second

### Requirement: Setup snippets
The window SHALL show copyable hook configuration for Claude Code, Codex and OpenCode and the udev rule, and SHALL NOT modify agent configuration files itself.

#### Scenario: Copy snippet
- **WHEN** the user clicks Copy on the Claude Code snippet
- **THEN** the snippet is placed on the clipboard

### Requirement: Tray indicator
The system SHALL show a tray icon whose appearance reflects the aggregate agent state, and closing the window SHALL keep the app running in the tray.

#### Scenario: Close to tray
- **WHEN** the user closes the main window
- **THEN** the process keeps running and the window can be reopened from the tray
