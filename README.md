# MiniTower

Correctness-first orchestration MVP focused on deploy -> run -> observe with explicit, race-safe state transitions.

Status: Phase 5 in PLAN.md is implemented (observability and hardening). MVP complete.

**Requirements**
- Go 1.24+
- Python 3 with `venv` (runner execution)
- `tar` available on PATH
- SQLite driver `modernc.org/sqlite` (pure Go, no external runtime)

**Quickstart (Dev)**
1. Start the control plane:
   ```bash
   export MINITOWER_BOOTSTRAP_TOKEN=dev
   go run ./cmd/minitowerd
   ```
2. Bootstrap a team and get tokens:
   ```bash
   curl -sS -X POST http://localhost:8080/api/v1/bootstrap/team \
     -H "Authorization: Bearer $MINITOWER_BOOTSTRAP_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"slug":"acme","name":"Acme"}'
   ```
3. Create an app:
   ```bash
   export TEAM_TOKEN=<token from bootstrap>
   curl -sS -X POST http://localhost:8080/api/v1/apps \
     -H "Authorization: Bearer $TEAM_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"slug":"hello","description":"demo app"}'
   ```
4. Create a version artifact:
   ```bash
   mkdir -p ./artifact
   cat > ./artifact/main.py <<'PY'
   import os
   print("hello from minitower")
   print("input:", os.environ.get("MINITOWER_INPUT"))
   PY
   tar -czf ./hello.tar.gz -C ./artifact .
   ```
5. Upload a version artifact:
   ```bash
   curl -sS -X POST http://localhost:8080/api/v1/apps/hello/versions \
     -H "Authorization: Bearer $TEAM_TOKEN" \
     -F artifact=@./your_artifact.tar.gz \
     -F entrypoint=main.py \
     -F timeout_seconds=60 \
     -F params_schema_json='{"type":"object","properties":{"name":{"type":"string"}}}'
   ```
6. Enqueue a run:
   ```bash
   curl -sS -X POST http://localhost:8080/api/v1/apps/hello/runs \
     -H "Authorization: Bearer $TEAM_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"input":{"name":"world"}}'
   ```
7. Start a runner:
   ```bash
   export MINITOWER_SERVER_URL=http://localhost:8080
   export MINITOWER_RUNNER_NAME=runner-1
   export MINITOWER_REGISTRATION_TOKEN=<registration_token from bootstrap>
   go run ./cmd/minitower-runner
   ```

**Config**
Control plane:
- `MINITOWER_LISTEN_ADDR` (default `:8080`)
- `MINITOWER_DB_PATH` (default `./minitower.db`)
- `MINITOWER_OBJECTS_DIR` (default `./objects`)
- `MINITOWER_BOOTSTRAP_TOKEN` (required outside dev)
- `MINITOWER_LEASE_TTL` (default `60s`)
- `MINITOWER_EXPIRY_CHECK_INTERVAL` (default `10s`)
- `MINITOWER_MAX_REQUEST_BODY_SIZE` (default `10485760` / 10MB)
- `MINITOWER_MAX_ARTIFACT_SIZE` (default `104857600` / 100MB)

Runner:
- `MINITOWER_SERVER_URL` (required)
- `MINITOWER_RUNNER_NAME` (required)
- `MINITOWER_REGISTRATION_TOKEN` (required for first register)
- `MINITOWER_RUNNER_TOKEN` (optional after first register)
- `MINITOWER_PYTHON_BIN` (default `python3`)
- `MINITOWER_POLL_INTERVAL` (default `3s`)
- `MINITOWER_KILL_GRACE_PERIOD` (default `10s`)
- `MINITOWER_DATA_DIR` (default `~/.minitower`)

**Notes**
- An artifact is a `.tar.gz` package stored by the control plane and fetched by runners per run. It must include the `entrypoint` file path you pass on version creation. If `requirements.txt` is present, the runner installs it into a per-run `venv` before executing the entrypoint. The runner verifies the artifact SHA-256 before unpacking.
- Migrations are embedded from `internal/migrations` and applied on boot.
- Local object store writes to `MINITOWER_OBJECTS_DIR`.
- Run input is validated against `params_schema_json` when provided.
- Cancellation is idempotent via `POST /api/v1/runs/{run}/cancel`.
- Prometheus metrics available at `/metrics` (no auth required).

**Tests**
- Unit/handler/store tests:
  ```bash
  go test ./...
  ```
- Runner integration tests (requires Python + `tar`):
  ```bash
  go test -tags=integration ./cmd/minitower-runner -run TestRunner
  ```
- Smoke test (end-to-end):
  ```bash
  ./scripts/smoke.sh
  ```
