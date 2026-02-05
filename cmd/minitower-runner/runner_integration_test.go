//go:build integration
// +build integration

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testRunID      = int64(1)
	testLease      = "lease-token"
	testRunner     = "runner-token"
	testEntrypoint = "main.py"
)

func TestRunnerSelfFenceOnHeartbeatGone(t *testing.T) {
	python := requirePython(t)
	requireTar(t)

	artifact, sha := buildArtifact(t, "import time\nprint('hello', flush=True)\ntime.sleep(4)\n")

	server := newRunnerServer(t, serverConfig{
		artifact:       artifact,
		artifactSHA256: sha,
		heartbeatCode:  http.StatusGone,
		logsCode:       http.StatusOK,
		resultCode:     http.StatusOK,
	})

	runner := newTestRunner(t, "http://runner.test", python, server.handler)
	lease := makeLease(time.Now().Add(3*time.Second), 15)

	err := runner.executeRun(context.Background(), lease)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	if server.resultCalls.Load() != 0 {
		t.Fatalf("expected no result calls, got %d", server.resultCalls.Load())
	}
}

func TestRunnerSelfFenceOnLogsGone(t *testing.T) {
	python := requirePython(t)
	requireTar(t)

	artifact, sha := buildArtifact(t, "import time\nfor i in range(200):\n  print(f'line {i}', flush=True)\ntime.sleep(2)\n")

	server := newRunnerServer(t, serverConfig{
		artifact:       artifact,
		artifactSHA256: sha,
		heartbeatCode:  http.StatusOK,
		logsCode:       http.StatusGone,
		resultCode:     http.StatusOK,
	})

	runner := newTestRunner(t, "http://runner.test", python, server.handler)
	lease := makeLease(time.Now().Add(3*time.Second), 15)

	err := runner.executeRun(context.Background(), lease)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	if server.resultCalls.Load() != 0 {
		t.Fatalf("expected no result calls, got %d", server.resultCalls.Load())
	}
}

func TestRunnerHandlesResultGone(t *testing.T) {
	python := requirePython(t)
	requireTar(t)

	artifact, sha := buildArtifact(t, "print('done', flush=True)\n")

	server := newRunnerServer(t, serverConfig{
		artifact:       artifact,
		artifactSHA256: sha,
		heartbeatCode:  http.StatusOK,
		logsCode:       http.StatusOK,
		resultCode:     http.StatusGone,
	})

	runner := newTestRunner(t, "http://runner.test", python, server.handler)
	lease := makeLease(time.Now().Add(3*time.Second), 10)

	err := runner.executeRun(context.Background(), lease)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	if server.resultCalls.Load() != 1 {
		t.Fatalf("expected 1 result call, got %d", server.resultCalls.Load())
	}
}

func TestRunnerFailsGracefullyOnArtifactSHA256Mismatch(t *testing.T) {
	python := requirePython(t)
	requireTar(t)

	artifact, _ := buildArtifact(t, "print('should not run', flush=True)\n")

	// Server returns a wrong SHA256 hash
	wrongSHA := "0000000000000000000000000000000000000000000000000000000000000000"

	server := newRunnerServer(t, serverConfig{
		artifact:       artifact,
		artifactSHA256: wrongSHA,
		heartbeatCode:  http.StatusOK,
		logsCode:       http.StatusOK,
		resultCode:     http.StatusOK,
	})

	runner := newTestRunner(t, "http://runner.test", python, server.handler)
	lease := makeLease(time.Now().Add(10*time.Second), 60)

	err := runner.executeRun(context.Background(), lease)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}

	// Runner should have reported a failed result due to SHA256 mismatch
	if server.resultCalls.Load() != 1 {
		t.Fatalf("expected 1 result call, got %d", server.resultCalls.Load())
	}

	// Verify the result was a failure
	if server.lastResultStatus != "failed" {
		t.Fatalf("expected failed status, got %s", server.lastResultStatus)
	}
	if server.lastResultError == nil || !strings.Contains(*server.lastResultError, "artifact") {
		t.Fatalf("expected error message about artifact, got %v", server.lastResultError)
	}
}

type serverConfig struct {
	artifact       []byte
	artifactSHA256 string
	heartbeatCode  int
	logsCode       int
	resultCode     int
}

type runnerServer struct {
	cfg              serverConfig
	handler          http.Handler
	startCalls       atomic.Int32
	heartbeatCalls   atomic.Int32
	logCalls         atomic.Int32
	resultCalls      atomic.Int32
	lastResultStatus string
	lastResultError  *string
}

func newRunnerServer(t *testing.T, cfg serverConfig) *runnerServer {
	t.Helper()

	rs := &runnerServer{cfg: cfg}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/runs/1/start", func(w http.ResponseWriter, r *http.Request) {
		rs.startCalls.Add(1)
		if r.Header.Get("X-Lease-Token") != testLease {
			w.WriteHeader(http.StatusGone)
			return
		}
		writeJSON(w, map[string]any{
			"lease_expires_at": time.Now().Add(3 * time.Second).Format(time.RFC3339),
			"cancel_requested": false,
		})
	})

	mux.HandleFunc("/api/v1/runs/1/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		rs.heartbeatCalls.Add(1)
		if r.Header.Get("X-Lease-Token") != testLease {
			w.WriteHeader(http.StatusGone)
			return
		}
		if rs.cfg.heartbeatCode != http.StatusOK {
			w.WriteHeader(rs.cfg.heartbeatCode)
			return
		}
		writeJSON(w, map[string]any{
			"lease_expires_at": time.Now().Add(3 * time.Second).Format(time.RFC3339),
			"cancel_requested": false,
		})
	})

	mux.HandleFunc("/api/v1/runs/1/logs", func(w http.ResponseWriter, r *http.Request) {
		rs.logCalls.Add(1)
		if r.Header.Get("X-Lease-Token") != testLease {
			w.WriteHeader(http.StatusGone)
			return
		}
		if rs.cfg.logsCode != http.StatusOK {
			w.WriteHeader(rs.cfg.logsCode)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/runs/1/result", func(w http.ResponseWriter, r *http.Request) {
		rs.resultCalls.Add(1)
		if r.Header.Get("X-Lease-Token") != testLease {
			w.WriteHeader(http.StatusGone)
			return
		}

		// Parse and store the result for test assertions
		var payload struct {
			Status       string  `json:"status"`
			ErrorMessage *string `json:"error_message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			rs.lastResultStatus = payload.Status
			rs.lastResultError = payload.ErrorMessage
		}

		if rs.cfg.resultCode != http.StatusOK {
			w.WriteHeader(rs.cfg.resultCode)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/runs/1/artifact", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Lease-Token") != testLease {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.Header().Set("X-Artifact-SHA256", rs.cfg.artifactSHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rs.cfg.artifact)
	})

	rs.handler = authHandler(mux)
	return rs
}

func authHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testRunner {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newTestRunner(t *testing.T, serverURL, pythonBin string, handler http.Handler) *Runner {
	t.Helper()
	cfg := &Config{
		ServerURL:       serverURL,
		RunnerName:      "runner-test",
		DataDir:         t.TempDir(),
		PythonBin:       pythonBin,
		PollInterval:    100 * time.Millisecond,
		KillGracePeriod: 500 * time.Millisecond,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewRunner(cfg, logger)
	r.token = testRunner
	r.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}
	return r
}

func makeLease(expiry time.Time, timeoutSeconds int) *LeaseResponse {
	return &LeaseResponse{
		RunID:          testRunID,
		RunNo:          1,
		AppSlug:        "app",
		VersionNo:      1,
		Entrypoint:     testEntrypoint,
		TimeoutSeconds: &timeoutSeconds,
		AttemptID:      1,
		AttemptNo:      1,
		LeaseToken:     testLease,
		LeaseExpiresAt: expiry.Format(time.RFC3339),
	}
}

func buildArtifact(t *testing.T, script string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	data := []byte(script)
	hdr := &tar.Header{
		Name: testEntrypoint,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func requirePython(t *testing.T) string {
	t.Helper()
	python := os.Getenv("MINITOWER_PYTHON_BIN")
	if python == "" {
		python = "python3"
	}
	path, err := exec.LookPath(python)
	if err != nil {
		t.Skipf("python not found: %v", err)
	}
	check := exec.Command(path, "-c", "import venv")
	if err := check.Run(); err != nil {
		t.Skipf("python venv unavailable: %v", err)
	}
	return path
}

func requireTar(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skipf("tar not found: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
