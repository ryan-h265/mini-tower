package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"minitower/internal/auth"
	"minitower/internal/store"
)

// requireLeaseContext extracts the run ID from the URL path, the lease token
// from the X-Lease-Token header, and validates via GetActiveAttempt.
// On failure it writes the HTTP error and returns ok=false.
func (h *Handlers) requireLeaseContext(w http.ResponseWriter, r *http.Request, extractID func(string) int64) (runID int64, attempt *store.RunAttempt, leaseTokenHash string, ok bool) {
	runID = extractID(r.URL.Path)
	if runID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid run ID")
		return 0, nil, "", false
	}

	leaseToken := r.Header.Get("X-Lease-Token")
	if leaseToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing lease token")
		return 0, nil, "", false
	}
	leaseTokenHash = auth.HashToken(leaseToken)

	attempt, err := h.store.GetActiveAttempt(r.Context(), runID, leaseTokenHash)
	if writeStoreError(w, h.logger, err, "get active attempt") {
		return 0, nil, "", false
	}
	return runID, attempt, leaseTokenHash, true
}

// writeAttemptResponse fetches the run for cancel status and writes the
// standard attemptResponse JSON used by StartRun and HeartbeatRun.
func (h *Handlers) writeAttemptResponse(w http.ResponseWriter, r *http.Request, runID int64, attempt *store.RunAttempt) {
	teamID, _ := teamIDFromContext(r.Context())
	run, err := h.store.GetRunByID(r.Context(), teamID, runID)
	if err != nil {
		h.logger.Error("get run", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, attemptResponse{
		AttemptID:       attempt.ID,
		AttemptNo:       attempt.AttemptNo,
		Status:          attempt.Status,
		LeaseExpiresAt:  attempt.LeaseExpiresAt.Format(time.RFC3339),
		CancelRequested: run.CancelRequested,
		RunStatus:       run.Status,
	})
}

type registerRunnerRequest struct {
	Name string `json:"name"`
}

type registerRunnerResponse struct {
	RunnerID int64  `json:"runner_id"`
	Name     string `json:"name"`
	Token    string `json:"token"`
}

// RegisterRunner registers a new runner using a registration token.
func (h *Handlers) RegisterRunner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	var req registerRunnerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	// Check if runner already exists
	existing, err := h.store.GetRunnerByName(r.Context(), teamID, req.Name)
	if err != nil {
		h.logger.Error("check runner exists", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "conflict", "runner already exists")
		return
	}

	// Get default environment
	env, err := h.store.GetOrCreateDefaultEnvironment(r.Context(), teamID)
	if err != nil {
		h.logger.Error("get default environment", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	// Generate runner token
	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixRunnerToken)
	if err != nil {
		h.logger.Error("generate runner token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	runner, err := h.store.CreateRunner(r.Context(), teamID, req.Name, env.ID, tokenHash)
	if err != nil {
		h.logger.Error("create runner", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, registerRunnerResponse{
		RunnerID: runner.ID,
		Name:     runner.Name,
		Token:    token,
	})
}

type leaseResponse struct {
	RunID          int64          `json:"run_id"`
	RunNo          int64          `json:"run_no"`
	AppID          int64          `json:"app_id"`
	AppSlug        string         `json:"app_slug"`
	VersionNo      int64          `json:"version_no"`
	Entrypoint     string         `json:"entrypoint"`
	TimeoutSeconds *int           `json:"timeout_seconds,omitempty"`
	Input          map[string]any `json:"input,omitempty"`
	AttemptID      int64          `json:"attempt_id"`
	AttemptNo      int64          `json:"attempt_no"`
	LeaseToken     string         `json:"lease_token"`
	LeaseExpiresAt string         `json:"lease_expires_at"`
}

// LeaseRun attempts to lease a queued run.
func (h *Handlers) LeaseRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	runnerID, ok := runnerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing runner context")
		return
	}

	teamID, _ := teamIDFromContext(r.Context())
	envID, _ := environmentIDFromContext(r.Context())

	// Get runner
	runner := &store.Runner{
		ID:            runnerID,
		TeamID:        teamID,
		EnvironmentID: envID,
	}

	// Generate lease token
	leaseToken, leaseTokenHash, err := auth.GenerateToken()
	if err != nil {
		h.logger.Error("generate lease token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	run, attempt, err := h.store.LeaseRun(r.Context(), runner, leaseTokenHash, h.cfg.LeaseTTL)
	if errors.Is(err, store.ErrNoRunAvailable) {
		writeJSON(w, http.StatusNoContent, nil)
		return
	}
	if errors.Is(err, store.ErrLeaseConflict) {
		writeError(w, http.StatusConflict, "conflict", "runner already has an active lease")
		return
	}
	if err != nil {
		h.logger.Error("lease run", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	// Get app and version details
	app, err := h.store.GetAppByID(r.Context(), teamID, run.AppID)
	if err != nil || app == nil {
		h.logger.Error("get app for lease", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	version, err := h.store.GetVersionByID(r.Context(), run.AppVersionID)
	if err != nil || version == nil {
		h.logger.Error("get version for lease", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, leaseResponse{
		RunID:          run.ID,
		RunNo:          run.RunNo,
		AppID:          app.ID,
		AppSlug:        app.Slug,
		VersionNo:      version.VersionNo,
		Entrypoint:     version.Entrypoint,
		TimeoutSeconds: version.TimeoutSeconds,
		Input:          run.Input,
		AttemptID:      attempt.ID,
		AttemptNo:      attempt.AttemptNo,
		LeaseToken:     leaseToken,
		LeaseExpiresAt: attempt.LeaseExpiresAt.Format(time.RFC3339),
	})
}

type attemptResponse struct {
	AttemptID       int64  `json:"attempt_id"`
	AttemptNo       int64  `json:"attempt_no"`
	Status          string `json:"status"`
	LeaseExpiresAt  string `json:"lease_expires_at"`
	CancelRequested bool   `json:"cancel_requested"`
	RunStatus       string `json:"run_status"`
}

// StartRun acknowledges a lease and transitions to running.
func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	runID, attempt, leaseTokenHash, ok := h.requireLeaseContext(w, r, extractRunIDFromPath)
	if !ok {
		return
	}

	attempt, err := h.store.StartAttempt(r.Context(), attempt.ID, leaseTokenHash)
	if writeStoreError(w, h.logger, err, "attempt is cancelling") {
		return
	}

	h.writeAttemptResponse(w, r, runID, attempt)
}

// HeartbeatRun extends the lease.
func (h *Handlers) HeartbeatRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	runID, attempt, leaseTokenHash, ok := h.requireLeaseContext(w, r, extractRunIDFromPath)
	if !ok {
		return
	}

	attempt, err := h.store.ExtendLease(r.Context(), attempt.ID, leaseTokenHash, h.cfg.LeaseTTL)
	if writeStoreError(w, h.logger, err, "extend lease") {
		return
	}

	h.writeAttemptResponse(w, r, runID, attempt)
}

type logBatchRequest struct {
	Logs []logEntryRequest `json:"logs"`
}

type logEntryRequest struct {
	Seq      int64  `json:"seq"`
	Stream   string `json:"stream"`
	Line     string `json:"line"`
	LoggedAt string `json:"logged_at"`
}

// SubmitLogs submits a batch of log entries.
func (h *Handlers) SubmitLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	_, attempt, _, ok := h.requireLeaseContext(w, r, extractRunIDFromPath)
	if !ok {
		return
	}

	var req logBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if len(req.Logs) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if len(req.Logs) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_request", "max 100 logs per batch")
		return
	}

	// Convert to store format
	logs := make([]store.LogEntry, 0, len(req.Logs))
	for _, l := range req.Logs {
		if l.Stream != "stdout" && l.Stream != "stderr" {
			writeError(w, http.StatusBadRequest, "invalid_request", "stream must be stdout or stderr")
			return
		}
		if len(l.Line) > 8192 {
			writeError(w, http.StatusBadRequest, "invalid_request", "log line exceeds 8KB")
			return
		}
		loggedAt, err := time.Parse(time.RFC3339, l.LoggedAt)
		if err != nil {
			loggedAt = time.Now()
		}
		logs = append(logs, store.LogEntry{
			Seq:      l.Seq,
			Stream:   l.Stream,
			Line:     l.Line,
			LoggedAt: loggedAt,
		})
	}

	if err := h.store.AppendLogs(r.Context(), attempt.ID, logs); err != nil {
		h.logger.Error("append logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type resultRequest struct {
	Status       string  `json:"status"`
	ExitCode     *int    `json:"exit_code"`
	ErrorMessage *string `json:"error_message"`
}

// SubmitResult submits the final result of a run.
func (h *Handlers) SubmitResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	_, attempt, leaseTokenHash, ok := h.requireLeaseContext(w, r, extractRunIDFromPath)
	if !ok {
		return
	}

	var req resultRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"completed": true,
		"failed":    true,
		"cancelled": true,
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid_request", "status must be completed, failed, or cancelled")
		return
	}

	err := h.store.CompleteAttempt(r.Context(), attempt.ID, leaseTokenHash, req.Status, req.ExitCode, req.ErrorMessage)
	if writeStoreError(w, h.logger, err, "result conflicts with attempt state") {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetArtifact streams the version artifact for a run.
func (h *Handlers) GetArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing context")
		return
	}

	runID, _, _, ok := h.requireLeaseContext(w, r, extractRunIDFromArtifactPath)
	if !ok {
		return
	}

	// Get run and version
	run, err := h.store.GetRunByID(r.Context(), teamID, runID)
	if err != nil || run == nil {
		h.logger.Error("get run for artifact", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	version, err := h.store.GetVersionByID(r.Context(), run.AppVersionID)
	if err != nil || version == nil {
		h.logger.Error("get version for artifact", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	// Load artifact
	reader, err := h.objects.Load(version.ArtifactObjectKey)
	if err != nil {
		h.logger.Error("load artifact", "error", err, "key", version.ArtifactObjectKey)
		writeError(w, http.StatusInternalServerError, "internal", "artifact not found")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("X-Artifact-SHA256", version.ArtifactSHA256)
	w.Header().Set("X-Entrypoint", version.Entrypoint)
	if version.TimeoutSeconds != nil {
		w.Header().Set("X-Timeout-Seconds", strconv.Itoa(*version.TimeoutSeconds))
	}

	io.Copy(w, reader)
}

// extractRunIDFromArtifactPath extracts run ID from /api/v1/runs/{run}/artifact
func extractRunIDFromArtifactPath(path string) int64 {
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
