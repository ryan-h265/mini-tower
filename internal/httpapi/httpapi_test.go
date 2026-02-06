package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"minitower/internal/auth"
	"minitower/internal/config"
	"minitower/internal/httpapi"
	"minitower/internal/objects"
	"minitower/internal/store"
	"minitower/internal/testutil"
)

func TestTeamTokenScopingBlocksCrossTeamAccess(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	teamA, tokenA := testutil.CreateTeam(t, s, "team-a")
	teamB, tokenB := testutil.CreateTeam(t, s, "team-b")

	_, err := s.CreateApp(ctx, teamB.ID, "app-b", nil)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	resp := doRequest(t, handler, http.MethodGet, "/api/v1/apps/app-b", tokenA, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-team access, got %d", resp.StatusCode)
	}

	resp = doRequest(t, handler, http.MethodGet, "/api/v1/apps/app-b", tokenB, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for team owner, got %d", resp.StatusCode)
	}

	_ = teamA
}

func TestRunnerEndpointsRejectStaleLeaseToken(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-runner")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-runner")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, runnerToken := testutil.CreateRunner(t, s, "runner-1", "default")
	leaseToken, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	badLeaseToken, _, _ := auth.GenerateToken()

	// start
	resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/start", runnerToken, badLeaseToken, nil)
	assertGone(t, resp)

	// heartbeat
	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/heartbeat", runnerToken, badLeaseToken, nil)
	assertGone(t, resp)

	// logs
	logsBody := map[string]any{
		"logs": []map[string]any{
			{
				"seq":       1,
				"stream":    "stdout",
				"line":      "hello",
				"logged_at": time.Now().Format(time.RFC3339),
			},
		},
	}
	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/logs", runnerToken, badLeaseToken, logsBody)
	assertGone(t, resp)

	// result
	resultBody := map[string]any{"status": "completed", "exit_code": 0}
	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/result", runnerToken, badLeaseToken, resultBody)
	assertGone(t, resp)

	// artifact
	resp = doRequest(t, handler, http.MethodGet, "/api/v1/runs/"+itoa(run.ID)+"/artifact", runnerToken, badLeaseToken, nil)
	assertGone(t, resp)

	_ = leaseToken
}

func TestRunnerStartHeartbeatResponsesIncludeLeaseFields(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-fields")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-fields")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, runnerToken := testutil.CreateRunner(t, s, "runner-fields", "default")
	leaseToken, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/start", runnerToken, leaseToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status: %d", resp.StatusCode)
	}
	assertLeaseFields(t, resp.Body)

	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/heartbeat", runnerToken, leaseToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status: %d", resp.StatusCode)
	}
	assertLeaseFields(t, resp.Body)
}

func TestCancelRunEndpointQueued(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, token := testutil.CreateTeam(t, s, "team-cancel-api")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-api")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/cancel", token, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "cancelled" {
		t.Fatalf("expected cancelled, got %v", payload["status"])
	}
	if payload["cancel_requested"] != true {
		t.Fatalf("expected cancel_requested true")
	}

	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/cancel", token, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel idempotent status: %d", resp.StatusCode)
	}
}

func TestCancelPropagationToRunnerHeartbeat(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, token := testutil.CreateTeam(t, s, "team-cancel-prop")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-prop")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, runnerToken := testutil.CreateRunner(t, s, "runner-cancel-prop", "default")
	leaseToken, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/cancel", token, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status: %d", resp.StatusCode)
	}

	resp = doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/heartbeat", runnerToken, leaseToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status: %d", resp.StatusCode)
	}
	assertCancelRequested(t, resp.Body)
}

func TestCancelResultRace(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, token := testutil.CreateTeam(t, s, "team-cancel-race")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-race")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, runnerToken := testutil.CreateRunner(t, s, "runner-cancel-race", "default")
	leaseToken, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var cancelStatus int
	var resultStatus int

	go func() {
		defer wg.Done()
		resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/cancel", token, "", nil)
		cancelStatus = resp.StatusCode
		resp.Body.Close()
	}()

	go func() {
		defer wg.Done()
		body := map[string]any{"status": "completed", "exit_code": 0}
		resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/result", runnerToken, leaseToken, body)
		resultStatus = resp.StatusCode
		resp.Body.Close()
	}()

	wg.Wait()

	if cancelStatus != http.StatusOK {
		t.Fatalf("cancel status: %d", cancelStatus)
	}
	if resultStatus != http.StatusOK && resultStatus != http.StatusConflict {
		t.Fatalf("result status: %d", resultStatus)
	}

	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "cancelled" && loaded.Status != "cancelling" && loaded.Status != "completed" {
		t.Fatalf("unexpected final status %s", loaded.Status)
	}
	if resultStatus == http.StatusOK && loaded.Status != "completed" {
		t.Fatalf("expected completed when result succeeded, got %s", loaded.Status)
	}
	if resultStatus == http.StatusConflict && loaded.Status == "completed" {
		t.Fatalf("expected non-completed when result conflicted, got %s", loaded.Status)
	}
}

func TestLateResultAfterExpiryReturnsGone(t *testing.T) {
	handler, s, dbConn, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-expiry-gone")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-expiry-gone")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, runnerToken := testutil.CreateRunner(t, s, "runner-expiry-gone", "default")
	leaseToken, leaseHash, _ := auth.GenerateToken()
	_, attempt, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute)
	if err != nil {
		t.Fatalf("lease run: %v", err)
	}

	mustExecHTTP(t, dbConn, `UPDATE run_attempts SET lease_expires_at = ? WHERE id = ?`, time.Now().Add(-2*time.Minute).UnixMilli(), attempt.ID)
	if _, err = s.ReapExpiredAttempts(ctx, time.Now(), 10); err != nil {
		t.Fatalf("reap attempts: %v", err)
	}

	body := map[string]any{"status": "failed", "exit_code": 1}
	resp := doRequest(t, handler, http.MethodPost, "/api/v1/runs/"+itoa(run.ID)+"/result", runnerToken, leaseToken, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected gone, got %d", resp.StatusCode)
	}

	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "dead" {
		t.Fatalf("expected run dead, got %s", loaded.Status)
	}
}

func newTestServer(t *testing.T) (http.Handler, *store.Store, *sql.DB, func()) {
	t.Helper()

	s, dbConn, cleanup := testutil.NewTestDB(t)
	objStore, err := objects.NewLocalStore(t.TempDir())
	if err != nil {
		cleanup.Close(t)
		t.Fatalf("objects store: %v", err)
	}

	cfg := config.Config{
		ListenAddr:              ":0",
		DBPath:                  "",
		ObjectsDir:              "",
		BootstrapToken:          "test",
		RunnerRegistrationToken: "test-runner-reg",
		LeaseTTL:                60 * time.Second,
		ExpiryCheckInterval:     10 * time.Second,
		MaxRequestBodySize:      10 * 1024 * 1024,
		MaxArtifactSize:         100 * 1024 * 1024,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Use a fresh registry per test to avoid duplicate registration errors
	reg := prometheus.NewRegistry()
	api := httpapi.New(cfg, dbConn, objStore, logger, httpapi.WithPrometheusRegisterer(reg))

	return api.Handler(), s, dbConn, func() {
		cleanup.Close(t)
	}
}

func doRequest(t *testing.T, handler http.Handler, method, path, bearerToken, leaseToken string, body any) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, "http://example"+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if leaseToken != "" {
		req.Header.Set("X-Lease-Token", leaseToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

func assertGone(t *testing.T, resp *http.Response) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410, got %d", resp.StatusCode)
	}
}

func assertLeaseFields(t *testing.T, body io.Reader) {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	leaseValue, ok := payload["lease_expires_at"].(string)
	if !ok || leaseValue == "" {
		t.Fatalf("missing lease_expires_at")
	}
	if _, ok := payload["cancel_requested"]; !ok {
		t.Fatalf("missing cancel_requested")
	}
}

func assertCancelRequested(t *testing.T, body io.Reader) {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["cancel_requested"] != true {
		t.Fatalf("expected cancel_requested true")
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func mustExecHTTP(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}
