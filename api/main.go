// main.go — entry point for the api service
//
// Loads configuration, connects to PostgreSQL (with embedded migrations),
// initializes structured logging, assembles the router, and starts the
// HTTP server with graceful shutdown on SIGINT/SIGTERM.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akamensky/argparse"

	"daily-patch/api/internal/config"
	"daily-patch/api/internal/postgres"
	"daily-patch/api/internal/router"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	programName        = "api"
	programDescription = `
REST API server for the daily-patch vulnerability intelligence pipeline.
Ingests CVE/advisory data, serves it over a versioned REST API, and supports
LLM-based relevance scoring via internal service-to-service auth.

Environment variables:
	API_INTERNAL_SECRET   Shared secret for X-Internal-Secret header auth. Required.
	DATABASE_URL          PostgreSQL DSN (e.g. postgres://user:pass@host/db). Required.
`

	configHelp = "Path to the YAML configuration file. Secrets (API_INTERNAL_SECRET, DATABASE_URL) are read from environment variables and overlay the file values."

	// shutdownTimeout is the maximum time the server waits for in-flight
	// requests to complete before forcefully closing connections.
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout limits how long the server waits for a client to
	// send request headers, mitigating Slowloris-style attacks.
	readHeaderTimeout = 5 * time.Second

	// exitCodeError is returned to the OS when run() fails.
	exitCodeError = 1
)

// -----------------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------------

// runOpts holds optional hooks for the run function. The ready channel, when
// non-nil, receives the resolved listen address once the server is serving.
// This is used exclusively by tests to wait for the server to be ready before
// issuing requests. When skipDB is true, database initialization is skipped
// so that unit tests can exercise the server without a running PostgreSQL.
type runOpts struct {
	ready  chan<- net.Addr
	skipDB bool
}

// -----------------------------------------------------------------------------
// Main
// -----------------------------------------------------------------------------

// main is the process entry point. It registers signal handlers for graceful
// shutdown and delegates all work to run(). The only responsibility of main
// is translating run's error return into an OS exit code.
func main() {
	// Capture SIGINT (Ctrl-C) and SIGTERM (Docker/K8s stop) so the server
	// can drain in-flight requests instead of being killed immediately.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args, os.Stdout, os.Stderr, nil); err != nil {
		slog.Error("Failed to start HTTP server", "error", err.Error())
		os.Exit(exitCodeError)
	}
}

// -----------------------------------------------------------------------------
// Run
// -----------------------------------------------------------------------------

// run is the testable core of the service. It owns the full lifecycle:
// parse flags → load config → create logger → build router → listen →
// serve → wait for cancellation → graceful shutdown. Every dependency
// (args, writers, context) is injected so tests can drive the function
// without real signals or OS state.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, opts *runOpts) error {

	// --- CLI parsing ---------------------------------------------------------
	configPath, err := parseFlags(args)
	if err != nil {
		return err
	}

	// --- Configuration -------------------------------------------------------
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Logging -------------------------------------------------------------
	// All structured logs are written as JSON to stdout so Docker log
	// drivers and cloud log collectors can parse them without extra config.
	log := slog.New(slog.NewJSONHandler(stdout, nil))
	log.Info("configuration loaded successfully", "config_path", configPath, "listen_addr", cfg.API.Listen)

	// --- Database ------------------------------------------------------------
	// Open a pgx connection pool and run embedded migrations. The pool is
	// closed after the HTTP server has drained all in-flight requests.
	// When skipDB is set (unit tests), this block is bypassed so tests can
	// exercise the server lifecycle without a running PostgreSQL instance.
	if opts == nil || !opts.skipDB {
		pool, err := postgres.New(ctx, cfg.Database.DSN, log)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer pool.Close()
	}

	// --- Router --------------------------------------------------------------
	// Store parameters are nil until the store implementations are wired.
	// The router still registers all routes; unimplemented handlers return 501.
	h := router.New(nil, nil, nil, cfg.API.InternalSecret, log)
	log.Info("router initialized, all routes registered")

	// --- Listener & server ---------------------------------------------------
	srv, ln, err := startListener(cfg.API.Listen, h, log)
	if err != nil {
		return err
	}

	// Notify tests that the server is ready to accept connections. Production
	// callers pass nil opts, so this branch is skipped at runtime.
	if opts != nil && opts.ready != nil {
		opts.ready <- ln.Addr()
	}

	// --- Serve requests in the background ------------------------------------
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	log.Info("server is ready and accepting connections", "addr", ln.Addr().String())

	// --- Wait for shutdown signal --------------------------------------------
	return awaitShutdown(ctx, srv, serveErr, log)
}

// -----------------------------------------------------------------------------
// Private helpers
// -----------------------------------------------------------------------------

// parseFlags parses the CLI arguments and returns the config file path.
// Returns an error containing the usage string when required flags are missing.
func parseFlags(args []string) (string, error) {
	parser := argparse.NewParser(programName, programDescription)

	configPath := parser.String("c", "config", &argparse.Options{
		Required: true,
		Help:     configHelp,
	})

	if err := parser.Parse(args); err != nil {
		fmt.Print(parser.Usage(fmt.Sprintf("failed to parse arguments (%s)", err)))
		return "", fmt.Errorf("failed to parse arguments (%s)", err)
	}

	return *configPath, nil
}

// startListener opens a TCP listener on addr and wraps it in an
// http.Server. Binding the port eagerly (before Serve) lets the process
// fail fast when the address is already in use.
func startListener(addr string, handler http.Handler, log *slog.Logger) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind TCP listener on %s: %w", addr, err)
	}

	log.Info("TCP listener bound successfully", "requested_addr", addr, "resolved_addr", ln.Addr().String())

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return srv, ln, nil
}

// awaitShutdown blocks until ctx is cancelled (signal received), then
// gracefully drains the server within shutdownTimeout. It returns nil on
// a clean shutdown and an error if the drain times out or Serve failed
// with something other than the expected ErrServerClosed.
func awaitShutdown(ctx context.Context, srv *http.Server, serveErr <-chan error, log *slog.Logger) error {
	// Block until the parent context is cancelled by a signal or test.
	<-ctx.Done()
	log.Info("shutdown signal received, beginning graceful drain", "timeout", shutdownTimeout.String())

	// Give in-flight requests up to shutdownTimeout to complete. After
	// that deadline the server forcefully closes remaining connections.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	// Serve always returns after Shutdown. ErrServerClosed is the normal
	// signal that Shutdown was called; anything else is unexpected.
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server exited with unexpected error: %w", err)
	}

	log.Info("graceful shutdown completed, all connections drained")
	return nil
}
