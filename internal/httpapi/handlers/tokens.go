package handlers

import (
	"errors"
	"io"
	"net/http"

	"minitower/internal/auth"
)

type createTokenRequest struct {
	Name *string `json:"name"`
	Role *string `json:"role"`
}

type createTokenResponse struct {
	TokenID int64   `json:"token_id"`
	Token   string  `json:"token"`
	Name    *string `json:"name,omitempty"`
	Role    string  `json:"role"`
}

// CreateToken creates a new team API token.
func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	teamID, ok := teamIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing team context")
		return
	}
	callerRole, _ := TokenRoleFromContext(r.Context())

	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		if errors.Is(err, io.EOF) {
			req = createTokenRequest{}
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
			return
		}
	}

	requestedRole := ""
	if req.Role != nil {
		requestedRole = *req.Role
		if requestedRole != "admin" && requestedRole != "member" {
			writeError(w, http.StatusBadRequest, "invalid_request", "role must be admin or member")
			return
		}
	}

	tokenRole := "member"
	if callerRole == "admin" && requestedRole != "" {
		tokenRole = requestedRole
	}

	// Generate team token
	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	teamToken, err := h.store.CreateTeamToken(r.Context(), teamID, tokenHash, req.Name, tokenRole)
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createTokenResponse{
		TokenID: teamToken.ID,
		Token:   token,
		Name:    teamToken.Name,
		Role:    teamToken.Role,
	})
}
