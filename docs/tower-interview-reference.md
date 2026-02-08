# MiniTower Interview Reference (Tower.dev - Distributed Systems)

Snapshot date: 2026-02-08

## Why this document exists

This is a system-level reference for understanding MiniTower end-to-end, and for preparing interview discussion topics aligned with a distributed systems software engineer role.

It covers:
- Architecture and runtime flow.
- State machine and correctness properties.
- Every main API action and where it is implemented.
- File-level and function-level source map.
- Interview topics, tradeoffs, and productionization gaps.

## 1) System summary

MiniTower is a correctness-first orchestration system with three executables:
- `minitowerd` (control plane): API + scheduler/reaper + persistence.
- `minitower-runner` (worker): leases work, executes workloads, heartbeats, streams logs, submits results.
- `minitower-cli` (deploy client): reads `Towerfile`, packages artifact, ensures app exists, uploads version.

Core architecture:
- Control plane is stateless except SQLite + local object storage.
- Runners are external workers that use lease tokens for attempt-scoped authority.
- Retry behavior is deterministic via lease-expiry reaping.
- CAS-style conditional updates prevent invalid state transitions and duplicate execution.

## 2) What to emphasize for a distributed-systems interview

The strongest themes in this codebase:
- Lease-based work ownership with active-attempt uniqueness.
- Idempotent and monotonic state transitions for attempts and runs.
- Self-fencing behavior in runner on stale lease.
- Deterministic queue selection and retry progression.
- Failure handling for cancel/result/expiry races.
- High-cardinality-safe metrics path normalization.
- Clear separation of control plane and data plane (runner execution).

Good interview framing:
- "This is a single-node control plane with distributed runners. I optimized for correctness semantics before horizontal scale."
- "I used explicit attempt state, lease token binding, and DB constraints to prevent double execution."
- "I constrained non-determinism with explicit ordering and transition guards."

## 3) End-to-end lifecycle

### Deploy flow

1. `minitower-cli` parses and validates `Towerfile`.
2. Source glob resolution builds a deterministic artifact set.
3. CLI ensures app exists (`GET /apps/{slug}`, then optional `POST /apps`).
4. CLI uploads artifact (`POST /apps/{slug}/versions`).
5. Server extracts `Towerfile` from archive, validates it, derives metadata, stores artifact + version row.

### Run execution flow

1. Team triggers run (`POST /apps/{slug}/runs`) -> run row inserted as `queued`.
2. Runner polls (`POST /runs/lease`) -> control plane atomically claims next eligible queued run (`queued -> leased`) and creates attempt.
3. Runner acknowledges lease (`POST /runs/{id}/start`) -> attempt transitions `leased -> running`.
4. Runner downloads artifact (`GET /runs/{id}/artifact`), verifies SHA256, unpacks workspace, creates venv for Python entrypoints.
5. Runner executes process while:
- heartbeat loop extends lease (`POST /heartbeat`),
- log loop flushes batches (`POST /logs`),
- cancellation is honored via heartbeat response flag.
6. Runner submits terminal result (`POST /result`) -> attempt and run finalize.
7. Reaper transitions expired attempts to `expired`, then requeue/dead/cancelled outcome based on retry budget + cancel intent.

## 4) Status model and correctness rules

Run statuses:
- Active: `queued`, `leased`, `running`, `cancelling`
- Terminal: `completed`, `failed`, `cancelled`, `dead`

Attempt statuses:
- Active: `leased`, `running`, `cancelling`
- Terminal: `completed`, `failed`, `cancelled`, `expired`

Key invariants implemented in code and schema:
- Only one active attempt per run.
- Only one active attempt per runner (MVP max concurrency 1).
- Lease token must match active attempt for start/heartbeat/log/result/artifact.
- Attempt transitions are monotonic; no backward transition to active after terminal.
- Runs in terminal states are immutable with respect to execution lifecycle.
- Requeue only happens through reaper expiry logic, not arbitrary API mutation.

How this is enforced:
- Conditional updates (CAS) in store methods.
- Partial unique indexes on active attempts.
- Check constraints on valid status sets.
- Store-level error mapping to HTTP (`410 gone`, `409 conflict`, etc.).

## 5) Concurrency and race handling

Important race scenarios covered:
- Concurrent lease attempts: only one succeeds, others get conflict/no-work.
- Cancel vs result races: converges via attempt status checks and conflict semantics.
- Late result after expiry: rejected (`ErrAttemptNotActive`) and does not resurrect run.
- Duplicate start/result submissions: idempotent where safe, conflict where inconsistent.

Queue selection policy (deterministic):
- Ordered by `priority DESC, queued_at ASC, id ASC` for eligible queued runs in runner environment.

## 6) Auth, identity, and authorization

Token types:
- Bootstrap token: control-plane setup (`/bootstrap/team`).
- Team tokens: app/version/run/token endpoints.
- Runner registration token: platform-level registration.
- Runner token: operational runner auth.
- Lease token: per-attempt authority for sensitive runner operations.

Role model:
- Team token role is `admin` or `member`.
- `/api/v1/admin/runners` guarded by admin role.
- Token creation behavior:
- Non-admin callers effectively create member tokens.
- Admin callers can choose admin/member.

Security details:
- Tokens stored hashed (`SHA-256`) server-side.
- Constant-time compare for bootstrap/registration secrets via hashed comparison.
- Team scoping on queries prevents cross-team data access.

## 7) Observability and operations

Metrics:
- HTTP metrics: request count, latency, request/response sizes.
- Domain counters: runs created/completed/retried/leased, runner registrations.
- Domain histograms: queue wait, execution duration, total duration.
- Domain gauges (collector querying DB on scrape): pending runs, online runners.

Operational endpoints:
- `GET /health` for liveness.
- `GET /ready` for readiness (DB ping).
- `GET /metrics` for Prometheus scraping.

Operational scripts:
- `scripts/smoke.sh` for end-to-end validation.
- `docker-compose.yml` for full local stack (`minitowerd`, runners, frontend, Prometheus, Grafana).

## 8) API action map (what action goes where)

Health and diagnostics:
- `GET /health` -> `internal/httpapi/server.go` (`handleHealth`)
- `GET /ready` -> `internal/httpapi/server.go` (`handleReady`)
- `GET /metrics` -> `internal/httpapi/metrics.go`

Bootstrap and identity:
- `POST /api/v1/bootstrap/team` -> `internal/httpapi/handlers/bootstrap.go`
- `POST /api/v1/teams/login` -> `internal/httpapi/handlers/login.go`
- `GET /api/v1/me` -> `internal/httpapi/handlers/me.go`
- `POST /api/v1/tokens` -> `internal/httpapi/handlers/tokens.go`

App and version operations:
- `POST /api/v1/apps` -> `internal/httpapi/handlers/apps.go`
- `GET /api/v1/apps` -> `internal/httpapi/handlers/apps.go`
- `GET /api/v1/apps/{app}` -> `internal/httpapi/handlers/apps.go`
- `POST /api/v1/apps/{app}/versions` -> `internal/httpapi/handlers/versions.go`
- `GET /api/v1/apps/{app}/versions` -> `internal/httpapi/handlers/versions.go`

Run operations:
- `POST /api/v1/apps/{app}/runs` -> `internal/httpapi/handlers/runs.go`
- `GET /api/v1/apps/{app}/runs` -> `internal/httpapi/handlers/runs.go`
- `GET /api/v1/runs` -> `internal/httpapi/handlers/runs.go`
- `GET /api/v1/runs/summary` -> `internal/httpapi/handlers/runs.go`
- `GET /api/v1/runs/{run}` -> `internal/httpapi/handlers/runs.go`
- `POST /api/v1/runs/{run}/cancel` -> `internal/httpapi/handlers/runs.go`
- `GET /api/v1/runs/{run}/logs` -> `internal/httpapi/handlers/runs.go`

Runner protocol:
- `POST /api/v1/runners/register` -> `internal/httpapi/handlers/runner.go`
- `POST /api/v1/runs/lease` -> `internal/httpapi/handlers/runner.go`
- `POST /api/v1/runs/{run}/start` -> `internal/httpapi/handlers/runner.go`
- `POST /api/v1/runs/{run}/heartbeat` -> `internal/httpapi/handlers/runner.go`
- `POST /api/v1/runs/{run}/logs` -> `internal/httpapi/handlers/runner.go`
- `POST /api/v1/runs/{run}/result` -> `internal/httpapi/handlers/runner.go`
- `GET /api/v1/runs/{run}/artifact` -> `internal/httpapi/handlers/runner.go`

Admin:
- `GET /api/v1/admin/runners` -> `internal/httpapi/handlers/admin.go`

## 9) Frontend action map

App bootstrapping and auth state:
- `frontend/src/main.ts`: creates app, pinia, vue-query, auth rehydrate, unauthorized handler.
- `frontend/src/stores/auth.ts`: signup/login/bootstrap/fetchMe/logout/rehydrate state machine.
- `frontend/src/router/index.ts`: auth + admin route guards.

Pages and their backend actions:
- `frontend/src/pages/LoginPage.vue`
- signup form -> `apiClient.signupTeam`
- login form -> `apiClient.loginTeam`
- `frontend/src/pages/HomePage.vue`
- dashboard cards -> `listApps`, `getRunsSummary`, `listRunsByTeam`
- `frontend/src/pages/AppsPage.vue`
- list/filter apps -> `listApps`
- create app modal -> `createApp`
- `frontend/src/pages/AppDetailPage.vue`
- app details -> `getApp`
- versions -> `listVersions`, upload via `createVersion`
- runs -> `listRunsByApp`, create run via `createRun`
- `frontend/src/pages/GlobalRunsPage.vue`
- global list/filter -> `listRunsByTeam`
- `frontend/src/pages/RunDetailPage.vue`
- run detail -> `getRun`
- incremental logs polling -> `getRunLogs`
- cancel -> `cancelRun`
- rerun -> `createRun`
- `frontend/src/pages/TokenSettingsPage.vue`
- create token -> `createToken`
- `frontend/src/pages/AdminRunnersPage.vue`
- admin runner visibility -> `listAdminRunners`

API client and transport:
- `frontend/src/api/client.ts`
- handles auth header injection.
- handles 401 by clearing token and calling unauthorized handler.
- maps error envelope to `ApiError`.

## 10) File-by-file reference

### Root and docs

- `README.md`: product overview, API list, config, quickstart.
- `PLAN.md`: design goals, invariants, phased roadmap, test criteria.
- `docs/architecture.md`: architecture diagrams and sequence flows.
- `Dockerfile`: multi-stage builds for control plane, runner, CLI.
- `docker-compose.yml`: full local stack.

### Executables (`cmd/`)

- `cmd/minitowerd/main.go`: service bootstrap and lifecycle.
- `cmd/minitower-cli/main.go`: deploy workflow from `Towerfile`.
- `cmd/minitower-runner/main.go`: runner runtime, polling, workspace prep, execution, heartbeat/log/result loops.

### Core infrastructure (`internal/config`, `internal/db`, `internal/migrate`, `internal/migrations`, `internal/objects`)

- `internal/config/config.go`: env parsing and defaults.
- `internal/db/db.go`: SQLite open and PRAGMAs.
- `internal/migrate/migrate.go`: migration engine.
- `internal/migrations/*.sql`: schema and incremental changes.
- `internal/objects/local.go`: local artifact object store.

### HTTP API layer (`internal/httpapi`)

- `internal/httpapi/server.go`: route wiring and middleware chain.
- `internal/httpapi/auth.go`: auth middleware and context injection.
- `internal/httpapi/middleware.go`: CORS, recovery, body limits.
- `internal/httpapi/metrics.go`: metric definitions + middleware.
- `internal/httpapi/domain_collector.go`: DB-backed gauge collector.
- `internal/httpapi/handlers/*.go`: endpoint-specific request handling and mapping to store operations.

### Persistence and domain logic (`internal/store`)

- `internal/store/store.go`: store wrapper.
- `internal/store/teams.go`: team + token persistence.
- `internal/store/apps.go`: app persistence.
- `internal/store/environments.go`: default environment provisioning.
- `internal/store/versions.go`: immutable app versions and metadata JSON marshalling.
- `internal/store/runs.go`: run lifecycle reads/writes, list/summary/log access, cancel path.
- `internal/store/runners.go`: runner registration, lease claims, attempt transitions, lease extension, log append, result completion.
- `internal/store/reaper.go`: expired attempt processing and retry/dead/cancel convergence.

### Towerfile and validation (`internal/towerfile`, `internal/validate`, `internal/auth`)

- `internal/towerfile/towerfile.go`: parse/validate Towerfile and parameter schema derivation.
- `internal/towerfile/resolve.go`: secure source glob resolution.
- `internal/towerfile/package.go`: deterministic artifact packing and SHA calculation.
- `internal/validate/schema.go`: JSON Schema subset validation.
- `internal/validate/slug.go`: slug constraints and reserved names.
- `internal/auth/tokens.go`: token generation and hashing.

### Frontend (`frontend/src`)

- `frontend/src/main.ts`: app bootstrap, auth rehydrate, unauthorized redirect.
- `frontend/src/router/index.ts`: public/private/admin route policy.
- `frontend/src/stores/auth.ts`: auth/session state.
- `frontend/src/api/client.ts`: HTTP client and typed endpoint wrappers.
- `frontend/src/api/types.ts`: API contracts.
- `frontend/src/pages/*.vue`: action surfaces for login, dashboard, apps, runs, run details, tokens, admin runners.
- `frontend/src/components/apps/*.vue`: create app/run modals with validation and optimistic behavior.
- `frontend/src/composables/useTheme.ts`: persisted theme toggling.

### Scripts and demos

- `scripts/smoke.sh`: smoke test from boot to run completion.
- `scripts/demo.sh`: manual local demo flow.
- `scripts/demo-compose.sh`: compose-friendly demo loop.
- `scripts/curl-examples.md`: manual API recipes.

## 11) Data model reference

Primary entities and relationships:
- Team -> Tokens, Environments, Apps.
- App -> Versions, Runs.
- Run -> Attempts -> Logs.
- Runner -> Attempts.

Key SQL-level controls:
- Run and attempt status checks via `CHECK` constraints.
- Active attempt uniqueness via partial unique indexes.
- Queue pick index for fast deterministic leasing.
- Composite uniqueness for scoped foreign keys.

Migration summary:
- `0001_init.up.sql`: base schema and indexes.
- `0002_team_password.up.sql`: team password hash support.
- `0003_token_role.up.sql`: team token role model (`admin|member`).
- `0004_towerfile.up.sql`: persisted Towerfile and import-path metadata.

## 12) Test map (what behavior is protected)

Backend:
- `internal/store/store_test.go`: lease concurrency, idempotency, cancellation semantics, runner status updates.
- `internal/store/reaper_test.go`: expiry -> retried/dead/cancelled outcomes and late-result safety.
- `internal/store/runs_team_test.go`: team run listing/summary/log windows and role persistence.
- `internal/httpapi/httpapi_test.go`: token scoping, stale lease handling, endpoint race behavior.

Runner integration:
- `cmd/minitower-runner/runner_integration_test.go`: stale-lease self-fencing, result handling on gone, checksum mismatch failure, streaming behavior.

Frontend:
- `frontend/src/api/client.test.ts`: error mapping + 401 token clearing.
- `frontend/src/stores/auth.test.ts`: login/rehydrate rollback and hydration behavior.
- `frontend/src/composables/useTheme.test.ts`: persistence behavior.
- `frontend/src/components/shared/StatusBadge.test.ts`: status rendering behavior.

## 13) Interview prompts to prepare

Design and tradeoffs:
- How would you move from SQLite single-writer to a replicated control plane?
- How would you shard queue selection across many teams/environments?
- How would you preserve attempt monotonicity across multi-region writes?

Correctness:
- Why lease token + runner token together?
- How does the system prevent double execution under concurrent lease attempts?
- What happens when cancel and result race?
- What are idempotent paths vs conflict paths?

Operational readiness:
- What SLOs would you set for queue latency and run success?
- Which metrics would be alerting signals vs dashboard-only?
- How to test chaos scenarios (network partitions, clock skew, process crashes)?

Security:
- How to evolve from this trust model to untrusted workload sandboxing?
- How to implement token revocation, rotation, and scoped permissions at scale?

Performance:
- How to optimize artifact distribution and dependency installation latency?
- How to support higher runner concurrency safely per node?

## 14) Current limitations and productionization backlog

Current limitations in this project:
- Single-node control plane and SQLite limit horizontal write scaling.
- Local filesystem object store (no durable replicated blob backend).
- No hardened sandbox isolation for untrusted workloads.
- Limited token lifecycle UX/API (no revoke/list in UI flow).
- Minimal scheduling policy (single default environment + priority/queued ordering).

Natural next steps:
- External object storage (S3-compatible) and checksum-based caching.
- Stronger auth model (token revocation, expiry, finer scopes).
- Multi-queue partitioning and scheduler architecture for high throughput.
- Worker sandboxing via containers or microVMs.
- Event-driven status streams for UI instead of interval polling.

## 15) Full non-test Go function index

The section below is generated from:

```bash
rg -n "^func " cmd internal -g '!**/*_test.go'
```

It is included so you can quickly locate every production function for interview prep.

```text
cmd/minitower-runner/main.go:58:func newRunState(leaseExpiry time.Time) *runState {
cmd/minitower-runner/main.go:62:func (s *runState) setLeaseExpiry(t time.Time) {
cmd/minitower-runner/main.go:68:func (s *runState) markCancel() {
cmd/minitower-runner/main.go:74:func (s *runState) markStale() {
cmd/minitower-runner/main.go:80:func (s *runState) markTimedOut() {
cmd/minitower-runner/main.go:86:func (s *runState) snapshot() (leaseExpiry time.Time, cancelRequested, staleLease, timedOut bool) {
cmd/minitower-runner/main.go:92:func loadConfig() (*Config, error) {
cmd/minitower-runner/main.go:151:func NewRunner(cfg *Config, logger *slog.Logger) *Runner {
cmd/minitower-runner/main.go:160:func (r *Runner) Run(ctx context.Context) error {
cmd/minitower-runner/main.go:209:func (r *Runner) register(ctx context.Context) error {
cmd/minitower-runner/main.go:259:func (r *Runner) poll(ctx context.Context) error {
cmd/minitower-runner/main.go:312:func (r *Runner) prepareWorkspace(ctx context.Context, lease *LeaseResponse) (*workspaceResult, error) {
cmd/minitower-runner/main.go:378:func (r *Runner) runHeartbeat(runCtx context.Context, lease *LeaseResponse, state *runState, terminate func(string)) {
cmd/minitower-runner/main.go:429:func (r *Runner) runProcess(ctx context.Context, runCtx context.Context, cancel context.CancelFunc, lease *LeaseResponse, state *runState, ws *workspaceResult, heartbeatDone <-chan struct{}, baseTerminate func(string)) error {
cmd/minitower-runner/main.go:556:func (r *Runner) executeRun(ctx context.Context, lease *LeaseResponse) error {
cmd/minitower-runner/main.go:626:func (r *Runner) startRun(ctx context.Context, lease *LeaseResponse) (*AttemptResponse, error) {
cmd/minitower-runner/main.go:655:func (r *Runner) heartbeat(ctx context.Context, lease *LeaseResponse) (*AttemptResponse, error) {
cmd/minitower-runner/main.go:690:func (r *Runner) downloadArtifact(ctx context.Context, lease *LeaseResponse, destPath string) (*downloadResult, error) {
cmd/minitower-runner/main.go:740:func (r *Runner) unpackArtifact(artifactPath, destDir string) error {
cmd/minitower-runner/main.go:745:func (r *Runner) createVenv(ctx context.Context, venvPath string) error {
cmd/minitower-runner/main.go:750:func (r *Runner) installRequirements(ctx context.Context, venvPath, reqPath string) error {
cmd/minitower-runner/main.go:776:func newLogCollector(r *Runner, lease *LeaseResponse, state *runState, terminate func(string)) *logCollector {
cmd/minitower-runner/main.go:786:func (lc *logCollector) collect(ctx context.Context, reader io.Reader, stream string) {
cmd/minitower-runner/main.go:818:func (lc *logCollector) periodicFlush(ctx context.Context) {
cmd/minitower-runner/main.go:832:func (lc *logCollector) flush(ctx context.Context) {
cmd/minitower-runner/main.go:853:func (lc *logCollector) flushRemaining() {
cmd/minitower-runner/main.go:876:func (r *Runner) flushLogs(ctx context.Context, lease *LeaseResponse, logs []logEntry) error {
cmd/minitower-runner/main.go:901:func (r *Runner) buildProcessEnv(base []string, input map[string]any) []string {
cmd/minitower-runner/main.go:918:func setEnvVar(env []string, key, value string) []string {
cmd/minitower-runner/main.go:933:func unsetEnvVar(env []string, key string) []string {
cmd/minitower-runner/main.go:948:func validEnvKey(key string) bool {
cmd/minitower-runner/main.go:955:func inputValueToEnvString(value any) string {
cmd/minitower-runner/main.go:970:func (r *Runner) submitResult(ctx context.Context, lease *LeaseResponse, status string, exitCode *int, errorMessage *string) error {
cmd/minitower-runner/main.go:1008:func (r *Runner) submitResultSafe(ctx context.Context, lease *LeaseResponse, status string, exitCode *int, errorMessage *string) error {
cmd/minitower-runner/main.go:1019:func (r *Runner) submitFailure(ctx context.Context, lease *LeaseResponse, errMsg string) error {
cmd/minitower-runner/main.go:1024:func (r *Runner) submitFinalResult(ctx context.Context, lease *LeaseResponse, state *runState, waitErr error) error {
cmd/minitower-runner/main.go:1055:func isStaleLeaseStatus(status int) bool {
cmd/minitower-runner/main.go:1059:func ptr(s string) *string {
cmd/minitower-runner/main.go:1063:func main() {
internal/towerfile/package.go:18:func Package(dir string, tf *Towerfile) (io.Reader, string, error) {
internal/towerfile/towerfile.go:61:func Parse(r io.Reader) (*Towerfile, error) {
internal/towerfile/towerfile.go:70:func Validate(tf *Towerfile) error {
internal/towerfile/towerfile.go:135:func checkDefaultType(val any, typ string, idx int) error {
internal/towerfile/towerfile.go:161:func containsTraversal(p string) bool {
internal/towerfile/towerfile.go:176:func ParamsSchemaFromParameters(params []Parameter) map[string]any {
cmd/minitower-cli/main.go:17:func main() {
cmd/minitower-cli/main.go:41:func parseFlags(args []string) (*config, error) {
cmd/minitower-cli/main.go:83:func deploy(cfg *config) error {
cmd/minitower-cli/main.go:132:func ensureApp(client *http.Client, server, token, slug string) error {
cmd/minitower-cli/main.go:184:func uploadVersion(client *http.Client, server, token, slug string, artifactData []byte) (*versionResponse, error) {
internal/towerfile/resolve.go:16:func ResolveSource(dir string, patterns []string) ([]string, error) {
internal/towerfile/resolve.go:89:func patternTargetsDotfiles(pattern string) bool {
internal/towerfile/resolve.go:105:func hasDotSegment(rel string) bool {
internal/migrate/migrate.go:20:func New(migrations fs.FS) *Migrator {
internal/migrate/migrate.go:24:func (m *Migrator) Apply(ctx context.Context, db *sql.DB) error {
internal/migrate/migrate.go:68:func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
internal/migrate/migrate.go:81:func isApplied(ctx context.Context, db *sql.DB, version int64) (bool, error) {
internal/migrate/migrate.go:94:func applyOne(ctx context.Context, db *sql.DB, version int64, sqlText string) error {
internal/migrate/migrate.go:123:func parseVersion(path string) (int64, error) {
internal/testutil/testutil.go:17:func NewTestDB(t *testing.T) (*store.Store, *sql.DB, *dbCleanup) {
internal/testutil/testutil.go:41:func (c *dbCleanup) Close(t *testing.T) {
internal/testutil/testutil.go:51:func CreateTeam(t *testing.T, s *store.Store, slug string) (*store.Team, string) {
internal/testutil/testutil.go:55:func CreateTeamWithRole(t *testing.T, s *store.Store, slug, role string) (*store.Team, string) {
internal/testutil/testutil.go:76:func CreateRunner(t *testing.T, s *store.Store, name, environment string) (*store.Runner, string) {
internal/testutil/testutil.go:93:func CreateApp(t *testing.T, s *store.Store, teamID int64, slug string) *store.App {
internal/testutil/testutil.go:104:func CreateVersion(t *testing.T, s *store.Store, appID int64) *store.AppVersion {
internal/testutil/testutil.go:115:func CreateRun(t *testing.T, s *store.Store, teamID, appID, envID, versionID int64, priority int, maxRetries int) *store.Run {
internal/testutil/testutil.go:126:func LeaseRun(t *testing.T, s *store.Store, runner *store.Runner) (*store.Run, *store.RunAttempt, string, string) {
cmd/minitowerd/main.go:24:func main() {
internal/objects/local.go:16:func NewLocalStore(dir string) (*LocalStore, error) {
internal/objects/local.go:24:func (s *LocalStore) Store(key string, r io.Reader) error {
internal/objects/local.go:45:func (s *LocalStore) Load(key string) (io.ReadCloser, error) {
internal/objects/local.go:55:func (s *LocalStore) Delete(key string) error {
internal/objects/local.go:64:func (s *LocalStore) Exists(key string) (bool, error) {
internal/auth/tokens.go:19:func GenerateToken() (string, string, error) {
internal/auth/tokens.go:30:func GeneratePrefixedToken(prefix string) (string, string, error) {
internal/auth/tokens.go:41:func HashToken(token string) string {
internal/store/apps.go:23:func (s *Store) CreateApp(ctx context.Context, teamID int64, slug string, description *string) (*App, error) {
internal/store/apps.go:52:func (s *Store) GetAppBySlug(ctx context.Context, teamID int64, slug string) (*App, error) {
internal/store/apps.go:74:func (s *Store) GetAppByID(ctx context.Context, teamID int64, appID int64) (*App, error) {
internal/store/apps.go:97:func (s *Store) GetAppByIDDirect(ctx context.Context, appID int64) (*App, error) {
internal/store/apps.go:119:func (s *Store) ListApps(ctx context.Context, teamID int64) ([]*App, error) {
internal/store/apps.go:147:func (s *Store) AppExistsBySlug(ctx context.Context, teamID int64, slug string) (bool, error) {
internal/config/config.go:38:func Load() (Config, error) {
internal/httpapi/context.go:12:func withTeamID(ctx context.Context, teamID int64) context.Context {
internal/httpapi/context.go:16:func teamIDFromContext(ctx context.Context) (int64, bool) {
internal/httpapi/context.go:22:func withTeamTokenID(ctx context.Context, tokenID int64) context.Context {
internal/httpapi/context.go:26:func teamTokenIDFromContext(ctx context.Context) (int64, bool) {
internal/store/teams.go:32:func (s *Store) CreateTeam(ctx context.Context, slug, name string) (*Team, error) {
internal/store/teams.go:59:func (s *Store) TeamExists(ctx context.Context) (bool, error) {
internal/store/teams.go:72:func (s *Store) TeamExistsBySlug(ctx context.Context, slug string) (bool, error) {
internal/store/teams.go:85:func (s *Store) GetTeamByID(ctx context.Context, id int64) (*Team, error) {
internal/store/teams.go:105:func (s *Store) GetTeamBySlug(ctx context.Context, slug string) (*Team, error) {
internal/store/teams.go:125:func (s *Store) SetTeamPassword(ctx context.Context, teamID int64, passwordHash string) error {
internal/store/teams.go:135:func (s *Store) CreateTeamToken(ctx context.Context, teamID int64, tokenHash string, name *string, role string) (*TeamToken, error) {
internal/httpapi/response.go:9:func writeJSON(w http.ResponseWriter, status int, payload any) {
internal/httpapi/response.go:13:func writeError(w http.ResponseWriter, status int, code, message string) {
internal/store/environments.go:20:func (s *Store) GetOrCreateDefaultEnvironment(ctx context.Context, teamID int64) (*Environment, error) {
internal/store/environments.go:52:func (s *Store) GetEnvironmentByID(ctx context.Context, teamID int64, envID int64) (*Environment, error) {
internal/httpapi/middleware.go:13:func BodyLimitMiddleware(maxBytes int64) Middleware {
internal/httpapi/middleware.go:25:func ArtifactBodyLimitMiddleware(maxArtifactBytes, maxDefaultBytes int64) Middleware {
internal/httpapi/middleware.go:44:func CORSMiddleware(allowedOrigins []string) Middleware {
internal/httpapi/middleware.go:85:func Chain(handler http.Handler, middleware ...Middleware) http.Handler {
internal/httpapi/middleware.go:93:func Recoverer(logger *slog.Logger) Middleware {
internal/httpapi/middleware.go:108:func addVaryHeader(header http.Header, value string) {
internal/store/versions.go:26:func (s *Store) CreateVersion(ctx context.Context, appID int64, artifactKey, artifactSHA256, entrypoint string, timeoutSeconds *int, paramsSchema map[string]any, towerfileTOML *string, importPaths []string) (*AppVersion, error) {
internal/store/versions.go:92:func scanVersion(scanner interface{ Scan(...any) error }) (*AppVersion, error) {
internal/store/versions.go:120:func (s *Store) GetLatestVersion(ctx context.Context, appID int64) (*AppVersion, error) {
internal/store/versions.go:133:func (s *Store) GetVersionByNumber(ctx context.Context, appID int64, versionNo int64) (*AppVersion, error) {
internal/store/versions.go:146:func (s *Store) GetVersionByID(ctx context.Context, versionID int64) (*AppVersion, error) {
internal/store/versions.go:159:func (s *Store) ListVersions(ctx context.Context, appID int64) ([]*AppVersion, error) {
internal/store/runners.go:48:func (s *Store) CreateRunner(ctx context.Context, name, environment, tokenHash string) (*Runner, error) {
internal/store/runners.go:78:func (s *Store) RefreshRunnerRegistration(ctx context.Context, runnerID int64, environment, tokenHash string) error {
internal/store/runners.go:90:func (s *Store) GetRunnerByTokenHash(ctx context.Context, tokenHash string) (*Runner, error) {
internal/store/runners.go:115:func (s *Store) GetRunnerByName(ctx context.Context, name string) (*Runner, error) {
internal/store/runners.go:140:func (s *Store) ListRunners(ctx context.Context) ([]*Runner, error) {
internal/store/runners.go:172:func (s *Store) UpdateRunnerLastSeen(ctx context.Context, runnerID int64) error {
internal/store/runners.go:183:func (s *Store) LeaseRun(ctx context.Context, runner *Runner, leaseTokenHash string, leaseTTL time.Duration) (*Run, *RunAttempt, error) {
internal/store/runners.go:304:func scanAttempt(row *sql.Row) (*RunAttempt, error) {
internal/store/runners.go:327:func (s *Store) GetActiveAttempt(ctx context.Context, runID int64, leaseTokenHash string) (*RunAttempt, error) {
internal/store/runners.go:341:func (s *Store) StartAttempt(ctx context.Context, attemptID int64, leaseTokenHash string) (*RunAttempt, error) {
internal/store/runners.go:403:func (s *Store) ExtendLease(ctx context.Context, attemptID int64, leaseTokenHash string, leaseTTL time.Duration) (*RunAttempt, error) {
internal/store/runners.go:443:func (s *Store) AppendLogs(ctx context.Context, attemptID int64, logs []LogEntry) error {
internal/store/runners.go:479:func (s *Store) CompleteAttempt(ctx context.Context, attemptID int64, leaseTokenHash string, status string, exitCode *int, errorMessage *string) error {
internal/store/runners.go:560:func (s *Store) GetRunWithCancelStatus(ctx context.Context, runID int64) (cancelRequested bool, err error) {
internal/store/runners.go:574:func (s *Store) MarkStaleRunnersOffline(ctx context.Context, threshold time.Time) (int, error) {
internal/store/runners.go:594:func (s *Store) MarkRunnerOnline(ctx context.Context, runnerID int64) error {
internal/store/runners.go:605:func (s *Store) GetRunnerByID(ctx context.Context, runnerID int64) (*Runner, error) {
internal/httpapi/domain_collector.go:21:func NewDomainCollector(db *sql.DB) *DomainCollector {
internal/httpapi/domain_collector.go:39:func (c *DomainCollector) Describe(ch chan<- *prometheus.Desc) {
internal/httpapi/domain_collector.go:44:func (c *DomainCollector) Collect(ch chan<- prometheus.Metric) {
internal/httpapi/domain_collector.go:52:func (c *DomainCollector) collectRunsPending(ctx context.Context, ch chan<- prometheus.Metric) {
internal/httpapi/domain_collector.go:76:func (c *DomainCollector) collectRunnersOnline(ctx context.Context, ch chan<- prometheus.Metric) {
internal/store/store.go:11:func New(db *sql.DB) *Store {
internal/store/runs.go:50:func (s *Store) CreateRun(ctx context.Context, teamID, appID, envID, versionID int64, input map[string]any, priority, maxRetries int) (*Run, error) {
internal/store/runs.go:123:func (s *Store) GetRunByID(ctx context.Context, teamID, runID int64) (*Run, error) {
internal/store/runs.go:163:func (s *Store) GetRunByIDDirect(ctx context.Context, runID int64) (*Run, error) {
internal/store/runs.go:202:func (s *Store) GetRunByAppAndRunNo(ctx context.Context, teamID, appID, runNo int64) (*Run, error) {
internal/store/runs.go:241:func (s *Store) ListRunsByApp(ctx context.Context, teamID, appID int64, limit, offset int) ([]*Run, error) {
internal/store/runs.go:293:func (s *Store) ListRunsByTeam(ctx context.Context, teamID int64, limit, offset int, statusFilter, appFilter string) ([]*Run, error) {
internal/store/runs.go:390:func (s *Store) GetRunSummaryByTeam(ctx context.Context, teamID int64) (*RunSummary, error) {
internal/store/runs.go:410:func (s *Store) GetRunLogs(ctx context.Context, runID int64, afterSeq int64) ([]*RunLog, error) {
internal/store/runs.go:440:func (s *Store) CancelRun(ctx context.Context, teamID, runID int64) (*Run, error) {
internal/httpapi/server.go:33:func WithPrometheusRegisterer(reg prometheus.Registerer) ServerOption {
internal/httpapi/server.go:39:func New(cfg config.Config, db *sql.DB, objects *objects.LocalStore, logger *slog.Logger, opts ...ServerOption) *Server {
internal/httpapi/server.go:78:func (s *Server) Handler() http.Handler {
internal/httpapi/server.go:83:func (s *Server) Metrics() *Metrics {
internal/httpapi/server.go:87:func (s *Server) routes() {
internal/httpapi/server.go:120:func (s *Server) routeApps(w http.ResponseWriter, r *http.Request) {
internal/httpapi/server.go:131:func (s *Server) routeAppsWithSlug(w http.ResponseWriter, r *http.Request) {
internal/httpapi/server.go:171:func runPathSegments(path string) []string {
internal/httpapi/server.go:183:func (s *Server) routeRunsMixed(w http.ResponseWriter, r *http.Request) {
internal/httpapi/server.go:244:func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
internal/httpapi/server.go:253:func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
internal/store/reaper.go:20:func (s *Store) ReapExpiredAttempts(ctx context.Context, now time.Time, limit int) ([]ReapResult, error) {
internal/store/reaper.go:66:func (s *Store) reapAttempt(ctx context.Context, attemptID int64, nowMs int64) (*ReapResult, error) {
internal/store/reaper.go:194:func updateAttemptStatus(tx *sql.Tx, attemptID int64, nowMs int64, status string) (bool, error) {
internal/store/reaper.go:210:func maybeCancelRun(ctx context.Context, tx *sql.Tx, runID int64, nowMs int64) error {
internal/httpapi/handlers/bootstrap.go:26:func (h *Handlers) BootstrapTeam(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/me.go:13:func (h *Handlers) GetMe(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/admin.go:21:func (h *Handlers) ListRunners(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/context.go:16:func WithTeamID(ctx context.Context, teamID int64) context.Context {
internal/httpapi/handlers/context.go:20:func teamIDFromContext(ctx context.Context) (int64, bool) {
internal/httpapi/handlers/context.go:26:func WithTeamSlug(ctx context.Context, slug string) context.Context {
internal/httpapi/handlers/context.go:30:func teamSlugFromContext(ctx context.Context) (string, bool) {
internal/httpapi/handlers/context.go:36:func WithTeamTokenID(ctx context.Context, tokenID int64) context.Context {
internal/httpapi/handlers/context.go:40:func teamTokenIDFromContext(ctx context.Context) (int64, bool) {
internal/httpapi/handlers/context.go:46:func WithTokenRole(ctx context.Context, role string) context.Context {
internal/httpapi/handlers/context.go:50:func TokenRoleFromContext(ctx context.Context) (string, bool) {
internal/httpapi/handlers/context.go:56:func WithRunnerID(ctx context.Context, runnerID int64) context.Context {
internal/httpapi/handlers/context.go:60:func runnerIDFromContext(ctx context.Context) (int64, bool) {
internal/httpapi/handlers/context.go:66:func WithEnvironment(ctx context.Context, env string) context.Context {
internal/httpapi/handlers/context.go:70:func environmentFromContext(ctx context.Context) (string, bool) {
internal/db/db.go:14:func Open(ctx context.Context, path string) (*sql.DB, error) {
internal/db/db.go:35:func applyPragmas(ctx context.Context, db *sql.DB) error {
internal/httpapi/handlers/apps.go:30:func (h *Handlers) CreateApp(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/apps.go:83:func (h *Handlers) ListApps(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/apps.go:118:func (h *Handlers) GetApp(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/apps.go:159:func extractPathParam(path, prefix string) string {
internal/validate/schema.go:20:func ValidateJSONSchema(schema map[string]any) error {
internal/validate/schema.go:28:func ValidateJSONInput(input any, schema map[string]any) error {
internal/validate/schema.go:38:func validateSchemaNode(schema map[string]any, path string) error {
internal/validate/schema.go:141:func validateValue(value any, schema map[string]any, path string) error {
internal/validate/schema.go:194:func validateObject(value map[string]any, schema map[string]any, path string) error {
internal/validate/schema.go:251:func validateArray(value []any, schema map[string]any, path string) error {
internal/validate/schema.go:272:func validateString(value string, schema map[string]any, path string) error {
internal/validate/schema.go:286:func validateNumber(value float64, schema map[string]any, path string) error {
internal/validate/schema.go:300:func parseTypeList(v any) ([]string, error) {
internal/validate/schema.go:319:func valueMatchesTypes(value any, types []string) bool {
internal/validate/schema.go:328:func matchesType(value any, typ string) bool {
internal/validate/schema.go:355:func schemaAllowsType(schema map[string]any, typ string) bool {
internal/validate/schema.go:371:func schemaWantsObject(schema map[string]any) bool {
internal/validate/schema.go:387:func asNumber(v any) (float64, bool) {
internal/validate/schema.go:402:func asInt(v any) (int, bool) {
internal/httpapi/handlers/runs.go:63:func (h *Handlers) CreateRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:182:func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:261:func (h *Handlers) ListRunsByTeam(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:331:func (h *Handlers) GetRunsSummary(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:359:func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:433:func (h *Handlers) CancelRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:511:func (h *Handlers) GetRunLogs(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runs.go:572:func extractAppSlugFromRunPath(path string) string {
internal/httpapi/handlers/runs.go:586:func extractRunIDFromPath(path string) int64 {
internal/httpapi/handlers/runs.go:604:func extractRunIDFromLogsPath(path string) int64 {
internal/httpapi/handlers/runs.go:621:func isValidRunStatus(status string) bool {
internal/validate/slug.go:27:func ValidateSlug(s string) error {
internal/httpapi/auth.go:20:func NewAuth(cfg config.Config, db *sql.DB) *Auth {
internal/httpapi/auth.go:27:func (a *Auth) RequireBootstrap(next http.Handler) http.Handler {
internal/httpapi/auth.go:39:func (a *Auth) RequireTeam(next http.Handler) http.Handler {
internal/httpapi/auth.go:79:func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
internal/httpapi/auth.go:91:func (a *Auth) RequireRunnerRegistration(next http.Handler) http.Handler {
internal/httpapi/auth.go:103:func (a *Auth) RequireRunner(next http.Handler) http.Handler {
internal/httpapi/auth.go:135:func parseBearerToken(r *http.Request) (string, bool) {
internal/httpapi/auth.go:160:func secureEqual(a, b string) bool {
internal/httpapi/handlers/tokens.go:24:func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
internal/httputil/response.go:20:func WriteJSON(w http.ResponseWriter, status int, payload any) {
internal/httputil/response.go:27:func WriteError(w http.ResponseWriter, status int, code, message string) {
internal/httpapi/handlers/runner.go:19:func (h *Handlers) requireLeaseContext(w http.ResponseWriter, r *http.Request, extractID func(string) int64) (runID int64, attempt *store.RunAttempt, leaseTokenHash string, ok bool) {
internal/httpapi/handlers/runner.go:42:func (h *Handlers) writeAttemptResponse(w http.ResponseWriter, r *http.Request, runID int64, attempt *store.RunAttempt) {
internal/httpapi/handlers/runner.go:72:func (h *Handlers) RegisterRunner(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:156:func (h *Handlers) LeaseRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:242:func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:262:func (h *Handlers) HeartbeatRun(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:293:func (h *Handlers) SubmitLogs(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:359:func (h *Handlers) SubmitResult(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:410:func (h *Handlers) GetArtifact(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/runner.go:461:func extractRunIDFromArtifactPath(path string) int64 {
internal/httpapi/handlers/versions.go:40:func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/versions.go:161:func extractTowerfileFromArchive(data []byte) (string, error) {
internal/httpapi/handlers/versions.go:199:func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/versions.go:254:func extractAppSlugFromVersionPath(path string) string {
internal/httpapi/metrics.go:46:func NewMetrics(reg prometheus.Registerer, db *sql.DB) *Metrics {
internal/httpapi/metrics.go:170:func (m *Metrics) Handler() http.Handler {
internal/httpapi/metrics.go:176:func (m *Metrics) RunCreated(team, app string) {
internal/httpapi/metrics.go:180:func (m *Metrics) RunCompleted(team, app, status string) {
internal/httpapi/metrics.go:184:func (m *Metrics) RunRetried(team, app string) {
internal/httpapi/metrics.go:188:func (m *Metrics) RunLeased(environment string) {
internal/httpapi/metrics.go:192:func (m *Metrics) RunnerRegistered(environment string) {
internal/httpapi/metrics.go:196:func (m *Metrics) ObserveQueueWait(team, app string, seconds float64) {
internal/httpapi/metrics.go:200:func (m *Metrics) ObserveExecution(team, app, status string, seconds float64) {
internal/httpapi/metrics.go:204:func (m *Metrics) ObserveTotal(team, app, status string, seconds float64) {
internal/httpapi/metrics.go:209:func (m *Metrics) Middleware() Middleware {
internal/httpapi/metrics.go:248:func (rw *responseWriter) WriteHeader(code int) {
internal/httpapi/metrics.go:253:func (rw *responseWriter) Write(b []byte) (int, error) {
internal/httpapi/metrics.go:263:func normalizePath(path string) string {
internal/httpapi/metrics.go:299:func isSlugOrID(segment string) bool {
internal/httpapi/handlers/login.go:24:func (h *Handlers) LoginTeam(w http.ResponseWriter, r *http.Request) {
internal/httpapi/handlers/handlers.go:31:func (NoOpMetrics) RunCreated(string, string)                          {}
internal/httpapi/handlers/handlers.go:32:func (NoOpMetrics) RunCompleted(string, string, string)                {}
internal/httpapi/handlers/handlers.go:33:func (NoOpMetrics) RunRetried(string, string)                          {}
internal/httpapi/handlers/handlers.go:34:func (NoOpMetrics) RunLeased(string)                                   {}
internal/httpapi/handlers/handlers.go:35:func (NoOpMetrics) RunnerRegistered(string)                            {}
internal/httpapi/handlers/handlers.go:36:func (NoOpMetrics) ObserveQueueWait(string, string, float64)           {}
internal/httpapi/handlers/handlers.go:37:func (NoOpMetrics) ObserveExecution(string, string, string, float64)   {}
internal/httpapi/handlers/handlers.go:38:func (NoOpMetrics) ObserveTotal(string, string, string, float64)       {}
internal/httpapi/handlers/handlers.go:55:func newStore(db *sql.DB) *Store {
internal/httpapi/handlers/handlers.go:60:func New(cfg config.Config, db *sql.DB, objects *objects.LocalStore, logger *slog.Logger, metrics DomainMetrics) *Handlers {
internal/httpapi/handlers/handlers.go:74:func writeJSON(w http.ResponseWriter, status int, payload any) {
internal/httpapi/handlers/handlers.go:78:func writeError(w http.ResponseWriter, status int, code, message string) {
internal/httpapi/handlers/handlers.go:82:func decodeJSON(r *http.Request, v any) error {
internal/httpapi/handlers/handlers.go:88:func writeStoreError(w http.ResponseWriter, logger *slog.Logger, err error, logMsg string) bool {
```

## 16) Quick file discovery commands

Use these when drilling into specific areas during prep:

```bash
# Full source file list
find cmd internal frontend/src scripts -maxdepth 3 -type f | sort

# All production Go functions
rg -n "^func " cmd internal -g '!**/*_test.go'

# API routes and route dispatch
rg -n "/api/v1|route" internal/httpapi/server.go internal/httpapi/handlers

# State transition logic
rg -n "status|Lease|Cancel|Reap|CompleteAttempt|StartAttempt|ExtendLease" internal/store internal/httpapi/handlers
```
