package handlers

import (
	"net/http"
	"strings"
	"time"

	"minitower/internal/validate"
)

type createAppRequest struct {
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
}

type appResponse struct {
	AppID       int64   `json:"app_id"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	Disabled    bool    `json:"disabled"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type listAppsResponse struct {
	Apps []appResponse `json:"apps"`
}

// CreateApp creates a new app.
func (h *Handlers) CreateApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	var req createAppRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if err := validate.ValidateSlug(req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		return
	}

	// Check if app slug exists
	exists, err := h.store.AppExistsBySlug(r.Context(), teamID, req.Slug)
	if err != nil {
		h.logger.Error("check app exists", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "slug_taken", "app slug already exists")
		return
	}

	app, err := h.store.CreateApp(r.Context(), teamID, req.Slug, req.Description)
	if err != nil {
		h.logger.Error("create app", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, appResponse{
		AppID:       app.ID,
		Slug:        app.Slug,
		Description: app.Description,
		Disabled:    app.Disabled,
		CreatedAt:   app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   app.UpdatedAt.Format(time.RFC3339),
	})
}

// ListApps returns all apps for the team.
func (h *Handlers) ListApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	apps, err := h.store.ListApps(r.Context(), teamID)
	if err != nil {
		h.logger.Error("list apps", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	resp := listAppsResponse{Apps: make([]appResponse, 0, len(apps))}
	for _, app := range apps {
		resp.Apps = append(resp.Apps, appResponse{
			AppID:       app.ID,
			Slug:        app.Slug,
			Description: app.Description,
			Disabled:    app.Disabled,
			CreatedAt:   app.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   app.UpdatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetApp returns a single app by slug.
func (h *Handlers) GetApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}

	// Extract app slug from path: /api/v1/apps/{app}
	slug := extractPathParam(r.URL.Path, "/api/v1/apps/")
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

	writeJSON(w, http.StatusOK, appResponse{
		AppID:       app.ID,
		Slug:        app.Slug,
		Description: app.Description,
		Disabled:    app.Disabled,
		CreatedAt:   app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   app.UpdatedAt.Format(time.RFC3339),
	})
}

// extractPathParam extracts the first path segment after a prefix.
func extractPathParam(path, prefix string) string {
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
