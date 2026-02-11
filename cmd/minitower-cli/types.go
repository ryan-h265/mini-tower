package main

type exitError struct {
	Code    int
	Message string
}

func (e *exitError) Error() string {
	return e.Message
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type loginResponse struct {
	TeamID  int64  `json:"team_id"`
	Token   string `json:"token"`
	TokenID int64  `json:"token_id"`
	Role    string `json:"role"`
}

type meResponse struct {
	TeamID   int64  `json:"team_id"`
	TeamSlug string `json:"team_slug"`
	TokenID  int64  `json:"token_id"`
	Role     string `json:"role"`
}

type appResponse struct {
	AppID       int64   `json:"app_id"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	Disabled    bool    `json:"disabled"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type listAppsResponse struct {
	Apps []appResponse `json:"apps"`
}

type versionResponse struct {
	VersionID      int64          `json:"version_id"`
	VersionNo      int64          `json:"version_no"`
	Entrypoint     string         `json:"entrypoint"`
	TimeoutSeconds *int           `json:"timeout_seconds,omitempty"`
	ParamsSchema   map[string]any `json:"params_schema,omitempty"`
	ArtifactSHA256 string         `json:"artifact_sha256"`
	TowerfileTOML  *string        `json:"towerfile_toml,omitempty"`
	ImportPaths    []string       `json:"import_paths,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

type listVersionsResponse struct {
	Versions []versionResponse `json:"versions"`
}

type runResponse struct {
	RunID           int64          `json:"run_id"`
	AppID           int64          `json:"app_id"`
	AppSlug         string         `json:"app_slug,omitempty"`
	RunNo           int64          `json:"run_no"`
	VersionNo       int64          `json:"version_no"`
	Status          string         `json:"status"`
	Input           map[string]any `json:"input,omitempty"`
	Priority        int            `json:"priority"`
	MaxRetries      int            `json:"max_retries"`
	RetryCount      int            `json:"retry_count"`
	CancelRequested bool           `json:"cancel_requested"`
	QueuedAt        string         `json:"queued_at"`
	StartedAt       *string        `json:"started_at,omitempty"`
	FinishedAt      *string        `json:"finished_at,omitempty"`
}

type listRunsResponse struct {
	Runs []runResponse `json:"runs"`
}

type runLogEntry struct {
	Seq      int64  `json:"seq"`
	Stream   string `json:"stream"`
	Line     string `json:"line"`
	LoggedAt string `json:"logged_at"`
}

type runLogsResponse struct {
	Logs []runLogEntry `json:"logs"`
}

type createTokenResponse struct {
	TokenID int64   `json:"token_id"`
	Token   string  `json:"token"`
	Name    *string `json:"name,omitempty"`
	Role    string  `json:"role"`
}

type adminRunnerResponse struct {
	RunnerID    int64   `json:"runner_id"`
	Name        string  `json:"name"`
	Environment string  `json:"environment"`
	Status      string  `json:"status"`
	LastSeenAt  *string `json:"last_seen_at,omitempty"`
}

type listAdminRunnersResponse struct {
	Runners []adminRunnerResponse `json:"runners"`
}

type profileConfig struct {
	CurrentProfile string              `json:"current_profile"`
	Profiles       map[string]*profile `json:"profiles"`
}

type profile struct {
	Server string `json:"server,omitempty"`
	Token  string `json:"token,omitempty"`
	Team   string `json:"team,omitempty"`
	App    string `json:"app,omitempty"`
}

type resolvedConnection struct {
	ProfileName string
	Server      string
	Token       string
	Team        string
	DefaultApp  string
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "dead":
		return true
	default:
		return false
	}
}
