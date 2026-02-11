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
	Role   string `json:"role"`
}

// BootstrapTeam creates a team or re-issues credentials for an existing slug.
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

	team, err := h.store.GetTeamBySlug(r.Context(), req.Slug)
	if err != nil {
		h.logger.Error("get team by slug", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	statusCode := http.StatusCreated
	if team == nil {
		// Single-team MVP: if the requested slug does not exist, reject when any team is already present.
		// This keeps bootstrap idempotent for one slug while still preventing creating a second team.
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

		team, err = h.store.CreateTeam(r.Context(), req.Slug, req.Name)
		if err != nil {
			h.logger.Error("create team", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
	} else {
		// Same slug re-bootstrap is allowed for local/dev recovery.
		statusCode = http.StatusOK
	}

	// Set (or reset) password if provided.
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

	// Generate initial/recovery team API token.
	teamToken, teamTokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	tokenName := "bootstrap"
	createdToken, err := h.store.CreateTeamToken(r.Context(), team.ID, teamTokenHash, &tokenName, "admin")
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, statusCode, bootstrapTeamResponse{
		TeamID: team.ID,
		Slug:   team.Slug,
		Token:  teamToken,
		Role:   createdToken.Role,
	})
}
