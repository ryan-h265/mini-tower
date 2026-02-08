package config

import (
	"strings"
	"testing"
)

func TestLoadDefaultsToPublicSignupWithoutBootstrapToken(t *testing.T) {
	t.Setenv("MINITOWER_RUNNER_REGISTRATION_TOKEN", "runner-secret")
	t.Setenv("MINITOWER_BOOTSTRAP_TOKEN", "")
	t.Setenv("MINITOWER_PUBLIC_SIGNUP_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}
	if !cfg.PublicSignupEnabled {
		t.Fatalf("expected public signup to default true")
	}
	if cfg.BootstrapToken != "" {
		t.Fatalf("expected empty bootstrap token, got %q", cfg.BootstrapToken)
	}
}

func TestLoadRequiresBootstrapWhenPublicSignupDisabled(t *testing.T) {
	t.Setenv("MINITOWER_RUNNER_REGISTRATION_TOKEN", "runner-secret")
	t.Setenv("MINITOWER_BOOTSTRAP_TOKEN", "")
	t.Setenv("MINITOWER_PUBLIC_SIGNUP_ENABLED", "false")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected config error when signup disabled without bootstrap token")
	}
	if !strings.Contains(err.Error(), "MINITOWER_BOOTSTRAP_TOKEN is required") {
		t.Fatalf("expected bootstrap token error, got: %v", err)
	}
}

func TestLoadAllowsBootstrapOnlyMode(t *testing.T) {
	t.Setenv("MINITOWER_RUNNER_REGISTRATION_TOKEN", "runner-secret")
	t.Setenv("MINITOWER_BOOTSTRAP_TOKEN", "bootstrap-secret")
	t.Setenv("MINITOWER_PUBLIC_SIGNUP_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load in bootstrap-only mode, got: %v", err)
	}
	if cfg.PublicSignupEnabled {
		t.Fatalf("expected public signup disabled")
	}
	if cfg.BootstrapToken != "bootstrap-secret" {
		t.Fatalf("unexpected bootstrap token: %q", cfg.BootstrapToken)
	}
}

func TestLoadRejectsInvalidPublicSignupValue(t *testing.T) {
	t.Setenv("MINITOWER_RUNNER_REGISTRATION_TOKEN", "runner-secret")
	t.Setenv("MINITOWER_PUBLIC_SIGNUP_ENABLED", "not-a-bool")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid bool parse error")
	}
	if !strings.Contains(err.Error(), "invalid MINITOWER_PUBLIC_SIGNUP_ENABLED") {
		t.Fatalf("expected public signup parse error, got: %v", err)
	}
}
