// main_test.go — tests for the run() entry-point function
//
// Verifies argument parsing, config loading, server startup, health endpoint
// reachability, and graceful shutdown behavior.

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	testInternalSecret = "test-secret"
	testDatabaseURL    = "postgres://localhost/test_daily_patch"
	testReadyTimeout   = 5 * time.Second
	testHealthPath     = "/health"
)

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestRun_CancelledContext_ShutsDownCleanly(t *testing.T) {
	cfgPath := writeTestConfig(t)

	// A pre-cancelled context exercises the shutdown path without ever
	// serving a request. run() should return nil (clean exit).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := run(ctx, []string{"api", "-c", cfgPath}, io.Discard, io.Discard, &runOpts{skipDB: true, registry: prometheus.NewRegistry()})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRun_StartsAndServesHealth(t *testing.T) {
	cfgPath := writeTestConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan net.Addr, 1)
	opts := &runOpts{ready: ready, skipDB: true, registry: prometheus.NewRegistry()}

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, []string{"api", "-c", cfgPath}, io.Discard, io.Discard, opts)
	}()

	baseURL := waitForReady(t, ready)

	resp, err := http.Get(baseURL + testHealthPath)
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Trigger graceful shutdown and verify run() returns without error.
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("expected nil error after shutdown, got %v", err)
	}
}

func TestRun_InvalidArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing config flag",
			args: []string{"api"},
		},
		{
			name: "nonexistent config file",
			args: []string{"api", "-c", "/nonexistent/config.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := run(ctx, tt.args, io.Discard, io.Discard, &runOpts{skipDB: true, registry: prometheus.NewRegistry()})
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tt.args)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// writeTestConfig writes a minimal valid YAML config to a temp file, sets
// required env vars via t.Setenv, and returns the file path. The listen
// address is ":0" so the OS assigns a free port.
func writeTestConfig(t *testing.T) string {
	t.Helper()

	t.Setenv("API_INTERNAL_SECRET", testInternalSecret)
	t.Setenv("DATABASE_URL", testDatabaseURL)

	content := []byte("api:\n  listen: \":0\"\n")
	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	return path
}

// waitForReady blocks until the ready channel receives an address or the
// timeout expires. Returns the base URL (http://host:port).
func waitForReady(t *testing.T, ready <-chan net.Addr) string {
	t.Helper()

	select {
	case addr := <-ready:
		return "http://" + addr.String()
	case <-time.After(testReadyTimeout):
		t.Fatal("timed out waiting for server to become ready")
		return ""
	}
}
