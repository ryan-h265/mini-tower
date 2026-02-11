# MiniTower

A minimal, correctness-first job orchestration system.

MiniTower deploys and runs Python or shell workloads with explicit run state, lease-based execution, and a simple control plane API.

## Requirements

- Go 1.24+
- Python 3 with `venv`
- `tar` available on `PATH`

## Quickstart

### 1. Start the control plane

```bash
export MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret
go run ./cmd/minitowerd
```

### 2. Create a team and get a token

Use the API examples in `docs/curl-examples.md`:

- Team signup: `docs/curl-examples.md#2-sign-up-team`
- Optional bootstrap flow: `docs/curl-examples.md#optional-operator-bootstrap-flow`

Then export the returned team token:

```bash
export TEAM_TOKEN="tt_..."
```

### 3. Configure `minitower-cli`

```bash
go run ./cmd/minitower-cli config set \
  --profile local \
  --server http://localhost:8080 \
  --token "$TEAM_TOKEN"
go run ./cmd/minitower-cli config use local
```

### 4. Create an app project and deploy

```bash
mkdir -p myapp
cat > myapp/main.py << 'PY'
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

go run ./cmd/minitower-cli deploy --dir myapp
```

### 5. Start a runner

```bash
MINITOWER_SERVER_URL=http://localhost:8080 \
MINITOWER_RUNNER_NAME=runner-1 \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
go run ./cmd/minitower-runner
```

### 6. Create and watch a run

```bash
go run ./cmd/minitower-cli runs create --app hello --input '{"name":"MiniTower"}'
go run ./cmd/minitower-cli runs watch --app hello
```

### 7. (Optional) Run the frontend

```bash
npm --prefix frontend install
npm --prefix frontend run dev
```

Open `http://localhost:5173`.

## Documentation

- CLI command reference: `docs/minitower-cli-reference.md`
- API endpoint catalog: `docs/api-endpoints.md`
- API curl examples: `docs/curl-examples.md`
- Configuration reference: `docs/configuration.md`
- Operations (migrations, monitoring, testing, compose): `docs/operations.md`
- Architecture details: `docs/architecture.md`
