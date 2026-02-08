package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr          = ":8080"
	defaultDBPath              = "./minitower.db"
	defaultObjectsDir          = "./objects"
	defaultPublicSignupEnabled = true
	defaultLeaseTTL            = 60 * time.Second
	defaultExpiryCheckInterval = 10 * time.Second
	defaultRunnerPruneAfter    = 24 * time.Hour
	defaultMaxRequestBodySize  = 10 * 1024 * 1024  // 10MB
	defaultMaxArtifactSize     = 100 * 1024 * 1024 // 100MB
)

// Config contains control-plane configuration.
type Config struct {
	ListenAddr              string
	DBPath                  string
	ObjectsDir              string
	BootstrapToken          string
	PublicSignupEnabled     bool
	RunnerRegistrationToken string
	CORSOrigins             []string
	LeaseTTL                time.Duration
	ExpiryCheckInterval     time.Duration
	RunnerPruneAfter        time.Duration
	MaxRequestBodySize      int64
	MaxArtifactSize         int64
}

// Load reads configuration from environment variables with defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:          defaultListenAddr,
		DBPath:              defaultDBPath,
		ObjectsDir:          defaultObjectsDir,
		PublicSignupEnabled: defaultPublicSignupEnabled,
		LeaseTTL:            defaultLeaseTTL,
		ExpiryCheckInterval: defaultExpiryCheckInterval,
		RunnerPruneAfter:    defaultRunnerPruneAfter,
		MaxRequestBodySize:  defaultMaxRequestBodySize,
		MaxArtifactSize:     defaultMaxArtifactSize,
	}

	if v := strings.TrimSpace(os.Getenv("MINITOWER_LISTEN_ADDR")); v != "" {
		cfg.ListenAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_DB_PATH")); v != "" {
		cfg.DBPath = v
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_OBJECTS_DIR")); v != "" {
		cfg.ObjectsDir = v
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_BOOTSTRAP_TOKEN")); v != "" {
		cfg.BootstrapToken = v
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_PUBLIC_SIGNUP_ENABLED")); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_PUBLIC_SIGNUP_ENABLED: %w", err)
		}
		cfg.PublicSignupEnabled = enabled
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_LEASE_TTL")); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_LEASE_TTL: %w", err)
		}
		cfg.LeaseTTL = dur
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_EXPIRY_CHECK_INTERVAL")); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_EXPIRY_CHECK_INTERVAL: %w", err)
		}
		cfg.ExpiryCheckInterval = dur
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_RUNNER_PRUNE_AFTER")); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_RUNNER_PRUNE_AFTER: %w", err)
		}
		cfg.RunnerPruneAfter = dur
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_MAX_REQUEST_BODY_SIZE")); v != "" {
		size, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_MAX_REQUEST_BODY_SIZE: %w", err)
		}
		cfg.MaxRequestBodySize = size
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_MAX_ARTIFACT_SIZE")); v != "" {
		size, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid MINITOWER_MAX_ARTIFACT_SIZE: %w", err)
		}
		cfg.MaxArtifactSize = size
	}

	if v := strings.TrimSpace(os.Getenv("MINITOWER_RUNNER_REGISTRATION_TOKEN")); v != "" {
		cfg.RunnerRegistrationToken = v
	}
	if v := strings.TrimSpace(os.Getenv("MINITOWER_CORS_ORIGINS")); v != "" {
		parts := strings.Split(v, ",")
		cfg.CORSOrigins = make([]string, 0, len(parts))
		for _, part := range parts {
			origin := strings.TrimSpace(part)
			if origin == "" {
				continue
			}
			cfg.CORSOrigins = append(cfg.CORSOrigins, origin)
		}
	}

	if cfg.RunnerRegistrationToken == "" {
		return cfg, errors.New("MINITOWER_RUNNER_REGISTRATION_TOKEN is required")
	}
	if !cfg.PublicSignupEnabled && cfg.BootstrapToken == "" {
		return cfg, errors.New("MINITOWER_BOOTSTRAP_TOKEN is required when MINITOWER_PUBLIC_SIGNUP_ENABLED is false")
	}

	return cfg, nil
}
