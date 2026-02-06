#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MINITOWER_URL:-http://localhost:8080}"
BOOTSTRAP_TOKEN="${MINITOWER_BOOTSTRAP_TOKEN:-dev}"
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

# Obtain a team token: try bootstrap first, fall back to login.
echo "Bootstrapping team..."
TEAM_RESP=$(api POST /api/v1/bootstrap/team "$BOOTSTRAP_TOKEN" \
  -d '{"slug":"demo","name":"Demo Team","password":"demo"}' 2>&1) || true
TEAM_TOKEN=$(echo "$TEAM_RESP" | extract_token)

if [ -z "$TEAM_TOKEN" ]; then
  echo "Bootstrap returned no token (team may already exist). Logging in..."
  LOGIN_RESP=$(curl -sS -X POST "${BASE_URL}/api/v1/teams/login" \
    -H "Content-Type: application/json" \
    -d '{"slug":"demo","password":"demo"}')
  TEAM_TOKEN=$(echo "$LOGIN_RESP" | extract_token)

  if [ -z "$TEAM_TOKEN" ]; then
    echo "ERROR: Could not bootstrap or login."
    echo "Response: $LOGIN_RESP"
    exit 1
  fi
  echo "Logged in successfully."
fi
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
import os, json, time, random
input_data = json.loads(os.environ.get("MINITOWER_INPUT", "{}"))
duration = random.uniform(2, 5)
name = input_data.get("name", "World")
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
echo "  Grafana:    http://localhost:3000"
echo "  Prometheus: http://localhost:9090"
echo "  MiniTower:  http://localhost:8080"
