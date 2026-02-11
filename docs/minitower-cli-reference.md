# MiniTower CLI Reference

This reference covers the current `minitower-cli` command surface.

## Connection and Profile Model

Most commands accept some combination of:

- `--server <url>`
- `--token <token>`
- `--profile <name>`

Resolution order is:

1. Explicit flags (`--server`, `--token`)
2. Environment (`MINITOWER_SERVER_URL`, `MINITOWER_API_TOKEN`)
3. Active profile from config file

Profile config path:

1. `MINITOWER_CLI_CONFIG` (if set)
2. `$XDG_CONFIG_HOME/minitower-cli/config.json`
3. `~/.config/minitower-cli/config.json`

## Global Help

```bash
minitower-cli --help
```

## `login`

Login with team credentials and store token in a profile.

```bash
minitower-cli login --server http://localhost:8080 --team acme
```

Non-interactive example:

```bash
minitower-cli login \
  --server http://localhost:8080 \
  --team acme \
  --password secret \
  --profile local
```

Flags:

- `--server <url>`
- `--team <slug>`
- `--password <password>`
- `--profile <name>`
- `--json`

## `config`

### `config set`

Set profile fields.

```bash
minitower-cli config set \
  --profile local \
  --server http://localhost:8080 \
  --token "$TEAM_TOKEN" \
  --team acme \
  --app hello
```

Flags:

- `--profile <name>`
- `--server <url>`
- `--token <token>`
- `--team <slug>`
- `--app <slug>`
- `--json`

### `config get`

```bash
minitower-cli config get
minitower-cli config get --profile local --json
```

### `config list`

```bash
minitower-cli config list
minitower-cli config list --json
```

### `config use`

```bash
minitower-cli config use local
```

## `me`

Resolve current identity.

```bash
minitower-cli me
minitower-cli me --json
```

## `apps`

### `apps list`

```bash
minitower-cli apps list
minitower-cli apps list --json
```

### `apps get <app>`

```bash
minitower-cli apps get hello
```

### `apps create <slug>`

```bash
minitower-cli apps create hello --description "Hello world app"
```

Alternative:

```bash
minitower-cli apps create --slug hello --description "Hello world app"
```

## `versions`

### `versions list --app <app>`

```bash
minitower-cli versions list --app hello
```

### `versions get <version-no> --app <app>`

```bash
minitower-cli versions get 3 --app hello
```

### `versions upload --app <app> --file <artifact>`

```bash
minitower-cli versions upload --app hello --file ./artifact.tar.gz
```

## `deploy`

Package project files from `Towerfile` and upload as a new version.

```bash
minitower-cli deploy --dir ./myapp
```

Flags:

- `--dir <path>` (default: `.`)
- `--server <url>`
- `--token <token>`
- `--profile <name>`
- `--json`

## `runs`

### `runs create`

```bash
minitower-cli runs create --app hello --input '{"name":"MiniTower"}'
```

Optional fields:

```bash
minitower-cli runs create \
  --app hello \
  --input '{"name":"MiniTower"}' \
  --version 2 \
  --priority 10 \
  --max-retries 3
```

### `runs list`

```bash
minitower-cli runs list --app hello --status running --limit 20
```

### `runs get <run-id>`

```bash
minitower-cli runs get 42
```

### `runs cancel <run-id>`

```bash
minitower-cli runs cancel 42
```

### `runs retry <run-id>`

Create a new run using input/version/priority/max-retries from an existing run.

```bash
minitower-cli runs retry 42
```

### `runs logs <run-id>`

Fetch logs once:

```bash
minitower-cli runs logs 42
```

Follow mode:

```bash
minitower-cli runs logs 42 --follow
```

Flags:

- `--follow`
- `--interval <duration>` (default: `2s`)
- `--after-seq <n>`
- `--json` (non-follow mode only)

### `runs watch [run-id]`

Status + logs watch:

```bash
minitower-cli runs watch 42
```

Latest run inference (requires app context):

```bash
minitower-cli runs watch --app hello
```

Status-only mode:

```bash
minitower-cli runs watch 42 --status-only
```

Flags:

- `--app <slug>` (used when run id omitted)
- `--status-only`
- `--interval <duration>` (default: `2s`)
- `--json` (allowed only with `--status-only`)

Watch exit codes:

- `0`: run completed
- `1`: run failed/dead
- `2`: run cancelled

## `tokens`

### `tokens create`

```bash
minitower-cli tokens create --name ci --role member
```

Flags:

- `--name <token-name>`
- `--role <admin|member>`
- `--json`

### `tokens list` and `tokens revoke`

These are currently unavailable because matching API endpoints are not implemented yet.

## `runners`

### `runners list`

```bash
minitower-cli runners list
minitower-cli runners list --json
```

Requires an admin token.

## Exit Code Notes

HTTP errors map to stable non-zero exit codes:

- `10`: auth (`401/403`)
- `11`: not found (`404`)
- `12`: conflict (`409`)
- `13`: gone (`410`)
- `1`: all other errors
