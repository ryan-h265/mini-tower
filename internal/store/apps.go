package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrAppNotFound = errors.New("app not found")

type App struct {
	ID          int64
	TeamID      int64
	Slug        string
	Description *string
	Disabled    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateApp creates a new app.
func (s *Store) CreateApp(ctx context.Context, teamID int64, slug string, description *string) (*App, error) {
	now := time.Now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO apps (team_id, slug, description, disabled, created_at, updated_at)
     VALUES (?, ?, ?, 0, ?, ?)`,
		teamID, slug, description, now, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &App{
		ID:          id,
		TeamID:      teamID,
		Slug:        slug,
		Description: description,
		Disabled:    false,
		CreatedAt:   time.UnixMilli(now),
		UpdatedAt:   time.UnixMilli(now),
	}, nil
}

// GetAppBySlug returns an app by team ID and slug.
func (s *Store) GetAppBySlug(ctx context.Context, teamID int64, slug string) (*App, error) {
	var a App
	var createdAt, updatedAt int64
	var disabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, slug, description, disabled, created_at, updated_at
     FROM apps WHERE team_id = ? AND slug = ?`,
		teamID, slug,
	).Scan(&a.ID, &a.TeamID, &a.Slug, &a.Description, &disabled, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Disabled = disabled == 1
	a.CreatedAt = time.UnixMilli(createdAt)
	a.UpdatedAt = time.UnixMilli(updatedAt)
	return &a, nil
}

// GetAppByID returns an app by ID (scoped to team).
func (s *Store) GetAppByID(ctx context.Context, teamID int64, appID int64) (*App, error) {
	var a App
	var createdAt, updatedAt int64
	var disabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, team_id, slug, description, disabled, created_at, updated_at
     FROM apps WHERE team_id = ? AND id = ?`,
		teamID, appID,
	).Scan(&a.ID, &a.TeamID, &a.Slug, &a.Description, &disabled, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Disabled = disabled == 1
	a.CreatedAt = time.UnixMilli(createdAt)
	a.UpdatedAt = time.UnixMilli(updatedAt)
	return &a, nil
}

// ListApps returns all apps for a team.
func (s *Store) ListApps(ctx context.Context, teamID int64) ([]*App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, team_id, slug, description, disabled, created_at, updated_at
     FROM apps WHERE team_id = ? ORDER BY slug`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []*App
	for rows.Next() {
		var a App
		var createdAt, updatedAt int64
		var disabled int
		if err := rows.Scan(&a.ID, &a.TeamID, &a.Slug, &a.Description, &disabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.Disabled = disabled == 1
		a.CreatedAt = time.UnixMilli(createdAt)
		a.UpdatedAt = time.UnixMilli(updatedAt)
		apps = append(apps, &a)
	}
	return apps, rows.Err()
}

// AppExistsBySlug checks if an app with the given slug exists for a team.
func (s *Store) AppExistsBySlug(ctx context.Context, teamID int64, slug string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM apps WHERE team_id = ? AND slug = ? LIMIT 1`,
		teamID, slug,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
