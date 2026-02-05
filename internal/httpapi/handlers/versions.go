package handlers

import (
  "bytes"
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "io"
  "net/http"
  "strconv"
  "strings"
  "time"

  "github.com/google/uuid"

  "minitower/internal/validate"
)

type versionResponse struct {
  VersionID      int64          `json:"version_id"`
  VersionNo      int64          `json:"version_no"`
  Entrypoint     string         `json:"entrypoint"`
  TimeoutSeconds *int           `json:"timeout_seconds,omitempty"`
  ParamsSchema   map[string]any `json:"params_schema,omitempty"`
  ArtifactSHA256 string         `json:"artifact_sha256"`
  CreatedAt      string         `json:"created_at"`
}

type listVersionsResponse struct {
  Versions []versionResponse `json:"versions"`
}

// CreateVersion uploads a new version for an app (multipart form).
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

  // Get entrypoint (required)
  entrypoint := r.FormValue("entrypoint")
  if entrypoint == "" {
    writeError(w, http.StatusBadRequest, "invalid_request", "entrypoint is required")
    return
  }

  // Get optional timeout
  var timeoutSeconds *int
  if ts := r.FormValue("timeout_seconds"); ts != "" {
    val, err := strconv.Atoi(ts)
    if err != nil || val < 1 {
      writeError(w, http.StatusBadRequest, "invalid_request", "invalid timeout_seconds")
      return
    }
    timeoutSeconds = &val
  }

  // Get optional params schema
  var paramsSchema map[string]any
  if raw := r.FormValue("params_schema_json"); raw != "" {
    if err := json.Unmarshal([]byte(raw), &paramsSchema); err != nil {
      writeError(w, http.StatusBadRequest, "invalid_request", "invalid params_schema_json")
      return
    }
    if err := validate.ValidateJSONSchema(paramsSchema); err != nil {
      writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid params_schema_json: %s", err.Error()))
      return
    }
  }

  // Get artifact file
  file, _, err := r.FormFile("artifact")
  if err != nil {
    writeError(w, http.StatusBadRequest, "invalid_request", "artifact file is required")
    return
  }
  defer file.Close()

  // Calculate SHA256 while writing to a temp buffer
  hasher := sha256.New()
  tempReader := io.TeeReader(file, hasher)

  // Read file into memory to get hash and store
  data, err := io.ReadAll(tempReader)
  if err != nil {
    h.logger.Error("read artifact", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "failed to read artifact")
    return
  }

  artifactSHA256 := hex.EncodeToString(hasher.Sum(nil))

  objectKey := fmt.Sprintf("%d/%s.tar.gz", app.ID, uuid.NewString())

  // Store artifact first
  if err := h.objects.Store(objectKey, bytes.NewReader(data)); err != nil {
    h.logger.Error("store artifact", "error", err)
    writeError(w, http.StatusInternalServerError, "internal", "failed to store artifact")
    return
  }

  // Create version record
  version, err := h.store.CreateVersion(r.Context(), app.ID, objectKey, artifactSHA256, entrypoint, timeoutSeconds, paramsSchema)
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
    ArtifactSHA256: artifactSHA256,
    CreatedAt:      version.CreatedAt.Format(time.RFC3339),
  })
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
