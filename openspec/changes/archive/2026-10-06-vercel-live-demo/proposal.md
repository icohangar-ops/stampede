## Why

The public demo has to run on Vercel. Google Cloud billing was cancelled, and Product Hunt needs a URL that does not depend on Cloud Run, Firestore, or Vertex.

## What Changes

- Add a Node API next to the existing Vite UI so the product loop runs as Vercel functions: ownership, a 30 second preset, a live chart, a template report, and a badge.
- Embed the Northwind demo shop on the same deployment so "Use the demo shop" works without DNS.
- Keep the Go orchestrator and `deploy.sh` as the local and optional self-host path.
- Replace Cloud Run and Vertex wording in the UI and README with Vercel, and document Hobby versus Pro function duration.

## Capabilities

### New Capabilities
- `vercel-hosting`: The public deployment, the embedded demo shop, and the function duration budget.

### Modified Capabilities
- `readiness-report`: The hosted report prices Vercel Functions. The self-host path can still price Cloud Run.

## Impact

- New `api/` functions, `vercel.json`, and root `package.json` tests.
- UI copy in `web/`. The Go services stay in place.
- No secrets for the basic demo. Runtime Cache is used when the code is running on Vercel.
