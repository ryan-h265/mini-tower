package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrTeamExists = errors.New("team already exists")

type Team struct {
	ID           int64
	Slug         string
	Name         string
	PasswordHash *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type TeamToken struct {
	ID        int64
	TeamID    int64
	TokenHash string
	Name      *string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// CreateTeam creates a new team.
func (s *Store) CreateTeam(ctx context.Context, slug, name string) (*Team, error) {
	now := time.Now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO teams (slug, name, created_at, updated_at)
     VALUES (?, ?, ?, ?)`,
		slug, name, now, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Team{
		ID:        id,
		Slug:      slug,
		Name:      name,
		CreatedAt: time.UnixMilli(now),
		UpdatedAt: time.UnixMilli(now),
	}, nil
}

// TeamExists checks if any team exists.
func (s *Store) TeamExists(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM teams LIMIT 1`).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// TeamExistsBySlug checks if a team with the given slug exists.
func (s *Store) TeamExistsBySlug(ctx context.Context, slug string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM teams WHERE slug = ? LIMIT 1`, slug).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetTeamByID returns a team by ID.
func (s *Store) GetTeamByID(ctx context.Context, id int64) (*Team, error) {
	var t Team
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, slug, name, password_hash, created_at, updated_at
     FROM teams WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.PasswordHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.UnixMilli(createdAt)
	t.UpdatedAt = time.UnixMilli(updatedAt)
	return &t, nil
}

// GetTeamBySlug returns a team by slug.
func (s *Store) GetTeamBySlug(ctx context.Context, slug string) (*Team, error) {
	var t Team
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, slug, name, password_hash, created_at, updated_at
     FROM teams WHERE slug = ?`,
		slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.PasswordHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.UnixMilli(createdAt)
	t.UpdatedAt = time.UnixMilli(updatedAt)
	return &t, nil
}

// SetTeamPassword updates a team's password hash.
func (s *Store) SetTeamPassword(ctx context.Context, teamID int64, passwordHash string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE teams SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, teamID,
	)
	return err
}

// CreateTeamToken creates a new team API token.
func (s *Store) CreateTeamToken(ctx context.Context, teamID int64, tokenHash string, name *string) (*TeamToken, error) {
	now := time.Now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO team_tokens (team_id, token_hash, name, created_at)
     VALUES (?, ?, ?, ?)`,
		teamID, tokenHash, name, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &TeamToken{
		ID:        id,
		TeamID:    teamID,
		TokenHash: tokenHash,
		Name:      name,
		CreatedAt: time.UnixMilli(now),
	}, nil
}
