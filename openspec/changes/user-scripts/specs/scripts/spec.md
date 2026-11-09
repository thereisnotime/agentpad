## Purpose

Let users run their own commands from pad controls and on agentpad events, safely and observably.

## ADDED Requirements

### Requirement: Script definitions
The system SHALL read named scripts from the settings file, each with either an argument vector or an explicit shell command, plus optional working directory, environment and timeout.

#### Scenario: Invalid script
- **WHEN** a script defines both `run` and `shell`
- **THEN** validation fails naming the script

### Requirement: Pad-triggered scripts
The system SHALL run a script when a control's action is `run:<name>`, in gesture mode for any gesture slot and in plain mode for plain actions while agentpad runs.

#### Scenario: Gesture hold runs a script
- **WHEN** key 2 hold is `run:mic-off` in gesture mode and the user holds key 2
- **THEN** the `mic-off` script runs once and nothing is typed

#### Scenario: Plain mode
- **WHEN** key 3 is `run:deploy` in plain mode and agentpad is running
- **THEN** pressing key 3 runs `deploy`

#### Scenario: Plain mode on another layer
- **WHEN** layer 1 has `run:a` on key 1, layer 2 has `run:b` on key 1, and the pad is switched to layer 2
- **THEN** pressing key 1 runs `b`, because each script control gets its own key from F19 to F24 while keys are left

### Requirement: Event hooks
The system SHALL run hooked scripts when matching agentpad events occur, passing event data as JSON on stdin and as `AGENTPAD_*` environment variables.

#### Scenario: Attention hook
- **WHEN** a hook `on: attention, agent: claude` exists and a Claude session starts waiting
- **THEN** the script runs with `AGENTPAD_EVENT=attention` and the session's details

### Requirement: Bounded, non-blocking execution
Script runs SHALL have a timeout, SHALL NOT block event handling, the UI or the gesture engine, and SHALL be recorded with exit status, duration and captured output.

#### Scenario: Timeout
- **WHEN** a script exceeds its timeout
- **THEN** its process group is killed and the run is recorded as timed out

### Requirement: Import consent
Importing a bundle that adds or changes scripts or hooks SHALL require explicit per-item consent; items without consent SHALL NOT be applied.

#### Scenario: Untrusted bundle
- **WHEN** a bundle adds a hook and the user doesn't tick it
- **THEN** the rest of the bundle is imported and the hook is not
