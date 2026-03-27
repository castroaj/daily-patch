// health.go — GET /health handler
//
// Returns a 200 envelope with service name and status. Used by Docker Compose
// for readiness checks. No auth required.

package handler

import (
	"net/http"

	"daily-patch/api/internal/response"
)

// healthResult is the payload returned by the health endpoint.
type healthResult struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Health handles GET /health.
func Health(w http.ResponseWriter, r *http.Request) {
	response.Write(w, http.StatusOK, "", "", healthResult{
		Status:  "ok",
		Service: "api",
	})
}
