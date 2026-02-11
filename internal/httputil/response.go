package httputil

import (
	"encoding/json"
	"net/http"
)

// ErrorEnvelope is the standard JSON error wrapper.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error code and message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON encodes payload as JSON and writes it with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes a standard error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorEnvelope{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	})
}
