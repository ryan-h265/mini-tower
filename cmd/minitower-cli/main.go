package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"minitower/internal/towerfile"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "deploy" {
		fmt.Fprintln(os.Stderr, "usage: minitower-cli deploy [--server URL] [--token TOKEN] [--dir DIR]")
		os.Exit(1)
	}

	cfg, err := parseFlags(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if err := deploy(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

type config struct {
	Server string
	Token  string
	Dir    string
}

func parseFlags(args []string) (*config, error) {
	cfg := &config{
		Server: os.Getenv("MINITOWER_SERVER_URL"),
		Token:  os.Getenv("MINITOWER_API_TOKEN"),
		Dir:    ".",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--server requires a value")
			}
			i++
			cfg.Server = args[i]
		case "--token":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--token requires a value")
			}
			i++
			cfg.Token = args[i]
		case "--dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--dir requires a value")
			}
			i++
			cfg.Dir = args[i]
		default:
			return nil, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if cfg.Server == "" {
		return nil, fmt.Errorf("server URL is required (--server or MINITOWER_SERVER_URL)")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("API token is required (--token or MINITOWER_API_TOKEN)")
	}

	return cfg, nil
}

func deploy(cfg *config) error {
	// Read and parse Towerfile.
	tfPath := filepath.Join(cfg.Dir, "Towerfile")
	f, err := os.Open(tfPath)
	if err != nil {
		return fmt.Errorf("cannot open Towerfile: %w", err)
	}
	defer f.Close()

	tf, err := towerfile.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing Towerfile: %w", err)
	}
	if err := towerfile.Validate(tf); err != nil {
		return fmt.Errorf("validating Towerfile: %w", err)
	}

	fmt.Printf("Deploying app %q from %s\n", tf.App.Name, cfg.Dir)

	// Package artifact.
	artifact, sha256, err := towerfile.Package(cfg.Dir, tf)
	if err != nil {
		return fmt.Errorf("packaging artifact: %w", err)
	}

	artifactData, err := io.ReadAll(artifact)
	if err != nil {
		return fmt.Errorf("reading artifact: %w", err)
	}
	fmt.Printf("Artifact packaged (%d bytes, sha256:%s)\n", len(artifactData), sha256[:12])

	client := &http.Client{}
	server := strings.TrimRight(cfg.Server, "/")

	// Ensure app exists.
	if err := ensureApp(client, server, cfg.Token, tf.App.Name); err != nil {
		return err
	}

	// Upload version.
	version, err := uploadVersion(client, server, cfg.Token, tf.App.Name, artifactData)
	if err != nil {
		return err
	}

	fmt.Printf("Version %d created (sha256:%s)\n", version.VersionNo, version.ArtifactSHA256[:12])
	return nil
}

func ensureApp(client *http.Client, server, token, slug string) error {
	// Check if app exists.
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/apps/%s", server, slug), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("checking app: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status checking app: %d", resp.StatusCode)
	}

	// Create app.
	body, _ := json.Marshal(map[string]string{"slug": slug})
	req, err = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/apps", server), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("creating app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict {
		fmt.Printf("App %q ready\n", slug)
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("creating app failed: %d %s", resp.StatusCode, string(respBody))
}

type versionResponse struct {
	VersionID      int64  `json:"version_id"`
	VersionNo      int64  `json:"version_no"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

func uploadVersion(client *http.Client, server, token, slug string, artifactData []byte) (*versionResponse, error) {
	// Build multipart form with artifact file.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("artifact", "artifact.tar.gz")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(artifactData); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/apps/%s/versions", server, slug), &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uploading version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed: %d %s", resp.StatusCode, string(respBody))
	}

	var version versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &version, nil
}
