package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"minitower/internal/towerfile"
)

type versionResponse struct {
	VersionID      int64          `json:"version_id"`
	VersionNo      int64          `json:"version_no"`
	Entrypoint     string         `json:"entrypoint"`
	TimeoutSeconds *int           `json:"timeout_seconds,omitempty"`
	ParamsSchema   map[string]any `json:"params_schema,omitempty"`
	ArtifactSHA256 string         `json:"artifact_sha256"`
	TowerfileTOML  *string        `json:"towerfile_toml,omitempty"`
	ImportPaths    []string       `json:"import_paths,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

type listVersionsResponse struct {
	Versions []versionResponse `json:"versions"`
}

// CreateVersion uploads a new version for an app.
// The artifact must be a tar.gz containing a Towerfile at its root.
// All metadata (entrypoint, timeout, parameters, import paths) is extracted
// from the Towerfile inside the archive.
func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	// Extract app slug from path: /api/v1/apps/{app}/versions
	slug := extractAppSlugFromVersionPath(r.URL.Path)
	if slug == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing app slug")
		return
	}

	app, err := h.store.GetAppBySlug(r.Context(), teamID, slug)
	if err != nil {
		h.logger.Error("get app", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "not_found", "app not found")
		return
	}

	// Parse multipart form (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart form")
		return
	}

	// Get artifact file
	file, _, err := r.FormFile("artifact")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "artifact file is required")
		return
	}
	defer file.Close()

	// Calculate SHA256 while reading into memory.
	hasher := sha256.New()
	data, err := io.ReadAll(io.TeeReader(file, hasher))
	if err != nil {
		h.logger.Error("read artifact", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to read artifact")
		return
	}
	artifactSHA256 := hex.EncodeToString(hasher.Sum(nil))

	// Extract and parse the Towerfile from the artifact.
	towerfileContent, err := extractTowerfileFromArchive(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "TOWERFILE_MISSING", err.Error())
		return
	}

	tf, err := towerfile.Parse(strings.NewReader(towerfileContent))
	if err != nil {
		writeError(w, http.StatusBadRequest, "TOWERFILE_INVALID", fmt.Sprintf("invalid Towerfile: %s", err.Error()))
		return
	}
	if err := towerfile.Validate(tf); err != nil {
		writeError(w, http.StatusBadRequest, "TOWERFILE_INVALID", fmt.Sprintf("invalid Towerfile: %s", err.Error()))
		return
	}

	// Derive version metadata from the Towerfile.
	entrypoint := tf.App.Script
	var timeoutSeconds *int
	if tf.App.Timeout != nil {
		timeoutSeconds = &tf.App.Timeout.Seconds
	}
	paramsSchema := towerfile.ParamsSchemaFromParameters(tf.Parameters)

	objectKey := fmt.Sprintf("%d/%s.tar.gz", app.ID, uuid.NewString())

	// Store artifact.
	if err := h.objects.Store(objectKey, bytes.NewReader(data)); err != nil {
		h.logger.Error("store artifact", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to store artifact")
		return
	}

	// Create version record.
	version, err := h.store.CreateVersion(
		r.Context(), app.ID, objectKey, artifactSHA256, entrypoint,
		timeoutSeconds, paramsSchema, &towerfileContent, tf.App.ImportPaths,
	)
	if err != nil {
		h.logger.Error("create version", "error", err)
		_ = h.objects.Delete(objectKey)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, versionResponse{
		VersionID:      version.ID,
		VersionNo:      version.VersionNo,
		Entrypoint:     entrypoint,
		TimeoutSeconds: timeoutSeconds,
		ParamsSchema:   paramsSchema,
		ArtifactSHA256: artifactSHA256,
		TowerfileTOML:  &towerfileContent,
		ImportPaths:    tf.App.ImportPaths,
		CreatedAt:      version.CreatedAt.Format(time.RFC3339),
	})
}

const (
	maxTowerfileEntries = 50
	maxTowerfileSize    = 256 * 1024 // 256 KB
)

// extractTowerfileFromArchive scans a tar.gz archive for a Towerfile at the
// archive root. Returns the file content as a string. Only scans the first N
// entries and rejects oversized Towerfile entries to prevent tar bombs.
func extractTowerfileFromArchive(data []byte) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("artifact is not a valid gzip archive")
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for i := 0; i < maxTowerfileEntries; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("artifact is not a valid tar archive")
		}

		// Match "Towerfile" at the archive root (no directory prefix).
		name := strings.TrimPrefix(hdr.Name, "./")
		if name != "Towerfile" {
			continue
		}

		if hdr.Size > maxTowerfileSize {
			return "", fmt.Errorf("Towerfile exceeds maximum size of %d bytes", maxTowerfileSize)
		}

		content, err := io.ReadAll(io.LimitReader(tr, maxTowerfileSize+1))
		if err != nil {
			return "", fmt.Errorf("reading Towerfile from archive: %w", err)
		}
		return string(content), nil
	}

	return "", fmt.Errorf("artifact does not contain a Towerfile")
}

// ListVersions returns all versions for an app.
func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	slug := extractAppSlugFromVersionPath(r.URL.Path)
	if slug == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing app slug")
		return
	}

	app, err := h.store.GetAppBySlug(r.Context(), teamID, slug)
	if err != nil {
		h.logger.Error("get app", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "not_found", "app not found")
		return
	}

	versions, err := h.store.ListVersions(r.Context(), app.ID)
	if err != nil {
		h.logger.Error("list versions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	resp := listVersionsResponse{Versions: make([]versionResponse, 0, len(versions))}
	for _, v := range versions {
		resp.Versions = append(resp.Versions, versionResponse{
			VersionID:      v.ID,
			VersionNo:      v.VersionNo,
			Entrypoint:     v.Entrypoint,
			TimeoutSeconds: v.TimeoutSeconds,
			ParamsSchema:   v.ParamsSchema,
			ArtifactSHA256: v.ArtifactSHA256,
			TowerfileTOML:  v.TowerfileTOML,
			ImportPaths:    v.ImportPaths,
			CreatedAt:      v.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// extractAppSlugFromVersionPath extracts app slug from /api/v1/apps/{app}/versions
func extractAppSlugFromVersionPath(path string) string {
	const prefix = "/api/v1/apps/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
