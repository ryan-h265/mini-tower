package handlers

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"minitower/internal/auth"
	"minitower/internal/validate"
)

type signupTeamRequest struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type signupTeamResponse struct {
	TeamID int64  `json:"team_id"`
	Slug   string `json:"slug"`
	Token  string `json:"token"`
	Role   string `json:"role"`
}

type authOptionsResponse struct {
	SignupEnabled    bool `json:"signup_enabled"`
	BootstrapEnabled bool `json:"bootstrap_enabled"`
}

// SignupTeam creates a new team via public signup and returns an admin token.
func (h *Handlers) SignupTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !h.cfg.PublicSignupEnabled {
		writeError(w, http.StatusForbidden, "forbidden", "team signup is disabled")
		return
	}

	var req signupTeamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if err := validate.ValidateSlug(req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "password is required")
		return
	}

	exists, err := h.store.TeamExistsBySlug(r.Context(), req.Slug)
	if err != nil {
		h.logger.Error("check team exists by slug", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "slug_taken", "team slug already exists")
		return
	}

	team, err := h.store.CreateTeam(r.Context(), req.Slug, strings.TrimSpace(req.Name))
	if err != nil {
		if isTeamSlugUniqueConflict(err) {
			writeError(w, http.StatusConflict, "slug_taken", "team slug already exists")
			return
		}
		h.logger.Error("create team", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		h.logger.Error("hash password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := h.store.SetTeamPassword(r.Context(), team.ID, string(passwordHash)); err != nil {
		h.logger.Error("set team password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	tokenName := "signup"
	createdToken, err := h.store.CreateTeamToken(r.Context(), team.ID, tokenHash, &tokenName, "admin")
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, signupTeamResponse{
		TeamID: team.ID,
		Slug:   team.Slug,
		Token:  token,
		Role:   createdToken.Role,
	})
}

// GetAuthOptions returns public auth features that control login page UX.
func (h *Handlers) GetAuthOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, authOptionsResponse{
		SignupEnabled:    h.cfg.PublicSignupEnabled,
		BootstrapEnabled: strings.TrimSpace(h.cfg.BootstrapToken) != "",
	})
}

func isTeamSlugUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "teams.slug")
}
