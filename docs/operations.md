# Operations Guide

## Migration Notes

- Migration `internal/migrations/0004_towerfile.up.sql` adds `towerfile_toml` and `import_paths_json` columns to `app_versions`.
- Migration `internal/migrations/0003_token_role.up.sql` adds `team_tokens.role` (`admin|member`).
- Existing environments should start `minitowerd` once after upgrading so migrations are applied.

## Monitoring and Metrics

MiniTower exposes Prometheus metrics at `GET /metrics`.

### HTTP Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_http_requests_total` | method, path, status | Total HTTP requests |
| `minitower_http_request_duration_seconds` | method, path | Request latency histogram |
| `minitower_http_request_size_bytes` | method, path | Request size histogram |
| `minitower_http_response_size_bytes` | method, path | Response size histogram |

### Domain Counters

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_runs_created_total` | team, app | Runs created |
| `minitower_runs_completed_total` | team, app, status | Runs reaching terminal state |
| `minitower_runs_retried_total` | team, app | Runs retried by reaper |
| `minitower_runs_leased_total` | environment | Runs leased by runners |
| `minitower_runners_registered_total` | environment | Runner registrations |

### Domain Histograms

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_run_queue_wait_seconds` | team, app | Queue wait duration |
| `minitower_run_execution_seconds` | team, app, status | Execution duration |
| `minitower_run_total_seconds` | team, app, status | Total run duration |

### Domain Gauges

| Metric | Labels | Description |
|--------|--------|-------------|
| `minitower_runs_pending` | team, app, environment | Current queued runs |
| `minitower_runners_online` | environment | Current online runners |

### Example PromQL

```promql
rate(minitower_runs_created_total[5m])
rate(minitower_runs_completed_total{status="failed"}[5m])
minitower_runs_pending
histogram_quantile(0.99, rate(minitower_run_execution_seconds_bucket[5m]))
```

## Testing

Backend tests:

```bash
go test ./...
go test -race ./...
go test -tags=integration ./cmd/minitower-runner
./scripts/smoke.sh
```

Frontend tests:

```bash
npm --prefix frontend run test -- --run
npm --prefix frontend run build
```

## Docker Compose Demo

Run full stack:

```bash
docker compose up -d --build
./scripts/demo-compose.sh --loop
```

Endpoints:

- Grafana: `http://localhost:3000`
- Prometheus: `http://localhost:9090`
- API: `http://localhost:8080`
- Frontend: `http://localhost:5173`

Tear down:

```bash
docker compose down -v
```
