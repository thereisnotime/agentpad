## Purpose

Provide per-agent key profiles (Claude Code, Codex, OpenCode, media) that users can edit, persist and apply to the pad.

## ADDED Requirements

### Requirement: Built-in profiles
The system SHALL ship built-in profiles named `claude-code`, `codex`, `opencode` and `media`, each defining three keys and the knob actions (counter-clockwise, press, clockwise).

#### Scenario: First run
- **WHEN** no user profiles exist
- **THEN** the four built-in profiles are available

### Requirement: Edit and persist profiles
The system SHALL let the user edit any key or knob action of a profile and persist the result in the user configuration directory; a persisted profile SHALL override the built-in profile of the same name.

#### Scenario: Save edit
- **WHEN** the user changes key 1 of `codex` to `enter` and saves
- **THEN** the profile is stored and still shows `enter` after an application restart

#### Scenario: Reset
- **WHEN** the user resets a profile
- **THEN** the persisted override is removed and the built-in values are shown

### Requirement: Validate actions
The system SHALL reject a profile containing an action the pad cannot encode and SHALL identify the offending control.

#### Scenario: Invalid action
- **WHEN** a key is set to an unknown key name
- **THEN** saving fails with an error naming that key

### Requirement: Layer assignment on apply
When applying, the system SHALL place the selected profile on layer 1 and the next two profiles, in list order, on layers 2 and 3.

#### Scenario: Apply codex
- **WHEN** the user applies `codex`
- **THEN** layer 1 contains the `codex` mapping
