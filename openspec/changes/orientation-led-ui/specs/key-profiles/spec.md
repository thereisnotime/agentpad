## ADDED Requirements

### Requirement: Orientation-aware mapping
The system SHALL generate the pad mapping for one of four orientations (normal, upside down, clockwise, counter-clockwise), keeping profile keys in the order they are seen by the user.

#### Scenario: Vertical placement
- **WHEN** the orientation is clockwise and the profile keys are `1`, `2`, `escape`
- **THEN** the top key as seen sends `1` and the bottom key sends `escape`

#### Scenario: Upside down
- **WHEN** the orientation is upside down
- **THEN** the leftmost key as seen sends the profile's first key
