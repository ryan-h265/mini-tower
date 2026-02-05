package handlers

import (
  "database/sql"
  "encoding/json"
  "log/slog"
  "net/http"

  "minitower/internal/config"
  "minitower/internal/objects"
  "minitower/internal/store"
)

// Handlers contains all HTTP handlers.
type Handlers struct {
  cfg     config.Config
  db      *sql.DB
  store   *Store
  objects *objects.LocalStore
  logger  *slog.Logger
}

// Store wraps the store.Store with additional methods for handlers.
type Store struct {
  *store.Store
}

func newStore(db *sql.DB) *Store {
  return &Store{Store: store.New(db)}
}

// New creates a new Handlers instance.
func New(cfg config.Config, db *sql.DB, objects *objects.LocalStore, logger *slog.Logger) *Handlers {
  return &Handlers{
    cfg:     cfg,
    db:      db,
    store:   newStore(db),
    objects: objects,
    logger:  logger,
  }
}

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

func decodeJSON(r *http.Request, v any) error {
  return json.NewDecoder(r.Body).Decode(v)
}
