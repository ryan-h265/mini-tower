package handlers

import (
	"net/http"

	"minitower/internal/auth"
	"minitower/internal/validate"
)

type bootstrapTeamRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type bootstrapTeamResponse struct {
	TeamID            int64  `json:"team_id"`
	Slug              string `json:"slug"`
	RegistrationToken string `json:"registration_token"`
	Token             string `json:"token"`
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

	// Generate registration token
	regToken, regTokenHash, err := auth.GeneratePrefixedToken(auth.PrefixRegistrationToken)
	if err != nil {
		h.logger.Error("generate registration token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	team, err := h.store.CreateTeam(r.Context(), req.Slug, req.Name, regTokenHash)
	if err != nil {
		h.logger.Error("create team", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
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
		TeamID:            team.ID,
		Slug:              team.Slug,
		RegistrationToken: regToken,
		Token:             teamToken,
	})
}
