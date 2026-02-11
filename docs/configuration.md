# Configuration Reference

## Control Plane (`minitowerd`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `MINITOWER_DB_PATH` | `./minitower.db` | SQLite database path |
| `MINITOWER_OBJECTS_DIR` | `./objects` | Artifact storage directory |
| `MINITOWER_PUBLIC_SIGNUP_ENABLED` | `true` | Enable public team signup |
| `MINITOWER_BOOTSTRAP_TOKEN` | empty | Optional operator bootstrap token |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | empty | Runner registration token (required) |
| `MINITOWER_CORS_ORIGINS` | empty | Comma-separated CORS allowlist |
| `MINITOWER_LEASE_TTL` | `60s` | Runner lease duration |
| `MINITOWER_EXPIRY_CHECK_INTERVAL` | `10s` | Lease expiry check interval |
| `MINITOWER_RUNNER_PRUNE_AFTER` | `24h` | Delete offline runners older than cutoff when they have no run-attempt history (`0` disables pruning) |
| `MINITOWER_MAX_REQUEST_BODY_SIZE` | `10485760` | Max request body bytes (10 MB) |
| `MINITOWER_MAX_ARTIFACT_SIZE` | `104857600` | Max artifact upload bytes (100 MB) |

## Runner (`minitower-runner`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_SERVER_URL` | empty | Control plane URL (required) |
| `MINITOWER_RUNNER_NAME` | empty | Unique runner name (required) |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | empty | Platform registration token |
| `MINITOWER_RUNNER_ENVIRONMENT` | `default` | Environment label for matching runs |
| `MINITOWER_PYTHON_BIN` | `python3` | Python interpreter path |
| `MINITOWER_POLL_INTERVAL` | `3s` | Work poll interval |
| `MINITOWER_KILL_GRACE_PERIOD` | `10s` | SIGTERM to SIGKILL grace period |
| `MINITOWER_DATA_DIR` | `~/.minitower` | Runner data directory |

## Frontend (`frontend`)

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_BASE_URL` | empty | Absolute API URL for separately deployed frontend |
| `VITE_DEV_PROXY_TARGET` | `http://localhost:8080` | Dev proxy target for `/api` |

## CLI (`minitower-cli`)

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_SERVER_URL` | empty | Default control plane URL for CLI commands |
| `MINITOWER_API_TOKEN` | empty | Default API token for CLI commands |
| `MINITOWER_CLI_CONFIG` | empty | Override CLI config file path |
| `XDG_CONFIG_HOME` | system default | Base config directory used when `MINITOWER_CLI_CONFIG` is unset |

Default CLI config path resolution:

1. `MINITOWER_CLI_CONFIG`
2. `$XDG_CONFIG_HOME/minitower-cli/config.json`
3. `~/.config/minitower-cli/config.json`

For command usage and precedence details, see `docs/minitower-cli-reference.md`.
