# Spec Delta

## Purpose

Show a maker, while the run is still going, whether the site is keeping up.

## ADDED Requirements

### Requirement: Live sample stream
While a run is in progress, the system SHALL expose a stream of samples. Each sample MUST include requests per second, p50 latency, p95 latency, error rate, and worker count.

#### Scenario: Sample fields
- **WHEN** a client subscribes to a running run
- **THEN** each sample event contains requests per second, p50, p95, error rate, and workers

### Requirement: Breaking point
The system SHALL record a breaking point when a sample's error rate exceeds 5 percent or its p95 latency exceeds 1.5 seconds, using the requests per second of the earliest such sample.

#### Scenario: Errors trip the breaking point
- **WHEN** a sample first exceeds 5 percent errors
- **THEN** the run's breaking point is that sample's requests per second

#### Scenario: Latency trips the breaking point
- **WHEN** a sample's p95 first exceeds 1.5 seconds and the error rate has stayed at or below 5 percent
- **THEN** the run's breaking point is that sample's requests per second

#### Scenario: Clean run has no breaking point
- **WHEN** every sample stays within both thresholds
- **THEN** the run reports no breaking point

### Requirement: Chart can render during the run
The main UI SHALL plot the sample stream while the run is in progress, including requests per second and p95, and SHALL show the latest error rate and worker count.

#### Scenario: Live chart updates
- **WHEN** a run is in progress and new samples arrive
- **THEN** the chart and the numeric readouts reflect the latest sample without waiting for the run to finish
