## Context

The UI is a Vite React app. The orchestrator, load generator, and demo shop are Go processes. Vercel cannot run that topology. The product loop and the safety caps still have to hold.

## Goals / Non-Goals

**Goals:**

- One deployment: static UI, Node API, and a demo shop route.
- In-process GET load so the chart and the breaking point do not depend on hundreds of extra function calls.
- A 30 second preset that finishes inside `maxDuration` 90.
- Template report only. No model key.

**Non-Goals:**

- Replacing `deploy.sh` or removing the Go services.
- A durable database. Runtime Cache is enough for quotas and shared results. Memory covers tests and a single instance.
- Nightly retests on Vercel.

## Decisions

1. **Node functions beside the Vite app, not a second product.** `vercel.json` builds `web/` and routes `/v1` to `api/v1/[...slug].ts`. The demo shop is `api/shop.ts` at `/demo`.
2. **The SSE request is the load generator.** A background task would freeze when the create-run response ends. `GET /v1/runs/:id/events` runs the curve and streams samples.
3. **Demo traffic stays in-process.** The public `/demo` page is real, but the ramp calls the same gate inside the function so the fold (slow at 24 req/s, 503 at 34) shows up on one instance.
4. **Ownership stays a live check for other hosts.** Challenge ids are signed so any instance can read the token. External runs fetch the well-known file or the TXT record again. A grant signed with the demo key only unlocks `/demo`. On this host, any other path is rejected.
5. **Duration.** Safety cap remains 180 seconds, 40 req/s, 50 workers. The hosted platform max is 40 seconds of curve time because slow responses stretch wall-clock. Default preset is 30 seconds. `maxDuration` is 90.
6. **Cost copy.** The Node report uses published Fluid Compute list prices and says it is not an invoice. The Go report is unchanged so self-host numbers stay Cloud Run list price.

## Risks / Trade-offs

- Runtime Cache can miss. The page embeds the badge SVG and keeps the run in sessionStorage.
- Per-instance quotas are weaker if the cache is down. `KILL_SWITCH=1` does not depend on the cache.
- A public default HMAC secret can mint demo grants. It cannot pass a live check for someone else's host.
