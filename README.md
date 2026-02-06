# MiniTower

**A minimal, correctness-first job orchestration system.**

MiniTower is a lightweight orchestration platform for deploying and running Python workloads with explicit state management. It prioritizes correctness over features: no double execution, deterministic retries, and race-safe state transitions throughout.

## Features

- **Apps & Versions** — Deploy immutable versioned artifacts with optional JSON Schema input validation
- **Distributed Runners** — Self-hosted workers with lease-based execution and automatic retry on failure
- **Cancellation** — Graceful cancellation with SIGTERM/SIGKILL and deterministic state convergence
- **Observability** — Prometheus metrics, structured JSON logs, and per-run log streaming
- **SQLite Storage** — Single-file database with WAL mode for concurrent reads

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│   Control Plane │◄───────►│     Runner      │
│   (minitowerd)  │  HTTP   │ (minitower-     │
│                 │         │     runner)     │
│  ┌───────────┐  │         │                 │
│  │  SQLite   │  │         │  ┌───────────┐  │
│  │    +      │  │         │  │  Python   │  │
│  │  Objects  │  │         │  │  Workload │  │
│  └───────────┘  │         │  └───────────┘  │
└─────────────────┘         └─────────────────┘
```

The control plane manages state and stores artifacts. Runners poll for work, execute Python scripts in isolated virtual environments, and report results. All state transitions use optimistic locking to prevent races.

## Requirements

- Go 1.24+
- Python 3 with `venv` module
- `tar` command available on PATH

## Quickstart

**1. Start the control plane:**
```bash
export MINITOWER_BOOTSTRAP_TOKEN=dev
export MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret
go run ./cmd/minitowerd
```

**2. Bootstrap a team:**
```bash
curl -sS -X POST http://localhost:8080/api/v1/bootstrap/team \
  -H "Authorization: Bearer dev" \
  -H "Content-Type: application/json" \
  -d '{"slug":"acme","name":"Acme Corp"}'
```

Save the `token` from the response.

**3. Create an app:**
```bash
curl -sS -X POST http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TEAM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"hello","description":"Hello world app"}'
```

**4. Package and upload a version:**
```bash
# Create artifact
mkdir -p artifact && cat > artifact/main.py << 'PY'
import os, json
input_data = json.loads(os.environ.get("MINITOWER_INPUT", "{}"))
print(f"Hello, {input_data.get('name', 'World')}!")
PY
tar -czf hello.tar.gz -C artifact .

# Upload
curl -sS -X POST http://localhost:8080/api/v1/apps/hello/versions \
  -H "Authorization: Bearer $TEAM_TOKEN" \
  -F artifact=@hello.tar.gz \
  -F entrypoint=main.py \
  -F timeout_seconds=60
```

**5. Trigger a run:**
```bash
curl -sS -X POST http://localhost:8080/api/v1/apps/hello/runs \
  -H "Authorization: Bearer $TEAM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"MiniTower"}}'
```

**6. Start a runner:**
```bash
MINITOWER_SERVER_URL=http://localhost:8080 \
MINITOWER_RUNNER_NAME=runner-1 \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
go run ./cmd/minitower-runner
```

**7. Check run status and logs:**
```bash
curl -sS http://localhost:8080/api/v1/runs/1 -H "Authorization: Bearer $TEAM_TOKEN"
curl -sS http://localhost:8080/api/v1/runs/1/logs -H "Authorization: Bearer $TEAM_TOKEN"
```

## Configuration

### Control Plane (`minitowerd`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `MINITOWER_DB_PATH` | `./minitower.db` | SQLite database path |
| `MINITOWER_OBJECTS_DIR` | `./objects` | Artifact storage directory |
| `MINITOWER_BOOTSTRAP_TOKEN` | `dev` | Token for team bootstrap |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | — | Token for runner registration (required) |
| `MINITOWER_LEASE_TTL` | `60s` | Runner lease duration |
| `MINITOWER_EXPIRY_CHECK_INTERVAL` | `10s` | Lease expiry check interval |
| `MINITOWER_MAX_REQUEST_BODY_SIZE` | `10485760` | Max request body (10MB) |
| `MINITOWER_MAX_ARTIFACT_SIZE` | `104857600` | Max artifact upload (100MB) |

### Runner (`minitower-runner`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_SERVER_URL` | — | Control plane URL (required) |
| `MINITOWER_RUNNER_NAME` | — | Unique runner name (required) |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | — | Platform token for registration |
| `MINITOWER_RUNNER_ENVIRONMENT` | `default` | Environment label for matching runs |
| `MINITOWER_PYTHON_BIN` | `python3` | Python interpreter path |
| `MINITOWER_POLL_INTERVAL` | `3s` | Work poll interval |
| `MINITOWER_KILL_GRACE_PERIOD` | `10s` | SIGTERM to SIGKILL grace |
| `MINITOWER_DATA_DIR` | `~/.minitower` | Runner data directory |

## API Endpoints

### Health & Metrics
- `GET /health` — Liveness check
- `GET /ready` — Readiness check (includes DB ping)
- `GET /metrics` — Prometheus metrics

### Team Management
- `POST /api/v1/bootstrap/team` — Create team (bootstrap token)
- `POST /api/v1/tokens` — Create additional API tokens

### Apps & Versions
- `POST /api/v1/apps` — Create app
- `GET /api/v1/apps` — List apps
- `GET /api/v1/apps/{app}` — Get app details
- `POST /api/v1/apps/{app}/versions` — Upload version (multipart)
- `GET /api/v1/apps/{app}/versions` — List versions

### Runs
- `POST /api/v1/apps/{app}/runs` — Trigger run
- `GET /api/v1/apps/{app}/runs` — List runs
- `GET /api/v1/runs/{run}` — Get run status
- `POST /api/v1/runs/{run}/cancel` — Cancel run
- `GET /api/v1/runs/{run}/logs` — Get run logs

### Runner Protocol
- `POST /api/v1/runners/register` — Register runner (registration token)
- `POST /api/v1/runs/lease` — Lease next queued run
- `POST /api/v1/runs/{run}/start` — Acknowledge lease, transition to running
- `POST /api/v1/runs/{run}/heartbeat` — Extend lease, check for cancellation
- `POST /api/v1/runs/{run}/logs` — Submit log batch (runner token + lease token)
- `POST /api/v1/runs/{run}/result` — Submit terminal result
- `GET /api/v1/runs/{run}/artifact` — Download version artifact

## Monitoring & Metrics

MiniTower exposes Prometheus metrics at `GET /metrics`. Metrics are divided into HTTP-level and domain-level categories.

### HTTP Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_http_requests_total` | method, path, status | Total HTTP requests |
| `minitower_http_request_duration_seconds` | method, path | Request latency histogram |
| `minitower_http_request_size_bytes` | method, path | Request body size histogram |
| `minitower_http_response_size_bytes` | method, path | Response body size histogram |

### Domain Metrics — Counters

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_runs_created_total` | team, app | Runs created |
| `minitower_runs_completed_total` | team, app, status | Runs reaching a terminal state |
| `minitower_runs_retried_total` | team, app | Runs re-queued by the reaper |
| `minitower_runs_leased_total` | environment | Runs leased by runners |
| `minitower_runners_registered_total` | environment | Runners registered |

### Domain Metrics — Histograms

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_run_queue_wait_seconds` | team, app | Time spent queued (started_at - queued_at) |
| `minitower_run_execution_seconds` | team, app, status | Execution duration (finished_at - started_at) |
| `minitower_run_total_seconds` | team, app, status | Total duration (finished_at - queued_at) |

### Domain Metrics — Gauges

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_runs_pending` | team, app, environment | Current queued run count |
| `minitower_runners_online` | environment | Current online runner count |

### Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: minitower
    static_configs:
      - targets: ['localhost:8080']
```

### Example PromQL Queries

```promql
# Run creation rate (per second, 5m window)
rate(minitower_runs_created_total[5m])

# Failure rate by app
rate(minitower_runs_completed_total{status="failed"}[5m])

# Queue depth
minitower_runs_pending

# p99 execution time
histogram_quantile(0.99, rate(minitower_run_execution_seconds_bucket[5m]))
```

### Quick Check

```bash
curl -s localhost:8080/metrics | grep minitower_runs
```

## How It Works

**Artifacts**: A version artifact is a `.tar.gz` containing your Python code. If `requirements.txt` is present, dependencies are installed into an isolated virtual environment before execution. Artifacts are SHA-256 verified on download.

**Execution**: Runners create a fresh workspace and `venv` for each run. The entrypoint script receives input via `MINITOWER_INPUT` environment variable as JSON.

**Leasing**: Runners acquire exclusive leases on runs. If a runner fails to heartbeat before lease expiry, the run is automatically retried (up to `max_retries`) or marked dead.

**Cancellation**: Cancel requests set `cancel_requested=true`. Runners receive this flag on heartbeat, send SIGTERM, wait for grace period, then SIGKILL if needed.

## Testing

```bash
# Unit tests
go test ./...

# Race detector
go test -race ./...

# Integration tests (requires Python)
go test -tags=integration ./cmd/minitower-runner

# End-to-end smoke test
./scripts/smoke.sh
```

## License

MIT
