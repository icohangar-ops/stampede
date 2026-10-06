# traffic-presets Specification

## Purpose

Offer realistic Product Hunt launch-day traffic shapes so a maker can see how a front-page spike feels, without exposing raw flood controls.

## Requirements

### Requirement: Named presets
The system SHALL offer at least two presets named "Top 5 of the Day" and "#1 Product of the Day". Each preset MUST describe its traffic shape and the planning assumptions behind it.

#### Scenario: Preset list
- **WHEN** a client requests the preset catalog
- **THEN** both presets are returned with a short description and the assumptions used to shape them

### Requirement: Worker ramp
A preset run SHALL increase parallel workers from about 10 at the start of the run to about 50 at the end, and MUST NOT exceed the worker cap.

#### Scenario: Ramp endpoints
- **WHEN** a preset run executes for its full duration
- **THEN** the first sample reports about 10 workers and the last sample reports about 50 workers, never above the cap

### Requirement: Distinct curve shapes
"#1 Product of the Day" MUST reach a higher fraction of the requests-per-second cap than "Top 5 of the Day", and its ramp to that peak MUST be steeper.

#### Scenario: Number one peaks higher
- **WHEN** both presets are sampled at the same safety cap
- **THEN** the peak requests per second of "#1 Product of the Day" is greater than the peak of "Top 5 of the Day"

### Requirement: Assumptions are documented
The system SHALL document, in the product UI and the repository README, that preset magnitudes are compressed launch-day shapes scaled under the safety cap, not an official Product Hunt traffic feed.

#### Scenario: README states the assumption
- **WHEN** a reader opens the README traffic section
- **THEN** it states the visitor-band assumptions, the requests-per-visitor assumption, the time compression, and the safety-cap scaling
