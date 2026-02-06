package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Environment struct {
	ID        int64
	TeamID    int64
	Name      string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetOrCreateDefaultEnvironment returns the default environment for a team, creating it if necessary.
func (s *Store) GetOrCreateDefaultEnvironment(ctx context.Context, teamID int64) (*Environment, error) {
	// Attempt insert; if it already exists the ON CONFLICT clause makes this a no-op.
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO environments (team_id, name, is_default, created_at, updated_at)
     VALUES (?, 'default', 1, ?, ?)
     ON CONFLICT(team_id, name) DO NOTHING`,
		teamID, now, now,
	)
	if err != nil {
		return nil, err
	}

	// Fetch the row (whether we just inserted it or it already existed).
	var e Environment
	var createdAt, updatedAt int64
	var isDefault int
	err = s.db.QueryRowContext(ctx,
		`SELECT id, team_id, name, is_default, created_at, updated_at
     FROM environments WHERE team_id = ? AND is_default = 1`,
		teamID,
	).Scan(&e.ID, &e.TeamID, &e.Name, &isDefault, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	e.IsDefault = isDefault == 1
	e.CreatedAt = time.UnixMilli(createdAt)
	e.UpdatedAt = time.UnixMilli(updatedAt)
	return &e, nil
}

// GetEnvironmentByID returns an environment by ID (scoped to team).
func (s *Store) GetEnvironmentByID(ctx context.Context, teamID int64, envID int64) (*Environment, error) {
	var e Environment
	var createdAt, updatedAt int64
	var isDefault int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, name, is_default, created_at, updated_at
     FROM environments WHERE team_id = ? AND id = ?`,
		teamID, envID,
	).Scan(&e.ID, &e.TeamID, &e.Name, &isDefault, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.IsDefault = isDefault == 1
	e.CreatedAt = time.UnixMilli(createdAt)
	e.UpdatedAt = time.UnixMilli(updatedAt)
	return &e, nil
}
