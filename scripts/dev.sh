#!/usr/bin/env bash
# Local demo: demo shop, orchestrator, and the web UI. No Google Cloud credentials.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p data

if [[ ! -d web/node_modules ]]; then
  (cd web && npm ci)
fi
(cd web && npm run build)

cleanup() {
  jobs -p | xargs -r kill
}
trap cleanup EXIT INT TERM

PORT=8090 go run ./cmd/target &
PORT=8081 \
  STORE=sqlite \
  DATABASE_PATH=data/stampede.db \
  ARTIFACT_DIR=data/artifacts \
  ALLOW_HOSTS=localhost,127.0.0.1 \
  DEMO_URL_PUBLIC=http://localhost:8090 \
  LOADGEN_MODE=inprocess \
  ADMIN_TOKEN=local-dev-admin \
  INTERNAL_TOKEN=local-dev-internal \
  STAMPEDE_ENV=local \
  MAX_RPS=40 \
  MAX_DURATION=180s \
  MAX_WORKERS=50 \
  go run ./cmd/orchestrator &

ready=0
for _ in $(seq 1 90); do
  if curl -sf http://127.0.0.1:8081/healthz >/dev/null && curl -sf http://127.0.0.1:8090/healthz >/dev/null; then
    ready=1
    break
  fi
  sleep 0.5
done
if [[ "$ready" -ne 1 ]]; then
  echo "demo shop or orchestrator did not become healthy" >&2
  exit 1
fi

PORT=8080 \
  ORCHESTRATOR_URL=http://127.0.0.1:8081 \
  STATIC_DIR=web/dist \
  go run ./cmd/web
