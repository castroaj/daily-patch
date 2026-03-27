// score.go — score handlers (stubs)
//
// Implements GET/POST for /api/v1/vulns/{id}/scores.
// All handlers return 501 Not Implemented until the store layer is wired.

package handler

import (
	"net/http"

	"daily-patch/api/internal/response"
	"daily-patch/api/internal/store"
)

// ListScores handles GET /api/v1/vulns/{id}/scores.
func ListScores(scores store.ScoreStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// CreateScore handles POST /api/v1/vulns/{id}/scores.
func CreateScore(scores store.ScoreStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}
