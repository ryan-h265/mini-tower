package httpapi

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Path patterns for normalization to prevent high cardinality
	uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)
)

// Metrics holds Prometheus collectors for HTTP and domain metrics.
type Metrics struct {
	reg      prometheus.Registerer
	gatherer prometheus.Gatherer

	// HTTP metrics
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec

	// Domain counters
	runsCreated      *prometheus.CounterVec
	runsCompleted    *prometheus.CounterVec
	runsRetried      *prometheus.CounterVec
	runsLeased       *prometheus.CounterVec
	runnersRegistered *prometheus.CounterVec

	// Domain histograms
	runQueueWait   *prometheus.HistogramVec
	runExecution   *prometheus.HistogramVec
	runTotal       *prometheus.HistogramVec
}

// NewMetrics creates a new Metrics instance with registered collectors.
func NewMetrics(reg prometheus.Registerer, db *sql.DB) *Metrics {
	// Determine the gatherer from the registerer.
	// If it's a *prometheus.Registry, use it directly; otherwise fall back to DefaultGatherer.
	var gatherer prometheus.Gatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	} else {
		gatherer = prometheus.DefaultGatherer
	}

	m := &Metrics{
		reg:      reg,
		gatherer: gatherer,

		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_http_requests_total",
				Help: "Total number of HTTP requests by method, path pattern, and status code.",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds by method and path pattern.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		requestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_http_request_size_bytes",
				Help:    "HTTP request body size in bytes.",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_http_response_size_bytes",
				Help:    "HTTP response body size in bytes.",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// Domain counters
		runsCreated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_runs_created_total",
				Help: "Total runs created, by team and app.",
			},
			[]string{"team", "app"},
		),
		runsCompleted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_runs_completed_total",
				Help: "Total runs completed, by team, app, and terminal status.",
			},
			[]string{"team", "app", "status"},
		),
		runsRetried: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_runs_retried_total",
				Help: "Total runs re-queued by the reaper, by team and app.",
			},
			[]string{"team", "app"},
		),
		runsLeased: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_runs_leased_total",
				Help: "Total runs leased by runners, by environment.",
			},
			[]string{"environment"},
		),
		runnersRegistered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "minitower_runners_registered_total",
				Help: "Total runners registered, by environment.",
			},
			[]string{"environment"},
		),

		// Domain histograms
		runQueueWait: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_run_queue_wait_seconds",
				Help:    "Time a run spent queued (started_at - queued_at).",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 15), // 0.1s to ~1638s
			},
			[]string{"team", "app"},
		),
		runExecution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_run_execution_seconds",
				Help:    "Run execution duration (finished_at - started_at).",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 15),
			},
			[]string{"team", "app", "status"},
		),
		runTotal: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_run_total_seconds",
				Help:    "Total run duration (finished_at - queued_at).",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 15),
			},
			[]string{"team", "app", "status"},
		),
	}

	reg.MustRegister(
		m.requestsTotal, m.requestDuration, m.requestSize, m.responseSize,
		m.runsCreated, m.runsCompleted, m.runsRetried, m.runsLeased, m.runnersRegistered,
		m.runQueueWait, m.runExecution, m.runTotal,
	)

	if db != nil {
		reg.MustRegister(NewDomainCollector(db))
	}

	return m
}

// Handler returns the Prometheus metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.gatherer, promhttp.HandlerOpts{})
}

// --- DomainMetrics interface implementation ---

func (m *Metrics) RunCreated(team, app string) {
	m.runsCreated.WithLabelValues(team, app).Inc()
}

func (m *Metrics) RunCompleted(team, app, status string) {
	m.runsCompleted.WithLabelValues(team, app, status).Inc()
}

func (m *Metrics) RunRetried(team, app string) {
	m.runsRetried.WithLabelValues(team, app).Inc()
}

func (m *Metrics) RunLeased(environment string) {
	m.runsLeased.WithLabelValues(environment).Inc()
}

func (m *Metrics) RunnerRegistered(environment string) {
	m.runnersRegistered.WithLabelValues(environment).Inc()
}

func (m *Metrics) ObserveQueueWait(team, app string, seconds float64) {
	m.runQueueWait.WithLabelValues(team, app).Observe(seconds)
}

func (m *Metrics) ObserveExecution(team, app, status string, seconds float64) {
	m.runExecution.WithLabelValues(team, app, status).Observe(seconds)
}

func (m *Metrics) ObserveTotal(team, app, status string, seconds float64) {
	m.runTotal.WithLabelValues(team, app, status).Observe(seconds)
}

// Middleware returns an HTTP middleware that records metrics.
func (m *Metrics) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := normalizePath(r.URL.Path)

			// Wrap response writer to capture status and size
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			// Record request size
			var reqSize int64
			if r.ContentLength > 0 {
				reqSize = r.ContentLength
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(rw.status)

			m.requestsTotal.WithLabelValues(r.Method, path, status).Inc()
			m.requestDuration.WithLabelValues(r.Method, path).Observe(duration)
			if reqSize > 0 {
				m.requestSize.WithLabelValues(r.Method, path).Observe(float64(reqSize))
			}
			if rw.size > 0 {
				m.responseSize.WithLabelValues(r.Method, path).Observe(float64(rw.size))
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and response size.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// normalizePath converts dynamic path segments to placeholders to prevent high cardinality.
// Examples:
//   - /api/v1/apps/hello -> /api/v1/apps/{app}
//   - /api/v1/runs/550e8400-e29b-41d4-a716-446655440000/logs -> /api/v1/runs/{run}/logs
func normalizePath(path string) string {
	// Skip paths that don't need normalization
	if !strings.HasPrefix(path, "/api/v1/") {
		return path
	}

	parts := strings.Split(path, "/")
	// parts[0] = "", parts[1] = "api", parts[2] = "v1", parts[3] = resource, parts[4] = id, ...

	if len(parts) < 5 {
		return path
	}

	resource := parts[3]

	switch resource {
	case "apps":
		// /api/v1/apps/{app}[/versions|/runs]
		if len(parts) >= 5 && isSlugOrID(parts[4]) {
			parts[4] = "{app}"
		}
	case "runs":
		// /api/v1/runs/{run}[/start|/heartbeat|/logs|/result|/artifact|/cancel]
		if len(parts) >= 5 && isSlugOrID(parts[4]) {
			parts[4] = "{run}"
		}
	case "runners":
		// /api/v1/runners/register - no dynamic segment
	case "tokens", "bootstrap":
		// No dynamic segments
	}

	return strings.Join(parts, "/")
}

// isSlugOrID returns true if the segment looks like a slug or UUID.
func isSlugOrID(segment string) bool {
	if segment == "" {
		return false
	}
	// Check for UUID
	if uuidPattern.MatchString(segment) {
		return true
	}
	// Check for slug (lowercase alphanumeric with hyphens)
	if slugPattern.MatchString(segment) {
		return true
	}
	return false
}
