package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"minitower/internal/config"
	"minitower/internal/httputil"
	"minitower/internal/objects"
	"minitower/internal/store"
)

// DomainMetrics defines the interface for recording domain-level Prometheus metrics.
type DomainMetrics interface {
	RunCreated(team, app string)
	RunCompleted(team, app, status string)
	RunRetried(team, app string)
	RunLeased(environment string)
	RunnerRegistered(environment string)
	ObserveQueueWait(team, app string, seconds float64)
	ObserveExecution(team, app, status string, seconds float64)
	ObserveTotal(team, app, status string, seconds float64)
}

// NoOpMetrics is a no-op implementation of DomainMetrics for tests.
type NoOpMetrics struct{}

func (NoOpMetrics) RunCreated(string, string)                          {}
func (NoOpMetrics) RunCompleted(string, string, string)                {}
func (NoOpMetrics) RunRetried(string, string)                          {}
func (NoOpMetrics) RunLeased(string)                                   {}
func (NoOpMetrics) RunnerRegistered(string)                            {}
func (NoOpMetrics) ObserveQueueWait(string, string, float64)           {}
func (NoOpMetrics) ObserveExecution(string, string, string, float64)   {}
func (NoOpMetrics) ObserveTotal(string, string, string, float64)       {}

// Handlers contains all HTTP handlers.
type Handlers struct {
	cfg     config.Config
	db      *sql.DB
	store   *Store
	objects *objects.LocalStore
	logger  *slog.Logger
	metrics DomainMetrics
}

// Store wraps the store.Store with additional methods for handlers.
type Store struct {
	*store.Store
}

func newStore(db *sql.DB) *Store {
	return &Store{Store: store.New(db)}
}

// New creates a new Handlers instance.
func New(cfg config.Config, db *sql.DB, objects *objects.LocalStore, logger *slog.Logger, metrics DomainMetrics) *Handlers {
	if metrics == nil {
		metrics = NoOpMetrics{}
	}
	return &Handlers{
		cfg:     cfg,
		db:      db,
		store:   newStore(db),
		objects: objects,
		logger:  logger,
		metrics: metrics,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	httputil.WriteJSON(w, status, payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	httputil.WriteError(w, status, code, message)
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// writeStoreError maps store sentinel errors to HTTP responses.
// Returns true if it handled the error (wrote a response).
func writeStoreError(w http.ResponseWriter, logger *slog.Logger, err error, logMsg string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrInvalidLeaseToken):
		writeError(w, http.StatusGone, "gone", "invalid or expired lease")
	case errors.Is(err, store.ErrLeaseConflict):
		writeError(w, http.StatusConflict, "conflict", logMsg)
	case errors.Is(err, store.ErrAttemptNotActive):
		writeError(w, http.StatusGone, "gone", "attempt not active")
	case errors.Is(err, store.ErrNoRunAvailable):
		w.WriteHeader(http.StatusNoContent)
	default:
		logger.Error(logMsg, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
	}
	return true
}
