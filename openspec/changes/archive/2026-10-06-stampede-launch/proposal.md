# Proposal

## Why

Makers about to launch on Product Hunt have no quick, safe way to learn whether their app will fold under front-page traffic. Stampede answers that in about a minute, and it has to ship on Cloud Run for the Product Hunt × Google Cloud Run hackathon (launch window Wednesday 14 October 2026) without being usable as a denial-of-service tool.

## What Changes

- Add a public web flow: paste a URL, prove ownership, pick a Product Hunt-shaped traffic preset, watch a live ramp, then receive a readiness report and a shareable badge.
- Add ownership proof via a well-known token file or a DNS TXT record before any load is sent.
- Add two documented traffic presets, "Top 5 of the Day" and "#1 Product of the Day", that ramp parallel workers from about 10 to 50 inside hard safety caps.
- Stream requests per second, p50, p95, error rate, worker count, and a breaking point while the run is in progress.
- Produce a readiness report with concrete fixes and a Cloud Run cost estimate. Use Vertex Gemini when configured, and a deterministic template when it is not.
- Issue a shareable badge (SVG) and a public result page.
- Run the same product locally and in CI with no Google Cloud credentials, and deploy it to Cloud Run (web service, orchestrator service, report service, load-generator job) with a one-command script.
- Optional nightly re-test for sites that opt in.
- Publish clear terms on the page.

## Capabilities

### New Capabilities

- `ownership-verification`: Prove control of a host before any run, by file or DNS TXT, and reject unverified targets.
- `safe-load`: GET-only traffic with hard caps, per-domain and per-IP quotas, SSRF blocking, a kill switch, and visible terms.
- `traffic-presets`: Product Hunt launch-day curve presets and the worker ramp those runs follow.
- `live-metrics`: Live samples for throughput, latency, errors, workers, and the breaking point.
- `readiness-report`: Post-run verdict, fixes, Cloud Run cost estimate, and pluggable Gemini or template generation.
- `shareable-result`: SVG badge and a result page someone can share after a run.

### Modified Capabilities

- None. This is a new product; `openspec/specs/` has no existing capabilities.

## Impact

- New services: web UI, orchestrator API, load generator, report step, and a local demo target.
- New public HTTP API under `/v1` for challenges, runs, events, reports, and badges.
- Persistence for runs and metrics (SQLite locally, Firestore on Cloud Run) and object storage for reports and badges (local disk or Cloud Storage).
- Deploy script, Cloud Build config, and IAM for Cloud Run, Artifact Registry, Firestore, Cloud Storage, Vertex AI, and optional Cloud Scheduler.
- Safety behavior is covered by automated tests and is a release blocker.
