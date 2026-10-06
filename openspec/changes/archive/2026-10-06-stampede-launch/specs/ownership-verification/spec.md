# Spec Delta

## Purpose

Prove the caller controls a website before Stampede sends any load to it, so the product cannot be aimed at someone else's host.

## ADDED Requirements

### Requirement: Challenge issued for a public URL
The system SHALL accept a target URL and return a single-use verification challenge containing a token, the well-known file URL, and the DNS TXT name and value. The system MUST reject URLs that are not http or https, that embed userinfo, or that fail the SSRF policy.

#### Scenario: Challenge for a normal URL
- **WHEN** a client submits `https://example.com/pricing`
- **THEN** the system returns a challenge token, a file URL on `example.com` under `/.well-known/`, and a DNS TXT record name and value for that host

#### Scenario: Challenge rejected for a private target
- **WHEN** a client submits a URL whose host resolves to a loopback, link-local, or private address and the host is not on the operator allowlist
- **THEN** the system rejects the challenge and stores no verification

### Requirement: File ownership check
The system SHALL treat a host as verified only when an HTTP GET of `/.well-known/stampede-<token>.txt` on that host returns a body whose trimmed contents equal the challenge token.

#### Scenario: Matching token file
- **WHEN** the client asks to verify by file and the well-known URL body equals the challenge token
- **THEN** the challenge is marked verified

#### Scenario: Wrong token file
- **WHEN** the client asks to verify by file and the body does not equal the token, the request fails, or the response is too large
- **THEN** the challenge stays unverified

### Requirement: DNS ownership check
The system SHALL treat a host as verified when a DNS TXT lookup of the challenge name returns the exact challenge value.

#### Scenario: Matching TXT record
- **WHEN** the client asks to verify by DNS and the TXT answer equals the issued value
- **THEN** the challenge is marked verified

#### Scenario: Missing TXT record
- **WHEN** the client asks to verify by DNS and no TXT answer equals the issued value
- **THEN** the challenge stays unverified

### Requirement: No traffic before verification
The system MUST NOT open a load-bearing connection to the target host until a non-expired challenge for that exact host is verified.

#### Scenario: Run blocked
- **WHEN** a client requests a run with a missing, unverified, or expired challenge
- **THEN** the system rejects the run and the target receives no load requests

#### Scenario: Run allowed after verification
- **WHEN** a client requests a run with a verified, unexpired challenge for the same host
- **THEN** the system accepts the run and sends load only to that host
