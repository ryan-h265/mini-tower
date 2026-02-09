# MiniTower

**A minimal, correctness-first job orchestration system.**

MiniTower is a lightweight orchestration platform for deploying and running Python workloads with explicit state management. It prioritizes correctness over features: no double execution, deterministic retries, and race-safe state transitions throughout.

## Features

- **Towerfile** — Declarative TOML config per project defines entrypoint, source globs, timeout, and parameters
- **CLI Deploy** — `minitower-cli deploy` reads the Towerfile, packages artifacts, and uploads versions in one command
- **Apps & Versions** — Deploy immutable versioned artifacts with optional JSON Schema input validation
- **Distributed Runners** — Self-hosted workers with lease-based execution and automatic retry on failure
- **Multi-Runtime** — Python (`.py` with venv) and shell (`.sh`) entrypoints out of the box
- **Cancellation** — Graceful cancellation with SIGTERM/SIGKILL and deterministic state convergence
- **Observability** — Prometheus metrics, structured JSON logs, and per-run log streaming
- **Frontend UI** — Vue 3 SPA with app management, version upload, run creation, and live log streaming
- **SQLite Storage** — Single-file database with WAL mode for concurrent reads

## Architecture

```
┌─────────────────┐
│  minitower-cli  │──── deploy ────┐
│   (Towerfile)   │                │
└─────────────────┘                ▼
┌─────────────────┐        ┌─────────────────┐         ┌─────────────────┐
│    Vue SPA      │──/api─►│   Control Plane │◄───────►│     Runner      │
│   (Frontend)    │        │   (minitowerd)  │  HTTP   │ (minitower-     │
└─────────────────┘        │                 │         │     runner)     │
                           │  ┌───────────┐  │         │                 │
                           │  │  SQLite   │  │         │  ┌───────────┐  │
                           │  │    +      │  │         │  │ .py / .sh │  │
                           │  │  Objects  │  │         │  │  Workload │  │
                           │  └───────────┘  │         │  └───────────┘  │
                           └─────────────────┘         └─────────────────┘
```

The control plane manages state and stores artifacts. A `Towerfile` (TOML) in each project declares the entrypoint, source globs, timeout, and parameters. The CLI packages and uploads artifacts; the server extracts metadata from the embedded Towerfile. Runners poll for work, execute scripts in isolated environments (Python venv for `.py`, `/bin/sh` for `.sh`), and report results. A Vue 3 SPA provides a browser UI for managing apps, uploading versions, triggering runs, and viewing logs. All state transitions use optimistic locking to prevent races.

## Requirements

- Go 1.24+
- Python 3 with `venv` module
- `tar` command available on PATH

## Quickstart

**1. Start the control plane:**
```bash
export MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret
go run ./cmd/minitowerd
```

**2. Sign up a team:**
```bash
curl -sS -X POST http://localhost:8080/api/v1/teams/signup \
  -H "Content-Type: application/json" \
  -d '{"slug":"acme","name":"Acme Corp","password":"secret"}'
```

Save the `token` from the response.

Optional operator bootstrap mode:
```bash
export MINITOWER_BOOTSTRAP_TOKEN=dev
export MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret
# restart minitowerd after setting the bootstrap token
go run ./cmd/minitowerd

curl -sS -X POST http://localhost:8080/api/v1/bootstrap/team \
  -H "Authorization: Bearer dev" \
  -H "Content-Type: application/json" \
  -d '{"slug":"acme","name":"Acme Corp","password":"secret"}'
```
Re-running bootstrap for the same slug is idempotent and can reset that team's password in local/dev flows.

**3. Create a project with a Towerfile:**
```bash
mkdir -p myapp && cat > myapp/main.py << 'PY'
import os
print(f"Hello, {os.getenv('name', 'World')}!")
PY

cat > myapp/Towerfile << 'TOML'
[app]
name = "hello"
script = "main.py"
source = ["./*.py"]

[app.timeout]
seconds = 60

[[parameters]]
name = "name"
description = "Name to greet"
type = "string"
default = "World"
TOML
```

**4. Deploy with the CLI:**
```bash
go run ./cmd/minitower-cli deploy \
  --server http://localhost:8080 \
  --token "$TEAM_TOKEN" \
  --dir myapp
```

The CLI reads the Towerfile, packages matching source files into a tar.gz, auto-creates the app if needed, and uploads the version.

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

**8. Run the frontend control plane UI (optional):**
```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173. In development, Vite proxies `/api` to `http://localhost:8080`.

## Configuration

### Control Plane (`minitowerd`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `MINITOWER_DB_PATH` | `./minitower.db` | SQLite database path |
| `MINITOWER_OBJECTS_DIR` | `./objects` | Artifact storage directory |
| `MINITOWER_PUBLIC_SIGNUP_ENABLED` | `true` | Enable unauthenticated team signup (`POST /api/v1/teams/signup`) |
| `MINITOWER_BOOTSTRAP_TOKEN` | — | Optional operator token for bootstrap/recovery (`POST /api/v1/bootstrap/team`) |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | — | Token for runner registration (required) |
| `MINITOWER_CORS_ORIGINS` | — | Comma-separated CORS allowlist (for separate frontend origin deployments) |
| `MINITOWER_LEASE_TTL` | `60s` | Runner lease duration |
| `MINITOWER_EXPIRY_CHECK_INTERVAL` | `10s` | Lease expiry check interval |
| `MINITOWER_RUNNER_PRUNE_AFTER` | `24h` | Delete offline runners older than this cutoff when they have no run-attempt history (set `0` to disable) |
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

### Frontend (`frontend/`)

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_BASE_URL` | empty | Absolute API URL for separately deployed frontend (for example `https://api.example.com`) |
| `VITE_DEV_PROXY_TARGET` | `http://localhost:8080` | Dev-only Vite proxy target for `/api` |

### Frontend Deploy Flow

```bash
# Build static frontend assets
npm --prefix frontend run build
```

- Deploy the generated assets from `frontend/dist` to your static host (Nginx, CDN, etc.).
- Set `VITE_API_BASE_URL` at build time to your control plane origin.
- Configure `MINITOWER_CORS_ORIGINS` on `minitowerd` to allow the frontend origin.

## Migration Notes

- Migration `internal/migrations/0004_towerfile.up.sql` adds `towerfile_toml` and `import_paths_json` columns to `app_versions`. Existing versions retain `NULL` for these columns and continue to work. New versions require a Towerfile in the uploaded artifact.
- Migration `internal/migrations/0003_token_role.up.sql` adds `team_tokens.role` with default `admin` and a DB-level role check.
- Existing tokens in older environments are auto-populated as `admin` when migration runs.
- Start `minitowerd` once after upgrading to apply pending migrations before running the frontend.

## API Endpoints

See [docs/api-endpoints.md](docs/api-endpoints.md) for the full endpoint catalog.

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

**Towerfile**: Each project contains a `Towerfile` (TOML) that declares the app name, entrypoint script, source globs, timeout, and input parameters. The CLI uses it to package artifacts; the server extracts and stores it on upload.

**Artifacts**: A version artifact is a `.tar.gz` containing your code and a `Towerfile` at its root. If `requirements.txt` is present, dependencies are installed into an isolated virtual environment before execution. Artifacts are SHA-256 verified on download.

**Execution**: Runners create a fresh workspace for each run. Python (`.py`) entrypoints run in an isolated venv; shell (`.sh`) entrypoints run under `/bin/sh`. Each run input key is exposed to the entrypoint as an environment variable.

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

```bash
# Frontend tests
npm --prefix frontend run test -- --run

# Frontend typecheck + production build
npm --prefix frontend run build
```

## Docker Compose Demo

Run the full stack (control plane, runners, frontend UI, Prometheus, Grafana):

```bash
docker compose up -d --build
./scripts/demo-compose.sh --loop
```

- **Grafana**: http://localhost:3000 (anonymous access, pre-built dashboard)
- **Prometheus**: http://localhost:9090
- **MiniTower API**: http://localhost:8080
- **MiniTower UI**: http://localhost:5173

The `frontend` Compose service is a dedicated interaction container for the control plane UI. It runs the Vue dev server and proxies `/api` traffic to `minitowerd` over the Compose network.

Tear down:
```bash
docker compose down -v
```

## License

MIT
