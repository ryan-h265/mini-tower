package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestBuildProcessEnv_ExportsInputAsEnvVars(t *testing.T) {
	r := &Runner{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	base := []string{
		"PATH=/usr/bin",
		"name=old",
		"MINITOWER_INPUT={\"name\":\"legacy\"}",
	}
	input := map[string]any{
		"name":    "MiniTower",
		"count":   10,
		"enabled": true,
		"tags":    []string{"alpha", "beta"},
		"empty":   nil,
		"bad=key": "skip-me",
	}

	env := envToMap(r.buildProcessEnv(base, input))

	if _, ok := env["MINITOWER_INPUT"]; ok {
		t.Fatal("MINITOWER_INPUT should not be present")
	}
	if got := env["name"]; got != "MiniTower" {
		t.Fatalf("name mismatch: got %q", got)
	}
	if got := env["count"]; got != "10" {
		t.Fatalf("count mismatch: got %q", got)
	}
	if got := env["enabled"]; got != "true" {
		t.Fatalf("enabled mismatch: got %q", got)
	}
	if got := env["tags"]; got != "[\"alpha\",\"beta\"]" {
		t.Fatalf("tags mismatch: got %q", got)
	}
	if got := env["empty"]; got != "" {
		t.Fatalf("empty mismatch: got %q", got)
	}
	if _, ok := env["bad=key"]; ok {
		t.Fatal("invalid env key should be skipped")
	}
}

func TestBuildProcessEnv_RemovesLegacyVarWithoutInput(t *testing.T) {
	r := &Runner{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	env := envToMap(r.buildProcessEnv([]string{
		"MINITOWER_INPUT={}",
		"PATH=/usr/bin",
	}, nil))

	if _, ok := env["MINITOWER_INPUT"]; ok {
		t.Fatal("MINITOWER_INPUT should be removed")
	}
	if got := env["PATH"]; got != "/usr/bin" {
		t.Fatalf("PATH mismatch: got %q", got)
	}
}

func envToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		for i := 0; i < len(kv); i++ {
			if kv[i] != '=' {
				continue
			}
			out[kv[:i]] = kv[i+1:]
			break
		}
	}
	return out
}
