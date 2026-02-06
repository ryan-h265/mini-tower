package handlers

import (
	"errors"
	"io"
	"net/http"

	"minitower/internal/auth"
)

type createTokenRequest struct {
	Name *string `json:"name"`
}

type createTokenResponse struct {
	TokenID int64   `json:"token_id"`
	Token   string  `json:"token"`
	Name    *string `json:"name,omitempty"`
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

	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		if errors.Is(err, io.EOF) {
			req = createTokenRequest{}
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
			return
		}
	}

	// Generate team token
	token, tokenHash, err := auth.GeneratePrefixedToken(auth.PrefixTeamToken)
	if err != nil {
		h.logger.Error("generate team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	teamToken, err := h.store.CreateTeamToken(r.Context(), teamID, tokenHash, req.Name)
	if err != nil {
		h.logger.Error("create team token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createTokenResponse{
		TokenID: teamToken.ID,
		Token:   token,
		Name:    teamToken.Name,
	})
}
