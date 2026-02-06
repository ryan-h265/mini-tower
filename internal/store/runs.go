package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type Run struct {
	ID              int64
	TeamID          int64
	AppID           int64
	EnvironmentID   int64
	AppVersionID    int64
	RunNo           int64
	VersionNo       int64 // Populated by ListRunsByApp (joined from app_versions)
	Input           map[string]any
	Status          string
	Priority        int
	MaxRetries      int
	RetryCount      int
	CancelRequested bool
	QueuedAt        time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RunLog struct {
	ID           int64
	RunAttemptID int64
	Seq          int64
	Stream       string
	Line         string
	LoggedAt     time.Time
}

// CreateRun creates a new run in queued state.
func (s *Store) CreateRun(ctx context.Context, teamID, appID, envID, versionID int64, input map[string]any, priority, maxRetries int) (*Run, error) {
	now := time.Now().UnixMilli()

	var inputJSON *string
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		s := string(data)
		inputJSON = &s
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get next run number for this app (inside transaction to prevent duplicates)
	var maxRunNo sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT MAX(run_no) FROM runs WHERE app_id = ?`,
		appID,
	).Scan(&maxRunNo)
	if err != nil {
		return nil, err
	}

	runNo := int64(1)
	if maxRunNo.Valid {
		runNo = maxRunNo.Int64 + 1
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO runs (team_id, app_id, environment_id, app_version_id, run_no, input_json, status, priority, max_retries, retry_count, cancel_requested, queued_at, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, ?, 0, 0, ?, ?, ?)`,
		teamID, appID, envID, versionID, runNo, inputJSON, priority, maxRetries, now, now, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	queuedAt := time.UnixMilli(now)
	return &Run{
		ID:              id,
		TeamID:          teamID,
		AppID:           appID,
		EnvironmentID:   envID,
		AppVersionID:    versionID,
		RunNo:           runNo,
		Input:           input,
		Status:          "queued",
		Priority:        priority,
		MaxRetries:      maxRetries,
		RetryCount:      0,
		CancelRequested: false,
		QueuedAt:        queuedAt,
		CreatedAt:       queuedAt,
		UpdatedAt:       queuedAt,
	}, nil
}

// GetRunByID returns a run by ID (scoped to team).
func (s *Store) GetRunByID(ctx context.Context, teamID, runID int64) (*Run, error) {
	var r Run
	var inputJSON sql.NullString
	var queuedAt, createdAt, updatedAt int64
	var startedAt, finishedAt sql.NullInt64
	var cancelRequested int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, app_id, environment_id, app_version_id, run_no, input_json, status, priority, max_retries, retry_count, cancel_requested, queued_at, started_at, finished_at, created_at, updated_at
     FROM runs WHERE team_id = ? AND id = ?`,
		teamID, runID,
	).Scan(&r.ID, &r.TeamID, &r.AppID, &r.EnvironmentID, &r.AppVersionID, &r.RunNo, &inputJSON, &r.Status, &r.Priority, &r.MaxRetries, &r.RetryCount, &cancelRequested, &queuedAt, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.CancelRequested = cancelRequested == 1
	r.QueuedAt = time.UnixMilli(queuedAt)
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if startedAt.Valid {
		t := time.UnixMilli(startedAt.Int64)
		r.StartedAt = &t
	}
	if finishedAt.Valid {
		t := time.UnixMilli(finishedAt.Int64)
		r.FinishedAt = &t
	}
	if inputJSON.Valid {
		if err := json.Unmarshal([]byte(inputJSON.String), &r.Input); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// GetRunByIDDirect returns a run by ID without team scoping.
// Used by runner-scoped handlers where the lease token proves authorization.
func (s *Store) GetRunByIDDirect(ctx context.Context, runID int64) (*Run, error) {
	var r Run
	var inputJSON sql.NullString
	var queuedAt, createdAt, updatedAt int64
	var startedAt, finishedAt sql.NullInt64
	var cancelRequested int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, app_id, environment_id, app_version_id, run_no, input_json, status, priority, max_retries, retry_count, cancel_requested, queued_at, started_at, finished_at, created_at, updated_at
     FROM runs WHERE id = ?`,
		runID,
	).Scan(&r.ID, &r.TeamID, &r.AppID, &r.EnvironmentID, &r.AppVersionID, &r.RunNo, &inputJSON, &r.Status, &r.Priority, &r.MaxRetries, &r.RetryCount, &cancelRequested, &queuedAt, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.CancelRequested = cancelRequested == 1
	r.QueuedAt = time.UnixMilli(queuedAt)
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if startedAt.Valid {
		t := time.UnixMilli(startedAt.Int64)
		r.StartedAt = &t
	}
	if finishedAt.Valid {
		t := time.UnixMilli(finishedAt.Int64)
		r.FinishedAt = &t
	}
	if inputJSON.Valid {
		if err := json.Unmarshal([]byte(inputJSON.String), &r.Input); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// GetRunByAppAndRunNo returns a run by app ID and run number.
func (s *Store) GetRunByAppAndRunNo(ctx context.Context, teamID, appID, runNo int64) (*Run, error) {
	var r Run
	var inputJSON sql.NullString
	var queuedAt, createdAt, updatedAt int64
	var startedAt, finishedAt sql.NullInt64
	var cancelRequested int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, app_id, environment_id, app_version_id, run_no, input_json, status, priority, max_retries, retry_count, cancel_requested, queued_at, started_at, finished_at, created_at, updated_at
     FROM runs WHERE team_id = ? AND app_id = ? AND run_no = ?`,
		teamID, appID, runNo,
	).Scan(&r.ID, &r.TeamID, &r.AppID, &r.EnvironmentID, &r.AppVersionID, &r.RunNo, &inputJSON, &r.Status, &r.Priority, &r.MaxRetries, &r.RetryCount, &cancelRequested, &queuedAt, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.CancelRequested = cancelRequested == 1
	r.QueuedAt = time.UnixMilli(queuedAt)
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if startedAt.Valid {
		t := time.UnixMilli(startedAt.Int64)
		r.StartedAt = &t
	}
	if finishedAt.Valid {
		t := time.UnixMilli(finishedAt.Int64)
		r.FinishedAt = &t
	}
	if inputJSON.Valid {
		if err := json.Unmarshal([]byte(inputJSON.String), &r.Input); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// ListRunsByApp returns all runs for an app, joining version_no to avoid N+1 queries.
func (s *Store) ListRunsByApp(ctx context.Context, teamID, appID int64, limit, offset int) ([]*Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.team_id, r.app_id, r.environment_id, r.app_version_id, r.run_no,
            r.input_json, r.status, r.priority, r.max_retries, r.retry_count,
            r.cancel_requested, r.queued_at, r.started_at, r.finished_at,
            r.created_at, r.updated_at, v.version_no
     FROM runs r
     JOIN app_versions v ON r.app_version_id = v.id
     WHERE r.team_id = ? AND r.app_id = ?
     ORDER BY r.run_no DESC LIMIT ? OFFSET ?`,
		teamID, appID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		var r Run
		var inputJSON sql.NullString
		var queuedAt, createdAt, updatedAt int64
		var startedAt, finishedAt sql.NullInt64
		var cancelRequested int
		var versionNo int64
		if err := rows.Scan(&r.ID, &r.TeamID, &r.AppID, &r.EnvironmentID, &r.AppVersionID, &r.RunNo, &inputJSON, &r.Status, &r.Priority, &r.MaxRetries, &r.RetryCount, &cancelRequested, &queuedAt, &startedAt, &finishedAt, &createdAt, &updatedAt, &versionNo); err != nil {
			return nil, err
		}
		r.CancelRequested = cancelRequested == 1
		r.VersionNo = versionNo
		r.QueuedAt = time.UnixMilli(queuedAt)
		r.CreatedAt = time.UnixMilli(createdAt)
		r.UpdatedAt = time.UnixMilli(updatedAt)
		if startedAt.Valid {
			t := time.UnixMilli(startedAt.Int64)
			r.StartedAt = &t
		}
		if finishedAt.Valid {
			t := time.UnixMilli(finishedAt.Int64)
			r.FinishedAt = &t
		}
		if inputJSON.Valid {
			if err := json.Unmarshal([]byte(inputJSON.String), &r.Input); err != nil {
				return nil, err
			}
		}
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// GetRunLogs returns logs for the latest attempt of a run.
func (s *Store) GetRunLogs(ctx context.Context, runID int64) ([]*RunLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l.id, l.run_attempt_id, l.seq, l.stream, l.line, l.logged_at
     FROM run_logs l
     JOIN run_attempts a ON l.run_attempt_id = a.id
     WHERE a.run_id = ?
       AND a.attempt_no = (SELECT MAX(attempt_no) FROM run_attempts WHERE run_id = ?)
     ORDER BY l.seq ASC`,
		runID, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*RunLog
	for rows.Next() {
		var l RunLog
		var loggedAt int64
		if err := rows.Scan(&l.ID, &l.RunAttemptID, &l.Seq, &l.Stream, &l.Line, &loggedAt); err != nil {
			return nil, err
		}
		l.LoggedAt = time.UnixMilli(loggedAt)
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// CancelRun requests cancellation for a run and returns the updated run.
func (s *Store) CancelRun(ctx context.Context, teamID, runID int64) (*Run, error) {
	now := time.Now().UnixMilli()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM runs WHERE id = ? AND team_id = ?`,
		runID, teamID,
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	switch status {
	case "queued":
		_, err = tx.ExecContext(ctx,
			`UPDATE runs SET status = 'cancelled', cancel_requested = 1, finished_at = ?, updated_at = ?
       WHERE id = ? AND team_id = ? AND status = 'queued'`,
			now, now, runID, teamID,
		)
		if err != nil {
			return nil, err
		}
	case "leased", "running", "cancelling":
		_, err = tx.ExecContext(ctx,
			`UPDATE runs SET status = 'cancelling', cancel_requested = 1, updated_at = ?
       WHERE id = ? AND team_id = ? AND status IN ('leased','running','cancelling')`,
			now, runID, teamID,
		)
		if err != nil {
			return nil, err
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE run_attempts SET status = 'cancelling', updated_at = ?
       WHERE run_id = ? AND status IN ('leased','running')`,
			now, runID,
		)
		if err != nil {
			return nil, err
		}
	default:
		// Terminal state, no change.
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetRunByID(ctx, teamID, runID)
}
