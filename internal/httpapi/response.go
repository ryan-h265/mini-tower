package httpapi

import (
	"net/http"

	"minitower/internal/httputil"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	httputil.WriteJSON(w, status, payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	httputil.WriteError(w, status, code, message)
}
