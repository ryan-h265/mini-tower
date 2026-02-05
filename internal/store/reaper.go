package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const defaultReapLimit = 100

// ReapExpiredAttempts processes expired leases and applies retry/dead/cancel rules.
func (s *Store) ReapExpiredAttempts(ctx context.Context, now time.Time, limit int) (int, error) {
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
		return 0, err
	}
	defer rows.Close()

	var attemptIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		attemptIDs = append(attemptIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	processed := 0
	for _, attemptID := range attemptIDs {
		updated, err := s.reapAttempt(ctx, attemptID, nowMs)
		if err != nil {
			return processed, err
		}
		if updated {
			processed++
		}
	}

	return processed, nil
}

func (s *Store) reapAttempt(ctx context.Context, attemptID int64, nowMs int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var runID int64
	var attemptStatus string
	var leaseExpiresAt int64
	var runStatus string
	var cancelRequested int
	var retryCount int
	var maxRetries int

	err = tx.QueryRowContext(ctx,
		`SELECT a.run_id, a.status, a.lease_expires_at, r.status, r.cancel_requested, r.retry_count, r.max_retries
     FROM run_attempts a
     JOIN runs r ON r.id = a.run_id
     WHERE a.id = ?`,
		attemptID,
	).Scan(&runID, &attemptStatus, &leaseExpiresAt, &runStatus, &cancelRequested, &retryCount, &maxRetries)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if leaseExpiresAt > nowMs {
		return false, nil
	}
	if attemptStatus != "leased" && attemptStatus != "running" && attemptStatus != "cancelling" {
		return false, nil
	}

	cancelPath := cancelRequested == 1 || attemptStatus == "cancelling" || runStatus == "cancelling"

	if cancelPath {
		attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "cancelled")
		if err != nil {
			return false, err
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE runs SET status = 'cancelled', finished_at = ?, updated_at = ?
       WHERE id = ? AND status IN ('leased', 'running', 'cancelling')`,
			nowMs, nowMs, runID,
		)
		if err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return attemptUpdated, nil
	}

	if retryCount < maxRetries {
		attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "expired")
		if err != nil {
			return false, err
		}

		result, err := tx.ExecContext(ctx,
			`UPDATE runs SET status = 'queued', retry_count = retry_count + 1, queued_at = ?, updated_at = ?
       WHERE id = ? AND status IN ('leased', 'running', 'cancelling') AND cancel_requested = 0 AND retry_count < max_retries`,
			nowMs, nowMs, runID,
		)
		if err != nil {
			return false, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if affected == 0 {
			if err := maybeCancelRun(tx, ctx, runID, nowMs); err != nil {
				return false, err
			}
		}

		if err := tx.Commit(); err != nil {
			return false, err
		}
		return attemptUpdated, nil
	}

	attemptUpdated, err := updateAttemptStatus(tx, attemptID, nowMs, "expired")
	if err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = 'dead', finished_at = ?, updated_at = ?
     WHERE id = ? AND status IN ('leased', 'running', 'cancelling') AND cancel_requested = 0 AND retry_count >= max_retries`,
		nowMs, nowMs, runID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		if err := maybeCancelRun(tx, ctx, runID, nowMs); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return attemptUpdated, nil
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

func maybeCancelRun(tx *sql.Tx, ctx context.Context, runID int64, nowMs int64) error {
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
