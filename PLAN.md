# MiniTower MVP Plan (Thin Tower Concepts)

## 0. Goal (MVP)

Build a small, **correctness-first orchestration system** in Go with Tower-like concepts, but minimal surface area:

- `app` and immutable `version`
- `run` execution via self-hosted `runner`
- tenant/environment scaffolding (`team`, `default` environment only)

**Correctness in MVP explicitly means:**
- No double execution
- Monotonic state transitions per run attempt
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
- Run trigger accepts optional JSON params validated against the selected version schema.
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
4. `cancelling → cancelled` on result report.
5. `queued → cancelled` on admin cancel before lease.
6. `leased | running → cancelling` on admin cancel in-flight.
7. `leased | running → queued` on lease expiry if retry budget remains and `cancel_requested=false` (current attempt becomes `expired`; next attempt is created on next lease).
8. `leased | running → dead` on lease expiry if retries exhausted and `cancel_requested=false`.
9. `cancelling → cancelled` on lease expiry (cancel intent wins; no retry).

### RunAttempt Statuses

- Non-terminal: `leased`, `running`, `cancelling`
- Terminal: `completed`, `failed`, `cancelled`, `expired`

### RunAttempt Transitions

1. `leased → running` on `/start` via CAS (`WHERE status='leased'`).
2. `leased | running → cancelling` on cancel request.
3. `running → completed | failed | cancelled` on `/result`.
4. `cancelling → cancelled` on `/result` or lease expiry.
5. `leased | running → expired` on lease expiry.
6. Terminal attempts are immutable.
7. `/start` MUST NOT move `cancelling`/terminal attempts back to `running`.

### Hard Invariants

1. Only one active lease per run.
2. Only current lease holder can fetch artifact/heartbeat/log/result.
3. Terminal runs are immutable.
4. Retries create new `run_attempt` (`attempt_no` increments).
5. Log dedupe key is `(run_attempt_id, seq)`.
6. All queries are team-scoped by token-derived `team_id`.
7. All time comparisons use server UTC Unix milliseconds.
8. **Attempt transitions are monotonic; attempts never move backwards.**
9. **A run can return to `queued` only after the current active attempt is finalized as `expired` and `retry_count` is incremented.**
10. **The next attempt (`attempt_no+1`) is created only by a later successful lease claim.**
11. **All transitions are enforced via conditional updates (CAS) at the DB layer.**
12. `cancel_requested=true` disables retry requeue and converges to `cancelled`.
13. Active-attempt uniqueness is enforced by DB partial unique indexes (not just application logic).
14. Once a run/attempt enters `cancelling`, cancel intent wins and terminal outcome is constrained to `cancelled`.

---

## 6. Database Schema (MVP)

### Tables

- `teams(id, slug, name, registration_token_hash, created_at, updated_at)`
- `team_tokens(id, team_id, token_hash, created_at, revoked_at, last_used_at)`
- `environments(id, team_id, name, is_default, created_at, updated_at, UNIQUE(team_id,name))`
- `apps(id, team_id, slug, description, disabled, created_at, updated_at, UNIQUE(team_id,slug))`
- `app_versions(id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at, UNIQUE(app_id,version_no))`
- `runs(id, team_id, app_id, environment_id, app_version_id, run_no, input_json, status, priority, max_retries, retry_count, cancel_requested, queued_at, started_at, finished_at, created_at, updated_at, UNIQUE(app_id,run_no))`
- `run_attempts(id, run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, exit_code, error_message, started_at, finished_at, created_at, updated_at, UNIQUE(run_id,attempt_no))`
- `run_logs(id, run_attempt_id, seq, stream, line, logged_at, UNIQUE(run_attempt_id,seq))`
- `runners(id, team_id, name, environment_id, labels_json, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at, UNIQUE(team_id,name))`

> `runs.status` and `run_attempts.status` are intentionally duplicated to avoid join-heavy hot paths during scheduling and monitoring.
> Lease identity lives on `run_attempts` (`lease_token_hash`, `lease_expires_at`) and is the source of truth for heartbeat/log/result authorization.

### Required Indexes and Constraints

- Partial unique index: one active attempt per run.
  - `CREATE UNIQUE INDEX run_attempts_active_run_uq ON run_attempts(run_id) WHERE status IN ('leased','running','cancelling');`
- Partial unique index: one active attempt per runner.
  - `CREATE UNIQUE INDEX run_attempts_active_runner_uq ON run_attempts(runner_id) WHERE status IN ('leased','running','cancelling');`
- Partial index for expiry scans.
  - `CREATE INDEX run_attempts_expiry_idx ON run_attempts(lease_expires_at) WHERE status IN ('leased','running','cancelling');`
- Partial index for deterministic queue selection in lease order.
  - `CREATE INDEX runs_queue_pick_idx ON runs(team_id, environment_id, status, priority DESC, queued_at ASC, id ASC) WHERE status='queued';`
- Composite uniqueness to support team-scoped foreign keys.
  - `CREATE UNIQUE INDEX apps_id_team_uq ON apps(id, team_id);`
  - `CREATE UNIQUE INDEX environments_id_team_uq ON environments(id, team_id);`
  - `CREATE UNIQUE INDEX app_versions_id_app_uq ON app_versions(id, app_id);`
- Foreign keys that enforce team-consistent ownership.
  - `runs(app_id, team_id) -> apps(id, team_id)`
  - `runs(environment_id, team_id) -> environments(id, team_id)`
  - `runs(app_version_id, app_id) -> app_versions(id, app_id)`
  - `runners(environment_id, team_id) -> environments(id, team_id)`
  - `run_attempts(run_id) -> runs(id)`, `run_attempts(runner_id) -> runners(id)`, `run_logs(run_attempt_id) -> run_attempts(id)`
- CHECK constraints:
  - `runs.status IN ('queued','leased','running','cancelling','completed','failed','cancelled','dead')`
  - `run_attempts.status IN ('leased','running','cancelling','completed','failed','cancelled','expired')`
  - `retry_count >= 0` and `max_retries >= 0`
- NOT NULL/default constraints (minimum; enforce in migrations):
  - `runs.team_id, app_id, environment_id, app_version_id, run_no, status, priority, max_retries, retry_count, cancel_requested, queued_at, created_at, updated_at` are `NOT NULL`.
  - `run_attempts.run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, created_at, updated_at` are `NOT NULL`.
  - `run_logs.run_attempt_id, seq, stream, line, logged_at` are `NOT NULL`.
  - Defaults: `runs.status='queued'`, `runs.priority=0`, `runs.max_retries=0`, `runs.retry_count=0`, `runs.cancel_requested=false`.
  - Defaults: `run_attempts.status='leased'`, `runners.status='online'`, `runners.max_concurrent=1`.

### SQLite Runtime

- `PRAGMA journal_mode=WAL`
- `PRAGMA foreign_keys=ON`
- `PRAGMA busy_timeout=5000`
- `PRAGMA synchronous=NORMAL`
- Write DB handle with `SetMaxOpenConns(1)`

SQLite is intentionally used to surface concurrency and transactional edge cases early, rather than masking them with a more forgiving database.

---

## 7. API Contract (MVP)

Authentication model:

- `POST /api/v1/bootstrap/team` uses bootstrap token.
- Control-plane app/version/run/token endpoints use team API tokens.
- Runner endpoints use runner token (issued at registration).
- Runner attempt-scoped endpoints (`start`, `heartbeat`, `logs`, `result`, `artifact`) require both runner token and current lease token.

### Bootstrap/Admin

1. `POST /api/v1/bootstrap/team`
2. `POST /api/v1/tokens`

### Apps and Versions

1. `POST /api/v1/apps`
2. `GET /api/v1/apps`
3. `GET /api/v1/apps/{app}`
4. `POST /api/v1/apps/{app}/versions` (multipart upload; creates immutable version and stores artifact)
5. `GET /api/v1/apps/{app}/versions`

Version creation request contract (single path):
- `Content-Type: multipart/form-data`
- Required parts:
  - `artifact` (`.tar.gz` package)
  - `entrypoint` (relative Python file path inside artifact)
- Optional parts:
  - `timeout_seconds`
  - `params_schema_json` (JSON Schema object)
- Control plane computes and stores `artifact_sha256`; client cannot provide override checksum in MVP.

### Runs

1. `POST /api/v1/apps/{app}/runs` (optional `input_json`, validated against `params_schema_json` when present)
2. `GET /api/v1/apps/{app}/runs`
3. `GET /api/v1/runs/{run}`
4. `POST /api/v1/runs/{run}/cancel`
5. `GET /api/v1/runs/{run}/logs`

### Runner

1. `POST /api/v1/runners/register` (auth: registration token; returns runner token)
2. `POST /api/v1/runs/lease`
3. `POST /api/v1/runs/{run}/start`
4. `POST /api/v1/runs/{run}/heartbeat`
5. `POST /api/v1/runs/{run}/logs`
6. `POST /api/v1/runs/{run}/result`
7. `GET /api/v1/runs/{run}/artifact` (runner + active lease token; returns streamed artifact payload + metadata headers)

### Runner Attempt Response Contract
- `/start` and `/heartbeat` responses must include: `run_attempt_id`, `attempt_no`, `lease_expires_at`, `cancel_requested`, `run_status`.
- Runner uses these fields as the source of truth for lease liveness and cancellation intent.

### Ops
- `GET /health`
- `GET /ready`
- `GET /metrics`

### Error Envelope
```json
{"error":{"code":"conflict","message":"..."}}
```

Error codes: `invalid_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `gone`, `internal`.

### Idempotency and Conflict Rules

- `POST /api/v1/runs/{run}/cancel` is idempotent; repeated calls return current run state.
- `POST /api/v1/runs/{run}/start` with the current lease token is CAS-gated:
  - if attempt status is `leased` => transition to `running`.
  - if attempt status is already `running` for the same active attempt => idempotent success.
  - if attempt status is `cancelling` => `conflict` (no state change).
- `POST /api/v1/runs/{run}/heartbeat` with the current lease token is idempotent and only extends the same active lease window.
- `POST /api/v1/runs/{run}/result` is first-write-wins:
  - duplicate with same terminal payload for same attempt => idempotent success.
  - duplicate with different payload for same attempt => `conflict`.
- Once a run/attempt is `cancelling`, `/result` may only finalize as `cancelled`; `completed`/`failed` payloads return `conflict`.
- Attempt-scoped calls (`start`, `heartbeat`, `logs`, `result`, `artifact`) with stale/expired/non-current lease token => `gone`.

## 8. Lease and Retry Algorithm

Lease assignment is the **only code path** that transitions a run from `queued → leased`.

### Lease Claim (single transaction)

1. Verify runner active and has no active attempt.
2. Select queued runs for same `team_id` + `default` environment ordered by `priority DESC, queued_at ASC, id ASC`.
3. Conditional update run state (`queued → leased`) via CAS (`WHERE status='queued' AND cancel_requested=false`).
4. Create new `run_attempt` with `status='leased'`, `lease_token_hash`, and `lease_expires_at`.
5. Commit.

### Retry Policy (deterministic)

- MVP retry source is lease expiry only. A reported `failed` result is terminal and not retried.
- No backoff in MVP: retry requeue is immediate.
- Requeue operation must atomically:
  - mark expiring attempt `expired`.
  - increment `retry_count`.
  - set run state back to `queued`.
  - set `queued_at=now` (UTC Unix ms).
- Requeue does not pre-create a new attempt; the next `run_attempt` is created only on the next successful lease claim.
- `max_retries` means "additional attempts after the initial attempt."
- Retry limit rule: with `retry_count` counting consumed retries, when `retry_count == max_retries`, the current expiring attempt transitions run to `dead` (no requeue).
- Example: `max_retries=2` allows up to 3 total attempts (`attempt_no` 1, 2, 3).

### Expiry Reaper

Runs every `MINITOWER_EXPIRY_CHECK_INTERVAL`:
- Expired attempts:
  - `cancel_requested=true` or attempt status `cancelling` -> mark attempt `cancelled`, mark run `cancelled` (no retry)
  - retries remain and `cancel_requested=false` -> mark attempt `expired`, requeue run (new attempt created only on next lease claim)
  - retries exhausted and `cancel_requested=false` -> mark attempt `expired`, mark run `dead`
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
- update local lease deadline from `/start` and `/heartbeat` responses; if lease renewal is not acknowledged before local deadline (with a small clock-skew safety margin), self-fence by terminating the workload and stop reporting
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
13. `cancel` and `result` racing in parallel converges to one terminal state (idempotent).
14. Lease expiry racing with late `result` does not resurrect or regress terminal state.
15. Duplicate `/start` with same lease token is idempotent.
16. Duplicate `/heartbeat` with same lease token is idempotent.
17. Queue selection is deterministic (`priority DESC, queued_at ASC, id ASC`).
18. Duplicate `/result` with conflicting payload returns `conflict`.
19. Runner self-fences if lease renewal is not acknowledged before local lease deadline.
20. `/start` and `/heartbeat` responses always return `lease_expires_at` and `cancel_requested`, and runner behavior follows them.
21. `/artifact` with stale/expired/non-current lease token returns `gone`.

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
