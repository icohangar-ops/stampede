# Design

## Context

See proposal.md for why Stampede exists. The repository starts empty. There is no Google Cloud project in this environment. The product still has to deploy to Cloud Run later, and it has to run the full demo locally and in CI.

Judging for this hackathon rewards a working demo, more than one Cloud Run service, Gemini, and a clear scale-out story. Safety constraints in the proposal are release blockers.

## Goals / Non-Goals

**Goals:**

- One product behavior for local and Cloud Run, with storage, launch, and the report backend swapped by configuration.
- A load path whose caps and SSRF checks are enforced in the dialer, not only in the UI.
- A Cloud Run shape of `web` (scale to zero), `orchestrator`, `report`, and a `loadgen` job with parallel tasks.
- A local path where the same engine runs as in-process workers and SQLite replaces Firestore.
- Spend stays small: low max instances, a short job timeout, and app-level quotas. Document a budget alert. No GPUs.

**Non-Goals:**

- POST or authenticated user flows, multi-step browser journeys, or distributed attack features.
- Official Product Hunt traffic data. Presets are documented planning shapes.
- A production multi-tenant billing system.
- Running the load test at uncapped front-page volume.

## Decisions

### Services

- `web` serves the built SPA and reverse-proxies `/v1` to the orchestrator. On Cloud Run it attaches an identity token when an audience is set. It scales to zero.
- `orchestrator` owns challenges, verification, quotas, the kill switch, run records, the sample stream, and badges. It is the only component that decides a run may start.
- `loadgen` is a Cloud Run job. Each task pulls a plan, runs a slice of the curve, and posts per-second samples back. Locally the orchestrator runs the same engine in-process with task count 1, so the chart still ramps workers from 10 to 50.
- `report` is a separate Cloud Run service so Gemini sits behind its own IAM boundary. The orchestrator calls it when `REPORT_URL` is set. With no URL, the orchestrator uses the same report package in-process. Either way, a missing or failing model falls back to the template.

Alternative considered: one Cloud Run service for everything. Rejected because the hackathon story is multiple services plus a job that scales out, and a single service cannot show that.

### Storage

- A `Store` interface covers challenges, runs, samples, quotas, host opt-in, and the kill flag.
- Local and CI use SQLite (`modernc.org/sqlite`, no cgo) or an in-memory store in tests.
- Cloud Run uses Firestore in Native mode. Reports and badge SVG bytes also go to a blob store: a local directory, or a private Cloud Storage bucket. The API serves them; the bucket is not public.

Alternative considered: Firestore only, with a skip in tests. Rejected because CI and the local demo must work with no credentials.

### Safety dialer

- Every outbound HTTP call (verification and load) uses one client.
- The dialer resolves the host and connects to a pinned address that passed the policy. It does not dial the original name after the check, so a DNS rebinding between check and connect does not land on a new address.
- Blocked ranges include loopback, RFC1918, link-local, CGNAT (`100.64/10`), documentation, multicast, reserved, IPv6 unique-local and link-local, and cloud metadata (`169.254.169.254`, `metadata.google.internal`). Metadata stays blocked even if the hostname is allowlisted.
- Redirects are limited, must stay on the original host, and are checked again.
- Non-allowlisted single-label hostnames are rejected. The operator allowlist is how the local demo target (`localhost`, `target`) is reachable. Users cannot edit it.
- The engine constructs only `GET` requests. A non-GET plan is an error.

### Caps, quotas, kill switch

- Defaults: 40 requests/second, 50 workers, 180 second duration, 3 runs per domain per UTC day, 5 runs per client IP per UTC day. All are configurable downward; nothing the client sends can raise them.
- Preset duration default is 45 seconds so the pitch ("about 60 seconds" including verify) stays honest and the Cloud Run job stays cheap.
- Quotas are reserved in the same critical section as run creation.
- Kill switch sources, any of which stops new work: env `KILL_SWITCH`, a kill file, and a store flag toggled by `POST /v1/admin/kill` with a bearer token compared in constant time. The engine checks the flag between seconds and before each request batch.

### Presets

- Shared assumptions (also in the README and UI): a launch morning is front-loaded; plan on about 10 requests per visitor; compress the morning shape into the run; scale magnitude to the safety cap. Visitor bands are planning assumptions, not Product Hunt official figures. Top 5 uses roughly 2,000–4,000 launch-day uniques. Number one uses roughly 6,000–15,000, with a steeper first spike.
- Top 5 intensity climbs to 0.75 of the cap and eases off. Number one climbs steeply to 1.0 of the cap and decays slowly.
- Workers interpolate from 10 to 50 across the run, independent of intensity, so the chart shows scale-out even while RPS follows the curve. On Cloud Run, 10 parallel tasks split those workers and the RPS. The orchestrator sums samples per second.

### Breaking point and report

- Breaking point is the RPS of the first one-second sample whose error rate is above 5% or whose p95 is above 1.5s.
- The template report maps that, plus a one-shot same-origin asset probe (slow assets, large images), to fixes: caching, CDN, image weight, slow endpoints, and Cloud Run min instances / concurrency for launch hour.
- Cost model (labeled as a list-price estimate, request-based billing, us-central1 order of magnitude, free tier not subtracted): peak concurrency ≈ peak RPS × p95 seconds; instances = ceil(concurrency / 80) at 1 vCPU and 512 MiB; 2 hours at peak and 10 hours at 20% of peak; CPU $0.000024/vCPU-s, memory $0.0000025/GiB-s, requests $0.40 per million.
- Gemini (`gemini-2.5-flash` via Vertex `generateContent`) may rewrite headline, summary, and fixes. Breaking point, rates, and cost are overwritten with the computed values. Temperature is low. No GPU.

### Demo target

- A small first-party app serves a page, a generated hero image, a deliberately slow endpoint, and `/.well-known/stampede-<token>.txt` echoing the token (we own this host).
- Under rising RPS it adds latency and then returns 503 so a safe-capped run still shows a breaking point.
- Docker Compose rewrites the public demo URL (`http://localhost:8090`) to the internal service name. Production sets no allowlist and no demo URL.

### Local run and deploy

- `scripts/dev.sh` starts the demo target, orchestrator, and Vite UI. `docker-compose.yml` runs the same topology in containers.
- `deploy.sh PROJECT_ID REGION` enables APIs, creates Artifact Registry, Firestore, the bucket, service accounts, builds images, deploys the three services and the job, and wires invoker IAM. Tokens are generated into an untracked env file.
- Orchestrator runs with CPU always allocated so a run finishes even if the browser disconnects. Max instances stay at 2 (web, orchestrator) and 1 (report). The job is capped at 10 tasks, 180s timeout, no retries.
- Optional `--with-scheduler` creates a nightly Cloud Scheduler call to the opt-in retest endpoint. Default deploy does not, to avoid surprise traffic.

### Identity

- Browser talks only to `web`.
- `web` → orchestrator, `loadgen` → orchestrator, and orchestrator → `report` use Cloud Run invoker plus a shared internal token on job and cron routes.
- Public routes are the product API proxied by web. Internal routes require the internal token.

## Risks / Trade-offs

- [Preset RPS is far below a real #1 launch] → The UI and README say the shape is real and the magnitude is capped on purpose. The breaking point is still useful for a small app, and the demo target is built to fail inside the cap.
- [SQLite and Firestore drift] → One interface, and the HTTP tests run against the memory store. Firestore is exercised by compile plus a document-mapping test; full emulator coverage waits on a GCP project.
- [CPU always allocated on the orchestrator costs more than throttled CPU] → Max instances 2 and scale-to-zero keep it inside the budget. The README tells the operator to set a $25 budget alert.
- [Cooperative demo target echoes any well-formed token] → Only that allowlisted host does this. Every other host must actually serve the file or TXT record.
- [Merged p95 from parallel tasks is only as good as the latency lists they upload] → Each task sends the latencies for that second (small at the cap). The orchestrator concatenates them before computing percentiles.
- [No GCP credentials yet] → Deploy script is ready but unrun. Local mode is the verification path until Sam provides a project.

## Migration Plan

- First deploy is greenfield. `deploy.sh` is the rollout. Rollback is `gcloud run services delete` and `gcloud run jobs delete` for the Stampede names, plus deleting the bucket and Firestore database if desired. No existing users to migrate.
- Kill switch is the incident control: env flag or admin endpoint stops new load immediately.

## Open Questions

None that change the spec, the approach, or the task breakdown. GCP project id, region, and billing account arrive later and are inputs to `deploy.sh`, not design forks.
