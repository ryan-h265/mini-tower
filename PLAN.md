# MiniTower MVP Plan (Thin Tower Concepts)

## 0. Goal (MVP)

Build a small, **correctness-first orchestration system** in Go with Tower-like concepts, but minimal surface area:

- `app` and immutable `version`
- `run` execution via self-hosted `runner`
- tenant/environment scaffolding (`team`, `default` environment only)

**Correctness in MVP explicitly means:**
- No double execution
- Monotonic state transitions
- Deterministic retries
- Race-safe leasing and mutation

MVP target: deploy a version, trigger runs, stream logs, support retries/cancel, and keep state race-safe.

---

## 1. Simplicity Rules

1. Keep schema future-proof, keep API surface small.
2. Prefer one clear path per workflow (no duplicate ways to do the same thing).
3. Defer any feature that is not required for deploy → run → observe.
4. **Prefer explicit state over implicit behavior.**

---

## 2. Scope

### In Scope (MVP)

- Single bootstrap team (no public team management API).
- Single required environment: `default` (stored in DB, no environment CRUD API yet).
- App CRUD (minimal), version creation from packaged source artifact, run trigger/list/get/cancel, run logs.
- Runner registration + lease/start/heartbeat/log/result protocol.
- Retry/dead-letter behavior with run attempts.
- Per-run private Python virtual environment (`venv`) for dependency isolation.
- Structured logs, Prometheus metrics, health/readiness.

### Deferred (Post-MVP)

Deferred features are **intentionally excluded** to minimize the failure surface while validating orchestration correctness.

- Multi-team self-service management.
- Environment CRUD and environment-specific overrides.
- Schedules/cron.
- Webhooks.
- Secrets management.
- Hardened sandboxing (containers/microVMs/seccomp/cgroups).
- UI and advanced RBAC.

---

## 3. Architecture

### Control Plane

- HTTP+JSON API.
- SQLite as system of record.
- Local object store path for version artifacts.
- Scheduler goroutine for lease expiry/retry/dead transitions.
- **Never executes customer workload.**

### Runner

- Registers once, polls for work, executes via `os/exec`.
- Heartbeats, streams logs, reports result.
- One active run at a time (`max_concurrent = 1` in MVP).
- Creates an isolated per-run workspace and private `venv`.

### Isolation Note (MVP)

- Per-run `venv` + process controls provide dependency isolation.
- This is **not** a hardened security sandbox.
- **Threat model explicitly excluded:** malicious workloads, privilege escalation, host compromise.

---

## 4. Domain Model (MVP)

1. `Team`: tenant boundary (bootstrap-created once).
2. `Environment`: one row per team for `default`.
3. `App`: logical workload identity.
4. `AppVersion`: immutable packaged artifact spec (`artifact_object_key`, `artifact_sha256`, `entrypoint`, `timeout_seconds`, optional params schema).
5. `Run`: lifecycle state for execution request.
6. `RunAttempt`: retry attempt with lease identity.
7. `Runner`: self-hosted worker bound to team + environment.
8. `RunLog`: stdout/stderr lines keyed by attempt sequence.

---

## 5. Status Model and Invariants

### Run Statuses

- Non-terminal: `queued`, `leased`, `running`, `cancelling`
- Terminal: `completed`, `failed`, `cancelled`, `dead`

### Run Transitions

1. `queued → leased` on successful lease.
2. `leased → running` on runner start acknowledgement.
3. `running → completed | failed | cancelled` on result report.
4. `queued → cancelled` on admin cancel before lease.
5. `leased | running → cancelling` on admin cancel in-flight.
6. `leased | running → queued` on lease expiry if retry budget remains.
7. `leased | running → dead` on lease expiry if retries exhausted.

### Hard Invariants

1. Only one active lease per run.
2. Only current lease holder can heartbeat/log/result.
3. Terminal runs are immutable.
4. Retries create new `run_attempt` (`attempt_no` increments).
5. Log dedupe key is `(run_attempt_id, seq)`.
6. All queries are team-scoped by token-derived `team_id`.
7. All time comparisons use server UTC Unix milliseconds.
8. **State transitions are monotonic; attempts never move backwards.**
9. **All transitions are enforced via conditional updates (CAS) at the DB layer.**

---

## 6. Database Schema (MVP)

### Tables

- `teams(id, slug, name, registration_token_hash, created_at, updated_at)`
- `team_tokens(id, team_id, token_hash, created_at, revoked_at, last_used_at)`
- `environments(id, team_id, name, is_default, created_at, updated_at, UNIQUE(team_id,name))`
- `apps(id, team_id, slug, description, disabled, created_at, updated_at, UNIQUE(team_id,slug))`
- `app_versions(id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at, UNIQUE(app_id,version_no))`
- `runs(id, team_id, app_id, environment_id, app_version_id, run_no, status, priority, max_retries, retry_count, cancel_requested, queued_at, started_at, finished_at, created_at, updated_at, UNIQUE(app_id,run_no))`
- `run_attempts(id, run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, exit_code, error_message, started_at, finished_at, created_at, updated_at, UNIQUE(run_id,attempt_no))`
- `run_logs(id, run_attempt_id, seq, stream, line, logged_at, UNIQUE(run_attempt_id,seq))`
- `runners(id, team_id, name, environment_id, labels_json, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at, UNIQUE(team_id,name))`

> `runs.status` and `run_attempts.status` are intentionally duplicated to avoid join-heavy hot paths during scheduling and monitoring.

### SQLite Runtime

- `PRAGMA journal_mode=WAL`
- `PRAGMA foreign_keys=ON`
- `PRAGMA busy_timeout=5000`
- `PRAGMA synchronous=NORMAL`
- Write DB handle with `SetMaxOpenConns(1)`

SQLite is intentionally used to surface concurrency and transactional edge cases early, rather than masking them with a more forgiving database.

---

## 7. API Contract (MVP)

All endpoints are authenticated via team-scoped tokens.  
Runner endpoints additionally require **lease token authentication** where applicable.

### Bootstrap/Admin

1. `POST /api/v1/bootstrap/team`
2. `POST /api/v1/tokens`

### Apps and Versions

1. `POST /api/v1/apps`
2. `GET /api/v1/apps`
3. `GET /api/v1/apps/{app}`
4. `POST /api/v1/apps/{app}/versions`
5. `GET /api/v1/apps/{app}/versions`

### Runs

1. `POST /api/v1/apps/{app}/runs`
2. `GET /api/v1/apps/{app}/runs`
3. `GET /api/v1/runs/{run}`
4. `POST /api/v1/runs/{run}/cancel`
5. `GET /api/v1/runs/{run}/logs`

### Runner

1. `POST /api/v1/runners/register`
2. `POST /api/v1/runs/lease`
3. `POST /api/v1/runs/{run}/start`
4. `POST /api/v1/runs/{run}/heartbeat`
5. `POST /api/v1/runs/{run}/logs`
6. `POST /api/v1/runs/{run}/result`
7. `GET /api/v1/runs/{run}/artifact` (runner-only; returns artifact download URL or streamed payload + metadata)

### Ops
- `GET /health`
- `GET /ready`
- `GET /metrics`

### Error Envelope
```json
{"error":{"code":"conflict","message":"..."}}
```

Error codes: `invalid_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `gone`, `internal`.

## 8. Lease and Retry Algorithm

Lease assignment is the **only code path** that transitions a run from `queued → leased`.

### Lease Claim (single transaction)

1. Verify runner active and has no active attempt.
2. Select queued runs for same `team_id` + `default` environment.
3. Conditional update run state (`queued → leased`) with lease token + expiry.
4. Create new `run_attempt`.
5. Commit.

### Expiry Reaper

Runs every `MINITOWER_EXPIRY_CHECK_INTERVAL`:
- Expired attempts:
  - retries remain → requeue
  - retries exhausted → mark run `dead`
- Mark stale runners offline.

---

## 9. Runner Protocol (MVP)

- Heartbeats and log streaming are **intentionally decoupled** to avoid log backpressure affecting liveness.


1. Register using registration token if runner token missing.
2. Poll lease endpoint with jitter.
3. On lease:
- call `/start`
- download artifact, verify `sha256`, unpack into per-run temp workspace
- create private `venv` (`python -m venv .venv`)
- install dependencies if `requirements.txt` exists
- execute configured `entrypoint` via `.venv/bin/python` with timeout
- heartbeat every `lease_ttl/3`
- send log batches (max 100 lines/batch, max 8 KiB/line)
- send terminal result once
4. On `cancel_requested=true`:
- SIGTERM -> wait grace -> SIGKILL if needed -> report `cancelled`.
5. On stale lease conflict:
- stop local process and stop reporting.
6. On completion/failure:
- remove per-run workspace and venv (best effort).

## 10. Implementation Phases

### Phase 1: Foundation
- Module, config, migrations, auth/error middleware.

Acceptance:
- Server boots, migrations apply.

### Phase 2: Core Domain
- Team bootstrap, default environment seed, app/version/run schema + store.
- Add artifact persistence layer (local filesystem object store).

Acceptance:
- App/version creation and run enqueue work.

### Phase 3: Runner Protocol
- Register/lease/start/heartbeat/log/result endpoints + runner client/agent.
- Add artifact fetch/verify/unpack + per-run venv execution path.

Acceptance:
- End-to-end run succeeds on a real runner.

### Phase 4: Retry/Cancellation Correctness
- Expiry reaper, retries, dead-letter, cancellation paths.

Acceptance:
- Lease expiry/cancel tests pass consistently.

### Phase 5: Observability + Hardening
- Metrics, structured logs, request limits, smoke script/docs.

Acceptance:
- `go test ./...`, `go test -race ./...`, `go vet ./...` pass.

## 11. Test Plan (Must-Have)
1. Cross-team access blocked by token scoping.
2. Concurrent lease requests cannot double-assign.
3. Runner with active attempt cannot lease another run.
4. Stale lease token cannot heartbeat/log/result.
5. Lease expiry requeues then moves to `dead` at retry limit.
6. In-flight cancel transitions to `cancelling` then `cancelled`.
7. Duplicate logs dedupe by `(run_attempt_id, seq)`.
8. Duplicate result report is idempotent.
9. Terminal run rejects further mutation.
10. Runner offline/online status updates are correct.
11. Artifact checksum mismatch fails run and marks `failed`.
12. Per-run workspace/venv is cleaned up after run completion.

## 12. Configuration

### Control Plane
- `MINITOWER_LISTEN_ADDR` (default `:8080`)
- `MINITOWER_DB_PATH` (default `./minitower.db`)
- `MINITOWER_OBJECTS_DIR` (default `./objects`)
- `MINITOWER_BOOTSTRAP_TOKEN` (required)
- `MINITOWER_LEASE_TTL` (default `60s`)
- `MINITOWER_EXPIRY_CHECK_INTERVAL` (default `10s`)

### Runner
- `MINITOWER_SERVER_URL` (required)
- `MINITOWER_TEAM_SLUG` (required)
- `MINITOWER_RUNNER_NAME` (required)
- `MINITOWER_RUNNER_TOKEN` (optional after first register)
- `MINITOWER_REGISTRATION_TOKEN` (required for first register)
- `MINITOWER_PYTHON_BIN` (default `python3`)
- `MINITOWER_POLL_INTERVAL` (default `3s`)
- `MINITOWER_KILL_GRACE_PERIOD` (default `10s`)
- `MINITOWER_DATA_DIR` (default `~/.minitower`)

## 13. Definition of Done
1. Bootstrap team + default environment works.
2. App/version/run endpoints work with team tokens.
3. Runner executes real runs end-to-end with logs.
4. Retry/dead and cancel flows are deterministic.
5. Required tests pass with race detector.
6. Smoke flow works: bootstrap -> token -> app -> upload version artifact -> run -> logs -> completion.

## 14. Post-MVP Backlog (Next)

1. Environment CRUD + override semantics.
2. Secrets CRUD + runtime injection.
3. Schedules/cron materializer.
4. Signed webhooks and delivery retries.
5. Rich artifact packaging and dependency caching optimizations.
6. **Pluggable object store backend (S3-compatible).**
