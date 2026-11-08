## Purpose

Receive lifecycle events from coding agents (Claude Code, Codex CLI, OpenCode), track sessions, keep a decision log and drive the pad LED from aggregate agent state.

## ADDED Requirements

### Requirement: Local event receiver
The system SHALL accept agent events over HTTP on the loopback interface only.

#### Scenario: Loopback only
- **WHEN** the receiver starts
- **THEN** it listens on 127.0.0.1 and is not reachable from other hosts

### Requirement: Hook command
The system SHALL provide a `hook <agent>` command for `claude`, `codex` and `opencode` that forwards the agent payload to the receiver, finishes within 1 second, and always exits with status 0.

#### Scenario: Receiver not running
- **WHEN** an agent invokes the hook command and the receiver is not running
- **THEN** the command exits 0 without output that would affect the agent

#### Scenario: Codex payload in argv
- **WHEN** Codex invokes `hook codex '<json>'`
- **THEN** the JSON argument is forwarded as the event payload

### Requirement: Event normalisation
The system SHALL normalise agent payloads into events with agent, session ID, kind, tool, working directory and timestamp, keeping the raw payload; unrecognised payloads SHALL be kept with kind `other`.

#### Scenario: Claude permission prompt
- **WHEN** a Claude Code `Notification` payload with `notification_type` `permission_prompt` arrives
- **THEN** an event of kind `tool_pending` is recorded for that session

#### Scenario: Unknown payload
- **WHEN** a payload with an unknown event name arrives
- **THEN** it is logged with kind `other` and does not change session state

### Requirement: Session tracking and aggregate state
The system SHALL track the latest state per session and derive an aggregate state of `attention` (any session waiting for a permission decision), `busy` (any session working) or `idle`.

#### Scenario: Attention
- **WHEN** one session is waiting for permission and another is working
- **THEN** the aggregate state is `attention`

#### Scenario: Session ends
- **WHEN** a session ends
- **THEN** it no longer contributes to the aggregate state

### Requirement: Event log
The system SHALL append every received event as one JSON object per line to a log file in the user state directory.

#### Scenario: Append
- **WHEN** an event is received
- **THEN** one line is appended and existing lines are unchanged

### Requirement: LED follows state
The system SHALL map the aggregate state to an LED mode (default: `attention` = 1, otherwise 0) and request that mode from the pad whenever the aggregate state changes.

#### Scenario: Permission prompt lights the pad
- **WHEN** the aggregate state changes from `busy` to `attention`
- **THEN** LED mode 1 is requested
