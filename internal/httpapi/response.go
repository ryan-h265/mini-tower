package httpapi

import (
  "encoding/json"
  "net/http"
)

type errorEnvelope struct {
  Error errorBody `json:"error"`
}

type errorBody struct {
  Code    string `json:"code"`
  Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(status)
  _ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
  writeJSON(w, status, errorEnvelope{
    Error: errorBody{
      Code:    code,
      Message: message,
    },
  })
}
