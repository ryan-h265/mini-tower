package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type AppVersion struct {
	ID                int64
	AppID             int64
	VersionNo         int64
	ArtifactObjectKey string
	ArtifactSHA256    string
	Entrypoint        string
	TimeoutSeconds    *int
	ParamsSchema      map[string]any
	CreatedAt         time.Time
}

// CreateVersion creates a new app version with an atomically assigned version number.
func (s *Store) CreateVersion(ctx context.Context, appID int64, artifactKey, artifactSHA256, entrypoint string, timeoutSeconds *int, paramsSchema map[string]any) (*AppVersion, error) {
	now := time.Now().UnixMilli()

	var paramsSchemaJSON *string
	if paramsSchema != nil {
		data, err := json.Marshal(paramsSchema)
		if err != nil {
			return nil, err
		}
		str := string(data)
		paramsSchemaJSON = &str
	}

	// Atomic INSERT ... SELECT computes and inserts the version number in one statement,
	// preventing race conditions between concurrent uploads for the same app.
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO app_versions (app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at)
     VALUES (?, COALESCE((SELECT MAX(version_no) FROM app_versions WHERE app_id = ?), 0) + 1, ?, ?, ?, ?, ?, ?)`,
		appID, appID, artifactKey, artifactSHA256, entrypoint, timeoutSeconds, paramsSchemaJSON, now,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Read back the assigned version number.
	var versionNo int64
	err = s.db.QueryRowContext(ctx,
		`SELECT version_no FROM app_versions WHERE id = ?`, id,
	).Scan(&versionNo)
	if err != nil {
		return nil, err
	}

	return &AppVersion{
		ID:                id,
		AppID:             appID,
		VersionNo:         versionNo,
		ArtifactObjectKey: artifactKey,
		ArtifactSHA256:    artifactSHA256,
		Entrypoint:        entrypoint,
		TimeoutSeconds:    timeoutSeconds,
		ParamsSchema:      paramsSchema,
		CreatedAt:         time.UnixMilli(now),
	}, nil
}

// GetLatestVersion returns the latest version of an app.
func (s *Store) GetLatestVersion(ctx context.Context, appID int64) (*AppVersion, error) {
	var v AppVersion
	var createdAt int64
	var paramsSchemaJSON sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at
     FROM app_versions WHERE app_id = ? ORDER BY version_no DESC LIMIT 1`,
		appID,
	).Scan(&v.ID, &v.AppID, &v.VersionNo, &v.ArtifactObjectKey, &v.ArtifactSHA256, &v.Entrypoint, &v.TimeoutSeconds, &paramsSchemaJSON, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt = time.UnixMilli(createdAt)
	if paramsSchemaJSON.Valid {
		if err := json.Unmarshal([]byte(paramsSchemaJSON.String), &v.ParamsSchema); err != nil {
			return nil, err
		}
	}
	return &v, nil
}

// GetVersionByNumber returns a specific version of an app.
func (s *Store) GetVersionByNumber(ctx context.Context, appID int64, versionNo int64) (*AppVersion, error) {
	var v AppVersion
	var createdAt int64
	var paramsSchemaJSON sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at
     FROM app_versions WHERE app_id = ? AND version_no = ?`,
		appID, versionNo,
	).Scan(&v.ID, &v.AppID, &v.VersionNo, &v.ArtifactObjectKey, &v.ArtifactSHA256, &v.Entrypoint, &v.TimeoutSeconds, &paramsSchemaJSON, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt = time.UnixMilli(createdAt)
	if paramsSchemaJSON.Valid {
		if err := json.Unmarshal([]byte(paramsSchemaJSON.String), &v.ParamsSchema); err != nil {
			return nil, err
		}
	}
	return &v, nil
}

// GetVersionByID returns a version by ID (used for runs).
func (s *Store) GetVersionByID(ctx context.Context, versionID int64) (*AppVersion, error) {
	var v AppVersion
	var createdAt int64
	var paramsSchemaJSON sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at
     FROM app_versions WHERE id = ?`,
		versionID,
	).Scan(&v.ID, &v.AppID, &v.VersionNo, &v.ArtifactObjectKey, &v.ArtifactSHA256, &v.Entrypoint, &v.TimeoutSeconds, &paramsSchemaJSON, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt = time.UnixMilli(createdAt)
	if paramsSchemaJSON.Valid {
		if err := json.Unmarshal([]byte(paramsSchemaJSON.String), &v.ParamsSchema); err != nil {
			return nil, err
		}
	}
	return &v, nil
}

// ListVersions returns all versions of an app.
func (s *Store) ListVersions(ctx context.Context, appID int64) ([]*AppVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, version_no, artifact_object_key, artifact_sha256, entrypoint, timeout_seconds, params_schema_json, created_at
     FROM app_versions WHERE app_id = ? ORDER BY version_no DESC`,
		appID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*AppVersion
	for rows.Next() {
		var v AppVersion
		var createdAt int64
		var paramsSchemaJSON sql.NullString
		if err := rows.Scan(&v.ID, &v.AppID, &v.VersionNo, &v.ArtifactObjectKey, &v.ArtifactSHA256, &v.Entrypoint, &v.TimeoutSeconds, &paramsSchemaJSON, &createdAt); err != nil {
			return nil, err
		}
		v.CreatedAt = time.UnixMilli(createdAt)
		if paramsSchemaJSON.Valid {
			if err := json.Unmarshal([]byte(paramsSchemaJSON.String), &v.ParamsSchema); err != nil {
				return nil, err
			}
		}
		versions = append(versions, &v)
	}
	return versions, rows.Err()
}
