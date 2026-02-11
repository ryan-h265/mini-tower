package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envServerURL   = "MINITOWER_SERVER_URL"
	envAPIToken    = "MINITOWER_API_TOKEN"
	envCLIConfig   = "MINITOWER_CLI_CONFIG"
	defaultProfile = "default"
)

func configPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(envCLIConfig)); p != "" {
		return p, nil
	}

	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "minitower-cli", "config.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".config", "minitower-cli", "config.json"), nil
}

func loadProfileConfig() (*profileConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &profileConfig{Profiles: map[string]*profile{}}, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg profileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*profile{}
	}
	return &cfg, nil
}

func saveProfileConfig(cfg *profileConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*profile{}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func normalizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultProfile
	}
	return name
}

func pickProfile(cfg *profileConfig, explicitName string) (string, *profile, error) {
	if cfg == nil {
		return "", nil, fmt.Errorf("config is nil")
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*profile{}
	}

	if explicitName != "" {
		name := normalizeProfileName(explicitName)
		p := cfg.Profiles[name]
		if p == nil {
			return "", nil, fmt.Errorf("profile %q not found", name)
		}
		return name, p, nil
	}

	if cfg.CurrentProfile != "" {
		name := normalizeProfileName(cfg.CurrentProfile)
		if p := cfg.Profiles[name]; p != nil {
			return name, p, nil
		}
	}

	if p := cfg.Profiles[defaultProfile]; p != nil {
		return defaultProfile, p, nil
	}

	return "", nil, nil
}

func resolveConnection(profileName, serverOverride, tokenOverride string, requireToken bool) (*resolvedConnection, error) {
	cfg, err := loadProfileConfig()
	if err != nil {
		return nil, err
	}

	name, p, err := pickProfile(cfg, profileName)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = &profile{}
	}

	server := strings.TrimSpace(serverOverride)
	if server == "" {
		server = strings.TrimSpace(os.Getenv(envServerURL))
	}
	if server == "" {
		server = strings.TrimSpace(p.Server)
	}
	if server == "" {
		return nil, fmt.Errorf("server URL is required (--server, %s, or config profile)", envServerURL)
	}

	token := strings.TrimSpace(tokenOverride)
	if token == "" {
		token = strings.TrimSpace(os.Getenv(envAPIToken))
	}
	if token == "" {
		token = strings.TrimSpace(p.Token)
	}
	if requireToken && token == "" {
		return nil, fmt.Errorf("API token is required (--token, %s, or config profile)", envAPIToken)
	}

	team := strings.TrimSpace(p.Team)
	defaultApp := strings.TrimSpace(p.App)

	return &resolvedConnection{
		ProfileName: name,
		Server:      server,
		Token:       token,
		Team:        team,
		DefaultApp:  defaultApp,
	}, nil
}
