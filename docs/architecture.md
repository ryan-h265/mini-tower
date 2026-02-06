# MiniTower Architecture

This document contains architecture diagrams for the MiniTower orchestration system.

## Table of Contents

- [Component Architecture](#component-architecture)
- [Layered Architecture](#layered-architecture)
- [Run Lifecycle Sequence](#run-lifecycle-sequence)
- [Deployment Topology](#deployment-topology)
- [Data Model](#data-model)
- [ASCII Diagram](#ascii-diagram)

---

## Component Architecture

High-level view of system components and their interactions.

```mermaid
graph TB
    subgraph "Clients"
        CLI[CLI / API Client]
    end

    subgraph "Control Plane (minitowerd)"
        API[HTTP API Server]
        Auth[Auth Middleware]
        Handlers[Request Handlers]
        Store[Store Layer]
        Reaper[Expiry Reaper<br/>Goroutine]
        Objects[Object Store<br/>Local FS]
    end

    subgraph "Data Layer"
        SQLite[(SQLite<br/>WAL Mode)]
    end

    subgraph "Runner (minitower-runner)"
        RunnerMain[Runner Agent]
        Executor[Process Executor]
        VEnv[Python venv]
        Workspace[Temp Workspace]
    end

    CLI -->|Team Token| API
    API --> Auth
    Auth --> Handlers
    Handlers --> Store
    Store --> SQLite
    Handlers -->|Artifacts| Objects
    Reaper -->|Lease Expiry| Store

    RunnerMain -->|Runner Token| API
    RunnerMain -->|Lease Token| API
    API -->|Artifact Download| RunnerMain
    RunnerMain --> Executor
    Executor --> VEnv
    Executor --> Workspace
```

---

## Layered Architecture

Code organization showing architectural layers and dependencies.

```mermaid
graph TD
    subgraph "Presentation Layer"
        HTTP[HTTP+JSON API<br/>/api/v1/*]
        Health[Health/Ready<br/>Endpoints]
    end

    subgraph "Application Layer"
        Bootstrap[Bootstrap Handler]
        Apps[Apps/Versions Handler]
        Runs[Runs Handler]
        Runner[Runner Handler]
        TokenAuth[Token Auth]
    end

    subgraph "Domain Layer"
        TeamStore[Teams Store]
        AppStore[Apps Store]
        VersionStore[Versions Store]
        RunStore[Runs Store]
        RunnerStore[Runners Store]
        ReaperSvc[Reaper Service]
    end

    subgraph "Infrastructure Layer"
        DB[(SQLite)]
        ObjStore[Local Object Store]
        Migrations[Migrations]
    end

    HTTP --> Bootstrap
    HTTP --> Apps
    HTTP --> Runs
    HTTP --> Runner
    Health --> DB

    Bootstrap --> TeamStore
    Apps --> AppStore
    Apps --> VersionStore
    Runs --> RunStore
    Runner --> RunnerStore
    ReaperSvc --> RunStore

    TeamStore --> DB
    AppStore --> DB
    VersionStore --> DB
    RunStore --> DB
    RunnerStore --> DB
    VersionStore --> ObjStore
    Migrations --> DB
```

---

## Run Lifecycle Sequence

Complete flow from run creation through execution and completion.

```mermaid
sequenceDiagram
    participant C as Client
    participant API as Control Plane
    participant DB as SQLite
    participant R as Runner

    Note over C,R: Run Creation & Execution Flow

    C->>API: POST /apps/{app}/runs
    API->>DB: Insert run (status=queued)
    API-->>C: 201 Created {run_id}

    loop Poll for Work
        R->>API: POST /runs/lease
        API->>DB: SELECT queued run + CAS update
        API->>DB: Create run_attempt (status=leased)
        API-->>R: 200 {run_id, lease_token, ...}
    end

    R->>API: POST /runs/{run}/start
    API->>DB: CAS: leased → running
    API-->>R: 200 {lease_expires_at}

    R->>API: GET /runs/{run}/artifact
    API-->>R: Artifact + SHA256

    Note over R: Execute Python in venv

    loop During Execution
        R->>API: POST /runs/{run}/heartbeat
        API->>DB: Extend lease_expires_at
        API-->>R: 200 {cancel_requested}

        R->>API: POST /runs/{run}/logs
        API->>DB: Insert run_logs
        API-->>R: 200 OK
    end

    R->>API: POST /runs/{run}/result
    API->>DB: CAS: running → completed/failed
    API-->>R: 200 OK
```

---

## Deployment Topology

Physical deployment structure for a single-host setup.

```mermaid
graph TB
    subgraph "Host Machine"
        subgraph "Control Plane Process"
            MTWD[minitowerd<br/>:8080]
            SQLITE[(minitower.db)]
            OBJDIR[./objects/]
        end

        subgraph "Runner Process(es)"
            RUNNER1[minitower-runner]
            WORK1[~/.minitower/]
        end
    end

    MTWD --> SQLITE
    MTWD --> OBJDIR
    RUNNER1 -->|HTTP| MTWD
    RUNNER1 --> WORK1

    CLIENT[API Client] -->|HTTP :8080| MTWD
```

---

## Data Model

Entity relationship diagram showing database schema.

```mermaid
erDiagram
    TEAM ||--o{ TEAM_TOKEN : has
    TEAM ||--o{ ENVIRONMENT : has
    TEAM ||--o{ APP : owns

    APP ||--o{ APP_VERSION : has
    APP ||--o{ RUN : has

    ENVIRONMENT ||--o{ RUN : scopes

    APP_VERSION ||--o{ RUN : triggers

    RUN ||--o{ RUN_ATTEMPT : has
    RUN_ATTEMPT ||--o{ RUN_LOG : produces

    RUNNER ||--o{ RUN_ATTEMPT : executes

    TEAM {
        int id PK
        string slug UK
        string name
    }

    APP {
        int id PK
        int team_id FK
        string slug
        bool disabled
    }

    APP_VERSION {
        int id PK
        int app_id FK
        int version_no
        string artifact_object_key
        string artifact_sha256
        string entrypoint
        int timeout_seconds
    }

    RUN {
        int id PK
        int team_id FK
        int app_id FK
        int environment_id FK
        int app_version_id FK
        string status
        int max_retries
        int retry_count
        bool cancel_requested
    }

    RUN_ATTEMPT {
        int id PK
        int run_id FK
        int runner_id FK
        int attempt_no
        string lease_token_hash
        datetime lease_expires_at
        string status
    }

    RUNNER {
        int id PK
        string name UK
        string environment
        string status
    }
```

---

## ASCII Diagram

Terminal-friendly representation for plain-text documentation.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         MINITOWER ARCHITECTURE                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   ┌──────────────┐         HTTP/JSON           ┌──────────────────┐ │
│   │  API Client  │ ──────────────────────────▶ │  Control Plane   │ │
│   │  (Team Token)│                             │   (minitowerd)   │ │
│   └──────────────┘                             │                  │ │
│                                                │  ┌────────────┐  │ │
│                                                │  │  HTTP API  │  │ │
│   ┌──────────────┐         HTTP/JSON           │  ├────────────┤  │ │
│   │    Runner    │ ◀─────────────────────────▶ │  │  Handlers  │  │ │
│   │(Runner Token)│                             │  ├────────────┤  │ │
│   │              │                             │  │   Store    │  │ │
│   │ ┌──────────┐ │                             │  ├────────────┤  │ │
│   │ │ Executor │ │                             │  │   Reaper   │  │ │
│   │ │ (Python) │ │                             │  └────────────┘  │ │
│   │ └──────────┘ │                             │        │         │ │
│   │ ┌──────────┐ │                             │        ▼         │ │
│   │ │  venv    │ │                             │  ┌────────────┐  │ │
│   │ └──────────┘ │                             │  │  SQLite    │  │ │
│   └──────────────┘                             │  │  (WAL)     │  │ │
│                                                │  └────────────┘  │ │
│                                                │        │         │ │
│                                                │        ▼         │ │
│                                                │  ┌────────────┐  │ │
│                                                │  │  Objects   │  │ │
│                                                │  │  (Local)   │  │ │
│                                                │  └────────────┘  │ │
│                                                └──────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

Run Status Flow:
  queued ──▶ leased ──▶ running ──▶ completed
    │           │           │
    │           │           ▼
    │           │      cancelling ──▶ cancelled
    │           │           │
    ▼           └─────┬─────┘
  cancelled          ▼
  (pre-lease)       dead
                  (retries exhausted)
```

---

## Diagram Usage Guide

| Diagram | Best For |
|---------|----------|
| **Component** | High-level system overview, onboarding docs |
| **Layered** | Understanding code organization, architectural boundaries |
| **Sequence** | Debugging flows, API documentation |
| **Deployment** | Ops runbooks, infrastructure docs |
| **ER Diagram** | Database schema discussions, data modeling |
| **ASCII** | Terminal presentations, plain-text docs |

## Rendering

These Mermaid diagrams render natively on GitHub. For other uses:

- **CLI**: `npx @mermaid-js/mermaid-cli -i architecture.md -o output.png`
- **Web**: Paste diagrams into [mermaid.live](https://mermaid.live)
- **VS Code**: Install "Markdown Preview Mermaid Support" extension
