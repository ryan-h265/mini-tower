package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"minitower/internal/config"
	"minitower/internal/httpapi"
	"minitower/internal/httpapi/handlers"
	"minitower/internal/testutil"
)

func TestRequireTeamSetsRoleInContext(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	_, token := testutil.CreateTeamWithRole(t, s, "role-member-team", "member")

	authn := httpapi.NewAuth(config.Config{
		BootstrapToken:          "test",
		RunnerRegistrationToken: "test-runner",
	}, dbConn)

	protected := authn.RequireTeam(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := handlers.TokenRoleFromContext(r.Context())
		if !ok {
			t.Fatalf("expected role in context")
		}
		_, _ = io.WriteString(w, role)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.local/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "member" {
		t.Fatalf("expected member role, got %q", string(body))
	}
}

func TestRequireAdminBlocksMemberAllowsAdmin(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	_, memberToken := testutil.CreateTeamWithRole(t, s, "member-team", "member")
	_, adminToken := testutil.CreateTeamWithRole(t, s, "admin-team", "admin")

	authn := httpapi.NewAuth(config.Config{
		BootstrapToken:          "test",
		RunnerRegistrationToken: "test-runner",
	}, dbConn)

	protected := authn.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	memberReq := httptest.NewRequest(http.MethodGet, "http://example.local/admin", nil)
	memberReq.Header.Set("Authorization", "Bearer "+memberToken)
	memberRec := httptest.NewRecorder()
	protected.ServeHTTP(memberRec, memberReq)
	if memberRec.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for member, got %d", memberRec.Result().StatusCode)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "http://example.local/admin", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRec := httptest.NewRecorder()
	protected.ServeHTTP(adminRec, adminReq)
	if adminRec.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for admin, got %d", adminRec.Result().StatusCode)
	}
}
