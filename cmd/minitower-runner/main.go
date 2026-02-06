package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ServerURL         string
	RunnerName        string
	RegistrationToken string
	Environment       string
	DataDir           string
	PythonBin         string
	PollInterval      time.Duration
	KillGracePeriod   time.Duration
}

var ErrStaleLease = errors.New("stale lease")

const (
	leaseSkew            = 5 * time.Second
	minHeartbeatInterval = 2 * time.Second
	defaultTimeout       = 300 * time.Second
	defaultLeaseExpiry   = 60 * time.Second
	logBatchSize         = 100
	logScanBufSize       = 8192
	logFlushInterval     = 2 * time.Second
)

// runState holds mutex-protected shared state for a run's lifetime.
type runState struct {
	mu              sync.Mutex
	leaseExpiry     time.Time
	cancelRequested bool
	staleLease      bool
	timedOut        bool
}

func newRunState(leaseExpiry time.Time) *runState {
	return &runState{leaseExpiry: leaseExpiry}
}

func (s *runState) setLeaseExpiry(t time.Time) {
	s.mu.Lock()
	s.leaseExpiry = t
	s.mu.Unlock()
}

func (s *runState) markCancel() {
	s.mu.Lock()
	s.cancelRequested = true
	s.mu.Unlock()
}

func (s *runState) markStale() {
	s.mu.Lock()
	s.staleLease = true
	s.mu.Unlock()
}

func (s *runState) markTimedOut() {
	s.mu.Lock()
	s.timedOut = true
	s.mu.Unlock()
}

func (s *runState) snapshot() (leaseExpiry time.Time, cancelRequested, staleLease, timedOut bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leaseExpiry, s.cancelRequested, s.staleLease, s.timedOut
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		DataDir:         os.Getenv("MINITOWER_DATA_DIR"),
		PythonBin:       os.Getenv("MINITOWER_PYTHON_BIN"),
		PollInterval:    3 * time.Second,
		KillGracePeriod: 10 * time.Second,
	}

	cfg.ServerURL = os.Getenv("MINITOWER_SERVER_URL")
	if cfg.ServerURL == "" {
		return nil, errors.New("MINITOWER_SERVER_URL is required")
	}

	cfg.RunnerName = os.Getenv("MINITOWER_RUNNER_NAME")
	if cfg.RunnerName == "" {
		return nil, errors.New("MINITOWER_RUNNER_NAME is required")
	}

	cfg.RegistrationToken = os.Getenv("MINITOWER_RUNNER_REGISTRATION_TOKEN")

	cfg.Environment = os.Getenv("MINITOWER_RUNNER_ENVIRONMENT")
	if cfg.Environment == "" {
		cfg.Environment = "default"
	}

	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = filepath.Join(home, ".minitower")
	}

	if cfg.PythonBin == "" {
		cfg.PythonBin = "python3"
	}

	if v := os.Getenv("MINITOWER_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			cfg.PollInterval = d
		}
	}

	if v := os.Getenv("MINITOWER_KILL_GRACE_PERIOD"); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			cfg.KillGracePeriod = d
		}
	}

	return cfg, nil
}

type Runner struct {
	cfg        *Config
	logger     *slog.Logger
	httpClient *http.Client
	token      string
	tokenPath  string
}

func NewRunner(cfg *Config, logger *slog.Logger) *Runner {
	return &Runner{
		cfg:        cfg,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenPath:  filepath.Join(cfg.DataDir, "runner_token"),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if err := os.MkdirAll(r.cfg.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Try to load saved token
	if data, err := os.ReadFile(r.tokenPath); err == nil {
		r.token = strings.TrimSpace(string(data))
		r.logger.Info("loaded saved token")
	}

	// Register if no token
	if r.token == "" {
		if r.cfg.RegistrationToken == "" {
			return errors.New("no saved token and MINITOWER_RUNNER_REGISTRATION_TOKEN not set")
		}
		if err := r.register(ctx); err != nil {
			return fmt.Errorf("register: %w", err)
		}
	}

	r.logger.Info("runner started", "name", r.cfg.RunnerName)

	// Main loop
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("shutting down")
			return nil
		default:
		}

		if err := r.poll(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			r.logger.Error("poll error", "error", err)
		}

		// Add jitter to poll interval
		jitter := time.Duration(rand.Int63n(int64(r.cfg.PollInterval / 2)))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.cfg.PollInterval + jitter):
		}
	}
}

func (r *Runner) register(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"name": r.cfg.RunnerName, "environment": r.cfg.Environment})
	req, err := http.NewRequestWithContext(ctx, "POST", r.cfg.ServerURL+"/api/v1/runners/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.cfg.RegistrationToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed: %d %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	r.token = result.Token
	if err := os.WriteFile(r.tokenPath, []byte(r.token), 0600); err != nil {
		r.logger.Warn("failed to save token", "error", err)
	}

	r.logger.Info("registered successfully")
	return nil
}

type LeaseResponse struct {
	RunID          int64          `json:"run_id"`
	RunNo          int64          `json:"run_no"`
	AppSlug        string         `json:"app_slug"`
	VersionNo      int64          `json:"version_no"`
	Entrypoint     string         `json:"entrypoint"`
	TimeoutSeconds *int           `json:"timeout_seconds"`
	Input          map[string]any `json:"input"`
	AttemptID      int64          `json:"attempt_id"`
	AttemptNo      int64          `json:"attempt_no"`
	LeaseToken     string         `json:"lease_token"`
	LeaseExpiresAt string         `json:"lease_expires_at"`
}

func (r *Runner) poll(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", r.cfg.ServerURL+"/api/v1/runs/lease", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil // No work available
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token might be invalid, try re-registering
		r.token = ""
		os.Remove(r.tokenPath)
		if r.cfg.RegistrationToken != "" {
			return r.register(ctx)
		}
		return errors.New("unauthorized and no registration token")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lease failed: %d %s", resp.StatusCode, string(respBody))
	}

	var lease LeaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&lease); err != nil {
		return err
	}

	r.logger.Info("leased run", "run_id", lease.RunID, "app", lease.AppSlug, "attempt", lease.AttemptNo)

	return r.executeRun(ctx, &lease)
}

// prepareWorkspace creates a temp directory, downloads and unpacks the artifact,
// creates a venv, and installs requirements. Returns the work directory and a
// cleanup function. Propagates ErrStaleLease from download; other errors are
// submitted as user-facing failure messages.
func (r *Runner) prepareWorkspace(ctx context.Context, lease *LeaseResponse) (string, func(), error) {
	workDir, err := os.MkdirTemp("", fmt.Sprintf("minitower-run-%d-", lease.RunID))
	if err != nil {
		if submitErr := r.submitFailure(ctx, lease, "failed to create workspace"); submitErr != nil {
			return "", nil, submitErr
		}
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(workDir) }

	sha256Hash, err := r.downloadArtifact(ctx, lease, filepath.Join(workDir, "artifact.tar.gz"))
	if err != nil {
		r.logger.Error("artifact download failed", "error", err)
		cleanup()
		if errors.Is(err, ErrStaleLease) {
			return "", nil, ErrStaleLease
		}
		if submitErr := r.submitFailure(ctx, lease, "failed to download artifact"); submitErr != nil {
			return "", nil, submitErr
		}
		return "", nil, err
	}

	if err := r.unpackArtifact(filepath.Join(workDir, "artifact.tar.gz"), workDir); err != nil {
		r.logger.Error("unpack failed", "error", err)
		cleanup()
		if submitErr := r.submitFailure(ctx, lease, "failed to unpack artifact"); submitErr != nil {
			return "", nil, submitErr
		}
		return "", nil, err
	}
	r.logger.Info("artifact unpacked", "sha256", sha256Hash)

	venvPath := filepath.Join(workDir, ".venv")
	if err := r.createVenv(ctx, venvPath); err != nil {
		r.logger.Error("venv creation failed", "error", err)
		cleanup()
		if submitErr := r.submitFailure(ctx, lease, "failed to create venv"); submitErr != nil {
			return "", nil, submitErr
		}
		return "", nil, err
	}

	reqPath := filepath.Join(workDir, "requirements.txt")
	if _, err := os.Stat(reqPath); err == nil {
		if err := r.installRequirements(ctx, venvPath, reqPath); err != nil {
			r.logger.Error("requirements install failed", "error", err)
			cleanup()
			if submitErr := r.submitFailure(ctx, lease, "failed to install requirements"); submitErr != nil {
				return "", nil, submitErr
			}
			return "", nil, err
		}
	}

	return workDir, cleanup, nil
}

// runHeartbeat runs the heartbeat loop until the run context is cancelled.
func (r *Runner) runHeartbeat(runCtx context.Context, lease *LeaseResponse, state *runState, terminate func(string)) {
	for {
		expiry, _, _, _ := state.snapshot()
		interval := minHeartbeatInterval
		if expiry.After(time.Now()) {
			remaining := time.Until(expiry.Add(-leaseSkew))
			if remaining > 0 {
				interval = remaining / 3
				if interval < minHeartbeatInterval {
					interval = minHeartbeatInterval
				}
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-runCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		resp, err := r.heartbeat(context.Background(), lease)
		if err != nil {
			if errors.Is(err, ErrStaleLease) {
				r.logger.Warn("stale lease on heartbeat")
				state.markStale()
				terminate("stale lease")
				return
			}
			r.logger.Error("heartbeat failed", "error", err)
			expiry, _, _, _ := state.snapshot()
			if time.Now().After(expiry.Add(-leaseSkew)) {
				r.logger.Warn("lease expired, self-fencing")
				state.markStale()
				terminate("lease expired")
				return
			}
			continue
		}
		if t, err := time.Parse(time.RFC3339, resp.LeaseExpiresAt); err == nil {
			state.setLeaseExpiry(t)
		}
		if resp.CancelRequested {
			state.markCancel()
			terminate("cancel requested")
			return
		}
	}
}

// runProcess sets up and runs the user process, streams logs, and submits the final result.
// The heartbeat goroutine is already running; heartbeatDone closes when it exits.
func (r *Runner) runProcess(ctx context.Context, runCtx context.Context, cancel context.CancelFunc, lease *LeaseResponse, state *runState, workDir string, heartbeatDone <-chan struct{}, baseTerminate func(string)) error {
	venvPath := filepath.Join(workDir, ".venv")
	pythonBin := filepath.Join(venvPath, "bin", "python")
	entrypoint := filepath.Join(workDir, lease.Entrypoint)

	timeout := defaultTimeout
	if lease.TimeoutSeconds != nil {
		timeout = time.Duration(*lease.TimeoutSeconds) * time.Second
	}

	cmd := exec.Command(pythonBin, entrypoint)
	cmd.Dir = workDir

	if lease.Input != nil {
		inputJSON, _ := json.Marshal(lease.Input)
		cmd.Env = append(os.Environ(), "MINITOWER_INPUT="+string(inputJSON))
	}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	processDone := make(chan struct{})
	// Wrap baseTerminate to also signal the process.
	var killOnce sync.Once
	terminate := func(reason string) {
		baseTerminate(reason)
		killOnce.Do(func() {
			if cmd.Process == nil {
				return
			}
			_ = cmd.Process.Signal(syscall.SIGTERM)
			go func() {
				timer := time.NewTimer(r.cfg.KillGracePeriod)
				defer timer.Stop()
				select {
				case <-processDone:
					return
				case <-timer.C:
				}
				_ = cmd.Process.Kill()
			}()
		})
	}

	if runCtx.Err() != nil {
		<-heartbeatDone
		_, wasCancelled, isStale, _ := state.snapshot()
		if isStale {
			r.logger.Warn("stale lease before process start")
			return nil
		}
		if wasCancelled {
			r.logger.Info("run cancelled before process start")
			return r.submitResultSafe(ctx, lease, "cancelled", nil, nil)
		}
		return nil
	}

	if err := cmd.Start(); err != nil {
		cancel()
		<-heartbeatDone
		r.logger.Error("process start failed", "error", err)
		return r.submitFailure(ctx, lease, "failed to start process")
	}

	// Timeout watcher
	timeoutDone := make(chan struct{})
	go func() {
		defer close(timeoutDone)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-runCtx.Done():
			return
		case <-timer.C:
			state.markTimedOut()
			terminate("timeout")
		}
	}()

	// Stream logs
	lc := newLogCollector(r, lease, state, terminate)

	logFlushDone := make(chan struct{})
	go func() {
		defer close(logFlushDone)
		lc.periodicFlush(runCtx)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		lc.collect(runCtx, stdout, "stdout")
	}()
	go func() {
		defer wg.Done()
		lc.collect(runCtx, stderr, "stderr")
	}()

	// Wait for process
	waitErr := cmd.Wait()
	close(processDone)
	wg.Wait()
	cancel()
	<-heartbeatDone
	<-logFlushDone
	<-timeoutDone

	lc.flushRemaining()

	return r.submitFinalResult(ctx, lease, state, waitErr)
}

func (r *Runner) executeRun(ctx context.Context, lease *LeaseResponse) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Parse lease expiry
	leaseExpiry, err := time.Parse(time.RFC3339, lease.LeaseExpiresAt)
	if err != nil {
		leaseExpiry = time.Now().Add(defaultLeaseExpiry)
	}

	// Start the run
	startResp, err := r.startRun(runCtx, lease)
	if errors.Is(err, ErrStaleLease) {
		r.logger.Warn("stale lease on start")
		return nil
	}
	if err != nil {
		r.logger.Error("start failed", "error", err)
		return err
	}

	// Update lease expiry from response
	if t, err := time.Parse(time.RFC3339, startResp.LeaseExpiresAt); err == nil {
		leaseExpiry = t
	}

	// Check for early cancel
	if startResp.CancelRequested {
		r.logger.Info("run cancelled before start")
		return r.submitResultSafe(ctx, lease, "cancelled", nil, nil)
	}

	state := newRunState(leaseExpiry)

	// Start heartbeat immediately so the lease stays alive during workspace prep.
	heartbeatDone := make(chan struct{})
	var terminateOnce sync.Once
	terminate := func(reason string) {
		terminateOnce.Do(func() {
			r.logger.Warn("terminating run", "reason", reason)
			cancel()
		})
	}
	go func() {
		defer close(heartbeatDone)
		r.runHeartbeat(runCtx, lease, state, terminate)
	}()

	// Prepare workspace
	workDir, cleanup, err := r.prepareWorkspace(runCtx, lease)
	if err != nil {
		cancel()
		<-heartbeatDone
		if errors.Is(err, ErrStaleLease) {
			r.logger.Warn("stale lease during workspace preparation")
			return nil
		}
		// submitFailure already called inside prepareWorkspace
		return nil
	}
	defer cleanup()

	return r.runProcess(ctx, runCtx, cancel, lease, state, workDir, heartbeatDone, terminate)
}

type AttemptResponse struct {
	LeaseExpiresAt  string `json:"lease_expires_at"`
	CancelRequested bool   `json:"cancel_requested"`
}

func (r *Runner) startRun(ctx context.Context, lease *LeaseResponse) (*AttemptResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/runs/%d/start", r.cfg.ServerURL, lease.RunID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("X-Lease-Token", lease.LeaseToken)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if isStaleLeaseStatus(resp.StatusCode) {
			return nil, ErrStaleLease
		}
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("start failed: %d %s", resp.StatusCode, string(respBody))
	}

	var result AttemptResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *Runner) heartbeat(ctx context.Context, lease *LeaseResponse) (*AttemptResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/runs/%d/heartbeat", r.cfg.ServerURL, lease.RunID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("X-Lease-Token", lease.LeaseToken)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if isStaleLeaseStatus(resp.StatusCode) {
			return nil, ErrStaleLease
		}
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("heartbeat failed: %d %s", resp.StatusCode, string(respBody))
	}

	var result AttemptResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *Runner) downloadArtifact(ctx context.Context, lease *LeaseResponse, destPath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/v1/runs/%d/artifact", r.cfg.ServerURL, lease.RunID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("X-Lease-Token", lease.LeaseToken)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if isStaleLeaseStatus(resp.StatusCode) {
			return "", ErrStaleLease
		}
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("download failed: %d %s", resp.StatusCode, string(respBody))
	}

	expectedSHA256 := resp.Header.Get("X-Artifact-SHA256")

	f, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		return "", err
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && actualSHA256 != expectedSHA256 {
		return "", fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA256, actualSHA256)
	}

	return actualSHA256, nil
}

func (r *Runner) unpackArtifact(artifactPath, destDir string) error {
	cmd := exec.Command("tar", "-xzf", artifactPath, "-C", destDir)
	return cmd.Run()
}

func (r *Runner) createVenv(ctx context.Context, venvPath string) error {
	cmd := exec.CommandContext(ctx, r.cfg.PythonBin, "-m", "venv", venvPath)
	return cmd.Run()
}

func (r *Runner) installRequirements(ctx context.Context, venvPath, reqPath string) error {
	pip := filepath.Join(venvPath, "bin", "pip")
	cmd := exec.CommandContext(ctx, pip, "install", "-r", reqPath)
	return cmd.Run()
}

type logEntry struct {
	Seq      int64  `json:"seq"`
	Stream   string `json:"stream"`
	Line     string `json:"line"`
	LoggedAt string `json:"logged_at"`
}

// logCollector buffers log lines and flushes them in batches.
type logCollector struct {
	r     *Runner
	lease *LeaseResponse
	state *runState

	mu   sync.Mutex
	logs []logEntry
	seq  int64

	terminate func(string)
}

func newLogCollector(r *Runner, lease *LeaseResponse, state *runState, terminate func(string)) *logCollector {
	return &logCollector{
		r:         r,
		lease:     lease,
		state:     state,
		terminate: terminate,
	}
}

// collect reads lines from reader and appends them to the log buffer, flushing when the batch is full.
func (lc *logCollector) collect(ctx context.Context, reader io.Reader, stream string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, logScanBufSize), logScanBufSize)
	for scanner.Scan() {
		lc.mu.Lock()
		lc.seq++
		lc.logs = append(lc.logs, logEntry{
			Seq:      lc.seq,
			Stream:   stream,
			Line:     scanner.Text(),
			LoggedAt: time.Now().Format(time.RFC3339),
		})
		if len(lc.logs) >= logBatchSize {
			toFlush := lc.logs
			lc.logs = nil
			lc.mu.Unlock()
			if err := lc.r.flushLogs(ctx, lc.lease, toFlush); err != nil {
				if errors.Is(err, ErrStaleLease) {
					lc.r.logger.Warn("stale lease on log flush")
					lc.state.markStale()
					lc.terminate("stale lease")
					return
				}
				lc.r.logger.Warn("log flush failed", "error", err)
			}
		} else {
			lc.mu.Unlock()
		}
	}
}

// periodicFlush flushes buffered logs at regular intervals until ctx is cancelled.
func (lc *logCollector) periodicFlush(ctx context.Context) {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lc.flush(ctx)
		}
	}
}

// flush sends any buffered logs to the server.
func (lc *logCollector) flush(ctx context.Context) {
	lc.mu.Lock()
	if len(lc.logs) == 0 {
		lc.mu.Unlock()
		return
	}
	toFlush := lc.logs
	lc.logs = nil
	lc.mu.Unlock()
	if err := lc.r.flushLogs(ctx, lc.lease, toFlush); err != nil {
		if errors.Is(err, ErrStaleLease) {
			lc.r.logger.Warn("stale lease on log flush")
			lc.state.markStale()
			lc.terminate("stale lease")
			return
		}
		lc.r.logger.Warn("log flush failed", "error", err)
	}
}

// flushRemaining sends any remaining buffered logs using a background context.
func (lc *logCollector) flushRemaining() {
	_, _, isStale, _ := lc.state.snapshot()
	if isStale {
		return
	}
	lc.mu.Lock()
	if len(lc.logs) == 0 {
		lc.mu.Unlock()
		return
	}
	remaining := lc.logs
	lc.logs = nil
	lc.mu.Unlock()
	if err := lc.r.flushLogs(context.Background(), lc.lease, remaining); err != nil {
		if errors.Is(err, ErrStaleLease) {
			lc.r.logger.Warn("stale lease on final log flush")
			lc.state.markStale()
		} else {
			lc.r.logger.Warn("final log flush failed", "error", err)
		}
	}
}

func (r *Runner) flushLogs(ctx context.Context, lease *LeaseResponse, logs []logEntry) error {
	body, _ := json.Marshal(map[string]any{"logs": logs})
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/runs/%d/logs", r.cfg.ServerURL, lease.RunID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("X-Lease-Token", lease.LeaseToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if isStaleLeaseStatus(resp.StatusCode) {
			return ErrStaleLease
		}
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("log flush failed: %d %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (r *Runner) submitResult(ctx context.Context, lease *LeaseResponse, status string, exitCode *int, errorMessage *string) error {
	payload := map[string]any{
		"status": status,
	}
	if exitCode != nil {
		payload["exit_code"] = *exitCode
	}
	if errorMessage != nil {
		payload["error_message"] = *errorMessage
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/runs/%d/result", r.cfg.ServerURL, lease.RunID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("X-Lease-Token", lease.LeaseToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if isStaleLeaseStatus(resp.StatusCode) {
			return ErrStaleLease
		}
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("result failed: %d %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// submitResultSafe wraps submitResult and silently returns nil on stale lease.
func (r *Runner) submitResultSafe(ctx context.Context, lease *LeaseResponse, status string, exitCode *int, errorMessage *string) error {
	if err := r.submitResult(ctx, lease, status, exitCode, errorMessage); errors.Is(err, ErrStaleLease) {
		r.logger.Warn("stale lease on result submit")
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

// submitFailure is a convenience for submitting a failed status with an error message.
func (r *Runner) submitFailure(ctx context.Context, lease *LeaseResponse, errMsg string) error {
	return r.submitResultSafe(ctx, lease, "failed", nil, ptr(errMsg))
}

// submitFinalResult determines the final status from the run state and wait error, then submits.
func (r *Runner) submitFinalResult(ctx context.Context, lease *LeaseResponse, state *runState, waitErr error) error {
	_, wasCancelled, isStale, wasTimedOut := state.snapshot()
	if isStale {
		r.logger.Warn("stale lease, skipping result")
		return nil
	}

	if wasCancelled {
		r.logger.Info("run cancelled")
		return r.submitResultSafe(ctx, lease, "cancelled", nil, nil)
	}

	if wasTimedOut {
		r.logger.Info("run timed out")
		return r.submitResultSafe(ctx, lease, "failed", nil, ptr("timeout"))
	}

	if waitErr != nil {
		exitCode := 1
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		r.logger.Info("run failed", "exit_code", exitCode)
		return r.submitResultSafe(ctx, lease, "failed", &exitCode, ptr(waitErr.Error()))
	}

	exitCode := 0
	r.logger.Info("run completed", "exit_code", exitCode)
	return r.submitResultSafe(ctx, lease, "completed", &exitCode, nil)
}

func isStaleLeaseStatus(status int) bool {
	return status == http.StatusGone || status == http.StatusConflict
}

func ptr(s string) *string {
	return &s
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := NewRunner(cfg, logger)
	if err := runner.Run(ctx); err != nil {
		logger.Error("runner error", "error", err)
		os.Exit(1)
	}
}
