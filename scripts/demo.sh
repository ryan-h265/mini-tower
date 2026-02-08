#!/usr/bin/env bash
set -euo pipefail

# MiniTower local demo (non-compose).
# Usage:
#   ./scripts/demo.sh           # run demo and exit
#   ./scripts/demo.sh --hold    # keep server/runner running at the end

cd "$(dirname "$0")/.."

PORT="${MINITOWER_DEMO_PORT:-18080}"
BASE_URL="http://localhost:${PORT}"
BOOTSTRAP_TOKEN="${MINITOWER_BOOTSTRAP_TOKEN:-secret}"
RUNNER_REG_TOKEN="${MINITOWER_RUNNER_REGISTRATION_TOKEN:-runner-secret}"
HOLD=false

for arg in "$@"; do
  case "$arg" in
    --hold) HOLD=true ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "ERROR: required command not found: $1"; exit 1; }
}

need_cmd curl
need_cmd jq
need_cmd tar
need_cmd go

SERVER_PID=""
RUNNER_PID=""
DEMO_TMP_DIR="/tmp/minitower-demo"
DEMO_DB="./minitower-demo.db"
DEMO_OBJ="./objects-demo"

cleanup() {
  echo
  echo "Cleaning up..."
  if [ -n "${RUNNER_PID}" ]; then
    kill "${RUNNER_PID}" 2>/dev/null || true
  fi
  if [ -n "${SERVER_PID}" ]; then
    kill "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${DEMO_TMP_DIR}"
}
trap cleanup EXIT

echo "=== MiniTower Demo ==="
echo ""

# Cleanup local demo state only.
rm -f "${DEMO_DB}" "${DEMO_DB}-wal" "${DEMO_DB}-shm"
rm -rf "${DEMO_OBJ}"
rm -rf "${DEMO_TMP_DIR}"

# Build binaries.
echo "Building..."
GOCACHE="${GOCACHE:-/tmp/minitower-gocache}" go build -o bin/minitowerd ./cmd/minitowerd
GOCACHE="${GOCACHE:-/tmp/minitower-gocache}" go build -o bin/minitower-runner ./cmd/minitower-runner
GOCACHE="${GOCACHE:-/tmp/minitower-gocache}" go build -o bin/minitower-cli ./cmd/minitower-cli
echo "Built binaries in ./bin/"
echo ""

# Start server.
echo "Starting server on ${BASE_URL}..."
MINITOWER_LISTEN_ADDR=":${PORT}" \
MINITOWER_DB_PATH="${DEMO_DB}" \
MINITOWER_OBJECTS_DIR="${DEMO_OBJ}" \
MINITOWER_BOOTSTRAP_TOKEN="${BOOTSTRAP_TOKEN}" \
MINITOWER_RUNNER_REGISTRATION_TOKEN="${RUNNER_REG_TOKEN}" \
./bin/minitowerd &
SERVER_PID=$!

# Wait for server readiness.
for i in $(seq 1 50); do
  if curl -sf "${BASE_URL}/health" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 50 ]; then
    echo "ERROR: server failed to start"
    exit 1
  fi
  sleep 0.2
done

echo "Server running (PID: ${SERVER_PID})"
echo ""

echo "=== 1. Bootstrap Team ==="
BOOTSTRAP_HTTP=$(curl -sS -o /tmp/minitower-demo-bootstrap.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bootstrap/team" \
  -H "Authorization: Bearer ${BOOTSTRAP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","name":"My Team"}')

if [ "${BOOTSTRAP_HTTP}" != "200" ] && [ "${BOOTSTRAP_HTTP}" != "201" ]; then
  echo "ERROR: bootstrap failed (HTTP ${BOOTSTRAP_HTTP})"
  cat /tmp/minitower-demo-bootstrap.json
  exit 1
fi

cat /tmp/minitower-demo-bootstrap.json | jq .
TOKEN=$(jq -r '.token // empty' /tmp/minitower-demo-bootstrap.json)
if [ -z "${TOKEN}" ]; then
  echo "ERROR: bootstrap returned no token"
  exit 1
fi
echo ""
echo "Team API Token: ${TOKEN}"
echo ""

echo "=== 2. Prepare Demo Project ==="
PROJECT_DIR="${DEMO_TMP_DIR}/project"
mkdir -p "${PROJECT_DIR}"
cat > "${PROJECT_DIR}/main.py" << 'PYTHON'
#!/usr/bin/env python3
import os
import time

name = os.getenv("name", "World")
count = int(os.getenv("count", "10"))

print(f"Starting job for {name}...")

for i in range(1, count + 1):
    print(f"Step {i}/{count}: Processing...")
    time.sleep(1)

print(f"Hello, {name}! Job completed successfully.")
PYTHON

cat > "${PROJECT_DIR}/Towerfile" << 'TOML'
[app]
name = "hello-world"
script = "main.py"
source = ["./*.py"]

[app.timeout]
seconds = 60

[[parameters]]
name = "name"
description = "Name to greet"
type = "string"
default = "World"

[[parameters]]
name = "count"
description = "Loop count"
type = "integer"
default = 10
TOML

echo "Project created at ${PROJECT_DIR}"
echo ""

echo "=== 3. Deploy with CLI ==="
DEPLOY_OUTPUT=$(./bin/minitower-cli deploy \
  --server "${BASE_URL}" \
  --token "${TOKEN}" \
  --dir "${PROJECT_DIR}" 2>&1)

echo "${DEPLOY_OUTPUT}"
echo ""

echo "=== 4. Create Run ==="
RUN_HTTP=$(curl -sS -o /tmp/minitower-demo-run.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/apps/hello-world/runs" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"MiniTower","count":10}}')

if [ "${RUN_HTTP}" != "201" ]; then
  echo "ERROR: create run failed (HTTP ${RUN_HTTP})"
  cat /tmp/minitower-demo-run.json
  exit 1
fi

cat /tmp/minitower-demo-run.json | jq .
RUN_ID=$(jq -r '.run_id // empty' /tmp/minitower-demo-run.json)
if [ -z "${RUN_ID}" ]; then
  echo "ERROR: run response missing run_id"
  exit 1
fi
echo ""

echo "=== 5. Start Runner ==="
mkdir -p "${DEMO_TMP_DIR}/runner"
MINITOWER_SERVER_URL="${BASE_URL}" \
MINITOWER_RUNNER_NAME="demo-runner" \
MINITOWER_RUNNER_REGISTRATION_TOKEN="${RUNNER_REG_TOKEN}" \
MINITOWER_DATA_DIR="${DEMO_TMP_DIR}/runner" \
MINITOWER_POLL_INTERVAL=2s \
./bin/minitower-runner &
RUNNER_PID=$!
echo "Runner started (PID: ${RUNNER_PID})"
echo ""

echo "Waiting for run to complete..."
for i in $(seq 1 90); do
  STATUS=$(curl -sS "${BASE_URL}/api/v1/runs/${RUN_ID}" \
    -H "Authorization: Bearer ${TOKEN}" | jq -r '.status')
  if [ "${STATUS}" = "completed" ] || [ "${STATUS}" = "failed" ] || [ "${STATUS}" = "cancelled" ] || [ "${STATUS}" = "dead" ]; then
    break
  fi
  sleep 1
done

echo ""
echo "=== 6. Check Run Status ==="
curl -sS "${BASE_URL}/api/v1/runs/${RUN_ID}" \
  -H "Authorization: Bearer ${TOKEN}" | jq .

echo ""
echo "=== 7. Check Run Logs ==="
curl -sS "${BASE_URL}/api/v1/runs/${RUN_ID}/logs" \
  -H "Authorization: Bearer ${TOKEN}" | jq .

FINAL_STATUS=$(curl -sS "${BASE_URL}/api/v1/runs/${RUN_ID}" \
  -H "Authorization: Bearer ${TOKEN}" | jq -r '.status')
if [ "${FINAL_STATUS}" != "completed" ]; then
  echo "ERROR: expected completed status, got ${FINAL_STATUS}"
  exit 1
fi

echo ""
echo "=== Demo Complete ==="
echo "UI: ${BASE_URL}"
echo ""

if [ "${HOLD}" = true ]; then
  echo "Hold mode enabled; services remain running. Press Ctrl+C to stop."
  wait
fi
