package httpapi

import (
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

// Metrics holds Prometheus collectors for HTTP metrics.
type Metrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
}

// NewMetrics creates a new Metrics instance with registered collectors.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
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
				Buckets: prometheus.ExponentialBuckets(100, 10, 8), // 100B to 1GB
			},
			[]string{"method", "path"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "minitower_http_response_size_bytes",
				Help:    "HTTP response body size in bytes.",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8), // 100B to 1GB
			},
			[]string{"method", "path"},
		),
	}

	reg.MustRegister(m.requestsTotal, m.requestDuration, m.requestSize, m.responseSize)
	return m
}

// Handler returns the Prometheus metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
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
