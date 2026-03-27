// main.go — entry point for the api service
//
// Loads configuration, initialises structured logging, assembles the router,
// and starts the HTTP server. Database initialisation and store wiring are
// added in a follow-on iteration once the postgres layer exists.

package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/akamensky/argparse"

	"daily-patch/api/internal/config"
	"daily-patch/api/internal/router"
)

const (
	programName        = "api"
	programDescription = `REST API server for the daily-patch vulnerability intelligence pipeline.
Ingests CVE/advisory data, serves it over a versioned REST API, and supports
LLM-based relevance scoring via internal service-to-service auth.

Environment variables:
  API_INTERNAL_SECRET   Shared secret for X-Internal-Secret header auth. Required.
  DATABASE_URL          PostgreSQL DSN (e.g. postgres://user:pass@host/db). Required.

Examples:
  # Run inside Docker Compose:
  api -c /app/config.yaml

  # Run locally against a dev config:
  API_INTERNAL_SECRET=dev-secret DATABASE_URL=postgres://localhost/daily_patch \
    api -c config.local.yaml`

	configHelp = "Path to the YAML configuration file. Secrets (API_INTERNAL_SECRET, DATABASE_URL) are read from environment variables and overlay the file values."
)

func main() {
	parser := argparse.NewParser(programName, programDescription)

	configPath := parser.String("c", "config", &argparse.Options{
		Required: true,
		Help:     configHelp,
	})

	if err := parser.Parse(os.Args); err != nil {
		os.Stderr.WriteString(parser.Usage(err))
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	h := router.New(nil, nil, nil, cfg.API.InternalSecret, log)

	srv := &http.Server{
		Addr:    cfg.API.Listen,
		Handler: h,
	}

	log.Info("api listening", "addr", cfg.API.Listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
