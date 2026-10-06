# safe-load Specification

## Purpose

Keep Stampede from being used as a flood or SSRF tool by capping every run, restricting the request shape, and giving operators a kill switch.

## Requirements

### Requirement: GET-only requests
The load generator MUST send only HTTP GET requests. It MUST NOT send request bodies or any other method.

#### Scenario: Engine method lock
- **WHEN** a run is executing against a target
- **THEN** every request the target receives uses the GET method and an empty body

### Requirement: Hard caps
The system SHALL enforce operator caps on requests per second, run duration, and parallel workers. Client input MUST NOT raise those caps. Duration MUST be at most 3 minutes.

#### Scenario: Over-cap plan is clamped
- **WHEN** a run would otherwise exceed the configured requests-per-second, duration, or worker cap
- **THEN** the executed plan stays at or below each cap

#### Scenario: Duration ceiling
- **WHEN** a client requests a run longer than 3 minutes
- **THEN** the system refuses or shortens it so the run does not exceed 3 minutes

### Requirement: Daily quotas
The system SHALL limit how many runs a single target domain and a single client IP can start per UTC day.

#### Scenario: Domain quota exceeded
- **WHEN** a domain has already consumed its daily run quota
- **THEN** the system rejects another run for that domain

#### Scenario: IP quota exceeded
- **WHEN** a client IP has already consumed its daily run quota
- **THEN** the system rejects another run from that IP

### Requirement: SSRF protection
The system MUST refuse to connect to loopback, link-local, private, carrier-grade NAT, documentation, multicast, or otherwise non-public addresses, including cloud metadata addresses. The block MUST apply on every dial, including after redirects. An operator allowlist MAY permit specific demo hostnames to use non-public addresses, but metadata addresses stay blocked even then.

#### Scenario: Metadata address blocked
- **WHEN** a target host is `metadata.google.internal` or resolves to `169.254.169.254`
- **THEN** the system does not connect, even if that hostname is on the operator allowlist

#### Scenario: Redirect to a private address blocked
- **WHEN** a public URL redirects to a private or link-local address
- **THEN** the system aborts and does not send the follow-up request

#### Scenario: Allowlisted demo host
- **WHEN** the operator allowlist contains the demo target hostname and that host resolves to a private address that is not a metadata address
- **THEN** verification and load for that hostname are allowed

### Requirement: Kill switch
The system SHALL refuse new runs and stop issuing further load requests when the kill switch is engaged.

#### Scenario: New run refused
- **WHEN** the kill switch is on and a client requests a run
- **THEN** the system rejects the run

#### Scenario: Active run stops
- **WHEN** the kill switch is engaged while a run is in progress
- **THEN** the load generator stops sending further requests and the run ends as cancelled

### Requirement: Terms are visible
The system SHALL show terms that state the tool may only be used against sites the caller owns or is authorized to test, that traffic is GET-only and capped, and that runs can be stopped.

#### Scenario: Terms linked from the main page
- **WHEN** a visitor opens the main page
- **THEN** the page links to the terms and the terms include the ownership, GET-only, cap, and kill-switch statements
