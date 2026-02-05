package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"minitower/internal/store"
	"minitower/internal/testutil"
)

func TestReapExpiredRequeueThenDead(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-reap")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-reap")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 1)

	runner, _ := testutil.CreateRunner(t, s, team.ID, env.ID, "runner-reap")
	_, attempt1, _, _ := testutil.LeaseRun(t, s, runner)
	expireAttempt(t, dbConn, attempt1.ID, time.Now().Add(-2*time.Minute))

	processed, err := s.ReapExpiredAttempts(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("reap attempts: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed attempt, got %d", processed)
	}

	assertAttemptStatus(t, dbConn, attempt1.ID, "expired")
	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "queued" {
		t.Fatalf("expected run queued, got %s", loaded.Status)
	}
	if loaded.RetryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", loaded.RetryCount)
	}

	_, attempt2, _, _ := testutil.LeaseRun(t, s, runner)
	expireAttempt(t, dbConn, attempt2.ID, time.Now().Add(-2*time.Minute))

	processed, err = s.ReapExpiredAttempts(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("reap attempts: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed attempt, got %d", processed)
	}

	assertAttemptStatus(t, dbConn, attempt2.ID, "expired")
	loaded, err = s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "dead" {
		t.Fatalf("expected run dead, got %s", loaded.Status)
	}
	if loaded.FinishedAt == nil {
		t.Fatalf("expected finished_at set")
	}
}

func TestReapExpiredCancelRequested(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-cancel")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 2)

	runner, _ := testutil.CreateRunner(t, s, team.ID, env.ID, "runner-cancel")
	_, attempt, _, _ := testutil.LeaseRun(t, s, runner)
	expireAttempt(t, dbConn, attempt.ID, time.Now().Add(-2*time.Minute))
	mustExec(t, dbConn, `UPDATE runs SET cancel_requested = 1 WHERE id = ?`, run.ID)

	processed, err := s.ReapExpiredAttempts(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("reap attempts: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed attempt, got %d", processed)
	}

	assertAttemptStatus(t, dbConn, attempt.ID, "cancelled")
	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "cancelled" {
		t.Fatalf("expected run cancelled, got %s", loaded.Status)
	}
}

func TestReapExpiredCancellingAttempt(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-cancelling")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancelling")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, team.ID, env.ID, "runner-cancelling")
	_, attempt, _, _ := testutil.LeaseRun(t, s, runner)
	expireAttempt(t, dbConn, attempt.ID, time.Now().Add(-2*time.Minute))
	mustExec(t, dbConn, `UPDATE run_attempts SET status = 'cancelling' WHERE id = ?`, attempt.ID)
	mustExec(t, dbConn, `UPDATE runs SET status = 'cancelling' WHERE id = ?`, run.ID)

	processed, err := s.ReapExpiredAttempts(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("reap attempts: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed attempt, got %d", processed)
	}

	assertAttemptStatus(t, dbConn, attempt.ID, "cancelled")
	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "cancelled" {
		t.Fatalf("expected run cancelled, got %s", loaded.Status)
	}
}

func TestLateResultAfterExpiryDoesNotResurrect(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-late-result")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-late-result")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, team.ID, env.ID, "runner-late-result")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)
	expireAttempt(t, dbConn, attempt.ID, time.Now().Add(-2*time.Minute))

	if _, err := s.ReapExpiredAttempts(ctx, time.Now(), 10); err != nil {
		t.Fatalf("reap attempts: %v", err)
	}

	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "dead" {
		t.Fatalf("expected run dead, got %s", loaded.Status)
	}

	exitCode := 1
	err = s.CompleteAttempt(ctx, attempt.ID, leaseHash, "failed", &exitCode, nil)
	if !errors.Is(err, store.ErrAttemptNotActive) {
		t.Fatalf("expected attempt not active, got %v", err)
	}

	loaded, err = s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "dead" {
		t.Fatalf("expected run dead, got %s", loaded.Status)
	}
}

func expireAttempt(t *testing.T, dbConn *sql.DB, attemptID int64, at time.Time) {
	t.Helper()
	_, err := dbConn.ExecContext(context.Background(),
		`UPDATE run_attempts SET lease_expires_at = ? WHERE id = ?`,
		at.UnixMilli(), attemptID,
	)
	if err != nil {
		t.Fatalf("expire attempt: %v", err)
	}
}

func assertAttemptStatus(t *testing.T, dbConn *sql.DB, attemptID int64, status string) {
	t.Helper()
	var current string
	err := dbConn.QueryRowContext(context.Background(),
		`SELECT status FROM run_attempts WHERE id = ?`,
		attemptID,
	).Scan(&current)
	if err != nil {
		t.Fatalf("get attempt status: %v", err)
	}
	if current != status {
		t.Fatalf("expected attempt status %s, got %s", status, current)
	}
}
