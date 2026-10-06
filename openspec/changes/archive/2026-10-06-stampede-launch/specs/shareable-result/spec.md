# Spec Delta

## Purpose

Let a maker share the outcome of a run as a badge and a result page.

## ADDED Requirements

### Requirement: SVG badge
The system SHALL serve an SVG badge for a finished run. A run that stayed inside the breaking thresholds MUST be labeled launch-ready. A run that recorded a breaking point MUST be labeled as not launch-ready.

#### Scenario: Launch-ready badge
- **WHEN** a completed run has no breaking point and an acceptable error rate
- **THEN** the SVG response contains the text "Launch-ready"

#### Scenario: Needs-work badge
- **WHEN** a completed run recorded a breaking point
- **THEN** the SVG response does not claim the run is launch-ready and contains a needs-work label

### Requirement: Result page
The system SHALL provide a page for a run id that shows the verdict, the key metrics, and the badge.

#### Scenario: Open a finished result
- **WHEN** a visitor opens the result page for a completed run
- **THEN** the page shows the report headline, peak requests per second, p95, error rate, and the badge

### Requirement: Unfinished run is not claimed ready
The system MUST NOT serve a launch-ready badge for a run that has not completed successfully.

#### Scenario: Running run
- **WHEN** a client requests the badge for a run that is still running or that failed
- **THEN** the response is either unavailable or an in-progress or failed label, not "Launch-ready"
