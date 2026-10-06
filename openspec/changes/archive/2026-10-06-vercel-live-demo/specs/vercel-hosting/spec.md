# Spec Delta

## ADDED Requirements

### Requirement: Vercel product loop
The hosted deployment SHALL serve the existing UI and a Node API that accepts a URL, requires ownership proof, runs a capped traffic preset, streams live samples, and returns a template report plus a badge. The basic demo MUST NOT require secrets.

#### Scenario: Demo shop on the deployment
- **WHEN** a visitor chooses the demo shop on the hosted site
- **THEN** the target is a route on that same deployment, ownership is proved without DNS, and a preset run shows a chart, a report, and a badge

### Requirement: Function duration
The hosted preset SHALL be short enough to finish inside the configured function `maxDuration`. The safety ceiling of 3 minutes MUST still be enforced, and the deployment MUST refuse a curve that would outlive the function budget.

#### Scenario: Over-long curve refused
- **WHEN** a client requests a curve longer than the deployment's platform max
- **THEN** the system rejects it and does not start load

### Requirement: Same safety limits
The Node path MUST keep GET-only traffic, the rate and worker caps, daily quotas, SSRF checks, and the kill switch.

#### Scenario: Metadata target blocked
- **WHEN** a client submits a cloud metadata address
- **THEN** the system rejects it before any run starts
