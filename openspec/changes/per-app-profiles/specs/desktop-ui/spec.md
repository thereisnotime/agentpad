## ADDED Requirements

### Requirement: Active profile indicator and override
The window SHALL show the active profile and why it was chosen (rule name or manual), offer a rules editor, and let the user pin a profile to disable automatic switching.

#### Scenario: Pin
- **WHEN** the user pins `media`
- **THEN** focus changes no longer change the active profile until unpinned
