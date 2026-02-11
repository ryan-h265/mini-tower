package store_test

import (
	"context"
	"testing"
	"time"

	"minitower/internal/store"
	"minitower/internal/testutil"
)

func TestCreateTeamTokenStoresRole(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, err := s.CreateTeam(ctx, "role-team", "Role Team")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	token, err := s.CreateTeamToken(ctx, team.ID, "token-hash", nil, "member")
	if err != nil {
		t.Fatalf("create team token: %v", err)
	}
	if token.Role != "member" {
		t.Fatalf("expected role member, got %q", token.Role)
	}
}

func TestListRunsByTeamAndSummary(t *testing.T) {
	s, dbConn, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-runs-global")
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

	mustExec(t, dbConn, `UPDATE runs SET status = 'queued', queued_at = ? WHERE id = ?`, 1000, runQueued.ID)
	mustExec(t, dbConn, `UPDATE runs SET status = 'running', queued_at = ? WHERE id = ?`, 500, runRunning.ID)
	mustExec(t, dbConn, `UPDATE runs SET status = 'leased', queued_at = ? WHERE id = ?`, 1500, runLeased.ID)
	mustExec(t, dbConn, `UPDATE runs SET status = 'failed', queued_at = ? WHERE id = ?`, 2500, runFailed.ID)

	runs, err := s.ListRunsByTeam(ctx, team.ID, 20, 0, "", "")
	if err != nil {
		t.Fatalf("list runs by team: %v", err)
	}
	if len(runs) != 4 {
		t.Fatalf("expected 4 runs, got %d", len(runs))
	}
	if runs[0].ID != runRunning.ID || runs[1].ID != runLeased.ID || runs[2].ID != runQueued.ID || runs[3].ID != runFailed.ID {
		t.Fatalf("unexpected run order: got [%d %d %d %d]", runs[0].ID, runs[1].ID, runs[2].ID, runs[3].ID)
	}
	if runs[0].AppSlug != "app-b" || runs[2].AppSlug != "app-a" {
		t.Fatalf("expected app slugs on runs, got %q and %q", runs[0].AppSlug, runs[2].AppSlug)
	}

	queuedRuns, err := s.ListRunsByTeam(ctx, team.ID, 20, 0, "queued", "")
	if err != nil {
		t.Fatalf("list queued runs: %v", err)
	}
	if len(queuedRuns) != 1 || queuedRuns[0].ID != runQueued.ID {
		t.Fatalf("expected only queued run %d, got %+v", runQueued.ID, queuedRuns)
	}

	appARuns, err := s.ListRunsByTeam(ctx, team.ID, 20, 0, "", "app-a")
	if err != nil {
		t.Fatalf("list app-a runs: %v", err)
	}
	if len(appARuns) != 2 {
		t.Fatalf("expected 2 app-a runs, got %d", len(appARuns))
	}
	for _, run := range appARuns {
		if run.AppSlug != "app-a" {
			t.Fatalf("expected app slug app-a, got %q", run.AppSlug)
		}
	}

	summary, err := s.GetRunSummaryByTeam(ctx, team.ID)
	if err != nil {
		t.Fatalf("get run summary: %v", err)
	}
	if summary.TotalRuns != 4 || summary.ActiveRuns != 2 || summary.QueuedRuns != 1 || summary.TerminalRuns != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestGetRunLogsAfterSeq(t *testing.T) {
	s, _, cleanup := testutil.NewTestDB(t)
	defer cleanup.Close(t)

	ctx := context.Background()
	team, _ := testutil.CreateTeam(t, s, "team-log-seq")
	env, err := s.GetOrCreateDefaultEnvironment(ctx, team.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	app := testutil.CreateApp(t, s, team.ID, "app-log-seq")
	version := testutil.CreateVersion(t, s, app.ID)
	run := testutil.CreateRun(t, s, team.ID, app.ID, env.ID, version.ID, 0, 0)
	runner, _ := testutil.CreateRunner(t, s, "runner-log-seq", "default")
	_, attempt, _, _ := testutil.LeaseRun(t, s, runner)

	err = s.AppendLogs(ctx, attempt.ID, []store.LogEntry{
		{Seq: 1, Stream: "stdout", Line: "one", LoggedAt: time.Now()},
		{Seq: 2, Stream: "stdout", Line: "two", LoggedAt: time.Now()},
		{Seq: 3, Stream: "stderr", Line: "three", LoggedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("append logs: %v", err)
	}

	allLogs, err := s.GetRunLogs(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("get all logs: %v", err)
	}
	if len(allLogs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(allLogs))
	}

	incrementalLogs, err := s.GetRunLogs(ctx, run.ID, 2)
	if err != nil {
		t.Fatalf("get incremental logs: %v", err)
	}
	if len(incrementalLogs) != 1 || incrementalLogs[0].Seq != 3 {
		t.Fatalf("expected only seq 3, got %+v", incrementalLogs)
	}
}
