package handlers

import "net/http"

type meResponse struct {
	TeamID   int64  `json:"team_id"`
	TeamSlug string `json:"team_slug"`
	TokenID  int64  `json:"token_id"`
	Role     string `json:"role"`
}

// GetMe returns the current team/token identity.
func (h *Handlers) GetMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}
	teamSlug, ok := teamSlugFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}
	tokenID, ok := teamTokenIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing token context")
		return
	}
	role, ok := TokenRoleFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing token role")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		TeamID:   teamID,
		TeamSlug: teamSlug,
		TokenID:  tokenID,
		Role:     role,
	})
}
