// vuln.go — vulnerability handlers (stubs)
//
// Implements GET/POST/PUT for /api/v1/vulns and /api/v1/vulns/{id}.
// All handlers return 501 Not Implemented until the store layer is wired.

package handler

import (
	"net/http"

	"daily-patch/api/internal/response"
	"daily-patch/api/internal/store"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	errNotImplemented       = "not_implemented"
	errDetailNotImplemented = "this endpoint is not yet implemented"
)

// -----------------------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------------------

// ListVulns handles GET /api/v1/vulns.
func ListVulns(vulns store.VulnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// GetVuln handles GET /api/v1/vulns/{id}.
func GetVuln(vulns store.VulnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// CreateVuln handles POST /api/v1/vulns.
func CreateVuln(vulns store.VulnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}

// UpdateVuln handles PUT /api/v1/vulns/{id}.
func UpdateVuln(vulns store.VulnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusNotImplemented, errNotImplemented, errDetailNotImplemented, nil)
	}
}
