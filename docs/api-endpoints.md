# API Endpoints

## Health & Metrics
- `GET /health` — Liveness check
- `GET /ready` — Readiness check (includes DB ping)
- `GET /metrics` — Prometheus metrics

## Team Management
- `GET /api/v1/auth/options` — Public auth feature flags (`signup_enabled`, `bootstrap_enabled`)
- `POST /api/v1/teams/signup` — Create a team (`slug`, `name`, `password`) and return an admin token
- `POST /api/v1/teams/login` — Authenticate with slug + password, returns token + role
- `POST /api/v1/bootstrap/team` — Operator bootstrap/recovery API only (not exposed in frontend UI; route exists only when bootstrap token is configured)
- `GET /api/v1/me` — Resolve team identity + token role
- `POST /api/v1/tokens` — Create additional API tokens (admin/member role assignment for admins)

## Apps & Versions
- `POST /api/v1/apps` — Create app
- `GET /api/v1/apps` — List apps
- `GET /api/v1/apps/{app}` — Get app details
- `POST /api/v1/apps/{app}/versions` — Upload version (multipart artifact with Towerfile)
- `GET /api/v1/apps/{app}/versions` — List versions

## Runs
- `POST /api/v1/apps/{app}/runs` — Trigger run
- `GET /api/v1/apps/{app}/runs` — List runs
- `GET /api/v1/runs` — List team-wide runs (`limit`, `offset`, `status`, `app` filters)
- `GET /api/v1/runs/summary` — Team run aggregate counts for dashboard cards
- `GET /api/v1/runs/{run}` — Get run status
- `POST /api/v1/runs/{run}/cancel` — Cancel run
- `GET /api/v1/runs/{run}/logs` — Get run logs (`after_seq` supports incremental fetch)

## Admin
- `GET /api/v1/admin/runners` — List registered runners (admin token required)

## Runner Protocol
- `POST /api/v1/runners/register` — Register runner (registration token)
- `POST /api/v1/runs/lease` — Lease next queued run
- `POST /api/v1/runs/{run}/start` — Acknowledge lease, transition to running
- `POST /api/v1/runs/{run}/heartbeat` — Extend lease, check for cancellation
- `POST /api/v1/runs/{run}/logs` — Submit log batch (runner token + lease token)
- `POST /api/v1/runs/{run}/result` — Submit terminal result
- `GET /api/v1/runs/{run}/artifact` — Download version artifact
