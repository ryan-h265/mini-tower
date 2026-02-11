package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"minitower/internal/config"
	"minitower/internal/httpapi"
	"minitower/internal/objects"
	"minitower/internal/store"
	"minitower/internal/testutil"
)

func TestGetMeReturnsRoleAndIdentity(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	team, token := testutil.CreateTeam(t, s, "team-me")

	resp := doRequest(t, handler, http.MethodGet, "/api/v1/me", token, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		TeamID   int64  `json:"team_id"`
		TeamSlug string `json:"team_slug"`
		TokenID  int64  `json:"token_id"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.TeamID != team.ID {
		t.Fatalf("expected team_id %d, got %d", team.ID, payload.TeamID)
	}
	if payload.TeamSlug != team.Slug {
		t.Fatalf("expected team_slug %q, got %q", team.Slug, payload.TeamSlug)
	}
	if payload.TokenID == 0 {
		t.Fatalf("expected non-zero token_id")
	}
	if payload.Role != "admin" {
		t.Fatalf("expected role admin, got %q", payload.Role)
	}
}

func TestSignupTeamCreatesTeamAndAllowsMultipleSlugs(t *testing.T) {
	handler, _, _, cleanup := newTestServer(t)
	defer cleanup()

	firstResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/signup", "", "", map[string]any{
		"slug":     "acme",
		"name":     "Acme Corp",
		"password": "acme-secret",
	})
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for first signup, got %d", firstResp.StatusCode)
	}
	var firstPayload struct {
		TeamID int64  `json:"team_id"`
		Slug   string `json:"slug"`
		Token  string `json:"token"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(firstResp.Body).Decode(&firstPayload); err != nil {
		t.Fatalf("decode first signup response: %v", err)
	}
	if firstPayload.TeamID == 0 || firstPayload.Token == "" || firstPayload.Slug != "acme" || firstPayload.Role != "admin" {
		t.Fatalf("unexpected first signup payload: %+v", firstPayload)
	}

	secondResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/signup", "", "", map[string]any{
		"slug":     "beta",
		"name":     "Beta Labs",
		"password": "beta-secret",
	})
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for second signup, got %d", secondResp.StatusCode)
	}
	var secondPayload struct {
		TeamID int64  `json:"team_id"`
		Slug   string `json:"slug"`
		Token  string `json:"token"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&secondPayload); err != nil {
		t.Fatalf("decode second signup response: %v", err)
	}
	if secondPayload.TeamID == 0 || secondPayload.Token == "" || secondPayload.Slug != "beta" || secondPayload.Role != "admin" {
		t.Fatalf("unexpected second signup payload: %+v", secondPayload)
	}
	if firstPayload.TeamID == secondPayload.TeamID {
		t.Fatalf("expected different teams for different slugs, got shared ID %d", firstPayload.TeamID)
	}
}

func TestSignupTeamRejectsDuplicateSlug(t *testing.T) {
	handler, _, _, cleanup := newTestServer(t)
	defer cleanup()

	firstResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/signup", "", "", map[string]any{
		"slug":     "acme",
		"name":     "Acme Corp",
		"password": "secret",
	})
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for first signup, got %d", firstResp.StatusCode)
	}

	secondResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/signup", "", "", map[string]any{
		"slug":     "acme",
		"name":     "Acme Duplicate",
		"password": "other",
	})
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate signup slug, got %d", secondResp.StatusCode)
	}

	var errPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode duplicate signup error: %v", err)
	}
	if errPayload.Error.Code != "slug_taken" {
		t.Fatalf("expected slug_taken error code, got %q", errPayload.Error.Code)
	}
}

func TestSignupTeamDisabledReturnsForbidden(t *testing.T) {
	handler, _, _, cleanup := newTestServerWithAuthConfig(t, false, "test")
	defer cleanup()

	resp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/signup", "", "", map[string]any{
		"slug":     "acme",
		"name":     "Acme Corp",
		"password": "secret",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when signup is disabled, got %d", resp.StatusCode)
	}
}

func TestCreateTokenRoleAssignment(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	_, adminToken := testutil.CreateTeam(t, s, "team-role-admin")
	_, memberToken := testutil.CreateTeamWithRole(t, s, "team-role-member", "member")

	adminResp := doRequest(t, handler, http.MethodPost, "/api/v1/tokens", adminToken, "", map[string]any{"role": "admin"})
	defer adminResp.Body.Close()
	if adminResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for admin token creation, got %d", adminResp.StatusCode)
	}
	var adminPayload struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(adminResp.Body).Decode(&adminPayload); err != nil {
		t.Fatalf("decode admin token response: %v", err)
	}
	if adminPayload.Role != "admin" {
		t.Fatalf("expected admin-created token role admin, got %q", adminPayload.Role)
	}

	memberResp := doRequest(t, handler, http.MethodPost, "/api/v1/tokens", memberToken, "", map[string]any{"role": "admin"})
	defer memberResp.Body.Close()
	if memberResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for member token creation, got %d", memberResp.StatusCode)
	}
	var memberPayload struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(memberResp.Body).Decode(&memberPayload); err != nil {
		t.Fatalf("decode member token response: %v", err)
	}
	if memberPayload.Role != "member" {
		t.Fatalf("expected member-created token role member, got %q", memberPayload.Role)
	}

	invalidResp := doRequest(t, handler, http.MethodPost, "/api/v1/tokens", adminToken, "", map[string]any{"role": "owner"})
	defer invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d", invalidResp.StatusCode)
	}
}

func TestBootstrapTeamReusesExistingSlugAndResetsPassword(t *testing.T) {
	handler, _, _, cleanup := newTestServer(t)
	defer cleanup()

	firstResp := doRequest(t, handler, http.MethodPost, "/api/v1/bootstrap/team", "test", "", map[string]any{
		"slug":     "demo",
		"name":     "Demo Team",
		"password": "old-password",
	})
	if firstResp.StatusCode != http.StatusCreated {
		defer firstResp.Body.Close()
		t.Fatalf("expected 201 for first bootstrap, got %d", firstResp.StatusCode)
	}
	var firstPayload struct {
		TeamID int64  `json:"team_id"`
		Token  string `json:"token"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(firstResp.Body).Decode(&firstPayload); err != nil {
		firstResp.Body.Close()
		t.Fatalf("decode first bootstrap response: %v", err)
	}
	firstResp.Body.Close()
	if firstPayload.TeamID == 0 || firstPayload.Token == "" || firstPayload.Role != "admin" {
		t.Fatalf("unexpected first bootstrap payload: %+v", firstPayload)
	}

	secondResp := doRequest(t, handler, http.MethodPost, "/api/v1/bootstrap/team", "test", "", map[string]any{
		"slug":     "demo",
		"name":     "Demo Team",
		"password": "demo",
	})
	if secondResp.StatusCode != http.StatusOK {
		defer secondResp.Body.Close()
		t.Fatalf("expected 200 for same-slug bootstrap, got %d", secondResp.StatusCode)
	}
	var secondPayload struct {
		TeamID int64  `json:"team_id"`
		Token  string `json:"token"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&secondPayload); err != nil {
		secondResp.Body.Close()
		t.Fatalf("decode second bootstrap response: %v", err)
	}
	secondResp.Body.Close()
	if secondPayload.TeamID != firstPayload.TeamID || secondPayload.Token == "" || secondPayload.Role != "admin" {
		t.Fatalf("unexpected second bootstrap payload: %+v", secondPayload)
	}

	oldLoginResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/login", "", "", map[string]any{
		"slug":     "demo",
		"password": "old-password",
	})
	defer oldLoginResp.Body.Close()
	if oldLoginResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected old password login to fail with 401, got %d", oldLoginResp.StatusCode)
	}

	loginResp := doRequest(t, handler, http.MethodPost, "/api/v1/teams/login", "", "", map[string]any{
		"slug":     "demo",
		"password": "demo",
	})
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected updated password login to return 201, got %d", loginResp.StatusCode)
	}
	var loginPayload struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginPayload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginPayload.Token == "" || loginPayload.Role != "admin" {
		t.Fatalf("unexpected login payload after password reset: %+v", loginPayload)
	}
}

func TestBootstrapTeamRejectsDifferentSlugWhenTeamAlreadyExists(t *testing.T) {
	handler, _, _, cleanup := newTestServer(t)
	defer cleanup()

	firstResp := doRequest(t, handler, http.MethodPost, "/api/v1/bootstrap/team", "test", "", map[string]any{
		"slug":     "demo",
		"name":     "Demo Team",
		"password": "demo",
	})
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for first bootstrap, got %d", firstResp.StatusCode)
	}

	secondResp := doRequest(t, handler, http.MethodPost, "/api/v1/bootstrap/team", "test", "", map[string]any{
		"slug":     "other",
		"name":     "Other Team",
		"password": "other",
	})
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for second slug bootstrap, got %d", secondResp.StatusCode)
	}

	var errPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode second bootstrap error: %v", err)
	}
	if errPayload.Error.Code != "team_exists" {
		t.Fatalf("expected team_exists error code, got %q", errPayload.Error.Code)
	}
}

func TestAuthOptionsReflectServerConfig(t *testing.T) {
	enabledHandler, _, _, enabledCleanup := newTestServerWithAuthConfig(t, true, "test")
	defer enabledCleanup()

	enabledResp := doRequest(t, enabledHandler, http.MethodGet, "/api/v1/auth/options", "", "", nil)
	defer enabledResp.Body.Close()
	if enabledResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for auth options with bootstrap token, got %d", enabledResp.StatusCode)
	}
	var enabledPayload struct {
		SignupEnabled    bool `json:"signup_enabled"`
		BootstrapEnabled bool `json:"bootstrap_enabled"`
	}
	if err := json.NewDecoder(enabledResp.Body).Decode(&enabledPayload); err != nil {
		t.Fatalf("decode enabled auth options response: %v", err)
	}
	if !enabledPayload.SignupEnabled || !enabledPayload.BootstrapEnabled {
		t.Fatalf("unexpected enabled auth options payload: %+v", enabledPayload)
	}

	disabledBootstrapHandler, _, _, disabledBootstrapCleanup := newTestServerWithAuthConfig(t, true, "")
	defer disabledBootstrapCleanup()

	disabledBootstrapResp := doRequest(t, disabledBootstrapHandler, http.MethodGet, "/api/v1/auth/options", "", "", nil)
	defer disabledBootstrapResp.Body.Close()
	if disabledBootstrapResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for auth options without bootstrap token, got %d", disabledBootstrapResp.StatusCode)
	}
	var disabledBootstrapPayload struct {
		SignupEnabled    bool `json:"signup_enabled"`
		BootstrapEnabled bool `json:"bootstrap_enabled"`
	}
	if err := json.NewDecoder(disabledBootstrapResp.Body).Decode(&disabledBootstrapPayload); err != nil {
		t.Fatalf("decode disabled-bootstrap auth options response: %v", err)
	}
	if !disabledBootstrapPayload.SignupEnabled || disabledBootstrapPayload.BootstrapEnabled {
		t.Fatalf("unexpected disabled-bootstrap auth options payload: %+v", disabledBootstrapPayload)
	}

	bootstrapResp := doRequest(t, disabledBootstrapHandler, http.MethodPost, "/api/v1/bootstrap/team", "test", "", map[string]any{
		"slug":     "no-bootstrap",
		"name":     "No Bootstrap",
		"password": "secret",
	})
	defer bootstrapResp.Body.Close()
	if bootstrapResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when bootstrap route is disabled, got %d", bootstrapResp.StatusCode)
	}
}

func TestGlobalRunsSummaryDetailAndIncrementalLogs(t *testing.T) {
	handler, s, dbConn, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	team, token := testutil.CreateTeam(t, s, "team-runs-api")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	appA := testutil.CreateApp(t, s, team.ID, "app-a")
	appB := testutil.CreateApp(t, s, team.ID, "app-b")
	verA := testutil.CreateVersion(t, s, appA.ID)
	verB := testutil.CreateVersion(t, s, appB.ID)

	runQueued := testutil.CreateRun(t, s, team.ID, appA.ID, env.ID, verA.ID, 0, 0)
	runRunning := testutil.CreateRun(t, s, team.ID, appB.ID, env.ID, verB.ID, 0, 0)
	runLeased := testutil.CreateRun(t, s, team.ID, appB.ID, env.ID, verB.ID, 0, 0)
	runFailed := testutil.CreateRun(t, s, team.ID, appA.ID, env.ID, verA.ID, 0, 0)

	mustExecHTTP(t, dbConn, `UPDATE runs SET status = 'queued', queued_at = ? WHERE id = ?`, 1000, runQueued.ID)
	mustExecHTTP(t, dbConn, `UPDATE runs SET status = 'running', queued_at = ? WHERE id = ?`, 500, runRunning.ID)
	mustExecHTTP(t, dbConn, `UPDATE runs SET status = 'leased', queued_at = ? WHERE id = ?`, 1500, runLeased.ID)
	mustExecHTTP(t, dbConn, `UPDATE runs SET status = 'failed', queued_at = ? WHERE id = ?`, 2500, runFailed.ID)

	runsResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs", token, "", nil)
	defer runsResp.Body.Close()
	if runsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /runs, got %d", runsResp.StatusCode)
	}
	var runsPayload struct {
		Runs []struct {
			RunID   int64  `json:"run_id"`
			AppSlug string `json:"app_slug"`
			Status  string `json:"status"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(runsResp.Body).Decode(&runsPayload); err != nil {
		t.Fatalf("decode runs response: %v", err)
	}
	if len(runsPayload.Runs) != 4 {
		t.Fatalf("expected 4 runs, got %d", len(runsPayload.Runs))
	}
	if runsPayload.Runs[0].RunID != runRunning.ID || runsPayload.Runs[1].RunID != runLeased.ID || runsPayload.Runs[2].RunID != runQueued.ID || runsPayload.Runs[3].RunID != runFailed.ID {
		t.Fatalf("unexpected run order: %+v", runsPayload.Runs)
	}
	if runsPayload.Runs[0].AppSlug != "app-b" || runsPayload.Runs[2].AppSlug != "app-a" {
		t.Fatalf("expected app slugs in global runs, got %+v", runsPayload.Runs)
	}

	statusResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs?status=queued", token, "", nil)
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for status filter, got %d", statusResp.StatusCode)
	}
	var statusPayload struct {
		Runs []struct {
			RunID int64 `json:"run_id"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&statusPayload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if len(statusPayload.Runs) != 1 || statusPayload.Runs[0].RunID != runQueued.ID {
		t.Fatalf("unexpected status filter result: %+v", statusPayload.Runs)
	}

	appResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs?app=app-a", token, "", nil)
	defer appResp.Body.Close()
	if appResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for app filter, got %d", appResp.StatusCode)
	}
	var appPayload struct {
		Runs []struct {
			AppSlug string `json:"app_slug"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(appResp.Body).Decode(&appPayload); err != nil {
		t.Fatalf("decode app filter response: %v", err)
	}
	if len(appPayload.Runs) != 2 {
		t.Fatalf("expected 2 app-a runs, got %d", len(appPayload.Runs))
	}
	for _, run := range appPayload.Runs {
		if run.AppSlug != "app-a" {
			t.Fatalf("expected app-a slug, got %q", run.AppSlug)
		}
	}

	summaryResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs/summary", token, "", nil)
	defer summaryResp.Body.Close()
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for runs summary, got %d", summaryResp.StatusCode)
	}
	var summaryPayload struct {
		TotalRuns    int64 `json:"total_runs"`
		ActiveRuns   int64 `json:"active_runs"`
		QueuedRuns   int64 `json:"queued_runs"`
		TerminalRuns int64 `json:"terminal_runs"`
	}
	if err := json.NewDecoder(summaryResp.Body).Decode(&summaryPayload); err != nil {
		t.Fatalf("decode summary response: %v", err)
	}
	if summaryPayload.TotalRuns != 4 || summaryPayload.ActiveRuns != 2 || summaryPayload.QueuedRuns != 1 || summaryPayload.TerminalRuns != 1 {
		t.Fatalf("unexpected summary: %+v", summaryPayload)
	}

	detailResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs/"+itoa(runQueued.ID), token, "", nil)
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for run detail, got %d", detailResp.StatusCode)
	}
	var detailPayload struct {
		AppSlug string `json:"app_slug"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailPayload); err != nil {
		t.Fatalf("decode run detail: %v", err)
	}
	if detailPayload.AppSlug != "app-a" {
		t.Fatalf("expected app_slug app-a, got %q", detailPayload.AppSlug)
	}

	runner, _ := testutil.CreateRunner(t, s, "runner-log-api", "default")
	leasedRun, attempt, _, _ := testutil.LeaseRun(t, s, runner)
	if leasedRun.ID != runQueued.ID {
		t.Fatalf("expected queued run %d to be leased, got %d", runQueued.ID, leasedRun.ID)
	}
	if err := s.AppendLogs(ctx, attempt.ID, []store.LogEntry{
		{Seq: 1, Stream: "stdout", Line: "one", LoggedAt: time.Now()},
		{Seq: 2, Stream: "stdout", Line: "two", LoggedAt: time.Now()},
		{Seq: 3, Stream: "stderr", Line: "three", LoggedAt: time.Now()},
	}); err != nil {
		t.Fatalf("append logs: %v", err)
	}

	logsResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs/"+itoa(runQueued.ID)+"/logs?after_seq=1", token, "", nil)
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for logs, got %d", logsResp.StatusCode)
	}
	var logsPayload struct {
		Logs []struct {
			Seq int64 `json:"seq"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsPayload); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	if len(logsPayload.Logs) != 2 || logsPayload.Logs[0].Seq != 2 || logsPayload.Logs[1].Seq != 3 {
		t.Fatalf("unexpected incremental logs payload: %+v", logsPayload.Logs)
	}

	badAfterSeqResp := doRequest(t, handler, http.MethodGet, "/api/v1/runs/"+itoa(runQueued.ID)+"/logs?after_seq=-1", token, "", nil)
	defer badAfterSeqResp.Body.Close()
	if badAfterSeqResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid after_seq, got %d", badAfterSeqResp.StatusCode)
	}
}

func TestAdminRunnersEndpointIsAdminOnly(t *testing.T) {
	handler, s, _, cleanup := newTestServer(t)
	defer cleanup()

	_, _ = testutil.CreateRunner(t, s, "runner-admin-view", "default")
	_, adminToken := testutil.CreateTeam(t, s, "team-admin-access")
	_, memberToken := testutil.CreateTeamWithRole(t, s, "team-member-access", "member")

	memberResp := doRequest(t, handler, http.MethodGet, "/api/v1/admin/runners", memberToken, "", nil)
	defer memberResp.Body.Close()
	if memberResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for member access, got %d", memberResp.StatusCode)
	}

	adminResp := doRequest(t, handler, http.MethodGet, "/api/v1/admin/runners", adminToken, "", nil)
	defer adminResp.Body.Close()
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for admin access, got %d", adminResp.StatusCode)
	}
	var payload struct {
		Runners []struct {
			Name string `json:"name"`
		} `json:"runners"`
	}
	if err := json.NewDecoder(adminResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin runners response: %v", err)
	}
	if len(payload.Runners) != 1 || payload.Runners[0].Name != "runner-admin-view" {
		t.Fatalf("unexpected runners payload: %+v", payload.Runners)
	}
}

func TestCORSAllowlistPreflightAndOriginReflection(t *testing.T) {
	handler, _, _, cleanup := newTestServerWithCORS(t, []string{"http://localhost:5173"})
	defer cleanup()

	allowedReq := httptest.NewRequest(http.MethodOptions, "http://example/api/v1/apps", nil)
	allowedReq.Header.Set("Origin", "http://localhost:5173")
	allowedReq.Header.Set("Access-Control-Request-Method", "GET")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowedReq)
	allowedResp := allowedRec.Result()
	defer allowedResp.Body.Close()
	if allowedResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for allowed preflight, got %d", allowedResp.StatusCode)
	}
	if got := allowedResp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("unexpected allow-origin header: %q", got)
	}
	if got := allowedResp.Header.Get("Vary"); got == "" {
		t.Fatalf("expected Vary header to be set")
	}

	disallowedReq := httptest.NewRequest(http.MethodOptions, "http://example/api/v1/apps", nil)
	disallowedReq.Header.Set("Origin", "http://evil.example")
	disallowedReq.Header.Set("Access-Control-Request-Method", "GET")
	disallowedRec := httptest.NewRecorder()
	handler.ServeHTTP(disallowedRec, disallowedReq)
	disallowedResp := disallowedRec.Result()
	defer disallowedResp.Body.Close()
	if disallowedResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for disallowed preflight, got %d", disallowedResp.StatusCode)
	}
	if got := disallowedResp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow-origin for disallowed origin, got %q", got)
	}
}

func newTestServerWithCORS(t *testing.T, corsOrigins []string) (http.Handler, *store.Store, *sql.DB, func()) {
	return newTestServerWithOptions(t, corsOrigins, true, "test")
}

func newTestServerWithAuthConfig(t *testing.T, signupEnabled bool, bootstrapToken string) (http.Handler, *store.Store, *sql.DB, func()) {
	return newTestServerWithOptions(t, nil, signupEnabled, bootstrapToken)
}

func newTestServerWithOptions(t *testing.T, corsOrigins []string, signupEnabled bool, bootstrapToken string) (http.Handler, *store.Store, *sql.DB, func()) {
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
		BootstrapToken:          bootstrapToken,
		PublicSignupEnabled:     signupEnabled,
		RunnerRegistrationToken: "test-runner-reg",
		CORSOrigins:             corsOrigins,
		LeaseTTL:                60 * time.Second,
		ExpiryCheckInterval:     10 * time.Second,
		MaxRequestBodySize:      10 * 1024 * 1024,
		MaxArtifactSize:         100 * 1024 * 1024,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := prometheus.NewRegistry()
	api := httpapi.New(cfg, dbConn, objStore, logger, httpapi.WithPrometheusRegisterer(reg))

	return api.Handler(), s, dbConn, func() {
		cleanup.Close(t)
	}
}
