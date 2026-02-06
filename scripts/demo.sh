#!/bin/bash
set -e

# MiniTower Demo Script
# Usage: ./scripts/demo.sh

cd "$(dirname "$0")/.."

echo "=== MiniTower Demo ==="
echo ""

# Kill any existing processes
pkill -f "bin/minitowerd" 2>/dev/null || true
pkill -f "bin/minitower-runner" 2>/dev/null || true
sleep 1

# Cleanup
rm -f minitower.db minitower.db-wal minitower.db-shm
rm -rf objects
rm -rf /tmp/minitower-demo

# Build
echo "Building..."
go build -o bin/minitowerd ./cmd/minitowerd
go build -o bin/minitower-runner ./cmd/minitower-runner
echo "Built binaries in ./bin/"
echo ""

# Start server
echo "Starting server..."
MINITOWER_BOOTSTRAP_TOKEN=secret MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret ./bin/minitowerd &
SERVER_PID=$!
sleep 2
echo "Server running (PID: $SERVER_PID)"
echo ""

# Cleanup function
cleanup() {
  echo ""
  echo "Cleaning up..."
  kill $SERVER_PID 2>/dev/null || true
  kill $RUNNER_PID 2>/dev/null || true
  rm -rf /tmp/minitower-demo
}
trap cleanup EXIT

# Bootstrap team
echo "=== 1. Bootstrap Team ==="
BOOTSTRAP_RESP=$(curl -s -X POST http://localhost:8080/api/v1/bootstrap/team \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","name":"My Team"}')
echo "$BOOTSTRAP_RESP" | jq .

TOKEN=$(echo "$BOOTSTRAP_RESP" | jq -r '.token')

echo ""
echo "Team API Token: $TOKEN"
echo ""

# Create app
echo "=== 2. Create App ==="
curl -s -X POST http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"hello-world","description":"A simple hello world app"}' | jq .
echo ""

# Create a Python script for the version
mkdir -p /tmp/minitower-demo
cat > /tmp/minitower-demo/main.py << 'PYTHON'
#!/usr/bin/env python3
import os
import json
import time

# Get input from environment
input_json = os.environ.get("MINITOWER_INPUT", "{}")
input_data = json.loads(input_json)

name = input_data.get("name", "World")
count = input_data.get("count", 10)

print(f"Starting job for {name}...")

for i in range(1, count + 1):
    print(f"Step {i}/{count}: Processing...")
    time.sleep(1)

print(f"Hello, {name}! Job completed successfully.")
PYTHON

# Package as tarball
tar -czf /tmp/minitower-demo/artifact.tar.gz -C /tmp/minitower-demo main.py

echo "=== 3. Upload Version ==="
curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/versions \
  -H "Authorization: Bearer $TOKEN" \
  -F "artifact=@/tmp/minitower-demo/artifact.tar.gz" \
  -F "entrypoint=main.py" \
  -F "timeout_seconds=60" | jq .
echo ""

echo "=== 4. Create Run ==="
RUN_RESP=$(curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"MiniTower","count":10}}')
echo "$RUN_RESP" | jq .
RUN_ID=$(echo "$RUN_RESP" | jq -r '.run_id')
echo ""

echo "=== 5. Start Runner ==="
mkdir -p /tmp/minitower-demo/runner
MINITOWER_SERVER_URL=http://localhost:8080 \
MINITOWER_RUNNER_NAME=demo-runner \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
MINITOWER_DATA_DIR=/tmp/minitower-demo/runner \
MINITOWER_POLL_INTERVAL=2s \
./bin/minitower-runner &
RUNNER_PID=$!
echo "Runner started (PID: $RUNNER_PID)"
echo ""

echo "Waiting for run to complete..."
for i in {1..30}; do
  STATUS=$(curl -s "http://localhost:8080/api/v1/runs/$RUN_ID" \
    -H "Authorization: Bearer $TOKEN" | jq -r '.status')
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    break
  fi
  sleep 1
done
echo ""

echo "=== 6. Check Run Status ==="
curl -s "http://localhost:8080/api/v1/runs/$RUN_ID" \
  -H "Authorization: Bearer $TOKEN" | jq .
echo ""

echo "=== 7. Check Run Logs ==="
curl -s "http://localhost:8080/api/v1/runs/$RUN_ID/logs" \
  -H "Authorization: Bearer $TOKEN" | jq .
echo ""

echo "=== Demo Complete ==="
echo ""
echo "The server is still running. You can now run manual commands:"
echo ""
echo "# List apps"
echo "curl -s http://localhost:8080/api/v1/apps -H 'Authorization: Bearer $TOKEN' | jq ."
echo ""
echo "# Create another run"
echo "curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/runs \\"
echo "  -H 'Authorization: Bearer $TOKEN' \\"
echo "  -H 'Content-Type: application/json' \\"
echo "  -d '{\"input\":{\"name\":\"Test\",\"count\":2}}' | jq ."
echo ""
echo "# Check run status"
echo "curl -s http://localhost:8080/api/v1/runs/2 -H 'Authorization: Bearer $TOKEN' | jq ."
echo ""
echo "Press Ctrl+C to stop..."
wait
