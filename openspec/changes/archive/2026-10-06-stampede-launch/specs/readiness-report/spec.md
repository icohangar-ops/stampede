# Spec Delta

## Purpose

Turn a finished run into a verdict a maker can act on, including what to fix and what that traffic would cost on Cloud Run.

## ADDED Requirements

### Requirement: Report after the run
When a run finishes, the system SHALL produce a report containing a verdict headline, a short summary, at least one concrete fix, the requests per second past which latency or errors degrade when a breaking point exists, and a Cloud Run cost estimate for a launch day at the observed shape.

#### Scenario: Completed run has a report
- **WHEN** a run reaches a completed status
- **THEN** fetching the run returns a report with a headline, summary, fixes, and a Cloud Run cost estimate

#### Scenario: Degraded headline names a rate
- **WHEN** a run recorded a breaking point
- **THEN** the report headline states that p95 or errors degrade past that approximate requests-per-second value

### Requirement: Template fallback
The system SHALL generate the report with a deterministic template when no large-language-model backend is configured or when that backend fails. The numeric verdict and cost MUST come from the measured samples, not from model prose.

#### Scenario: No model configured
- **WHEN** a run completes and no model backend is configured
- **THEN** the report is still returned and is marked as a template report

#### Scenario: Model failure falls back
- **WHEN** the model backend returns an error or unusable output
- **THEN** the system returns the template report instead of failing the run

### Requirement: Gemini when configured
When Vertex AI settings are present, the report step MUST call Gemini and MAY use the model's wording for the summary and fixes. Measured rates, the breaking point, and the cost estimate MUST remain the values computed from samples.

#### Scenario: Model wording with measured numbers
- **WHEN** Gemini returns a usable report and the samples contain a breaking point
- **THEN** the stored report keeps the sample-derived breaking point and cost estimate

### Requirement: Cost estimate is labeled
The cost estimate MUST state that it is a list-price approximation for Cloud Run request-based billing and not an invoice.

#### Scenario: Estimate carries the label
- **WHEN** a report includes a cost estimate
- **THEN** the estimate includes the currency amount and a note that it is an approximation of Cloud Run list price
