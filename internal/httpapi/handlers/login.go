package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"minitower/internal/auth"
)

type loginRequest struct {
	Slug     string `json:"slug"`
	Password string `json:"password"`
}

type loginResponse struct {
	TeamID  int64  `json:"team_id"`
	Token   string `json:"token"`
	TokenID int64  `json:"token_id"`
	Role    string `json:"role"`
}

// LoginTeam authenticates a team by slug + password and returns a new API token.
func (h *Handlers) LoginTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if req.Slug == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "slug and password are required")
		return
	}

	team, err := h.store.GetTeamBySlug(r.Context(), req.Slug)
	if err != nil {
		h.logger.Error("get team by slug", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	// Generic 401 for: team not found, no password set, or wrong password.
	if team == nil || team.PasswordHash == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid slug or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*team.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid slug or password")
		return
	}

	// Generate a new team API token.
	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	tokenName := "login"
	teamToken, err := h.store.CreateTeamToken(r.Context(), team.ID, tokenHash, &tokenName, "admin")
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, loginResponse{
		TeamID:  team.ID,
		Token:   token,
		TokenID: teamToken.ID,
		Role:    teamToken.Role,
	})
}
