# MiniTower API Examples

## Setup

### 1. Start the Server

```bash
# Build
go build -o bin/minitowerd ./cmd/minitowerd
go build -o bin/minitower-runner ./cmd/minitower-runner

# Run server (in one terminal)
MINITOWER_BOOTSTRAP_TOKEN=secret \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
./bin/minitowerd
```

### 2. Bootstrap Team

```bash
curl -s -X POST http://localhost:8080/api/v1/bootstrap/team \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","name":"My Team"}'
```

Response:
```json
{
  "team_id": 1,
  "slug": "myteam",
  "token": "mtk_..."
}
```

Save the token:
```bash
export TOKEN="mtk_..."           # Team API token
```

## App Management

### Create App

```bash
curl -s -X POST http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"hello-world","description":"A test app"}'
```

### List Apps

```bash
curl -s http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TOKEN"
```

### Get App

```bash
curl -s http://localhost:8080/api/v1/apps/hello-world \
  -H "Authorization: Bearer $TOKEN"
```

## Versions

### Deploy with CLI (Recommended)

Create a project directory with a `Towerfile`:
```bash
mkdir -p /tmp/myapp
cat > /tmp/myapp/main.py << 'EOF'
import os, json
input_data = json.loads(os.environ.get("MINITOWER_INPUT", "{}"))
print(f"Hello, {input_data.get('name', 'World')}!")
EOF

cat > /tmp/myapp/Towerfile << 'EOF'
[app]
name = "hello-world"
script = "main.py"
source = ["./*.py"]

[app.timeout]
seconds = 60

[[parameters]]
name = "name"
description = "Name to greet"
type = "string"
default = "World"
EOF
```

Deploy (auto-creates app if needed, packages artifact, uploads version):
```bash
minitower-cli deploy --server http://localhost:8080 --token $TOKEN --dir /tmp/myapp
```

Or use environment variables:
```bash
export MINITOWER_SERVER_URL=http://localhost:8080
export MINITOWER_API_TOKEN=$TOKEN
minitower-cli deploy --dir /tmp/myapp
```

### Upload Version via curl

The artifact must be a tar.gz containing a `Towerfile` at its root.
All metadata (entrypoint, timeout, params) is extracted from the Towerfile.

```bash
tar -czf /tmp/myapp/artifact.tar.gz -C /tmp/myapp main.py Towerfile
curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/versions \
  -H "Authorization: Bearer $TOKEN" \
  -F "artifact=@/tmp/myapp/artifact.tar.gz"
```

### List Versions

```bash
curl -s http://localhost:8080/api/v1/apps/hello-world/versions \
  -H "Authorization: Bearer $TOKEN"
```

## Runs

### Create Run

```bash
curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"MiniTower"}}'
```

With options:
```bash
curl -s -X POST http://localhost:8080/api/v1/apps/hello-world/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "input": {"name": "MiniTower"},
    "version_no": 1,
    "priority": 10,
    "max_retries": 3
  }'
```

### List Runs

```bash
curl -s "http://localhost:8080/api/v1/apps/hello-world/runs?limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

### Get Run

```bash
curl -s http://localhost:8080/api/v1/runs/1 \
  -H "Authorization: Bearer $TOKEN"
```

### Get Run Logs

```bash
curl -s http://localhost:8080/api/v1/runs/1/logs \
  -H "Authorization: Bearer $TOKEN"
```

## Runner

### Start Runner (in another terminal)

```bash
MINITOWER_SERVER_URL=http://localhost:8080 \
MINITOWER_RUNNER_NAME=my-runner \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
MINITOWER_DATA_DIR=~/.minitower \
./bin/minitower-runner
```

The runner will:
1. Register with the server (first time only)
2. Save its token to `~/.minitower/runner_token`
3. Poll for work
4. Execute runs and report results

### Runner Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MINITOWER_SERVER_URL` | Yes | - | Server URL |
| `MINITOWER_RUNNER_NAME` | Yes | - | Unique runner name |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | First run | - | Platform registration token |
| `MINITOWER_RUNNER_ENVIRONMENT` | No | `default` | Environment label for matching runs |
| `MINITOWER_DATA_DIR` | No | `~/.minitower` | Data directory |
| `MINITOWER_PYTHON_BIN` | No | `python3` | Python binary |
| `MINITOWER_POLL_INTERVAL` | No | `3s` | Poll interval |
| `MINITOWER_KILL_GRACE_PERIOD` | No | `10s` | Grace period before SIGKILL |

## Tokens

### Create Additional API Token

```bash
curl -s -X POST http://localhost:8080/api/v1/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"ci-pipeline"}'
```

## Health Checks

```bash
# Liveness
curl -s http://localhost:8080/health

# Readiness (checks DB)
curl -s http://localhost:8080/ready
```

## Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MINITOWER_LISTEN_ADDR` | `:8080` | Listen address |
| `MINITOWER_DB_PATH` | `./minitower.db` | SQLite database path |
| `MINITOWER_OBJECTS_DIR` | `./objects` | Artifact storage directory |
| `MINITOWER_BOOTSTRAP_TOKEN` | Required | Bootstrap token |
| `MINITOWER_RUNNER_REGISTRATION_TOKEN` | Required | Runner registration token |
| `MINITOWER_LEASE_TTL` | `60s` | Lease duration |
| `MINITOWER_EXPIRY_CHECK_INTERVAL` | `10s` | Expiry check interval |
