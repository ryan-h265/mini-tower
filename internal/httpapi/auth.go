package httpapi

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"minitower/internal/auth"
	"minitower/internal/config"
	"minitower/internal/httpapi/handlers"
)

type Auth struct {
	cfg config.Config
	db  *sql.DB
}

func NewAuth(cfg config.Config, db *sql.DB) *Auth {
	return &Auth{
		cfg: cfg,
		db:  db,
	}
}

func (a *Auth) RequireBootstrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r)
		if !ok || !secureEqual(token, a.cfg.BootstrapToken) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) RequireTeam(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}

		tokenHash := auth.HashToken(token)

		var tokenID int64
		var teamID int64
		err := a.db.QueryRowContext(
			r.Context(),
			`SELECT id, team_id FROM team_tokens WHERE token_hash = ? AND revoked_at IS NULL LIMIT 1`,
			tokenHash,
		).Scan(&tokenID, &teamID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}

		ctx := handlers.WithTeamID(r.Context(), teamID)
		ctx = handlers.WithTeamTokenID(ctx, tokenID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) RequireRegistrationToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}

		tokenHash := auth.HashToken(token)

		var teamID int64
		err := a.db.QueryRowContext(
			r.Context(),
			`SELECT id FROM teams WHERE registration_token_hash = ? LIMIT 1`,
			tokenHash,
		).Scan(&teamID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}

		ctx := handlers.WithTeamID(r.Context(), teamID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) RequireRunner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := parseBearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}

		tokenHash := auth.HashToken(token)

		var runnerID int64
		var teamID int64
		var environmentID int64
		err := a.db.QueryRowContext(
			r.Context(),
			`SELECT id, team_id, environment_id FROM runners WHERE token_hash = ? AND status = 'online' LIMIT 1`,
			tokenHash,
		).Scan(&runnerID, &teamID, &environmentID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}

		ctx := handlers.WithTeamID(r.Context(), teamID)
		ctx = handlers.WithRunnerID(ctx, runnerID)
		ctx = handlers.WithEnvironmentID(ctx, environmentID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseBearerToken(r *http.Request) (string, bool) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return "", false
	}

	if len(authHeader) < len("Bearer ") {
		return "", false
	}

	prefix := authHeader[:len("Bearer ")]
	if strings.ToLower(prefix) != "bearer " {
		return "", false
	}

	token := strings.TrimSpace(authHeader[len("Bearer "):])
	if token == "" {
		return "", false
	}

	return token, true
}

// secureEqual compares two strings in constant time by hashing both to a
// fixed-length digest first, avoiding length-based timing side-channels.
func secureEqual(a, b string) bool {
	hashA := auth.HashToken(a)
	hashB := auth.HashToken(b)
	return subtle.ConstantTimeCompare([]byte(hashA), []byte(hashB)) == 1
}
