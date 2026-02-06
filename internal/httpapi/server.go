package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"minitower/internal/config"
	"minitower/internal/httpapi/handlers"
	"minitower/internal/objects"
)

type Server struct {
	cfg      config.Config
	db       *sql.DB
	mux      *http.ServeMux
	handler  http.Handler
	auth     *Auth
	handlers *handlers.Handlers
	logger   *slog.Logger
	metrics  *Metrics
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithPrometheusRegisterer sets a custom prometheus registerer (for testing).
func WithPrometheusRegisterer(reg prometheus.Registerer) ServerOption {
	return func(s *Server) {
		s.metrics = NewMetrics(reg)
	}
}

func New(cfg config.Config, db *sql.DB, objects *objects.LocalStore, logger *slog.Logger, opts ...ServerOption) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:      cfg,
		db:       db,
		mux:      http.NewServeMux(),
		auth:     NewAuth(cfg, db),
		handlers: handlers.New(cfg, db, objects, logger),
		logger:   logger,
	}

	// Apply options before creating metrics (allows custom registry)
	for _, opt := range opts {
		opt(s)
	}

	// Create metrics with default registry if not set by option
	if s.metrics == nil {
		s.metrics = NewMetrics(prometheus.DefaultRegisterer)
	}

	s.routes()
	s.handler = Chain(
		s.mux,
		Recoverer(logger),
		s.metrics.Middleware(),
		ArtifactBodyLimitMiddleware(cfg.MaxArtifactSize, cfg.MaxRequestBodySize),
	)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes() {
	// Health checks (no auth)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Metrics (no auth)
	s.mux.Handle("/metrics", s.metrics.Handler())

	// Bootstrap (bootstrap token auth)
	s.mux.Handle("/api/v1/bootstrap/team", s.auth.RequireBootstrap(http.HandlerFunc(s.handlers.BootstrapTeam)))

	// Runner registration (registration token auth)
	s.mux.Handle("/api/v1/runners/register", s.auth.RequireRegistrationToken(http.HandlerFunc(s.handlers.RegisterRunner)))

	// Runner lease (runner token auth)
	s.mux.Handle("/api/v1/runs/lease", s.auth.RequireRunner(http.HandlerFunc(s.handlers.LeaseRun)))

	// Team API (team token auth)
	s.mux.Handle("/api/v1/tokens", s.auth.RequireTeam(http.HandlerFunc(s.handlers.CreateToken)))
	s.mux.Handle("/api/v1/apps", s.auth.RequireTeam(http.HandlerFunc(s.routeApps)))
	s.mux.Handle("/api/v1/apps/", s.auth.RequireTeam(http.HandlerFunc(s.routeAppsWithSlug)))

	// Runs - mixed auth depending on method/path
	s.mux.HandleFunc("/api/v1/runs/", s.routeRunsMixed)
}

func (s *Server) routeApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handlers.ListApps(w, r)
	case http.MethodPost:
		s.handlers.CreateApp(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) routeAppsWithSlug(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// /api/v1/apps/{app}/versions
	if strings.HasSuffix(path, "/versions") {
		switch r.Method {
		case http.MethodGet:
			s.handlers.ListVersions(w, r)
		case http.MethodPost:
			s.handlers.CreateVersion(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	// /api/v1/apps/{app}/runs
	if strings.HasSuffix(path, "/runs") {
		switch r.Method {
		case http.MethodGet:
			s.handlers.ListRuns(w, r)
		case http.MethodPost:
			s.handlers.CreateRun(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	// /api/v1/apps/{app}
	s.handlers.GetApp(w, r)
}

// routeRunsMixed handles /api/v1/runs/* with mixed auth based on method and path.
// Team auth: GET /runs/{run}, GET /runs/{run}/logs
// Runner auth: POST /runs/{run}/start, POST /runs/{run}/heartbeat, POST /runs/{run}/logs, POST /runs/{run}/result, GET /runs/{run}/artifact
func (s *Server) routeRunsMixed(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Runner endpoints (POST or artifact GET)
	if strings.HasSuffix(path, "/start") && r.Method == http.MethodPost {
		s.auth.RequireRunner(http.HandlerFunc(s.handlers.StartRun)).ServeHTTP(w, r)
		return
	}
	if strings.HasSuffix(path, "/heartbeat") && r.Method == http.MethodPost {
		s.auth.RequireRunner(http.HandlerFunc(s.handlers.HeartbeatRun)).ServeHTTP(w, r)
		return
	}
	if strings.HasSuffix(path, "/logs") && r.Method == http.MethodPost {
		s.auth.RequireRunner(http.HandlerFunc(s.handlers.SubmitLogs)).ServeHTTP(w, r)
		return
	}
	if strings.HasSuffix(path, "/result") && r.Method == http.MethodPost {
		s.auth.RequireRunner(http.HandlerFunc(s.handlers.SubmitResult)).ServeHTTP(w, r)
		return
	}
	if strings.HasSuffix(path, "/artifact") && r.Method == http.MethodGet {
		s.auth.RequireRunner(http.HandlerFunc(s.handlers.GetArtifact)).ServeHTTP(w, r)
		return
	}

	// Team endpoints (GET)
	if strings.HasSuffix(path, "/cancel") && r.Method == http.MethodPost {
		s.auth.RequireTeam(http.HandlerFunc(s.handlers.CancelRun)).ServeHTTP(w, r)
		return
	}
	if strings.HasSuffix(path, "/logs") && r.Method == http.MethodGet {
		s.auth.RequireTeam(http.HandlerFunc(s.handlers.GetRunLogs)).ServeHTTP(w, r)
		return
	}
	if r.Method == http.MethodGet {
		s.auth.RequireTeam(http.HandlerFunc(s.handlers.GetRun)).ServeHTTP(w, r)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "db not ready")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
