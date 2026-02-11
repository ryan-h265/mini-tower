package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
)

type Middleware func(http.Handler) http.Handler

// BodyLimitMiddleware returns middleware that limits request body size.
// If the body exceeds maxBytes, it returns 413 Request Entity Too Large.
func BodyLimitMiddleware(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.ContentLength != 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ArtifactBodyLimitMiddleware applies a larger body limit specifically for artifact upload endpoints.
func ArtifactBodyLimitMiddleware(maxArtifactBytes, maxDefaultBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := maxDefaultBytes

			// Use larger limit for version artifact uploads
			// POST /api/v1/apps/{app}/versions
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/versions") {
				limit = maxArtifactBytes
			}

			if r.Body != nil && r.ContentLength != 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CORSMiddleware(allowedOrigins []string) Middleware {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin == "" {
			continue
		}
		allowed[origin] = struct{}{}
	}
	if len(allowed) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			_, originAllowed := allowed[origin]
			if originAllowed {
				addVaryHeader(w.Header(), "Origin")
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Lease-Token")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func Chain(handler http.Handler, middleware ...Middleware) http.Handler {
	wrapped := handler
	for i := len(middleware) - 1; i >= 0; i-- {
		wrapped = middleware[i](wrapped)
	}
	return wrapped
}

func Recoverer(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic", "error", recovered)
					writeError(w, http.StatusInternalServerError, "internal", "internal error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func addVaryHeader(header http.Header, value string) {
	existing := header.Values("Vary")
	for _, entry := range existing {
		parts := strings.Split(entry, ",")
		for _, part := range parts {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
