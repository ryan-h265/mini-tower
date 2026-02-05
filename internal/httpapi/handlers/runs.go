package handlers

import (
  "fmt"
  "net/http"
  "strconv"
  "strings"
  "time"

  "minitower/internal/store"
  "minitower/internal/validate"
)

type createRunRequest struct {
  Input      map[string]any `json:"input"`
  VersionNo  *int64         `json:"version_no"`
  Priority   *int           `json:"priority"`
  MaxRetries *int           `json:"max_retries"`
}

type runResponse struct {
  RunID           int64          `json:"run_id"`
  AppID           int64          `json:"app_id"`
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

// CreateRun creates a new run for an app.
func (h *Handlers) CreateRun(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }

  teamID, ok := teamIDFromContext(r.Context())
  if !ok {
    writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
    return
  }

  slug := extractAppSlugFromRunPath(r.URL.Path)
  if slug == "" {
    writeError(w, http.StatusBadRequest, "invalid_request", "missing app slug")
    return
  }

  app, err := h.store.GetAppBySlug(r.Context(), teamID, slug)
  if err != nil {
    h.logger.Error("get app", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }
  if app == nil {
    writeError(w, http.StatusNotFound, "not_found", "app not found")
    return
  }

  var req createRunRequest
  if err := decodeJSON(r, &req); err != nil {
    // Empty body is OK
    req = createRunRequest{}
  }

  // Get version to run
  var version *store.AppVersion
  if req.VersionNo != nil {
    v, err := h.store.GetVersionByNumber(r.Context(), app.ID, *req.VersionNo)
    if err != nil {
      h.logger.Error("get version", "error", err)
      writeError(w, http.StatusInternalServerError, "internal", "internal error")
      return
    }
    if v == nil {
      writeError(w, http.StatusNotFound, "not_found", "version not found")
      return
    }
    version = v
  } else {
    // Use latest version
    v, err := h.store.GetLatestVersion(r.Context(), app.ID)
    if err != nil {
      h.logger.Error("get latest version", "error", err)
      writeError(w, http.StatusInternalServerError, "internal", "internal error")
      return
    }
    if v == nil {
      writeError(w, http.StatusBadRequest, "no_version", "app has no versions")
      return
    }
    version = v
  }

  if version.ParamsSchema != nil {
    if err := validate.ValidateJSONInput(req.Input, version.ParamsSchema); err != nil {
      writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("input does not match schema: %s", err.Error()))
      return
    }
  }

  // Get or create default environment
  env, err := h.store.GetOrCreateDefaultEnvironment(r.Context(), teamID)
  if err != nil {
    h.logger.Error("get default environment", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }

  priority := 0
  if req.Priority != nil {
    priority = *req.Priority
  }

  maxRetries := 0
  if req.MaxRetries != nil {
    maxRetries = *req.MaxRetries
  }

  run, err := h.store.CreateRun(r.Context(), teamID, app.ID, env.ID, version.ID, req.Input, priority, maxRetries)
  if err != nil {
    h.logger.Error("create run", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }

  writeJSON(w, http.StatusCreated, runResponse{
    RunID:           run.ID,
    AppID:           run.AppID,
    RunNo:           run.RunNo,
    VersionNo:       version.VersionNo,
    Status:          run.Status,
    Input:           run.Input,
    Priority:        run.Priority,
    MaxRetries:      run.MaxRetries,
    RetryCount:      run.RetryCount,
    CancelRequested: run.CancelRequested,
    QueuedAt:        run.QueuedAt.Format(time.RFC3339),
  })
}

// ListRuns returns all runs for an app.
func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodGet {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }

  teamID, ok := teamIDFromContext(r.Context())
  if !ok {
    writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
    return
  }

  slug := extractAppSlugFromRunPath(r.URL.Path)
  if slug == "" {
    writeError(w, http.StatusBadRequest, "invalid_request", "missing app slug")
    return
  }

  app, err := h.store.GetAppBySlug(r.Context(), teamID, slug)
  if err != nil {
    h.logger.Error("get app", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }
  if app == nil {
    writeError(w, http.StatusNotFound, "not_found", "app not found")
    return
  }

  limit := 50
  offset := 0
  if l := r.URL.Query().Get("limit"); l != "" {
    if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 100 {
      limit = val
    }
  }
  if o := r.URL.Query().Get("offset"); o != "" {
    if val, err := strconv.Atoi(o); err == nil && val >= 0 {
      offset = val
    }
  }

  runs, err := h.store.ListRunsByApp(r.Context(), teamID, app.ID, limit, offset)
  if err != nil {
    h.logger.Error("list runs", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }

  resp := listRunsResponse{Runs: make([]runResponse, 0, len(runs))}
  for _, run := range runs {
    // Get version number
    v, err := h.store.GetVersionByID(r.Context(), run.AppVersionID)
    if err != nil {
      h.logger.Error("get version", "error", err)
      writeError(w, http.StatusInternalServerError, "internal", "internal error")
      return
    }

    rr := runResponse{
      RunID:           run.ID,
      AppID:           run.AppID,
      RunNo:           run.RunNo,
      Status:          run.Status,
      Input:           run.Input,
      Priority:        run.Priority,
      MaxRetries:      run.MaxRetries,
      RetryCount:      run.RetryCount,
      CancelRequested: run.CancelRequested,
      QueuedAt:        run.QueuedAt.Format(time.RFC3339),
    }
    if v != nil {
      rr.VersionNo = v.VersionNo
    }
    if run.StartedAt != nil {
      s := run.StartedAt.Format(time.RFC3339)
      rr.StartedAt = &s
    }
    if run.FinishedAt != nil {
      f := run.FinishedAt.Format(time.RFC3339)
      rr.FinishedAt = &f
    }
    resp.Runs = append(resp.Runs, rr)
  }

  writeJSON(w, http.StatusOK, resp)
}

// GetRun returns a single run by ID.
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodGet {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }

  teamID, ok := teamIDFromContext(r.Context())
  if !ok {
    writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
    return
  }

  runID := extractRunIDFromPath(r.URL.Path)
  if runID == 0 {
    writeError(w, http.StatusBadRequest, "invalid_request", "invalid run ID")
    return
  }

  run, err := h.store.GetRunByID(r.Context(), teamID, runID)
  if err != nil {
    h.logger.Error("get run", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }
  if run == nil {
    writeError(w, http.StatusNotFound, "not_found", "run not found")
    return
  }

  v, err := h.store.GetVersionByID(r.Context(), run.AppVersionID)
  if err != nil {
    h.logger.Error("get version", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }

  rr := runResponse{
    RunID:           run.ID,
    AppID:           run.AppID,
    RunNo:           run.RunNo,
    Status:          run.Status,
    Input:           run.Input,
    Priority:        run.Priority,
    MaxRetries:      run.MaxRetries,
    RetryCount:      run.RetryCount,
    CancelRequested: run.CancelRequested,
    QueuedAt:        run.QueuedAt.Format(time.RFC3339),
  }
  if v != nil {
    rr.VersionNo = v.VersionNo
  }
  if run.StartedAt != nil {
    s := run.StartedAt.Format(time.RFC3339)
    rr.StartedAt = &s
  }
  if run.FinishedAt != nil {
    f := run.FinishedAt.Format(time.RFC3339)
    rr.FinishedAt = &f
  }

  writeJSON(w, http.StatusOK, rr)
}

// CancelRun requests cancellation for a run.
func (h *Handlers) CancelRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	runID := extractRunIDFromPath(r.URL.Path)
	if runID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid run ID")
		return
	}

	run, err := h.store.CancelRun(r.Context(), teamID, runID)
	if err != nil {
		h.logger.Error("cancel run", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	v, err := h.store.GetVersionByID(r.Context(), run.AppVersionID)
	if err != nil {
		h.logger.Error("get version", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	rr := runResponse{
		RunID:           run.ID,
		AppID:           run.AppID,
		RunNo:           run.RunNo,
		Status:          run.Status,
		Input:           run.Input,
		Priority:        run.Priority,
		MaxRetries:      run.MaxRetries,
		RetryCount:      run.RetryCount,
		CancelRequested: run.CancelRequested,
		QueuedAt:        run.QueuedAt.Format(time.RFC3339),
	}
	if v != nil {
		rr.VersionNo = v.VersionNo
	}
	if run.StartedAt != nil {
		s := run.StartedAt.Format(time.RFC3339)
		rr.StartedAt = &s
	}
	if run.FinishedAt != nil {
		f := run.FinishedAt.Format(time.RFC3339)
		rr.FinishedAt = &f
	}

	writeJSON(w, http.StatusOK, rr)
}

// GetRunLogs returns logs for a run.
func (h *Handlers) GetRunLogs(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodGet {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }

  teamID, ok := teamIDFromContext(r.Context())
  if !ok {
    writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
    return
  }

  runID := extractRunIDFromLogsPath(r.URL.Path)
  if runID == 0 {
    writeError(w, http.StatusBadRequest, "invalid_request", "invalid run ID")
    return
  }

  // Verify run belongs to team
  run, err := h.store.GetRunByID(r.Context(), teamID, runID)
  if err != nil {
    h.logger.Error("get run", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }
  if run == nil {
    writeError(w, http.StatusNotFound, "not_found", "run not found")
    return
  }

  logs, err := h.store.GetRunLogs(r.Context(), runID)
  if err != nil {
    h.logger.Error("get run logs", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "internal error")
    return
  }

  resp := runLogsResponse{Logs: make([]runLogEntry, 0, len(logs))}
  for _, l := range logs {
    resp.Logs = append(resp.Logs, runLogEntry{
      Seq:      l.Seq,
      Stream:   l.Stream,
      Line:     l.Line,
      LoggedAt: l.LoggedAt.Format(time.RFC3339),
    })
  }

  writeJSON(w, http.StatusOK, resp)
}

// extractAppSlugFromRunPath extracts app slug from /api/v1/apps/{app}/runs
func extractAppSlugFromRunPath(path string) string {
  const prefix = "/api/v1/apps/"
  if !strings.HasPrefix(path, prefix) {
    return ""
  }
  rest := strings.TrimPrefix(path, prefix)
  parts := strings.SplitN(rest, "/", 2)
  if len(parts) == 0 {
    return ""
  }
  return parts[0]
}

// extractRunIDFromPath extracts run ID from /api/v1/runs/{run}
func extractRunIDFromPath(path string) int64 {
  const prefix = "/api/v1/runs/"
  if !strings.HasPrefix(path, prefix) {
    return 0
  }
  rest := strings.TrimPrefix(path, prefix)
  parts := strings.SplitN(rest, "/", 2)
  if len(parts) == 0 {
    return 0
  }
  id, err := strconv.ParseInt(parts[0], 10, 64)
  if err != nil {
    return 0
  }
  return id
}

// extractRunIDFromLogsPath extracts run ID from /api/v1/runs/{run}/logs
func extractRunIDFromLogsPath(path string) int64 {
  const prefix = "/api/v1/runs/"
  if !strings.HasPrefix(path, prefix) {
    return 0
  }
  rest := strings.TrimPrefix(path, prefix)
  parts := strings.Split(rest, "/")
  if len(parts) < 1 {
    return 0
  }
  id, err := strconv.ParseInt(parts[0], 10, 64)
  if err != nil {
    return 0
  }
  return id
}
