## Purpose

Detect a connected CH57x macro pad and write key mappings and LED modes to it, so the pad behaves as an agent controller on any host.

## ADDED Requirements

### Requirement: Pad detection
The system SHALL detect a connected pad by USB vendor/product ID `1189:8890`, `1189:8840` or `1189:8842` without requiring root privileges.

#### Scenario: Pad connected
- **WHEN** a supported pad is enumerated on USB
- **THEN** the system reports it as connected with its vendor/product ID and bus path

#### Scenario: No pad
- **WHEN** no supported device is enumerated
- **THEN** the system reports the pad as disconnected and device writes are disabled

### Requirement: Write key mapping
The system SHALL write a key mapping of up to three layers (3 keys and 1 knob per layer) to the pad.

#### Scenario: Successful upload
- **WHEN** the user applies a profile and the pad is connected and writable
- **THEN** the mapping is written and the system reports success

#### Scenario: Permission denied
- **WHEN** the write fails because the user lacks access to the USB device
- **THEN** the system reports a permission error that includes the udev rule needed to fix it

#### Scenario: Programming tool missing
- **WHEN** the device programming backend is unavailable
- **THEN** the system reports it as unavailable with an install hint and does not crash

### Requirement: Set LED mode
The system SHALL set the pad LED mode by index and SHALL NOT issue more than one LED write per 2 seconds or repeat a write for an unchanged mode.

#### Scenario: Mode change
- **WHEN** a different LED mode is requested
- **THEN** the mode is written to the pad once

#### Scenario: Unchanged mode
- **WHEN** the currently applied LED mode is requested again
- **THEN** no device write occurs
