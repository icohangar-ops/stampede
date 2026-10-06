#!/usr/bin/env bash
# Deploy Stampede to Cloud Run.
# Usage: ./deploy.sh PROJECT_ID REGION [--with-scheduler]
#
# This script does not buy anything except the Cloud Run / build usage it
# creates. Read the budget note it prints before you point a real site at it.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: ./deploy.sh PROJECT_ID REGION [--with-scheduler]" >&2
  exit 2
fi

PROJECT_ID="$1"
REGION="$2"
WITH_SCHEDULER=0
if [[ "${3:-}" == "--with-scheduler" ]]; then
  WITH_SCHEDULER=1
elif [[ -n "${3:-}" ]]; then
  echo "unknown argument: $3" >&2
  exit 2
fi

cat <<'EOF'
================================================================
Stampede spend cap
================================================================
App limits this deploy installs:
  - 40 requests/second, 50 workers, 3 minute runs
  - 3 runs per domain per UTC day, 5 per client IP
  - web max instances 2, orchestrator max instances 2, report max instances 1
  - loadgen job: 10 parallel tasks, 180s timeout, 0 retries
  - no GPUs

A single demo run is a few cents. A busy afternoon of demos stays well
under $10. Do not let the project drift past $50.

Set a $25 budget alert before you share the URL. Budgets need a billing
account, so this script does not create one for you:

  gcloud billing budgets create \
    --billing-account=BILLING_ACCOUNT_ID \
    --display-name="Stampede 25" \
    --budget-amount=25USD \
    --filter-projects=projects/PROJECT_ID

The orchestrator runs with CPU always allocated so a test finishes even
if the browser disconnects. It still scales to zero.
================================================================
EOF

command -v gcloud >/dev/null || { echo "gcloud is required" >&2; exit 1; }

gcloud config set project "$PROJECT_ID"

echo "Enabling APIs..."
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  firestore.googleapis.com \
  storage.googleapis.com \
  aiplatform.googleapis.com \
  cloudscheduler.googleapis.com \
  iam.googleapis.com

echo "Artifact Registry..."
gcloud artifacts repositories describe stampede --location="$REGION" >/dev/null 2>&1 \
  || gcloud artifacts repositories create stampede \
    --repository-format=docker \
    --location="$REGION" \
    --description="Stampede images"

REPO="${REGION}-docker.pkg.dev/${PROJECT_ID}/stampede"
PROJECT_NUMBER="$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')"

# Cloud Build pushes the images. Grant the build service account writer on the repo.
for member in \
  "serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  "serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
do
  gcloud artifacts repositories add-iam-policy-binding stampede \
    --location="$REGION" \
    --member="$member" \
    --role="roles/artifactregistry.writer" >/dev/null
done

echo "Building images with Cloud Build..."
gcloud builds submit --config cloudbuild.yaml --substitutions="_REPO=${REPO}" .

echo "Firestore (native). If this region is not a Firestore location, rerun with us-central1."
gcloud firestore databases describe --database="(default)" >/dev/null 2>&1 \
  || gcloud firestore databases create --database="(default)" --location="$REGION" --type=firestore-native

BUCKET="stampede-artifacts-${PROJECT_ID}"
echo "Bucket gs://${BUCKET} (private)..."
gcloud storage buckets describe "gs://${BUCKET}" >/dev/null 2>&1 \
  || gcloud storage buckets create "gs://${BUCKET}" --location="$REGION" --uniform-bucket-level-access

create_sa() {
  local name="$1"
  gcloud iam service-accounts describe "${name}@${PROJECT_ID}.iam.gserviceaccount.com" >/dev/null 2>&1 \
    || gcloud iam service-accounts create "$name" --display-name="$name"
}

create_sa stampede-web
create_sa stampede-orchestrator
create_sa stampede-loadgen
create_sa stampede-report
create_sa stampede-scheduler

ORCH_SA="stampede-orchestrator@${PROJECT_ID}.iam.gserviceaccount.com"
WEB_SA="stampede-web@${PROJECT_ID}.iam.gserviceaccount.com"
LOAD_SA="stampede-loadgen@${PROJECT_ID}.iam.gserviceaccount.com"
REPORT_SA="stampede-report@${PROJECT_ID}.iam.gserviceaccount.com"
SCHED_SA="stampede-scheduler@${PROJECT_ID}.iam.gserviceaccount.com"

echo "IAM..."
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${ORCH_SA}" --role="roles/datastore.user" >/dev/null
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${REPORT_SA}" --role="roles/aiplatform.user" >/dev/null
# Run jobs with container overrides. developer is broader than jobs.run; it is what
# run.jobs.runWithOverrides ships in. Scoped to this fresh project.
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${ORCH_SA}" --role="roles/run.developer" >/dev/null
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET}" \
  --member="serviceAccount:${ORCH_SA}" --role="roles/storage.objectAdmin" >/dev/null
gcloud iam service-accounts add-iam-policy-binding "$LOAD_SA" \
  --member="serviceAccount:${ORCH_SA}" --role="roles/iam.serviceAccountUser" >/dev/null

umask 077
if [[ ! -f .stampede-deploy.env ]]; then
  cat > .stampede-deploy.env <<EOF
INTERNAL_TOKEN=$(openssl rand -hex 32)
ADMIN_TOKEN=$(openssl rand -hex 32)
REPORT_TOKEN=$(openssl rand -hex 32)
EOF
fi
umask 022
# shellcheck disable=SC1091
source .stampede-deploy.env

ENV_DELIM='^@^'

echo "Deploying report..."
gcloud run deploy stampede-report \
  --image="${REPO}/report:latest" \
  --region="$REGION" \
  --service-account="$REPORT_SA" \
  --no-allow-unauthenticated \
  --min-instances=0 --max-instances=1 \
  --cpu=1 --memory=512Mi \
  --timeout=60 \
  --set-env-vars="${ENV_DELIM}VERTEX_PROJECT=${PROJECT_ID}@VERTEX_LOCATION=${REGION}@VERTEX_MODEL=gemini-2.5-flash@REPORT_TOKEN=${REPORT_TOKEN}@STAMPEDE_ENV=prod"

REPORT_URL="$(gcloud run services describe stampede-report --region="$REGION" --format='value(status.url)')"

echo "Deploying orchestrator..."
gcloud run deploy stampede-orchestrator \
  --image="${REPO}/orchestrator:latest" \
  --region="$REGION" \
  --service-account="$ORCH_SA" \
  --no-allow-unauthenticated \
  --min-instances=0 --max-instances=2 \
  --cpu=1 --memory=512Mi \
  --timeout=300 \
  --no-cpu-throttling \
  --set-env-vars="${ENV_DELIM}STAMPEDE_ENV=prod@STORE=firestore@FIRESTORE_PROJECT=${PROJECT_ID}@GCS_BUCKET=${BUCKET}@MAX_RPS=40@MAX_DURATION=180s@MAX_WORKERS=50@DOMAIN_DAILY_QUOTA=3@IP_DAILY_QUOTA=5@ADMIN_TOKEN=${ADMIN_TOKEN}@INTERNAL_TOKEN=${INTERNAL_TOKEN}@KILL_SWITCH=0@LOADGEN_MODE=cloudrun@CLOUD_RUN_PROJECT=${PROJECT_ID}@CLOUD_RUN_REGION=${REGION}@CLOUD_RUN_JOB=stampede-loadgen@REPORT_URL=${REPORT_URL}@REPORT_TOKEN=${REPORT_TOKEN}@REPORT_AUDIENCE=${REPORT_URL}@TRUST_PROXY=1"

ORCH_URL="$(gcloud run services describe stampede-orchestrator --region="$REGION" --format='value(status.url)')"

echo "Deploying loadgen job..."
gcloud run jobs deploy stampede-loadgen \
  --image="${REPO}/loadgen:latest" \
  --region="$REGION" \
  --service-account="$LOAD_SA" \
  --tasks=10 \
  --parallelism=10 \
  --max-retries=0 \
  --task-timeout=180s \
  --cpu=1 \
  --memory=512Mi \
  --set-env-vars="${ENV_DELIM}ORCHESTRATOR_URL=${ORCH_URL}@ORCHESTRATOR_AUDIENCE=${ORCH_URL}@INTERNAL_TOKEN=${INTERNAL_TOKEN}@STAMPEDE_ENV=prod"

echo "Invoker bindings..."
gcloud run services add-iam-policy-binding stampede-orchestrator \
  --region="$REGION" --member="serviceAccount:${WEB_SA}" --role="roles/run.invoker" >/dev/null
gcloud run services add-iam-policy-binding stampede-orchestrator \
  --region="$REGION" --member="serviceAccount:${LOAD_SA}" --role="roles/run.invoker" >/dev/null
gcloud run services add-iam-policy-binding stampede-report \
  --region="$REGION" --member="serviceAccount:${ORCH_SA}" --role="roles/run.invoker" >/dev/null

echo "Deploying web (public, scales to zero)..."
gcloud run deploy stampede-web \
  --image="${REPO}/web:latest" \
  --region="$REGION" \
  --service-account="$WEB_SA" \
  --allow-unauthenticated \
  --min-instances=0 --max-instances=2 \
  --cpu=1 --memory=256Mi \
  --timeout=360 \
  --set-env-vars="${ENV_DELIM}ORCHESTRATOR_URL=${ORCH_URL}@ORCHESTRATOR_AUDIENCE=${ORCH_URL}@TRUST_PROXY=1@STAMPEDE_ENV=prod"

WEB_URL="$(gcloud run services describe stampede-web --region="$REGION" --format='value(status.url)')"

if [[ "$WITH_SCHEDULER" -eq 1 ]]; then
  echo "Nightly opt-in retest (09:00 in the region)..."
  gcloud run services add-iam-policy-binding stampede-orchestrator \
    --region="$REGION" --member="serviceAccount:${SCHED_SA}" --role="roles/run.invoker" >/dev/null
  gcloud scheduler jobs describe stampede-retest --location="$REGION" >/dev/null 2>&1 \
    || gcloud scheduler jobs create http stampede-retest \
      --location="$REGION" \
      --schedule="0 9 * * *" \
      --uri="${ORCH_URL}/v1/internal/cron/retest" \
      --http-method=POST \
      --oidc-service-account-email="$SCHED_SA" \
      --oidc-token-audience="$ORCH_URL" \
      --headers="X-Stampede-Token=${INTERNAL_TOKEN}"
fi

cat <<EOF

Deployed.
  Web:          ${WEB_URL}
  Orchestrator: ${ORCH_URL}  (not public; web and the loadgen job invoke it)
  Report:       ${REPORT_URL}  (not public; orchestrator invokes it)
  Job:          stampede-loadgen (10 parallel tasks)

Tokens are in .stampede-deploy.env (mode 600). Do not commit that file.
Admin kill switch (web forwards the bearer token as X-Stampede-Admin and attaches its own Cloud Run identity):
  curl -X POST -H "Authorization: Bearer \$ADMIN_TOKEN" ${WEB_URL}/v1/admin/kill
  curl -X POST -H "Authorization: Bearer \$ADMIN_TOKEN" ${WEB_URL}/v1/admin/resume

To pause immediately without a token, redeploy the orchestrator with KILL_SWITCH=1.
EOF
