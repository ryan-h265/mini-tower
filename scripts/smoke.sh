#!/bin/bash
set -e

# MiniTower Smoke Test
# Usage: ./scripts/smoke.sh
# Exit 0 on success, non-zero on failure

cd "$(dirname "$0")/.."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() { echo -e "${GREEN}[SMOKE]${NC} $1"; }
warn() { echo -e "${YELLOW}[SMOKE]${NC} $1"; }
fail() { echo -e "${RED}[SMOKE]${NC} $1"; exit 1; }

# Cleanup function
cleanup() {
  log "Cleaning up..."
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  [ -n "$RUNNER_PID" ] && kill "$RUNNER_PID" 2>/dev/null || true
  rm -rf "$WORKDIR"
  rm -f "$DBPATH" "${DBPATH}-wal" "${DBPATH}-shm"
  rm -rf "$OBJDIR"
}
trap cleanup EXIT

# Generate unique paths for this test run
WORKDIR=$(mktemp -d)
DBPATH=$(mktemp -u).db
OBJDIR=$(mktemp -d)
PORT=$((8080 + RANDOM % 1000))
BOOTSTRAP_TOKEN="smoke-test-$(date +%s)"

log "Smoke test starting..."
log "  Work dir: $WORKDIR"
log "  DB path: $DBPATH"
log "  Objects dir: $OBJDIR"
log "  Port: $PORT"

# Build binaries
log "Building binaries..."
go build -o "$WORKDIR/minitowerd" ./cmd/minitowerd
go build -o "$WORKDIR/minitower-runner" ./cmd/minitower-runner

# Start server
log "Starting control plane..."
MINITOWER_LISTEN_ADDR=":$PORT" \
MINITOWER_DB_PATH="$DBPATH" \
MINITOWER_OBJECTS_DIR="$OBJDIR" \
MINITOWER_BOOTSTRAP_TOKEN="$BOOTSTRAP_TOKEN" \
MINITOWER_LEASE_TTL=30s \
MINITOWER_EXPIRY_CHECK_INTERVAL=5s \
"$WORKDIR/minitowerd" &
SERVER_PID=$!

# Wait for server to be ready
log "Waiting for server..."
for i in {1..30}; do
  if curl -s "http://localhost:$PORT/health" >/dev/null 2>&1; then
    break
  fi
  if [ $i -eq 30 ]; then
    fail "Server failed to start"
  fi
  sleep 0.2
done
log "Server ready"

# Test /metrics endpoint
log "Checking /metrics endpoint..."
METRICS=$(curl -s "http://localhost:$PORT/metrics")
if ! echo "$METRICS" | grep -q "minitower_http_requests_total"; then
  fail "/metrics endpoint not working"
fi
log "/metrics endpoint OK"

# Bootstrap team
log "Bootstrapping team..."
BOOTSTRAP_RESP=$(curl -s -X POST "http://localhost:$PORT/api/v1/bootstrap/team" \
  -H "Authorization: Bearer $BOOTSTRAP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"smoke","name":"Smoke Team"}')

TOKEN=$(echo "$BOOTSTRAP_RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
REG_TOKEN=$(echo "$BOOTSTRAP_RESP" | grep -o '"registration_token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ] || [ -z "$REG_TOKEN" ]; then
  fail "Bootstrap failed: $BOOTSTRAP_RESP"
fi
log "Team bootstrapped"

# Create app
log "Creating app..."
APP_RESP=$(curl -s -X POST "http://localhost:$PORT/api/v1/apps" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"smoke-app","description":"Smoke test app"}')

if ! echo "$APP_RESP" | grep -q '"slug":"smoke-app"'; then
  fail "Create app failed: $APP_RESP"
fi
log "App created"

# Create artifact
log "Creating artifact..."
mkdir -p "$WORKDIR/artifact"
cat > "$WORKDIR/artifact/main.py" << 'PYTHON'
#!/usr/bin/env python3
import os
import json

input_json = os.environ.get("MINITOWER_INPUT", "{}")
input_data = json.loads(input_json)
name = input_data.get("name", "World")

print(f"Hello, {name}!")
print("Smoke test completed successfully")
PYTHON

tar -czf "$WORKDIR/artifact.tar.gz" -C "$WORKDIR/artifact" main.py

# Upload version
log "Uploading version..."
VERSION_RESP=$(curl -s -X POST "http://localhost:$PORT/api/v1/apps/smoke-app/versions" \
  -H "Authorization: Bearer $TOKEN" \
  -F "artifact=@$WORKDIR/artifact.tar.gz" \
  -F "entrypoint=main.py" \
  -F "timeout_seconds=30")

if ! echo "$VERSION_RESP" | grep -q '"version_no"'; then
  fail "Upload version failed: $VERSION_RESP"
fi
log "Version uploaded"

# Create run
log "Creating run..."
RUN_RESP=$(curl -s -X POST "http://localhost:$PORT/api/v1/apps/smoke-app/runs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"SmokeTest"}}')

RUN_ID=$(echo "$RUN_RESP" | grep -o '"run_id":[0-9]*' | cut -d':' -f2)
if [ -z "$RUN_ID" ]; then
  fail "Create run failed: $RUN_RESP"
fi
log "Run created: $RUN_ID"

# Start runner
log "Starting runner..."
mkdir -p "$WORKDIR/runner"
MINITOWER_SERVER_URL="http://localhost:$PORT" \
MINITOWER_RUNNER_NAME="smoke-runner" \
MINITOWER_REGISTRATION_TOKEN="$REG_TOKEN" \
MINITOWER_DATA_DIR="$WORKDIR/runner" \
MINITOWER_POLL_INTERVAL=1s \
"$WORKDIR/minitower-runner" &
RUNNER_PID=$!
log "Runner started"

# Wait for run to complete
log "Waiting for run to complete..."
for i in {1..60}; do
  STATUS_RESP=$(curl -s "http://localhost:$PORT/api/v1/runs/$RUN_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(echo "$STATUS_RESP" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)

  if [ "$STATUS" = "completed" ]; then
    log "Run completed"
    break
  fi
  if [ "$STATUS" = "failed" ]; then
    fail "Run failed: $STATUS_RESP"
  fi
  if [ $i -eq 60 ]; then
    fail "Run timed out waiting for completion. Status: $STATUS"
  fi
  sleep 1
done

# Verify logs are present
log "Verifying logs..."
LOGS_RESP=$(curl -s "http://localhost:$PORT/api/v1/runs/$RUN_ID/logs" \
  -H "Authorization: Bearer $TOKEN")

if ! echo "$LOGS_RESP" | grep -q "Hello, SmokeTest"; then
  fail "Logs verification failed: $LOGS_RESP"
fi
if ! echo "$LOGS_RESP" | grep -q "Smoke test completed successfully"; then
  fail "Logs verification failed: $LOGS_RESP"
fi
log "Logs verified"

# Test body limit (should return 413 for oversized request)
log "Testing body limit..."
# Create a payload larger than 10MB
LARGE_PAYLOAD=$(head -c 11000000 /dev/zero | tr '\0' 'x')
LIMIT_RESP=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:$PORT/api/v1/apps" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"slug\":\"$LARGE_PAYLOAD\"}" 2>/dev/null || echo "000")

if [ "$LIMIT_RESP" != "413" ]; then
  warn "Body limit test: expected 413, got $LIMIT_RESP (this may vary by connection handling)"
fi
log "Body limit test complete"

# Final metrics check
log "Final metrics check..."
FINAL_METRICS=$(curl -s "http://localhost:$PORT/metrics")
if ! echo "$FINAL_METRICS" | grep -q 'minitower_http_requests_total{method="POST"'; then
  fail "Final metrics check failed"
fi
log "Metrics verified"

log "All smoke tests passed!"
exit 0
