package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrRunnerNotFound    = errors.New("runner not found")
	ErrNoRunAvailable    = errors.New("no run available")
	ErrLeaseConflict     = errors.New("lease conflict")
	ErrInvalidLeaseToken = errors.New("invalid lease token")
	ErrAttemptNotActive  = errors.New("attempt not active")
)

type Runner struct {
	ID            int64
	Name          string
	Environment   string
	Labels        map[string]string
	TokenHash     string
	Status        string
	MaxConcurrent int
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RunAttempt struct {
	ID             int64
	RunID          int64
	AttemptNo      int64
	RunnerID       int64
	LeaseTokenHash string
	LeaseExpiresAt time.Time
	Status         string
	ExitCode       *int
	ErrorMessage   *string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateRunner registers a new runner.
func (s *Store) CreateRunner(ctx context.Context, name, environment, tokenHash string) (*Runner, error) {
	now := time.Now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO runners (name, environment, token_hash, status, max_concurrent, created_at, updated_at)
     VALUES (?, ?, ?, 'online', 1, ?, ?)`,
		name, environment, tokenHash, now, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Runner{
		ID:            id,
		Name:          name,
		Environment:   environment,
		TokenHash:     tokenHash,
		Status:        "online",
		MaxConcurrent: 1,
		CreatedAt:     time.UnixMilli(now),
		UpdatedAt:     time.UnixMilli(now),
	}, nil
}

// RefreshRunnerRegistration re-issues a runner token and marks the runner online.
func (s *Store) RefreshRunnerRegistration(ctx context.Context, runnerID int64, environment, tokenHash string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE runners
		 SET environment = ?, token_hash = ?, status = 'online', last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		environment, tokenHash, now, now, runnerID,
	)
	return err
}

// GetRunnerByTokenHash finds a runner by token hash.
func (s *Store) GetRunnerByTokenHash(ctx context.Context, tokenHash string) (*Runner, error) {
	var r Runner
	var createdAt, updatedAt int64
	var lastSeenAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, environment, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at
     FROM runners WHERE token_hash = ?`,
		tokenHash,
	).Scan(&r.ID, &r.Name, &r.Environment, &r.TokenHash, &r.Status, &r.MaxConcurrent, &lastSeenAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if lastSeenAt.Valid {
		t := time.UnixMilli(lastSeenAt.Int64)
		r.LastSeenAt = &t
	}
	return &r, nil
}

// GetRunnerByName finds a runner by name (globally unique).
func (s *Store) GetRunnerByName(ctx context.Context, name string) (*Runner, error) {
	var r Runner
	var createdAt, updatedAt int64
	var lastSeenAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, environment, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at
     FROM runners WHERE name = ?`,
		name,
	).Scan(&r.ID, &r.Name, &r.Environment, &r.TokenHash, &r.Status, &r.MaxConcurrent, &lastSeenAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if lastSeenAt.Valid {
		t := time.UnixMilli(lastSeenAt.Int64)
		r.LastSeenAt = &t
	}
	return &r, nil
}

// ListRunners returns all runners, ordered by name.
func (s *Store) ListRunners(ctx context.Context) ([]*Runner, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, environment, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at
	     FROM runners
	     ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runners := make([]*Runner, 0)
	for rows.Next() {
		var r Runner
		var createdAt, updatedAt int64
		var lastSeenAt sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.Environment, &r.TokenHash, &r.Status, &r.MaxConcurrent, &lastSeenAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.CreatedAt = time.UnixMilli(createdAt)
		r.UpdatedAt = time.UnixMilli(updatedAt)
		if lastSeenAt.Valid {
			t := time.UnixMilli(lastSeenAt.Int64)
			r.LastSeenAt = &t
		}
		runners = append(runners, &r)
	}

	return runners, rows.Err()
}

// UpdateRunnerLastSeen updates the runner's last seen timestamp.
func (s *Store) UpdateRunnerLastSeen(ctx context.Context, runnerID int64) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE runners SET last_seen_at = ?, updated_at = ? WHERE id = ?`,
		now, now, runnerID,
	)
	return err
}

// LeaseRun attempts to lease a queued run for a runner.
// Returns the run, new attempt, and lease token, or ErrNoRunAvailable.
func (s *Store) LeaseRun(ctx context.Context, runner *Runner, leaseTokenHash string, leaseTTL time.Duration) (*Run, *RunAttempt, error) {
	now := time.Now()
	nowMs := now.UnixMilli()
	leaseExpiresAt := now.Add(leaseTTL).UnixMilli()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	// Check runner has no active attempt
	var activeCount int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_attempts WHERE runner_id = ? AND status IN ('leased', 'running', 'cancelling')`,
		runner.ID,
	).Scan(&activeCount)
	if err != nil {
		return nil, nil, err
	}
	if activeCount > 0 {
		return nil, nil, ErrLeaseConflict
	}

	// Find next queued run matching this runner's environment label
	var runID int64
	err = tx.QueryRowContext(ctx,
		`SELECT r.id FROM runs r
     JOIN environments e ON r.environment_id = e.id
     WHERE e.name = ? AND r.status = 'queued' AND r.cancel_requested = 0
     ORDER BY r.priority DESC, r.queued_at ASC, r.id ASC
     LIMIT 1`,
		runner.Environment,
	).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNoRunAvailable
	}
	if err != nil {
		return nil, nil, err
	}

	// CAS update run status from queued to leased
	result, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = 'leased', updated_at = ?
     WHERE id = ? AND status = 'queued' AND cancel_requested = 0`,
		nowMs, runID,
	)
	if err != nil {
		return nil, nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	if affected == 0 {
		return nil, nil, ErrLeaseConflict
	}

	// Get next attempt number
	var maxAttemptNo sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT MAX(attempt_no) FROM run_attempts WHERE run_id = ?`,
		runID,
	).Scan(&maxAttemptNo)
	if err != nil {
		return nil, nil, err
	}
	attemptNo := int64(1)
	if maxAttemptNo.Valid {
		attemptNo = maxAttemptNo.Int64 + 1
	}

	// Create attempt
	attemptResult, err := tx.ExecContext(ctx,
		`INSERT INTO run_attempts (run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, 'leased', ?, ?)`,
		runID, attemptNo, runner.ID, leaseTokenHash, leaseExpiresAt, nowMs, nowMs,
	)
	if err != nil {
		return nil, nil, err
	}
	attemptID, err := attemptResult.LastInsertId()
	if err != nil {
		return nil, nil, err
	}

	// Update runner last seen
	_, err = tx.ExecContext(ctx,
		`UPDATE runners SET last_seen_at = ?, updated_at = ? WHERE id = ?`,
		nowMs, nowMs, runner.ID,
	)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	// Fetch the leased run
	run, err := s.GetRunByIDDirect(ctx, runID)
	if err != nil {
		return nil, nil, err
	}

	attempt := &RunAttempt{
		ID:             attemptID,
		RunID:          runID,
		AttemptNo:      attemptNo,
		RunnerID:       runner.ID,
		LeaseTokenHash: leaseTokenHash,
		LeaseExpiresAt: time.UnixMilli(leaseExpiresAt),
		Status:         "leased",
		CreatedAt:      time.UnixMilli(nowMs),
		UpdatedAt:      time.UnixMilli(nowMs),
	}

	return run, attempt, nil
}

// scanAttempt scans a *sql.Row into a *RunAttempt, handling UnixMilli conversions and nullable times.
func scanAttempt(row *sql.Row) (*RunAttempt, error) {
	var a RunAttempt
	var leaseExpiresAt, createdAt, updatedAt int64
	var startedAt, finishedAt sql.NullInt64
	err := row.Scan(&a.ID, &a.RunID, &a.AttemptNo, &a.RunnerID, &a.LeaseTokenHash, &leaseExpiresAt, &a.Status, &a.ExitCode, &a.ErrorMessage, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	a.LeaseExpiresAt = time.UnixMilli(leaseExpiresAt)
	a.CreatedAt = time.UnixMilli(createdAt)
	a.UpdatedAt = time.UnixMilli(updatedAt)
	if startedAt.Valid {
		t := time.UnixMilli(startedAt.Int64)
		a.StartedAt = &t
	}
	if finishedAt.Valid {
		t := time.UnixMilli(finishedAt.Int64)
		a.FinishedAt = &t
	}
	return &a, nil
}

// GetActiveAttempt returns the current active attempt for a run, validating the lease token.
func (s *Store) GetActiveAttempt(ctx context.Context, runID int64, leaseTokenHash string) (*RunAttempt, error) {
	a, err := scanAttempt(s.db.QueryRowContext(ctx,
		`SELECT id, run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, exit_code, error_message, started_at, finished_at, created_at, updated_at
     FROM run_attempts
     WHERE run_id = ? AND lease_token_hash = ? AND status IN ('leased', 'running', 'cancelling')`,
		runID, leaseTokenHash,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidLeaseToken
	}
	return a, err
}

// StartAttempt transitions an attempt from leased to running.
func (s *Store) StartAttempt(ctx context.Context, attemptID int64, leaseTokenHash string) (*RunAttempt, error) {
	now := time.Now().UnixMilli()

	// CAS update: leased -> running
	result, err := s.db.ExecContext(ctx,
		`UPDATE run_attempts SET status = 'running', started_at = ?, updated_at = ?
     WHERE id = ? AND lease_token_hash = ? AND status = 'leased'`,
		now, now, attemptID, leaseTokenHash,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	// If no rows affected, check if already running (idempotent) or invalid
	if affected == 0 {
		var status string
		var tokenHash string
		err := s.db.QueryRowContext(ctx,
			`SELECT status, lease_token_hash FROM run_attempts WHERE id = ?`,
			attemptID,
		).Scan(&status, &tokenHash)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidLeaseToken
		}
		if err != nil {
			return nil, err
		}
		if tokenHash != leaseTokenHash {
			return nil, ErrInvalidLeaseToken
		}
		if status == "running" {
			// Idempotent - already running
		} else if status == "cancelling" {
			return nil, ErrLeaseConflict
		} else {
			return nil, ErrAttemptNotActive
		}
	}

	// Update run status to running
	_, err = s.db.ExecContext(ctx,
		`UPDATE runs SET status = 'running', started_at = COALESCE(started_at, ?), updated_at = ?
     WHERE id = (SELECT run_id FROM run_attempts WHERE id = ?)`,
		now, now, attemptID,
	)
	if err != nil {
		return nil, err
	}

	// Return updated attempt
	return scanAttempt(s.db.QueryRowContext(ctx,
		`SELECT id, run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, exit_code, error_message, started_at, finished_at, created_at, updated_at
     FROM run_attempts WHERE id = ?`,
		attemptID,
	))
}

// ExtendLease extends the lease expiry time (heartbeat).
func (s *Store) ExtendLease(ctx context.Context, attemptID int64, leaseTokenHash string, leaseTTL time.Duration) (*RunAttempt, error) {
	now := time.Now()
	nowMs := now.UnixMilli()
	newExpiry := now.Add(leaseTTL).UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`UPDATE run_attempts SET lease_expires_at = ?, updated_at = ?
     WHERE id = ? AND lease_token_hash = ? AND status IN ('leased', 'running', 'cancelling')`,
		newExpiry, nowMs, attemptID, leaseTokenHash,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrInvalidLeaseToken
	}

	// Update runner last seen
	_, err = s.db.ExecContext(ctx,
		`UPDATE runners SET last_seen_at = ?, updated_at = ?
     WHERE id = (SELECT runner_id FROM run_attempts WHERE id = ?)`,
		nowMs, nowMs, attemptID,
	)
	if err != nil {
		return nil, err
	}

	// Return updated attempt
	return scanAttempt(s.db.QueryRowContext(ctx,
		`SELECT id, run_id, attempt_no, runner_id, lease_token_hash, lease_expires_at, status, exit_code, error_message, started_at, finished_at, created_at, updated_at
     FROM run_attempts WHERE id = ?`,
		attemptID,
	))
}

// AppendLogs appends log entries for an attempt.
func (s *Store) AppendLogs(ctx context.Context, attemptID int64, logs []LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO run_logs (run_attempt_id, seq, stream, line, logged_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range logs {
		_, err := stmt.ExecContext(ctx, attemptID, l.Seq, l.Stream, l.Line, l.LoggedAt.UnixMilli())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type LogEntry struct {
	Seq      int64
	Stream   string
	Line     string
	LoggedAt time.Time
}

// CompleteAttempt finalizes an attempt with a result.
func (s *Store) CompleteAttempt(ctx context.Context, attemptID int64, leaseTokenHash string, status string, exitCode *int, errorMessage *string) error {
	now := time.Now().UnixMilli()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get current attempt state
	var currentStatus string
	var runID int64
	err = tx.QueryRowContext(ctx,
		`SELECT status, run_id FROM run_attempts WHERE id = ? AND lease_token_hash = ?`,
		attemptID, leaseTokenHash,
	).Scan(&currentStatus, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidLeaseToken
	}
	if err != nil {
		return err
	}

	// Check attempt is active or idempotent terminal
	if currentStatus != "leased" && currentStatus != "running" && currentStatus != "cancelling" {
		if currentStatus == "expired" {
			return ErrAttemptNotActive
		}
		if currentStatus == status {
			return nil // Idempotent terminal result
		}
		return ErrLeaseConflict
	}

	// If cancelling, only allow cancelled status
	if currentStatus == "cancelling" && status != "cancelled" {
		return ErrLeaseConflict
	}

	// Update attempt
	result, err := tx.ExecContext(ctx,
		`UPDATE run_attempts SET status = ?, exit_code = ?, error_message = ?, finished_at = ?, updated_at = ?
     WHERE id = ? AND lease_token_hash = ? AND status IN ('leased', 'running', 'cancelling')`,
		status, exitCode, errorMessage, now, now, attemptID, leaseTokenHash,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// Check if already completed with same status (idempotent)
		var finalStatus string
		err := tx.QueryRowContext(ctx,
			`SELECT status FROM run_attempts WHERE id = ? AND lease_token_hash = ?`,
			attemptID, leaseTokenHash,
		).Scan(&finalStatus)
		if err != nil {
			return ErrInvalidLeaseToken
		}
		if finalStatus == status {
			return nil // Idempotent
		}
		return ErrLeaseConflict
	}

	// Update run status
	_, err = tx.ExecContext(ctx,
		`UPDATE runs SET status = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		status, now, now, runID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetRunWithCancelStatus returns a run with its cancel_requested flag.
func (s *Store) GetRunWithCancelStatus(ctx context.Context, runID int64) (cancelRequested bool, err error) {
	var cr int
	err = s.db.QueryRowContext(ctx,
		`SELECT cancel_requested FROM runs WHERE id = ?`,
		runID,
	).Scan(&cr)
	if err != nil {
		return false, err
	}
	return cr == 1, nil
}

// MarkStaleRunnersOffline marks runners as offline if they haven't been seen since the threshold.
// Returns the number of runners marked offline.
func (s *Store) MarkStaleRunnersOffline(ctx context.Context, threshold time.Time) (int, error) {
	thresholdMs := threshold.UnixMilli()
	nowMs := time.Now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`UPDATE runners SET status = 'offline', updated_at = ?
     WHERE status = 'online' AND last_seen_at IS NOT NULL AND last_seen_at < ?`,
		nowMs, thresholdMs,
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// MarkRunnerOnline marks a runner as online.
func (s *Store) MarkRunnerOnline(ctx context.Context, runnerID int64) error {
	nowMs := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE runners SET status = 'online', last_seen_at = ?, updated_at = ?
     WHERE id = ?`,
		nowMs, nowMs, runnerID,
	)
	return err
}

// GetRunnerByID returns a runner by ID.
func (s *Store) GetRunnerByID(ctx context.Context, runnerID int64) (*Runner, error) {
	var r Runner
	var createdAt, updatedAt int64
	var lastSeenAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, environment, token_hash, status, max_concurrent, last_seen_at, created_at, updated_at
     FROM runners WHERE id = ?`,
		runnerID,
	).Scan(&r.ID, &r.Name, &r.Environment, &r.TokenHash, &r.Status, &r.MaxConcurrent, &lastSeenAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if lastSeenAt.Valid {
		t := time.UnixMilli(lastSeenAt.Int64)
		r.LastSeenAt = &t
	}
	return &r, nil
}
