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

extract_run_id() {
  grep -o '"run_id":[0-9]*' | head -1 | cut -d':' -f2 || true
}

extract_status() {
  grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || true
}

wait_healthy() {
  echo "Waiting for minitowerd to be healthy..."
  for i in $(seq 1 60); do
    if curl -sf "${BASE_URL}/health" >/dev/null 2>&1; then
      echo "minitowerd is healthy."
      return 0
    fi
    sleep 1
  done
  echo "ERROR: minitowerd did not become healthy after 60s"
  exit 1
}

wait_healthy

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

  if [ -z "$TEAM_TOKEN" ]; then
    echo "ERROR: Could not bootstrap or login."
    echo "Bootstrap response: ${TEAM_RESP}"
    echo "Login response: ${LOGIN_RESP}"
    echo "Tip: ensure MINITOWER_DEMO_TEAM_SLUG / MINITOWER_DEMO_TEAM_PASSWORD match existing team, or reset state with: docker compose down -v"
    exit 1
  fi
  echo "Logged in successfully for team '${TEAM_SLUG}'."
fi

echo "Team slug: ${TEAM_SLUG}"
echo "Team token: ${TEAM_TOKEN:0:12}..."

echo "Creating app..."
api POST /api/v1/apps "$TEAM_TOKEN" \
  -d '{"slug":"demo-app","description":"Demo workload"}' > /dev/null 2>&1 || true

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

cat > "${TMPDIR}/Towerfile" << 'TOML'
[app]
name = "demo-app"
script = "main.py"
source = ["./*.py"]

[app.timeout]
seconds = 60

[[parameters]]
name = "name"
description = "Name"
type = "string"
default = "World"
TOML

tar -czf "${TMPDIR}/artifact.tar.gz" -C "$TMPDIR" main.py Towerfile

VERSION_HTTP=$(curl -sS -o "${TMPDIR}/version.json" -w "%{http_code}" -X POST "${BASE_URL}/api/v1/apps/demo-app/versions" \
  -H "Authorization: Bearer ${TEAM_TOKEN}" \
  -F artifact=@"${TMPDIR}/artifact.tar.gz")

if [ "$VERSION_HTTP" != "201" ]; then
  echo "ERROR: version upload failed (HTTP ${VERSION_HTTP})"
  cat "${TMPDIR}/version.json"
  exit 1
fi

echo "Version uploaded."

RUN_IDS=()

submit_batch() {
  local count="$1"
  echo "Submitting ${count} runs..."
  for _ in $(seq 1 "$count"); do
    RESP=$(api POST /api/v1/apps/demo-app/runs "$TEAM_TOKEN" \
      -d "{\"input\":{\"name\":\"run-${RANDOM}\"}}")
    RUN_ID=$(echo "$RESP" | extract_run_id)
    if [ -z "$RUN_ID" ]; then
      echo "ERROR: failed to submit run"
      echo "$RESP"
      exit 1
    fi
    RUN_IDS+=("$RUN_ID")
  done
  echo "${count} runs submitted."
}

poll_runs() {
  echo "Polling for completion..."
  while true; do
    local queued=0
    local running=0
    local completed=0
    local failed=0
    local other=0

    for run_id in "${RUN_IDS[@]}"; do
      RESP=$(api GET "/api/v1/runs/${run_id}" "$TEAM_TOKEN")
      STATUS=$(echo "$RESP" | extract_status)
      case "$STATUS" in
        queued) queued=$((queued + 1)) ;;
        leased|running|cancelling) running=$((running + 1)) ;;
        completed) completed=$((completed + 1)) ;;
        failed|cancelled|dead) failed=$((failed + 1)) ;;
        *) other=$((other + 1)) ;;
      esac
    done

    echo "  queued=$queued running=$running completed=$completed failed=$failed other=$other total=${#RUN_IDS[@]}"

    if [ "$queued" -eq 0 ] && [ "$running" -eq 0 ] && [ "$other" -eq 0 ]; then
      if [ "$failed" -gt 0 ]; then
        echo "ERROR: one or more runs did not complete successfully"
        exit 1
      fi
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
    RESP=$(api POST /api/v1/apps/demo-app/runs "$TEAM_TOKEN" \
      -d "{\"input\":{\"name\":\"loop-${RANDOM}\"}}")
    RUN_ID=$(echo "$RESP" | extract_run_id)
    if [ -z "$RUN_ID" ]; then
      echo "ERROR: failed to submit loop run"
      echo "$RESP"
      exit 1
    fi
    echo "  Submitted run ${RUN_ID} ($(date +%H:%M:%S))"
    sleep 3
  done
fi

echo ""
echo "=== Demo URLs ==="
echo "  UI:         http://localhost:5173"
echo "  Grafana:    http://localhost:3000"
echo "  Prometheus: http://localhost:9090"
echo "  MiniTower:  http://localhost:8080"
