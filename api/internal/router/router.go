// router.go — chi mux, route registration, and middleware stack
//
// New assembles the full handler tree. Global middleware (RequestID, Recovery,
// Logger, Metrics) wraps all routes. The /api/v1/ sub-router additionally
// applies Auth. /health is public and outside the versioned prefix.

package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"daily-patch/api/internal/handler"
	"daily-patch/api/internal/middleware"
	"daily-patch/api/internal/store"
)

// New wires a chi mux with all middleware and routes. The returned handler
// is passed directly to http.Server.
func New(
	vulns store.VulnStore,
	scores store.ScoreStore,
	runs store.RunStore,
	secret string,
	log *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware applied to every route.
	r.Use(middleware.RequestID(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())

	// Public routes — no auth.
	r.Get("/health", handler.Health)

	// Authenticated routes under /api/v1/.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Auth(secret))

		r.Get("/vulns", handler.ListVulns(vulns))
		r.Post("/vulns", handler.CreateVuln(vulns))
		r.Get("/vulns/{id}", handler.GetVuln(vulns))
		r.Put("/vulns/{id}", handler.UpdateVuln(vulns))

		r.Get("/vulns/{id}/scores", handler.ListScores(scores))
		r.Post("/vulns/{id}/scores", handler.CreateScore(scores))

		r.Get("/runs/ingestion", handler.ListIngestionRuns(runs))
		r.Post("/runs/ingestion", handler.CreateIngestionRun(runs))
		r.Get("/runs/newsletter", handler.ListNewsletterRuns(runs))
		r.Post("/runs/newsletter", handler.CreateNewsletterRun(runs))
	})

	return r
}
