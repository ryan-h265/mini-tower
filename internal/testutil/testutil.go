package testutil

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"minitower/internal/auth"
	"minitower/internal/db"
	"minitower/internal/migrate"
	"minitower/internal/migrations"
	"minitower/internal/store"
)

func NewTestDB(t *testing.T) (*store.Store, *sql.DB, *dbCleanup) {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "minitower.db")
	conn, err := db.Open(ctx, path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	migrator := migrate.New(migrations.FS)
	if err := migrator.Apply(ctx, conn); err != nil {
		_ = conn.Close()
		t.Fatalf("apply migrations: %v", err)
	}

	cleanup := &dbCleanup{db: conn}
	return store.New(conn), conn, cleanup
}

type dbCleanup struct {
	db *sql.DB
}

func (c *dbCleanup) Close(t *testing.T) {
	t.Helper()
	if c == nil || c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}

func CreateTeam(t *testing.T, s *store.Store, slug string) (*store.Team, string) {
	return CreateTeamWithRole(t, s, slug, "admin")
}

func CreateTeamWithRole(t *testing.T, s *store.Store, slug, role string) (*store.Team, string) {
	t.Helper()
	ctx := context.Background()

	team, err := s.CreateTeam(ctx, slug, slug+" team")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	teamToken, teamTokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		t.Fatalf("generate team token: %v", err)
	}

	if _, err := s.CreateTeamToken(ctx, team.ID, teamTokenHash, nil, role); err != nil {
		t.Fatalf("create team token: %v", err)
	}

	return team, teamToken
}

func CreateRunner(t *testing.T, s *store.Store, name, environment string) (*store.Runner, string) {
	t.Helper()
	ctx := context.Background()

	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixRunnerToken)
	if err != nil {
		t.Fatalf("generate runner token: %v", err)
	}

	runner, err := s.CreateRunner(ctx, name, environment, tokenHash)
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	return runner, token
}

func CreateApp(t *testing.T, s *store.Store, teamID int64, slug string) *store.App {
	t.Helper()
	ctx := context.Background()

	app, err := s.CreateApp(ctx, teamID, slug, nil)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	return app
}

func CreateVersion(t *testing.T, s *store.Store, appID int64) *store.AppVersion {
	t.Helper()
	ctx := context.Background()

	version, err := s.CreateVersion(ctx, appID, "objects/fixture.tar.gz", "sha256", "main.py", nil, nil)
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	return version
}

func CreateRun(t *testing.T, s *store.Store, teamID, appID, envID, versionID int64, priority int, maxRetries int) *store.Run {
	t.Helper()
	ctx := context.Background()

	run, err := s.CreateRun(ctx, teamID, appID, envID, versionID, nil, priority, maxRetries)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func LeaseRun(t *testing.T, s *store.Store, runner *store.Runner) (*store.Run, *store.RunAttempt, string, string) {
	t.Helper()
	ctx := context.Background()

	leaseToken, leaseTokenHash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate lease token: %v", err)
	}

	run, attempt, err := s.LeaseRun(ctx, runner, leaseTokenHash, time.Minute)
	if err != nil {
		t.Fatalf("lease run: %v", err)
	}

	return run, attempt, leaseToken, leaseTokenHash
}
