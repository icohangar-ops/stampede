# Spec Delta

## MODIFIED Requirements

### Requirement: Cost estimate is labeled as an approximation
The cost estimate MUST state that it is a list-price approximation and not an invoice. The optional Cloud Run self-host path approximates Cloud Run request-based billing. The Vercel deployment approximates Vercel Functions Fluid Compute list price.

#### Scenario: Cost note is present
- **WHEN** a visitor reads the cost line in the report
- **THEN** the estimate includes the currency amount and a note that it is an approximation and not an invoice

#### Scenario: Vercel report names Vercel
- **WHEN** a run finishes on the Vercel deployment
- **THEN** the cost note names Vercel Functions and states that it is not an invoice
