# Tasks

## 1. Safety core

- [x] 1.1 Add the URL and dial policy (private ranges, metadata always blocked, allowlist, redirects, caps) and verify `go test ./internal/safety` covers SSRF cases and cap clamping
- [x] 1.2 Add Top 5 and #1 presets with documented assumptions and a 10-to-50 worker ramp, and verify `go test ./internal/preset` checks peak order and ramp endpoints

## 2. Ownership and storage

- [x] 2.1 Add the store interface plus memory and SQLite implementations (challenges, runs, samples, quotas, opt-in, kill flag) and verify quota reservation tests pass
- [x] 2.2 Add file and DNS verification on the safe HTTP client and verify mismatch, match, and expiry tests pass
- [x] 2.3 Add a Firestore store implementation selected by configuration, and verify the package builds with `go test ./internal/store/...`

## 3. Load, report, badge

- [x] 3.1 Add the GET-only engine with per-second samples, breaking point, kill checks, and a same-origin asset probe, and verify engine tests reject non-GET, honor caps, and stop when killed
- [x] 3.2 Add the template report, Cloud Run cost estimate, and Vertex Gemini client with template fallback, and verify report tests cover the degraded headline and the fallback
- [x] 3.3 Add SVG badge rendering and verify launch-ready versus needs-work tests pass

## 4. API and services

- [x] 4.1 Add the orchestrator HTTP API (challenge, verify, presets, runs, SSE samples, report, badge, admin kill, internal task protocol) and verify API tests block unverified runs, quotas, and the kill switch
- [x] 4.2 Add the loadgen command and Cloud Run job launcher, and verify a fake launcher test plus the internal sample-merge path
- [x] 4.3 Add the report service command, the demo target (token file, slow asset, degrading homepage), and the web static server with `/v1` proxy

## 5. Product UI

- [x] 5.1 Build the dark mobile-friendly UI for URL entry, ownership proof, preset choice, live chart, report, badge, and terms, and verify `npm run build` succeeds
- [x] 5.2 Add the result page and badge embed, and verify a completed run renders headline, metrics, and badge in the UI flow

## 6. Local run, deploy, docs

- [x] 6.1 Add docker-compose, `scripts/dev.sh`, and CI so `go test ./...` and the web build run without GCP, and verify those commands pass
- [x] 6.2 Add `deploy.sh`, Cloud Build, and Dockerfiles that enable APIs, build images, deploy web, orchestrator, report, and the loadgen job with IAM, and verify the script is present and rejects missing args
- [x] 6.3 Write the README with the mermaid architecture, traffic assumptions, local run steps, GCP roles, and the budget note, and verify the traffic assumptions from the preset spec are present

## 7. End-to-end check

- [x] 7.1 Run the local demo from verify through ramp, live samples, report, and badge, and verify an API-level end-to-end test plus a browser pass of that flow

## Workflow follow-up

- Archive the change after implementation is verified.
- Open the pull request to main.
