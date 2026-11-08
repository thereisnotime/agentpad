## Purpose

Keep agentpad configuration in a human-editable, versioned settings file and let users export, import and share settings and profiles safely.

## ADDED Requirements

### Requirement: Human-editable settings file
The system SHALL store application settings in `settings.yaml` in the agentpad config directory, with a schema version, documented defaults and comments on first creation.

#### Scenario: First start
- **WHEN** agentpad starts and no settings file exists
- **THEN** a commented settings file with defaults (and any migrated preferences) is created

### Requirement: Live reload and validation
The system SHALL apply valid external edits to the settings file within 2 seconds, keep the last valid settings when an edit is invalid, and report the error with its location.

#### Scenario: Valid edit
- **WHEN** the user changes `pad.orientation` to `clockwise` in an editor and saves
- **THEN** the app uses the clockwise orientation without restart

#### Scenario: Invalid edit
- **WHEN** the user saves `pad.orientation: sideways`
- **THEN** the previous orientation stays active and an error naming the field and line is shown

### Requirement: Preserve user content
The system SHALL NOT drop unknown keys or user comments when it writes the settings file.

#### Scenario: Unknown key
- **WHEN** the file contains `experimental: {foo: 1}` and the app saves a setting
- **THEN** `experimental` is still present afterwards and a warning lists it as unknown

### Requirement: Export bundle
The system SHALL export settings and profile overrides (optionally all profiles) to a single bundle file with schema version and exporting app version.

#### Scenario: Export
- **WHEN** the user exports
- **THEN** a `.agentpad.yaml` bundle containing settings and profiles is written

### Requirement: Import with preview
The system SHALL validate an imported bundle completely, show a preview of additions and changes, support merge and replace, back up current configuration before applying, and apply nothing if validation fails.

#### Scenario: Merge import
- **WHEN** a bundle with a new profile `zellij-claude` and a changed `led.attention` is imported with merge
- **THEN** the preview lists one added profile and one changed setting, and after confirming both are applied and a backup exists

#### Scenario: Invalid bundle
- **WHEN** a bundle contains a profile with an unknown key name
- **THEN** import is refused with the profile and control named, and nothing changes

### Requirement: Share a single profile
The system SHALL export a single profile as a file or clipboard text and import one from either, renaming on name conflicts.

#### Scenario: Paste profile
- **WHEN** the user pastes profile YAML named `codex` while `codex` already has an override
- **THEN** it is imported as `codex-2`
