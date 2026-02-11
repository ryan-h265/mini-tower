package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const defaultReapLimit = 100

// ReapResult describes what happened to a single reaped attempt.
type ReapResult struct {
	TeamID int64
	AppID  int64
	Outcome string // "retried", "dead", "cancelled"
}

// ReapExpiredAttempts processes expired leases and applies retry/dead/cancel rules.
func (s *Store) ReapExpiredAttempts(ctx context.Context, now time.Time, limit int) ([]ReapResult, error) {
	if limit <= 0 {
		limit = defaultReapLimit
	}

	nowMs := now.UnixMilli()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id
     FROM run_attempts
     WHERE status IN ('leased', 'running', 'cancelling')
       AND lease_expires_at <= ?
     ORDER BY lease_expires_at ASC
     LIMIT ?`,
		nowMs, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attemptIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		attemptIDs = append(attemptIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var results []ReapResult
	for _, attemptID := range attemptIDs {
		result, err := s.reapAttempt(ctx, attemptID, nowMs)
		if err != nil {
			return results, err
		}
		if result != nil {
			results = append(results, *result)
		}
	}

	return results, nil
}

func (s *Store) reapAttempt(ctx context.Context, attemptID int64, nowMs int64) (*ReapResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var runID int64
	var attemptStatus string
	var leaseExpiresAt int64
	var runStatus string
	var cancelRequested int
	var retryCount int
	var maxRetries int
	var teamID int64
	var appID int64

	err = tx.QueryRowContext(ctx,
		`SELECT a.run_id, a.status, a.lease_expires_at, r.status, r.cancel_requested, r.retry_count, r.max_retries, r.team_id, r.app_id
     FROM run_attempts a
     JOIN runs r ON r.id = a.run_id
     WHERE a.id = ?`,
		attemptID,
	).Scan(&runID, &attemptStatus, &leaseExpiresAt, &runStatus, &cancelRequested, &retryCount, &maxRetries, &teamID, &appID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if leaseExpiresAt > nowMs {
		return nil, nil
	}
	if attemptStatus != "leased" && attemptStatus != "running" && attemptStatus != "cancelling" {
		return nil, nil
	}

	cancelPath := cancelRequested == 1 || attemptStatus == "cancelling" || runStatus == "cancelling"

	if cancelPath {
		attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "cancelled")
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE runs SET status = 'cancelled', finished_at = ?, updated_at = ?
       WHERE id = ? AND status IN ('leased', 'running', 'cancelling')`,
			nowMs, nowMs, runID,
		)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		if attemptUpdated {
			return &ReapResult{TeamID: teamID, AppID: appID, Outcome: "cancelled"}, nil
		}
		return nil, nil
	}

	if retryCount < maxRetries {
		attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "expired")
		if err != nil {
			return nil, err
		}

		result, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = 'queued', retry_count = retry_count + 1, queued_at = ?, updated_at = ?
       WHERE id = ? AND status IN ('leased', 'running', 'cancelling') AND cancel_requested = 0 AND retry_count < max_retries`,
			nowMs, nowMs, runID,
		)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			if err := maybeCancelRun(ctx, tx, runID, nowMs); err != nil {
				return nil, err
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, err
		}
		if attemptUpdated {
			return &ReapResult{TeamID: teamID, AppID: appID, Outcome: "retried"}, nil
		}
		return nil, nil
	}

	attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "expired")
	if err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = 'dead', finished_at = ?, updated_at = ?
     WHERE id = ? AND status IN ('leased', 'running', 'cancelling') AND cancel_requested = 0 AND retry_count >= max_retries`,
		nowMs, nowMs, runID,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if err := maybeCancelRun(ctx, tx, runID, nowMs); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if attemptUpdated {
		return &ReapResult{TeamID: teamID, AppID: appID, Outcome: "dead"}, nil
	}
	return nil, nil
}

func updateAttemptStatus(tx *sql.Tx, attemptID int64, nowMs int64, status string) (bool, error) {
	result, err := tx.Exec(
		`UPDATE run_attempts SET status = ?, finished_at = ?, updated_at = ?
     WHERE id = ? AND status IN ('leased', 'running', 'cancelling')`,
		status, nowMs, nowMs, attemptID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func maybeCancelRun(ctx context.Context, tx *sql.Tx, runID int64, nowMs int64) error {
	var cancelRequested int
	err := tx.QueryRowContext(ctx,
		`SELECT cancel_requested FROM runs WHERE id = ?`,
		runID,
	).Scan(&cancelRequested)
	if err != nil {
		return err
	}
	if cancelRequested == 1 {
		_, err = tx.ExecContext(ctx,
			`UPDATE runs SET status = 'cancelled', finished_at = ?, updated_at = ?
       WHERE id = ? AND status IN ('leased', 'running', 'cancelling')`,
			nowMs, nowMs, runID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
