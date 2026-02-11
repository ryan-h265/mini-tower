# MiniTower curl API Examples

This document is the canonical home for raw `curl` usage examples.

## Setup

### 1. Start the Server

```bash
# Build binaries
go build -o bin/minitowerd ./cmd/minitowerd
go build -o bin/minitower-runner ./cmd/minitower-runner

# Run server (terminal 1)
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
./bin/minitowerd
```

### 2. Sign Up Team

```bash
curl -sS -X POST http://localhost:8080/api/v1/teams/signup \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","name":"My Team","password":"secret"}'
```

Save team token:

```bash
export TOKEN="tt_..."
```

### Optional Operator Bootstrap Flow

```bash
MINITOWER_BOOTSTRAP_TOKEN=secret \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
./bin/minitowerd

curl -sS -X POST http://localhost:8080/api/v1/bootstrap/team \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","name":"My Team","password":"secret"}'
```

## Team and Auth

### Login

```bash
curl -sS -X POST http://localhost:8080/api/v1/teams/login \
  -H "Content-Type: application/json" \
  -d '{"slug":"myteam","password":"secret"}'
```

### Me

```bash
curl -sS http://localhost:8080/api/v1/me \
  -H "Authorization: Bearer $TOKEN"
```

## App Management

### Create App

```bash
curl -sS -X POST http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"hello-world","description":"A test app"}'
```

### List Apps

```bash
curl -sS http://localhost:8080/api/v1/apps \
  -H "Authorization: Bearer $TOKEN"
```

### Get App

```bash
curl -sS http://localhost:8080/api/v1/apps/hello-world \
  -H "Authorization: Bearer $TOKEN"
```

## Versions

### Upload Version via curl

The artifact must be a `.tar.gz` containing a `Towerfile` at archive root.

```bash
tar -czf /tmp/myapp/artifact.tar.gz -C /tmp/myapp main.py Towerfile
curl -sS -X POST http://localhost:8080/api/v1/apps/hello-world/versions \
  -H "Authorization: Bearer $TOKEN" \
  -F "artifact=@/tmp/myapp/artifact.tar.gz"
```

### List Versions

```bash
curl -sS http://localhost:8080/api/v1/apps/hello-world/versions \
  -H "Authorization: Bearer $TOKEN"
```

## Runs

### Create Run

```bash
curl -sS -X POST http://localhost:8080/api/v1/apps/hello-world/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"input":{"name":"MiniTower"}}'
```

### Create Run (with options)

```bash
curl -sS -X POST http://localhost:8080/api/v1/apps/hello-world/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "input": {"name": "MiniTower"},
    "version_no": 1,
    "priority": 10,
    "max_retries": 3
  }'
```

### List Runs (App)

```bash
curl -sS "http://localhost:8080/api/v1/apps/hello-world/runs?limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

### List Runs (Team)

```bash
curl -sS "http://localhost:8080/api/v1/runs?limit=20&status=running&app=hello-world" \
  -H "Authorization: Bearer $TOKEN"
```

### Get Run

```bash
curl -sS http://localhost:8080/api/v1/runs/1 \
  -H "Authorization: Bearer $TOKEN"
```

### Cancel Run

```bash
curl -sS -X POST http://localhost:8080/api/v1/runs/1/cancel \
  -H "Authorization: Bearer $TOKEN"
```

### Get Run Logs

```bash
curl -sS "http://localhost:8080/api/v1/runs/1/logs?after_seq=0" \
  -H "Authorization: Bearer $TOKEN"
```

## Tokens

### Create Team Token

```bash
curl -sS -X POST http://localhost:8080/api/v1/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"ci-pipeline","role":"member"}'
```

## Admin

### List Runners (admin token required)

```bash
curl -sS http://localhost:8080/api/v1/admin/runners \
  -H "Authorization: Bearer $TOKEN"
```

## Runner

### Start Runner

```bash
MINITOWER_SERVER_URL=http://localhost:8080 \
MINITOWER_RUNNER_NAME=my-runner \
MINITOWER_RUNNER_REGISTRATION_TOKEN=runner-secret \
MINITOWER_DATA_DIR=~/.minitower \
./bin/minitower-runner
```

## Health and Metrics

```bash
# Liveness
curl -sS http://localhost:8080/health

# Readiness
curl -sS http://localhost:8080/ready

# Metrics
curl -sS http://localhost:8080/metrics | grep minitower_runs
```
