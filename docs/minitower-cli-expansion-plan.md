# MiniTower CLI Expansion Plan (Tower.dev-aligned)

## Goals and alignment

MiniTower should expand `minitower-cli` beyond deploy to cover the common control-plane APIs while mirroring Tower CLI command/flag conventions wherever relevant. The CLI should be safe for CI automation, provide stable output formats, and include a first-class watch experience for runs (status + logs) with frontend-style log ordering, including setup logs. Tower CLI command and flag naming should be mirrored exactly where applicable; org concepts are omitted when MiniTower lacks them.

## Guiding principles

- **Tower CLI parity for relevant features**: match command names and flags that map to MiniTower capabilities; skip irrelevant Tower features.
- **Stable automation output**: default to human-friendly output but support Tower-style `--json` (or command-specific JSON flags) for CI stability.
- **Zero surprise defaults**: mirror Tower CLI flag naming and argument order; minimize additional flags unless required by MiniTower differences.
- **Observability-first UX**: run watch should be as ergonomic as the frontend (setup logs + runtime logs, deterministic ordering, and exit on completion).
- **CLI-first docs and scripts**: update documentation and scripts to prefer `minitower-cli` for routine operations (signup/login, deploy, runs, logs) over raw curl, except where only direct HTTP is possible.

## Command surface (Tower-aligned mapping)

> The exact verb/noun ordering should mirror Tower CLI. Below is a capability map and initial command list.

### Auth and profile management
- `login` (store profile/token/server context)
- `config` (set/get current profile and defaults)
- `me` (whoami equivalent)

### Apps
- `apps list`
- `apps get <app>`
- `apps create` (if supported by Tower CLI)
- `apps update` (if supported by Tower CLI)
- `apps delete` (if supported by Tower CLI)

### Versions
- `versions list --app <app>`
- `versions get <version> --app <app>` (or app-scoped ID if Tower CLI does it)
- `versions upload --app <app> --file <artifact>` (alias for deploy if Tower offers both)

### Runs
- `runs create --app <app> --input <json>`
- `runs list [--app <app>] [--status <status>] [--limit <n>]`
- `runs get <run>`
- `runs cancel <run>`
- `runs retry <run>` (if supported or can be mapped to create with previous input)
- `runs watch <run>` (see watch design)
- `runs logs <run> [--follow]`

### Tokens
- `tokens list`
- `tokens create [--role <role>]`
- `tokens revoke <token-id>`

### Runners / admin
- `runners list` (admin-only, mirrors Tower if present)

## Watch design (priority improvement)

### Desired behavior
- **Auto-attach to logs** when run starts; include setup logs in the same order as the frontend.
- **Exit on completion by default** with a non-zero exit for failed/cancelled/dead.
- **Log-only vs status-only** support:
  - `runs watch <run>`: status + logs.
  - `runs logs <run> --follow`: logs only.
  - `runs watch <run> --status-only`: status only.

### Implementation approach (polling)
- Poll run status (`GET /api/v1/runs/{run}`) at a fixed cadence (Tower-like default, configurable).
- Poll logs (`GET /api/v1/runs/{run}/logs`) with cursor/offset semantics matching the frontend.
- Ensure logs include the setup preamble (runner boot, download, extraction, venv, etc.) in order.
- Once run reaches a terminal state, drain remaining logs, then exit.

### “Latest run” UX
- If a run ID is omitted, infer latest run only when **`--app <app>`** is provided (e.g., `runs watch --app <app>`). Otherwise error with guidance.
- Implementation uses `GET /api/v1/apps/{app}/runs?limit=1&order=desc` (or closest available filter), then watch the newest run.

## Config and auth

- **Profiles**: store server, token, and default app/team in a config file (Tower-like location and naming), with `config` commands to list and select.
- **Login**: `minitower-cli login` obtains token using team credentials (`POST /api/v1/teams/login`), stores in profile.
- **Token roles**: allow `--role` when creating tokens; only enable if the user has admin role.
- **Profile file creation (clear flow)**:
  1. `minitower-cli login --server <url> --team <slug>` prompts for password and writes/updates the default profile.
  2. `minitower-cli config set --profile <name> --server <url> --token <token> [--app <app>]` for non-interactive CI.
  3. `minitower-cli config use <name>` selects the active profile.

## Output format

- Default: human-friendly tables or structured key-value output, matching Tower CLI.
- `--json`: emit a stable JSON payload (documented in CLI reference).
- Where Tower CLI offers a JSON flag, MiniTower should match name and behavior.

## Error handling and exit codes

- Map HTTP failures to stable exit codes and messages:
  - 401/403 auth errors
  - 404 not found
  - 409 conflict (e.g., run cancel conflicts)
  - 410 gone (expired attempts)
- `runs watch` exit codes: 0 for completed, 1 for failed/dead, 2 for cancelled.

## API coverage checklist

Based on existing control-plane endpoints:
- **Auth**: login, me, tokens.
- **Apps**: create/list/get.
- **Versions**: list/upload.
- **Runs**: create/list/get/cancel/logs/summary.
- **Admin**: runners list.

## Delivery plan and milestones

1. **Foundation**
   - Add shared HTTP client, auth config, and Tower-style output formatting.
   - Implement login + profile handling.
2. **Apps + Versions**
   - Implement list/get/create and version list/upload.
   - Align `deploy` with version upload command semantics.
3. **Runs**
   - Implement create/list/get/cancel/logs.
   - Add `runs watch` with polling + log streaming.
   - Add latest-run inference when `--app` provided.
4. **Tokens + Admin**
   - Implement token list/create/revoke and admin runners list.
5. **Docs & CLI reference**
   - Document commands, flags, and JSON schema outputs.
   - Add examples that mirror Tower CLI usage patterns.
6. **Cleanup**
   - Run a docs + scripts audit to replace curl-based flows with `minitower-cli` equivalents.
   - Normalize example commands to the Tower-style flag naming.

## Open items (decision log)

- Confirm Tower CLI’s exact command names and flags for parity.
- Confirm log ordering semantics from the frontend to match setup log expectations.
