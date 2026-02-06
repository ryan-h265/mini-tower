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
	defaultBootstrapToken      = "dev"
	defaultLeaseTTL            = 60 * time.Second
	defaultExpiryCheckInterval = 10 * time.Second
	defaultMaxRequestBodySize  = 10 * 1024 * 1024  // 10MB
	defaultMaxArtifactSize     = 100 * 1024 * 1024 // 100MB
)

// Config contains control-plane configuration.
type Config struct {
	ListenAddr          string
	DBPath              string
	ObjectsDir          string
	BootstrapToken      string
	LeaseTTL            time.Duration
	ExpiryCheckInterval time.Duration
	MaxRequestBodySize  int64
	MaxArtifactSize     int64
}

// Load reads configuration from environment variables with defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:          defaultListenAddr,
		DBPath:              defaultDBPath,
		ObjectsDir:          defaultObjectsDir,
		BootstrapToken:      defaultBootstrapToken,
		LeaseTTL:            defaultLeaseTTL,
		ExpiryCheckInterval: defaultExpiryCheckInterval,
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

	if cfg.BootstrapToken == "" {
		return cfg, errors.New("MINITOWER_BOOTSTRAP_TOKEN is required")
	}

	return cfg, nil
}
