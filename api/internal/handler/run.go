// run.go — ingestion and newsletter run handlers (stubs)
//
// Implements GET/POST for /api/v1/runs/ingestion and /api/v1/runs/newsletter.
// All handlers return 501 Not Implemented until the store layer is wired.

package handler

import (
	"net/http"

	"daily-patch/api/internal/response"
	"daily-patch/api/internal/store"
)

// ListIngestionRuns handles GET /api/v1/runs/ingestion.
func ListIngestionRuns(runs store.RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// CreateIngestionRun handles POST /api/v1/runs/ingestion.
func CreateIngestionRun(runs store.RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// ListNewsletterRuns handles GET /api/v1/runs/newsletter.
func ListNewsletterRuns(runs store.RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// CreateNewsletterRun handles POST /api/v1/runs/newsletter.
func CreateNewsletterRun(runs store.RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}
