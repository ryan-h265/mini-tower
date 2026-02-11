package store_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"minitower/internal/auth"
	"minitower/internal/store"
	"minitower/internal/testutil"
)

func TestLeaseRunConcurrentSingleAssignment(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-concurrent")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-concurrent")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner1, _ := testutil.CreateRunner(t, s, "runner-1", "default")
	runner2, _ := testutil.CreateRunner(t, s, "runner-2", "default")

	_, leaseHash1, _ := auth.GenerateToken()
	_, leaseHash2, _ := auth.GenerateToken()

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := s.LeaseRun(ctx, runner1, leaseHash1, time.Minute)
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, _, err := s.LeaseRun(ctx, runner2, leaseHash2, time.Minute)
		errs <- err
	}()
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, store.ErrNoRunAvailable) && !errors.Is(err, store.ErrLeaseConflict) {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf("expected 1 successful lease, got %d", successes)
	}

	var count int
	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_attempts WHERE run_id = ?`, run.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 attempt, got %d", count)
	}
}

func TestRunnerCannotLeaseSecondRun(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-lease")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-lease")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)
	run2 := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-1", "default")

	_, leaseHash1, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash1, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	_, leaseHash2, _ := auth.GenerateToken()
	_, _, err = s.LeaseRun(ctx, runner, leaseHash2, time.Minute)
	if !errors.Is(err, store.ErrLeaseConflict) {
		t.Fatalf("expected lease conflict, got %v", err)
	}

	loaded, err := s.GetRunByID(ctx, team.ID, run2.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "queued" {
		t.Fatalf("expected run to remain queued, got %s", loaded.Status)
	}
}

func TestQueueSelectionDeterministic(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-queue")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-queue")
	version := testutil.CreateVersion(t, s, app.ID)

	run1 := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)
	run2 := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 5, 0)
	run3 := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 5, 0)

	// Set queued_at to enforce ordering.
	mustExec(t, dbConn, `UPDATE runs SET queued_at = ? WHERE id = ?`, 2000, run2.ID)
	mustExec(t, dbConn, `UPDATE runs SET queued_at = ? WHERE id = ?`, 500, run3.ID)
	mustExec(t, dbConn, `UPDATE runs SET queued_at = ? WHERE id = ?`, 1000, run1.ID)

	runner, _ := testutil.CreateRunner(t, s, "runner-queue", "default")
	_, leaseHash, _ := auth.GenerateToken()
	leasedRun, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute)
	if err != nil {
		t.Fatalf("lease run: %v", err)
	}

	if leasedRun.ID != run3.ID {
		t.Fatalf("expected run %d, got %d", run3.ID, leasedRun.ID)
	}
}

func TestAppendLogsDedupe(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-logs")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-logs")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-logs", "default")
	_, attempt, _, _ := testutil.LeaseRun(t, s, runner)

	logs := []store.LogEntry{
		{Seq: 1, Stream: "stdout", Line: "one", LoggedAt: time.Now()},
		{Seq: 1, Stream: "stdout", Line: "dup", LoggedAt: time.Now()},
		{Seq: 2, Stream: "stderr", Line: "two", LoggedAt: time.Now()},
	}
	if err := s.AppendLogs(ctx, attempt.ID, logs); err != nil {
		t.Fatalf("append logs: %v", err)
	}

	var count int
	err = dbConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_logs WHERE run_attempt_id = ?`, attempt.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 logs, got %d", count)
	}
}

func TestStartAttemptIdempotent(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-start")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-start")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-start", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	a1, err := s.StartAttempt(ctx, attempt.ID, leaseHash)
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if a1.Status != "running" {
		t.Fatalf("expected running, got %s", a1.Status)
	}

	a2, err := s.StartAttempt(ctx, attempt.ID, leaseHash)
	if err != nil {
		t.Fatalf("idempotent start: %v", err)
	}
	if a2.Status != "running" {
		t.Fatalf("expected running, got %s", a2.Status)
	}
}

func TestStartAttemptReturnsCancellingOnCancelRace(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, token := testutil.CreateTeam(t, s, "team-start-cancel-race")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-start-cancel-race")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-start-cancel-race", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	updated, err := s.CancelRun(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if updated == nil || updated.Status != "cancelling" {
		t.Fatalf("expected cancelling run, got %#v", updated)
	}

	a, err := s.StartAttempt(ctx, attempt.ID, leaseHash)
	if err != nil {
		t.Fatalf("start attempt during cancellation: %v", err)
	}
	if a.Status != "cancelling" {
		t.Fatalf("expected cancelling attempt, got %s", a.Status)
	}

	loaded, err := s.GetRunByID(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Status != "cancelling" {
		t.Fatalf("expected run to remain cancelling, got %s", loaded.Status)
	}

	_ = token
}

func TestHeartbeatIdempotent(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-heartbeat")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-heartbeat")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-heartbeat", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	a1, err := s.ExtendLease(ctx, attempt.ID, leaseHash, time.Minute)
	if err != nil {
		t.Fatalf("extend lease: %v", err)
	}
	a2, err := s.ExtendLease(ctx, attempt.ID, leaseHash, time.Minute)
	if err != nil {
		t.Fatalf("extend lease again: %v", err)
	}
	if a2.LeaseExpiresAt.Before(a1.LeaseExpiresAt) {
		t.Fatalf("expected lease expiry to move forward")
	}
}

func TestCompleteAttemptIdempotent(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-complete")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-complete")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-complete", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	exitCode := 0
	if err := s.CompleteAttempt(ctx, attempt.ID, leaseHash, "completed", &exitCode, nil); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}
	if err := s.CompleteAttempt(ctx, attempt.ID, leaseHash, "completed", &exitCode, nil); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
}

func TestCompleteAttemptConflict(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-complete-conflict")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-complete-conflict")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-complete-conflict", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	exitCode := 0
	if err := s.CompleteAttempt(ctx, attempt.ID, leaseHash, "completed", &exitCode, nil); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}

	exitCode = 1
	err = s.CompleteAttempt(ctx, attempt.ID, leaseHash, "failed", &exitCode, nil)
	if !errors.Is(err, store.ErrLeaseConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestCancelQueuedRun(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-cancel-queued")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-queued")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	updated, err := s.CancelRun(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected run")
	}
	if updated.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", updated.Status)
	}
	if !updated.CancelRequested {
		t.Fatalf("expected cancel_requested true")
	}
	if updated.FinishedAt == nil {
		t.Fatalf("expected finished_at set")
	}

	updated, err = s.CancelRun(ctx, team.ID, run.ID)
	if err != nil {
		t.Fatalf("cancel run again: %v", err)
	}
	if updated.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", updated.Status)
	}
}

func TestCancelLeasedRunAndFinalize(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-cancel-leased")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-leased")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-cancel-leased", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	updated, err := s.CancelRun(ctx, team.ID, attempt.RunID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if updated.Status != "cancelling" {
		t.Fatalf("expected cancelling, got %s", updated.Status)
	}

	if status := getAttemptStatus(t, dbConn, attempt.ID); status != "cancelling" {
		t.Fatalf("expected attempt cancelling, got %s", status)
	}

	if err := s.CompleteAttempt(ctx, attempt.ID, leaseHash, "cancelled", nil, nil); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}

	updated, err = s.GetRunByID(ctx, team.ID, attempt.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", updated.Status)
	}
}

func TestCancelConflictsWithCompletedResult(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-cancel-conflict")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-cancel-conflict")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-cancel-conflict", "default")
	_, attempt, _, leaseHash := testutil.LeaseRun(t, s, runner)

	if _, err := s.CancelRun(ctx, team.ID, attempt.RunID); err != nil {
		t.Fatalf("cancel run: %v", err)
	}

	exitCode := 0
	err = s.CompleteAttempt(ctx, attempt.ID, leaseHash, "completed", &exitCode, nil)
	if !errors.Is(err, store.ErrLeaseConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if status := getAttemptStatus(t, dbConn, attempt.ID); status != "cancelling" {
		t.Fatalf("expected attempt cancelling, got %s", status)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func getAttemptStatus(t *testing.T, db *sql.DB, attemptID int64) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM run_attempts WHERE id = ?`, attemptID).Scan(&status); err != nil {
		t.Fatalf("get attempt status: %v", err)
	}
	return status
}

func TestRunnerStatusTransitionsOnlineToOffline(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()

	runner, _ := testutil.CreateRunner(t, s, "runner-status", "default")

	// Set last_seen_at to 10 minutes ago
	tenMinutesAgo := time.Now().Add(-10 * time.Minute).UnixMilli()
	mustExec(t, dbConn, `UPDATE runners SET last_seen_at = ? WHERE id = ?`, tenMinutesAgo, runner.ID)

	// Verify runner is online initially
	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "online" {
		t.Fatalf("expected online, got %s", loaded.Status)
	}

	// Mark stale runners offline (threshold = 5 minutes ago)
	threshold := time.Now().Add(-5 * time.Minute)
	marked, err := s.MarkStaleRunnersOffline(ctx, threshold)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if marked != 1 {
		t.Fatalf("expected 1 runner marked offline, got %d", marked)
	}

	// Verify runner is now offline
	loaded, err = s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "offline" {
		t.Fatalf("expected offline, got %s", loaded.Status)
	}
}

func TestRunnerStatusTransitionsOfflineToOnline(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()

	runner, _ := testutil.CreateRunner(t, s, "runner-status-online", "default")

	// Set runner to offline
	mustExec(t, dbConn, `UPDATE runners SET status = 'offline' WHERE id = ?`, runner.ID)

	// Verify runner is offline
	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "offline" {
		t.Fatalf("expected offline, got %s", loaded.Status)
	}

	// Mark runner online
	if err := s.MarkRunnerOnline(ctx, runner.ID); err != nil {
		t.Fatalf("mark online: %v", err)
	}

	// Verify runner is now online with updated last_seen_at
	loaded, err = s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "online" {
		t.Fatalf("expected online, got %s", loaded.Status)
	}
	if loaded.LastSeenAt == nil {
		t.Fatalf("expected last_seen_at to be set")
	}
}

func TestRunnerStaysOnlineWhenRecentlySeen(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()

	runner, _ := testutil.CreateRunner(t, s, "runner-status-recent", "default")

	// Set last_seen_at to 1 minute ago (recent)
	oneMinuteAgo := time.Now().Add(-1 * time.Minute).UnixMilli()
	mustExec(t, dbConn, `UPDATE runners SET last_seen_at = ? WHERE id = ?`, oneMinuteAgo, runner.ID)

	// Try to mark stale runners offline (threshold = 5 minutes ago)
	threshold := time.Now().Add(-5 * time.Minute)
	marked, err := s.MarkStaleRunnersOffline(ctx, threshold)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if marked != 0 {
		t.Fatalf("expected 0 runners marked offline, got %d", marked)
	}

	// Verify runner is still online
	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "online" {
		t.Fatalf("expected online, got %s", loaded.Status)
	}
}

func TestRunnerStatusTransitionsOfflineWhenLastSeenMissingAndStale(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	runner, _ := testutil.CreateRunner(t, s, "runner-status-null-seen", "default")

	tenMinutesAgo := time.Now().Add(-10 * time.Minute).UnixMilli()
	mustExec(t, dbConn, `UPDATE runners SET last_seen_at = NULL, updated_at = ? WHERE id = ?`, tenMinutesAgo, runner.ID)

	threshold := time.Now().Add(-5 * time.Minute)
	marked, err := s.MarkStaleRunnersOffline(ctx, threshold)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if marked != 1 {
		t.Fatalf("expected 1 runner marked offline, got %d", marked)
	}

	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded.Status != "offline" {
		t.Fatalf("expected offline, got %s", loaded.Status)
	}
}

func TestPruneOfflineRunnersDeletesStaleUnreferencedRunners(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	runner, _ := testutil.CreateRunner(t, s, "runner-prune-stale", "default")

	tenMinutesAgo := time.Now().Add(-10 * time.Minute).UnixMilli()
	mustExec(t, dbConn, `UPDATE runners SET status = 'offline', updated_at = ? WHERE id = ?`, tenMinutesAgo, runner.ID)

	pruned, err := s.PruneOfflineRunners(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("prune offline runners: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned runner, got %d", pruned)
	}

	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected runner to be deleted")
	}
}

func TestPruneOfflineRunnersSkipsReferencedRunners(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-prune-referenced")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-prune-referenced")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)
	runner, _ := testutil.CreateRunner(t, s, "runner-prune-referenced", "default")

	_, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	tenMinutesAgo := time.Now().Add(-10 * time.Minute).UnixMilli()
	mustExec(t, dbConn, `UPDATE runners SET status = 'offline', updated_at = ? WHERE id = ?`, tenMinutesAgo, runner.ID)

	pruned, err := s.PruneOfflineRunners(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("prune offline runners: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned runners, got %d", pruned)
	}

	loaded, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected referenced runner to remain")
	}
}

func TestLeaseRunUpdatesRunnerLastSeen(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-lease-seen")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-lease-seen")
	version := testutil.CreateVersion(t, s, app.ID)
	_ = testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)

	runner, _ := testutil.CreateRunner(t, s, "runner-lease-seen", "default")

	// Verify runner has no last_seen_at initially (or very recent)
	before, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Small delay to ensure timestamp difference

	// Lease a run
	_, leaseHash, _ := auth.GenerateToken()
	if _, _, err := s.LeaseRun(ctx, runner, leaseHash, time.Minute); err != nil {
		t.Fatalf("lease run: %v", err)
	}

	// Verify last_seen_at was updated
	after, err := s.GetRunnerByID(ctx, runner.ID)
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if after.LastSeenAt == nil {
		t.Fatalf("expected last_seen_at to be set")
	}
	if before.LastSeenAt != nil && !after.LastSeenAt.After(*before.LastSeenAt) {
		t.Fatalf("expected last_seen_at to be updated")
	}
}
