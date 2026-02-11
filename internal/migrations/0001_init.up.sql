CREATE TABLE IF NOT EXISTS teams (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(slug)
);

CREATE TABLE IF NOT EXISTS team_tokens (
  id INTEGER PRIMARY KEY,
  team_id INTEGER NOT NULL,
  token_hash TEXT NOT NULL,
  name TEXT,
  created_at INTEGER NOT NULL,
  revoked_at INTEGER,
  last_used_at INTEGER,
  FOREIGN KEY(team_id) REFERENCES teams(id)
);

CREATE TABLE IF NOT EXISTS environments (
  id INTEGER PRIMARY KEY,
  team_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(team_id, name),
  FOREIGN KEY(team_id) REFERENCES teams(id)
);

CREATE TABLE IF NOT EXISTS apps (
  id INTEGER PRIMARY KEY,
  team_id INTEGER NOT NULL,
  slug TEXT NOT NULL,
  description TEXT,
  disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(team_id, slug),
  FOREIGN KEY(team_id) REFERENCES teams(id)
);

CREATE TABLE IF NOT EXISTS app_versions (
  id INTEGER PRIMARY KEY,
  app_id INTEGER NOT NULL,
  version_no INTEGER NOT NULL,
  artifact_object_key TEXT NOT NULL,
  artifact_sha256 TEXT NOT NULL,
  entrypoint TEXT NOT NULL,
  timeout_seconds INTEGER,
  params_schema_json TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE(app_id, version_no),
  FOREIGN KEY(app_id) REFERENCES apps(id)
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  team_id INTEGER NOT NULL,
  app_id INTEGER NOT NULL,
  environment_id INTEGER NOT NULL,
  app_version_id INTEGER NOT NULL,
  run_no INTEGER NOT NULL,
  input_json TEXT,
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','leased','running','cancelling','completed','failed','cancelled','dead')),
  priority INTEGER NOT NULL DEFAULT 0,
  max_retries INTEGER NOT NULL DEFAULT 0 CHECK (max_retries >= 0),
  retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK (cancel_requested IN (0, 1)),
  queued_at INTEGER NOT NULL,
  started_at INTEGER,
  finished_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(app_id, run_no),
  FOREIGN KEY(app_id, team_id) REFERENCES apps(id, team_id),
  FOREIGN KEY(environment_id, team_id) REFERENCES environments(id, team_id),
  FOREIGN KEY(app_version_id, app_id) REFERENCES app_versions(id, app_id)
);

CREATE TABLE IF NOT EXISTS runners (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  environment TEXT NOT NULL DEFAULT 'default',
  labels_json TEXT,
  token_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'online',
  max_concurrent INTEGER NOT NULL DEFAULT 1,
  last_seen_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS run_attempts (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL,
  attempt_no INTEGER NOT NULL,
  runner_id INTEGER NOT NULL,
  lease_token_hash TEXT NOT NULL,
  lease_expires_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'leased' CHECK (status IN ('leased','running','cancelling','completed','failed','cancelled','expired')),
  exit_code INTEGER,
  error_message TEXT,
  started_at INTEGER,
  finished_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(run_id, attempt_no),
  FOREIGN KEY(run_id) REFERENCES runs(id),
  FOREIGN KEY(runner_id) REFERENCES runners(id)
);

CREATE TABLE IF NOT EXISTS run_logs (
  id INTEGER PRIMARY KEY,
  run_attempt_id INTEGER NOT NULL,
  seq INTEGER NOT NULL,
  stream TEXT NOT NULL CHECK (stream IN ('stdout','stderr')),
  line TEXT NOT NULL,
  logged_at INTEGER NOT NULL,
  UNIQUE(run_attempt_id, seq),
  FOREIGN KEY(run_attempt_id) REFERENCES run_attempts(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS run_attempts_active_run_uq
  ON run_attempts(run_id)
  WHERE status IN ('leased','running','cancelling');

CREATE UNIQUE INDEX IF NOT EXISTS run_attempts_active_runner_uq
  ON run_attempts(runner_id)
  WHERE status IN ('leased','running','cancelling');

CREATE INDEX IF NOT EXISTS run_attempts_expiry_idx
  ON run_attempts(lease_expires_at)
  WHERE status IN ('leased','running','cancelling');

CREATE INDEX IF NOT EXISTS runs_queue_pick_idx
  ON runs(environment_id, status, priority DESC, queued_at ASC, id ASC)
  WHERE status = 'queued';

CREATE UNIQUE INDEX IF NOT EXISTS apps_id_team_uq
  ON apps(id, team_id);

CREATE UNIQUE INDEX IF NOT EXISTS environments_id_team_uq
  ON environments(id, team_id);

CREATE UNIQUE INDEX IF NOT EXISTS app_versions_id_app_uq
  ON app_versions(id, app_id);
