package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"minitower/internal/auth"
	"minitower/internal/validate"
)

type bootstrapTeamRequest struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Password *string `json:"password,omitempty"`
}

type bootstrapTeamResponse struct {
	TeamID int64  `json:"team_id"`
	Slug   string `json:"slug"`
	Token  string `json:"token"`
}

// BootstrapTeam creates the initial team (single-use globally).
func (h *Handlers) BootstrapTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req bootstrapTeamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if err := validate.ValidateSlug(req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	// Check if any team already exists (single-team MVP)
	exists, err := h.store.TeamExists(r.Context())
	if err != nil {
		h.logger.Error("check team exists", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "team_exists", "a team already exists")
		return
	}

	// Check if slug is taken (for future multi-team support)
	slugExists, err := h.store.TeamExistsBySlug(r.Context(), req.Slug)
	if err != nil {
		h.logger.Error("check slug exists", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if slugExists {
		writeError(w, http.StatusConflict, "slug_taken", "team slug already exists")
		return
	}

	team, err := h.store.CreateTeam(r.Context(), req.Slug, req.Name)
	if err != nil {
		h.logger.Error("create team", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	// Set password if provided
	if req.Password != nil && *req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), 12)
		if err != nil {
			h.logger.Error("hash password", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		if err := h.store.SetTeamPassword(r.Context(), team.ID, string(hash)); err != nil {
			h.logger.Error("set team password", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
	}

	// Generate initial team API token
	teamToken, teamTokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	tokenName := "bootstrap"
	_, err = h.store.CreateTeamToken(r.Context(), team.ID, teamTokenHash, &tokenName)
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, bootstrapTeamResponse{
		TeamID: team.ID,
		Slug:   team.Slug,
		Token:  teamToken,
	})
}
