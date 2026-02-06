export type TokenRole = 'admin' | 'member'

export type RunStatus =
  | 'queued'
  | 'leased'
  | 'running'
  | 'cancelling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'dead'

export interface ErrorEnvelope {
  error: {
    code: string
    message: string
  }
}

export interface LoginRequest {
  slug: string
  password: string
}

export interface LoginResponse {
  team_id: number
  token: string
  token_id: number
  role: TokenRole
}

export interface BootstrapTeamRequest {
  slug: string
  name: string
  password?: string
}

export interface BootstrapTeamResponse {
  team_id: number
  slug: string
  token: string
  role: TokenRole
}

export interface MeResponse {
  team_id: number
  team_slug: string
  token_id: number
  role: TokenRole
}

export interface AppResponse {
  app_id: number
  slug: string
  description?: string
  disabled: boolean
  created_at: string
  updated_at: string
}

export interface ListAppsResponse {
  apps: AppResponse[]
}

export interface CreateAppRequest {
  slug: string
  description?: string
}

export interface VersionResponse {
  version_id: number
  version_no: number
  entrypoint: string
  timeout_seconds?: number
  params_schema?: Record<string, unknown>
  artifact_sha256: string
  created_at: string
}

export interface ListVersionsResponse {
  versions: VersionResponse[]
}

export interface CreateVersionRequest {
  artifact: File
  entrypoint: string
  timeout_seconds?: number
  params_schema_json?: string
}

export interface RunResponse {
  run_id: number
  app_id: number
  app_slug?: string
  run_no: number
  version_no: number
  status: RunStatus
  input?: Record<string, unknown>
  priority: number
  max_retries: number
  retry_count: number
  cancel_requested: boolean
  queued_at: string
  started_at?: string
  finished_at?: string
}

export interface ListRunsResponse {
  runs: RunResponse[]
}

export interface CreateRunRequest {
  input?: Record<string, unknown>
  version_no?: number
  priority?: number
  max_retries?: number
}

export interface RunsSummaryResponse {
  total_runs: number
  active_runs: number
  queued_runs: number
  terminal_runs: number
}

export interface RunLogEntry {
  seq: number
  stream: 'stdout' | 'stderr'
  line: string
  logged_at: string
}

export interface RunLogsResponse {
  logs: RunLogEntry[]
}

export interface AdminRunner {
  runner_id: number
  name: string
  environment: string
  status: string
  last_seen_at?: string
}

export interface AdminRunnersResponse {
  runners: AdminRunner[]
}

export interface CreateTokenRequest {
  name?: string
  role?: TokenRole
}

export interface CreateTokenResponse {
  token_id: number
  token: string
  name?: string
  role: TokenRole
}
