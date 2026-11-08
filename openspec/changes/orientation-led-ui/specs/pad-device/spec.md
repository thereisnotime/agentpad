## ADDED Requirements

### Requirement: Model-aware LED control
The system SHALL expose LED capabilities per pad model and SHALL only offer settings the model supports: effect modes by index for `8890`, and per-layer effects with colours for `8840` and `8842`.

#### Scenario: 8890 board
- **WHEN** the connected pad is `1189:8890`
- **THEN** LED modes are selectable by index and no colour choice is offered

#### Scenario: 8842 board
- **WHEN** the connected pad is `1189:8842`
- **THEN** each of the three layers can be given an effect and a colour

#### Scenario: Unsupported combination
- **WHEN** a colour is requested for a model or effect without colour support
- **THEN** the write is rejected with an error before any device access
