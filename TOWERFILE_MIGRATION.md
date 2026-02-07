# Towerfile Migration Plan

Replace the current manual tar.gz + multipart-form packaging workflow with a
declarative **Towerfile** that lives in the user's project root and drives
packaging, upload, and execution.

Reference: https://docs.tower.dev/docs/architecture/how-tower-works#knowing-what-to-run

---

## 1. Problem Statement

Today, deploying a version requires the user to:

1. Manually `tar -czf` their source files.
2. `POST /api/v1/apps/{app}/versions` with multipart form fields:
   `artifact` (tar.gz), `entrypoint`, `timeout_seconds`, `params_schema_json`.
3. Know exactly which files to include in the tarball.
4. Repeat all of this for every deploy.

This is error-prone, undocumented at the project level, and disconnected from
the source tree. There is no file in the repo that says "this is my app, here
is what to run."

---

## 2. Target State

A single `Towerfile` in the project root declares everything the platform needs:

```toml
[app]
name = "my-etl-pipeline"
script = "./pipeline.py"
source = [
    "./**/*.py",
    "./requirements.txt",
    "./config/*.yaml",
]

[app.timeout]
seconds = 120

[[parameters]]
name = "region"
description = "AWS region to process"
default = "us-east-1"

[[parameters]]
name = "batch_size"
description = "Number of records per batch"
```

A new `minitower deploy` CLI command reads this file, resolves source globs,
creates the artifact, and uploads it—one command, zero manual tar/curl.

---

## 3. Towerfile Specification (MiniTower Subset)

Format: TOML. File name: `Towerfile` (no extension). Must be at the project root.

### `[app]` section (required)

| Field          | Type       | Required | Description |
|----------------|------------|----------|-------------|
| `name`         | string     | yes      | App slug. Must match `[a-z0-9][a-z0-9-]*` (existing slug rules). Used to resolve or auto-create the target app on the server. |
| `script`       | string     | yes      | Relative path to the entrypoint. Must end in `.py` or `.sh`. Must be matched by at least one `source` glob. Shell entrypoints are supported and executed via `/bin/sh` (shebang ignored). |
| `source`       | [string]   | no       | Glob patterns for files to include. Relative to the Towerfile directory. If omitted, defaults to `["./**"]` (all files). Towerfile itself is always included. See “Glob semantics” below. |
| `import_paths` | [string]   | no       | Extra directories to prepend to `PYTHONPATH` at runtime. Relative to the unpacked artifact root. |

### `[app.timeout]` section (optional)

| Field     | Type | Required | Description |
|-----------|------|----------|-------------|
| `seconds` | int  | no       | Run timeout in seconds. Must be >= 1. Overrides the default 300s. |

### `[[parameters]]` section (optional, repeatable)

| Field         | Type   | Required | Description |
|---------------|--------|----------|-------------|
| `name`        | string | yes      | Parameter identifier. Must be non-empty. |
| `description` | string | no       | Human-readable description. |
| `type`        | string | no       | Parameter value type for JSON Schema. Allowed: `string`, `number`, `integer`, `boolean`. Defaults to `string`. |
| `default`     | any    | no       | Default value if not provided at run time. Parsed as a TOML literal and serialized into JSON Schema with the matching type. |

### Glob semantics (Tower.dev-aligned)

Implement globs using `github.com/bmatcuk/doublestar/v4` to match Tower.dev-style `**` behavior:

- Patterns are evaluated relative to the Towerfile directory.
- `**` matches zero or more path segments; `*` and `?` behave as standard wildcards.
- `/` is the path separator in patterns; normalize OS-specific separators before matching.
- Dotfiles are **not** matched by `*`/`**` unless the pattern segment explicitly starts with a dot (e.g., `./**/.*`).
- No brace expansion (`{a,b}`) and no character classes beyond the standard glob syntax.
- Confirm parity with the latest Tower.dev docs before implementation and adjust if their glob semantics differ.

### Symlink and path-traversal behavior

- Resolve patterns to file paths, then verify each resolved path stays within the Towerfile root using `filepath.Rel` and a `..` check.
- If a resolved path is a symlink:
  - Package it as a symlink entry in the tar archive (do not follow it) to
    preserve intent and avoid accidentally bundling external content.
  - Reject symlinks whose target resolves outside the project root.
- Reject any `source` or `import_paths` entry that contains `..` path traversal after cleaning.

### Validation Rules

1. `name` must pass existing `validate.Slug()` rules.
2. `script` path must be matched by at least one `source` glob (or exist in
   the default glob set).
3. `source` patterns must not escape the project root (`../` is rejected).
4. Parameter names must be unique.
5. `import_paths` entries must not escape the project root.
6. If a parameter `default` is provided, attempt to coerce it to the declared
   `type` (or the default type `string`) using safe conversions (e.g., `"1"`
   → `integer`, `"true"` → `boolean`). If coercion fails, return a validation
   error.
7. Parameter `type` must be one of: `string`, `number`, `integer`, `boolean`.

---

## 4. Changes Required

### Phase 1: Towerfile Parser (`internal/towerfile`)

New package: `internal/towerfile`

**Files:**
- `towerfile.go` — types and `Parse(reader) (*Towerfile, error)`
- `towerfile_test.go` — unit tests
- `resolve.go` — `ResolveSource(dir, patterns) ([]string, error)` glob resolution
- `resolve_test.go` — unit tests

**Types:**
```go
type Towerfile struct {
    App App `toml:"app"`
}

type App struct {
    Name        string   `toml:"name"`
    Script      string   `toml:"script"`
    Source      []string `toml:"source"`
    ImportPaths []string `toml:"import_paths"`
    Timeout     *Timeout `toml:"timeout"`
}

type Timeout struct {
    Seconds int `toml:"seconds"`
}

type Parameter struct {
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Type        string `toml:"type"`
    Default     any    `toml:"default"`
}
```

Note: `[[parameters]]` are top-level in the TOML array-of-tables syntax, so
the `Towerfile` struct needs a `Parameters []Parameter` field alongside `App`.

**Dependency:** Add `github.com/BurntSushi/toml` to `go.mod` (the standard Go
TOML library, pure Go, no CGo).

**Validation function:** `Validate(tf *Towerfile) error` checks all rules from
Section 3 above.

**Acceptance:**
- Parses valid Towerfiles into struct.
- Rejects missing `name`, missing `script`, path traversal in `source`,
  duplicate parameter names, invalid slug.
- `ResolveSource` expands globs relative to a given directory and returns a
  sorted, deduplicated file list.

---

### Phase 2: Artifact Packager (`internal/towerfile`)

Add to the same package:

```go
func Package(dir string, tf *Towerfile) (io.Reader, string, error)
```

1. Validate the Towerfile.
2. Resolve `source` globs against `dir`.
3. Verify `script` is in the resolved file list.
4. Create a tar.gz in memory (or temp file for large artifacts).
5. Always include the `Towerfile` in the archive root.
6. Compute SHA256 while writing.
7. Return the archive reader and hex-encoded SHA256.

**Acceptance:**
- Produces a tar.gz containing exactly the matched files + Towerfile.
- File paths in the archive are relative to the project root.
- SHA256 matches a manual computation of the same archive.
- Rejects if `script` is not in the resolved set.

---

### Phase 3: CLI Deploy Command (`cmd/minitower-cli`)

New binary: `cmd/minitower-cli/main.go`

Subcommands (start with `deploy` only):

```
minitower-cli deploy [--server URL] [--token TOKEN] [--dir DIR]
```

**Behavior:**

1. Read `Towerfile` from `--dir` (default: current directory).
2. Parse and validate.
3. Package the artifact using Phase 2.
4. Ensure the app exists on the server:
   - `GET /api/v1/apps/{name}` — if 404, `POST /api/v1/apps` to create it.
5. Upload the version:
   - `POST /api/v1/apps/{name}/versions` with:
     - `artifact`: the packaged tar.gz
     - No `entrypoint` field (server must read Towerfile from the artifact)
6. Print version info on success.

**Parameter-to-schema mapping:**

```go
// [[parameters]] with name="region", description="AWS region", type="string", default="us-east-1"
// becomes JSON Schema:
{
  "type": "object",
  "properties": {
    "region": {
      "type": "string",
      "description": "AWS region",
      "default": "us-east-1"
    }
  }
}
```

Default values are stored as schema `"default"` fields and must match the
parameter `type`.

**Config resolution order:**
1. CLI flags (`--server`, `--token`)
2. Environment variables (`MINITOWER_SERVER_URL`, `MINITOWER_API_TOKEN`)
3. Config file `~/.minitower/config.toml` (future)

**Acceptance:**
- `minitower-cli deploy` in a directory with a valid Towerfile creates/updates
  the app and uploads a new version.
- Without a Towerfile, it exits with a clear error.
- Auth failure produces a clear error.

---

### Phase 4: Server-Side Towerfile Awareness

Changes to the version upload endpoint to require a Towerfile *inside* the
artifact, eliminating legacy form fields.

#### 4a. Required upload mode: artifact-only

Add a second code path to `CreateVersion` in
`internal/httpapi/handlers/versions.go`:

The server:
1. Accepts the artifact.
2. Opens the tar.gz and reads the `Towerfile` from the archive root.
3. Parses it to extract `script`, `timeout`, `parameters`, and `import_paths`.
4. Synthesizes `params_schema_json` from `[[parameters]]`.
5. Uses those values as the source of truth for the version.
6. Stores the Towerfile content in a new `towerfile_toml` column (see 4b).

**Failure behavior:** if the Towerfile is missing or invalid, return HTTP 400
with a structured error code (e.g., `TOWERFILE_MISSING`, `TOWERFILE_INVALID`)
and a clear message. Do not create a version. Only scan the first N tar entries
(e.g., 50) and reject archives where the Towerfile entry is larger than a
small limit (e.g., 256 KB) to prevent tar bombs.

#### 4b. Database migration (`0004_towerfile.up.sql`)

```sql
ALTER TABLE app_versions ADD COLUMN towerfile_toml TEXT;
ALTER TABLE app_versions ADD COLUMN import_paths_json TEXT;
```

- `towerfile_toml`: raw Towerfile content, stored for auditability and display.
- `import_paths_json`: JSON array of import paths, used by the runner at
  execution time.

#### 4c. Store layer changes (`internal/store/versions.go`)

- Add `TowerfileTOML *string` and `ImportPaths []string` to `AppVersion` struct.
- Update `CreateVersion` to accept and persist the new fields.
- Update queries to read the new columns.

#### 4d. API response changes

Add to `versionResponse`:

```go
TowerfileTOML *string  `json:"towerfile_toml,omitempty"`
ImportPaths   []string `json:"import_paths,omitempty"`
```

#### 4e. Artifact response header

Add `X-Import-Paths` header (JSON array) to `GET /api/v1/runs/{run}/artifact`
so the runner can set up `PYTHONPATH` without parsing the Towerfile itself.

**Acceptance:**
- Upload with Towerfile-in-artifact (no `entrypoint` field) succeeds.
- Upload without a Towerfile fails with HTTP 400 and a clear error message.
- `GET /api/v1/apps/{app}/versions` shows `towerfile_toml` and `import_paths`
  when present.
- Migration applies cleanly on existing databases.

---

### Phase 5: Runner Import Path Support

Changes to `cmd/minitower-runner/main.go`:

1. Read `X-Import-Paths` header from artifact download response.
2. Parse as JSON string array.
3. When constructing the `exec.Cmd`, detect the entrypoint extension:
   - `.py`: use the existing Python runner behavior and prepend each import
     path (resolved relative to the workspace root) to `PYTHONPATH`.
   - `.sh`: always execute via `/bin/sh` regardless of shebang, without
     Python-specific env setup.

```go
if len(importPaths) > 0 {
    resolved := make([]string, len(importPaths))
    for i, p := range importPaths {
        resolved[i] = filepath.Join(workDir, p)
    }
    cmd.Env = append(cmd.Env,
        "PYTHONPATH="+strings.Join(resolved, ":")+":"+os.Getenv("PYTHONPATH"),
    )
}
```

**Acceptance:**
- A Towerfile with `import_paths = ["./lib", "./vendor"]` results in those
  directories being on `PYTHONPATH` during execution for `.py` entrypoints.
- `.sh` entrypoints execute via `/bin/sh` with the same working directory.
- Existing runs without import paths are unaffected.

---

### Phase 6: Smoke Test & Documentation Updates

#### 6a. Update `scripts/smoke.sh`

Add a new test case that:
1. Creates a directory with a `Towerfile` and Python source files.
2. Runs `minitower-cli deploy`.
3. Triggers a run.
4. Verifies completion and log output.

Remove the existing manual-tar test case (legacy mode is no longer supported).

#### 6b. Update `Dockerfile`

Add a build stage for the `minitower-cli` binary:

```dockerfile
RUN CGO_ENABLED=0 go build -o /bin/minitower-cli ./cmd/minitower-cli
```

Add a new Docker target or include in the existing `minitowerd` image for
distribution.

#### 6c. Update `PLAN.md`

Add the Towerfile to the domain model and API contract sections.

#### 6d. Update `scripts/curl-examples.md`

Add examples showing the Towerfile-based deploy workflow.

**Acceptance:**
- Smoke test passes for the Towerfile workflow.
- CLI binary builds in Docker.

---

### Phase 7: Frontend Changes

The Vue.js frontend has four areas that need updates to support Towerfile-based
packaging. These are **not optional**—the version upload form and run creation
modal both handle fields that change with the Towerfile migration.

#### 7a. TypeScript types (`frontend/src/api/types.ts`)

Update `VersionResponse` to include new server fields:

```typescript
export interface VersionResponse {
  version_id: number
  version_no: number
  entrypoint: string
  timeout_seconds?: number
  params_schema?: Record<string, unknown>
  artifact_sha256: string
  created_at: string
  // New fields from Phase 4d:
  towerfile_toml?: string          // Raw Towerfile content
  import_paths?: string[]          // PYTHONPATH additions
}
```

Update `CreateVersionRequest` so the client only sends the artifact. All
metadata (entrypoint, timeout, params schema) comes from the Towerfile inside
the archive.

```typescript
export interface CreateVersionRequest {
  artifact: File
}
```

#### 7b. Version upload form (`frontend/src/pages/AppDetailPage.vue`)

The versions tab currently has a form with required `entrypoint`, optional
`timeout_seconds`, optional `params_schema_json`, and required artifact file.

**Changes:**

1. Remove the `entrypoint`, `timeout_seconds`, and `params_schema_json` fields
   entirely. The server extracts these from the Towerfile inside the artifact.
2. After a successful upload, if the response contains `towerfile_toml`,
   display a success message noting the Towerfile was detected.

**API client change** (`frontend/src/api/client.ts`): In `createVersion()`,
omit `entrypoint`, `timeout_seconds`, and `params_schema_json` from `FormData`
and rely on Towerfile extraction on the server.

#### 7c. Version list display (`frontend/src/pages/AppDetailPage.vue`)

Currently each version row shows: version number, entrypoint, SHA256, timestamp.

**Changes:**

1. Add a small "Towerfile" badge/icon next to versions that have
   `towerfile_toml` set (indicates declarative packaging was used).
2. Add an expandable detail section (or modal) on click that shows:
   - Raw Towerfile content in a `<pre>` code block.
   - Parsed `import_paths` list.
   - Source patterns (parsed from the TOML).
3. Show `import_paths` as chips/tags when present.

#### 7d. Run creation parameter defaults (`frontend/src/components/apps/CreateRunModal.vue`)

The `CreateRunModal` already parses `params_schema` into form fields with type
awareness. However, it does **not** currently use `default` values from the
JSON Schema to pre-populate fields.

When `[[parameters]]` in the Towerfile specify `default` values, they get
mapped to JSON Schema `"default"` fields (Phase 3). The modal needs to:

1. Read `default` from each property in the schema.
2. Pre-populate `formValues` with defaults when the version is selected.
3. Show a visual indicator (e.g., italic placeholder text, "(default)" label)
   for fields using their default value.
4. Allow the user to clear or override defaults.

**Code change in `CreateRunModal.vue`:**

```typescript
// In the schemaFields computed, extract default:
return {
  name,
  kind: normalizeType(details.type),
  required: requiredNames.has(name),
  description: typeof details.description === 'string' ? details.description : undefined,
  enumValues: enumValues && enumValues.length > 0 ? enumValues : undefined,
  defaultValue: details.default,  // NEW: capture default from schema
}

// In the version selection watcher, seed formValues with defaults:
watch(selectedVersionNo, () => {
  formValues.value = {}
  for (const field of schemaFields.value) {
    if (field.defaultValue !== undefined) {
      formValues.value[field.name] = field.defaultValue
    }
  }
})
```

**Acceptance:**
- Uploading a Towerfile-based artifact without explicit `entrypoint` succeeds
  and the version shows a Towerfile badge.
- Version detail shows raw Towerfile content when available.
- Run creation form pre-populates parameter defaults from the schema.
- `VersionResponse` type includes `towerfile_toml` and `import_paths`.

---

## 5. File Change Summary

| File / Package | Change Type | Description |
|----------------|-------------|-------------|
| `go.mod` | modify | Add `github.com/BurntSushi/toml` dependency |
| `internal/towerfile/towerfile.go` | **new** | Towerfile types, parser, validator |
| `internal/towerfile/towerfile_test.go` | **new** | Parser and validator tests |
| `internal/towerfile/resolve.go` | **new** | Source glob resolution |
| `internal/towerfile/resolve_test.go` | **new** | Resolution tests |
| `internal/towerfile/package.go` | **new** | Artifact packaging from Towerfile |
| `internal/towerfile/package_test.go` | **new** | Packaging tests |
| `cmd/minitower-cli/main.go` | **new** | CLI deploy command |
| `internal/migrations/0004_towerfile.up.sql` | **new** | Add `towerfile_toml` and `import_paths_json` columns |
| `internal/store/versions.go` | modify | Add new fields to `AppVersion`, update queries |
| `internal/httpapi/handlers/versions.go` | modify | Towerfile-from-artifact extraction path |
| `internal/httpapi/handlers/runner.go` | modify | Add `X-Import-Paths` response header |
| `cmd/minitower-runner/main.go` | modify | Read import paths, set `PYTHONPATH` |
| `Dockerfile` | modify | Add `minitower-cli` build target |
| `scripts/smoke.sh` | modify | Add Towerfile-based test case |
| `scripts/curl-examples.md` | modify | Add Towerfile deploy examples |
| `PLAN.md` | modify | Document Towerfile in domain model |
| `frontend/src/api/types.ts` | modify | Add `towerfile_toml`, `import_paths` to `VersionResponse`; remove legacy fields from `CreateVersionRequest` |
| `frontend/src/api/client.ts` | modify | Support Towerfile-mode upload (send only artifact in FormData) |
| `frontend/src/pages/AppDetailPage.vue` | modify | Remove legacy fields from upload form; Towerfile badge + detail on version rows |
| `frontend/src/components/apps/CreateRunModal.vue` | modify | Pre-populate parameter defaults from JSON Schema `default` field |

---

## 6. Migration & Backward Compatibility

### Breaking change (intentional)

The existing multipart upload with explicit `entrypoint` is removed. The
Towerfile-in-artifact path becomes the only supported upload path.

### Rollout order

1. Merge Phase 1-2 (parser + packager) — no server changes, fully testable in
   isolation.
2. Merge Phase 3 (CLI) — requires Phase 4 on the server since it uploads
   artifact-only Towerfile bundles.
3. Merge Phase 4 (server Towerfile awareness) — enables artifact-only upload
   and persists Towerfile metadata.
4. Merge Phase 5 (runner import paths) — depends on Phase 4 header.
5. Merge Phase 6 (tests/docs) — can be incremental throughout.
6. Merge Phase 7 (frontend) — depends on Phase 4d API response changes. The
   parameter defaults fix (7d) is independently mergeable at any time.

### Data migration

The `ALTER TABLE ADD COLUMN` migration adds nullable columns, so existing rows
are unaffected. No backfill needed—versions created before the migration simply
have `towerfile_toml = NULL`.

---

## 7. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| TOML dependency adds supply chain surface | `BurntSushi/toml` is the de facto Go TOML library, widely audited, pure Go, no CGo. Minimal transitive deps. |
| Large source trees produce huge artifacts | Enforce existing `MINITOWER_MAX_ARTIFACT_SIZE` (100 MB). Add `.towerignore` support in a future iteration if needed. |
| Glob patterns match unexpected files (secrets, `.env`) | Document recommended patterns. Default `source` excludes dotfiles and common ignore patterns in a future iteration. For now, explicit `source` patterns are the safe path. |
| Towerfile-in-artifact parsing adds server-side tar scanning | Only scan the first N entries and cap Towerfile size; reject if not found within the first N entries. |
| CLI auto-creating apps could cause slug collisions | CLI reports clear error on conflict. `name` validation reuses existing `validate.Slug()` rules. |

---

## 8. Out of Scope (Future)

- `.towerignore` file for excluding paths from `source` globs.
- `minitower-cli run` command (trigger runs from CLI).
- `minitower-cli logs` command (stream logs from CLI).
- Multi-language entrypoints beyond `.py` and `.sh`.
- Towerfile `[env]` section for environment variable declarations.
- Towerfile `[secrets]` section for secret references.
- Multi-app monorepo support (multiple Towerfiles in subdirectories).
- Towerfile-driven dependency caching (reuse venvs across versions with
  identical `requirements.txt`).

---

## 9. Implementation Order Checklist

- [ ] **Phase 1:** `internal/towerfile` parser, types, validator, tests
- [ ] **Phase 2:** `internal/towerfile` packager, glob resolver, tests
- [ ] **Phase 3:** `cmd/minitower-cli` deploy command
- [ ] **Phase 4a:** Server artifact-only upload path (Towerfile extraction)
- [ ] **Phase 4b:** Database migration `0004_towerfile.up.sql`
- [ ] **Phase 4c:** Store layer updates for new columns
- [ ] **Phase 4d:** API response updates
- [ ] **Phase 4e:** Artifact download `X-Import-Paths` header
- [ ] **Phase 5:** Runner `PYTHONPATH` setup from import paths
- [ ] **Phase 6:** Smoke test, Dockerfile, documentation updates
- [ ] **Phase 7a:** Frontend TypeScript types (`VersionResponse`, `CreateVersionRequest`)
- [ ] **Phase 7b:** Frontend version upload form (remove legacy fields)
- [ ] **Phase 7c:** Frontend version list display (Towerfile badge, detail expand)
- [ ] **Phase 7d:** Frontend run creation parameter defaults from schema
