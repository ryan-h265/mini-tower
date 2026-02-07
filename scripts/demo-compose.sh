#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MINITOWER_URL:-http://localhost:8080}"
BOOTSTRAP_TOKEN="${MINITOWER_BOOTSTRAP_TOKEN:-dev}"
TEAM_SLUG="${MINITOWER_DEMO_TEAM_SLUG:-demo}"
TEAM_NAME="${MINITOWER_DEMO_TEAM_NAME:-Demo Team}"
TEAM_PASSWORD="${MINITOWER_DEMO_TEAM_PASSWORD:-demo}"
BATCH_SIZE="${BATCH_SIZE:-5}"
LOOP=false

for arg in "$@"; do
  case "$arg" in
    --loop) LOOP=true ;;
  esac
done

# --- Helpers ---

api() {
  local method="$1" path="$2" token="$3"
  shift 3
  curl -sS -X "$method" "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    "$@"
}

extract_token() {
  grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4 || true
}

extract_code() {
  grep -o '"code":"[^"]*"' | head -1 | cut -d'"' -f4 || true
}

wait_healthy() {
  echo "Waiting for minitowerd to be healthy..."
  for i in $(seq 1 60); do
    if curl -sf "${BASE_URL}/health" > /dev/null 2>&1; then
      echo "minitowerd is healthy."
      return 0
    fi
    sleep 1
  done
  echo "ERROR: minitowerd did not become healthy after 60s"
  exit 1
}

# --- Setup ---

wait_healthy

# Obtain a team token:
# 1) bootstrap requested team slug (idempotent for the same slug)
# 2) if bootstrap is denied, login with provided credentials
echo "Bootstrapping team slug '${TEAM_SLUG}'..."
TEAM_RESP=$(api POST /api/v1/bootstrap/team "$BOOTSTRAP_TOKEN" \
  -d "{\"slug\":\"${TEAM_SLUG}\",\"name\":\"${TEAM_NAME}\",\"password\":\"${TEAM_PASSWORD}\"}" 2>&1) || true
TEAM_TOKEN=$(echo "$TEAM_RESP" | extract_token)
TEAM_ERR_CODE=$(echo "$TEAM_RESP" | extract_code)

if [ -z "$TEAM_TOKEN" ]; then
  echo "Bootstrap returned no token (code='${TEAM_ERR_CODE:-unknown}'). Logging in..."
  LOGIN_RESP=$(curl -sS -X POST "${BASE_URL}/api/v1/teams/login" \
    -H "Content-Type: application/json" \
    -d "{\"slug\":\"${TEAM_SLUG}\",\"password\":\"${TEAM_PASSWORD}\"}")
  TEAM_TOKEN=$(echo "$LOGIN_RESP" | extract_token)
  LOGIN_ERR_CODE=$(echo "$LOGIN_RESP" | extract_code)

  if [ -z "$TEAM_TOKEN" ]; then
    echo "ERROR: Could not bootstrap or login."
    echo "Bootstrap response: ${TEAM_RESP}"
    echo "Login response: ${LOGIN_RESP}"
    echo "Tip: ensure MINITOWER_DEMO_TEAM_SLUG / MINITOWER_DEMO_TEAM_PASSWORD match the existing team, or reset state with: docker compose down -v"
    exit 1
  fi
  echo "Logged in successfully for team '${TEAM_SLUG}'."
fi
echo "Team slug: ${TEAM_SLUG}"
echo "Team token: ${TEAM_TOKEN:0:12}..."

# Create app (ignore "already exists" errors)
echo "Creating app..."
api POST /api/v1/apps "$TEAM_TOKEN" \
  -d '{"slug":"demo-app","description":"Demo workload"}' > /dev/null 2>&1 || true

# Upload a new version every time (versions are append-only)
echo "Uploading version..."
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

cat > "${TMPDIR}/main.py" << 'PYTHON'
import os, time, random
duration = random.uniform(2, 5)
name = os.getenv("name", "World")
print(f"Hello, {name}! Sleeping {duration:.1f}s...")
time.sleep(duration)
print("Done.")
PYTHON

tar -czf "${TMPDIR}/artifact.tar.gz" -C "$TMPDIR" main.py

curl -sS -X POST "${BASE_URL}/api/v1/apps/demo-app/versions" \
  -H "Authorization: Bearer ${TEAM_TOKEN}" \
  -F artifact=@"${TMPDIR}/artifact.tar.gz" \
  -F entrypoint=main.py \
  -F timeout_seconds=60 > /dev/null

echo "Version uploaded."

# --- Submit runs ---

submit_batch() {
  local count="$1"
  echo "Submitting ${count} runs..."
  for i in $(seq 1 "$count"); do
    api POST /api/v1/apps/demo-app/runs "$TEAM_TOKEN" \
      -d "{\"input\":{\"name\":\"run-${RANDOM}\"}}" > /dev/null
  done
  echo "${count} runs submitted."
}

poll_runs() {
  echo "Polling for completion..."
  sleep 1
  while true; do
    RESP=$(api GET /api/v1/apps/demo-app/runs "$TEAM_TOKEN")
    QUEUED=$({ echo "$RESP" | grep -o '"status":"queued"' | wc -l; } || true)
    RUNNING=$({ echo "$RESP" | grep -o '"status":"running"' | wc -l; } || true)
    COMPLETED=$({ echo "$RESP" | grep -o '"status":"completed"' | wc -l; } || true)
    FAILED=$({ echo "$RESP" | grep -o '"status":"failed"' | wc -l; } || true)
    TOTAL=$((QUEUED + RUNNING + COMPLETED + FAILED))
    echo "  queued=$QUEUED running=$RUNNING completed=$COMPLETED failed=$FAILED total=$TOTAL"
    if [ "$TOTAL" -gt 0 ] && [ "$QUEUED" -eq 0 ] && [ "$RUNNING" -eq 0 ]; then
      break
    fi
    sleep 2
  done
  echo "All runs finished."
}

submit_batch "$BATCH_SIZE"

if [ "$LOOP" = false ]; then
  poll_runs
else
  echo "Loop mode: submitting runs continuously (Ctrl-C to stop)..."
  while true; do
    api POST /api/v1/apps/demo-app/runs "$TEAM_TOKEN" \
      -d "{\"input\":{\"name\":\"loop-${RANDOM}\"}}" > /dev/null
    echo "  Submitted run ($(date +%H:%M:%S))"
    sleep 3
  done
fi

echo ""
echo "=== Demo URLs ==="
echo "  UI:         http://localhost:5173"
echo "  Grafana:    http://localhost:3000"
echo "  Prometheus: http://localhost:9090"
echo "  MiniTower:  http://localhost:8080"
