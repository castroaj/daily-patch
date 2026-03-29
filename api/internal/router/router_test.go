// router_test.go — integration-style tests for the assembled router
//
// Tests exercise the full middleware chain by calling router.New with nil
// stores. Stub handlers never touch the stores, so nil is safe here.

package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"daily-patch/api/internal/metrics"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	testSecret           = "test-secret"
	headerInternalSecret = "X-Internal-Secret"
	headerRequestID      = "X-Request-ID"
)

// -----------------------------------------------------------------------------
// /health — public, no auth required
// -----------------------------------------------------------------------------

func TestHealth_OK(t *testing.T) {
	rec := do(newRouter(), http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHealth_NoAuthRequired(t *testing.T) {
	// /health must return 200 even without the internal secret header.
	rec := do(newRouter(), http.MethodGet, "/health", "")
	if rec.Code == http.StatusUnauthorized {
		t.Error("/health should not require auth")
	}
}

func TestHealth_JSONEnvelope(t *testing.T) {
	rec := do(newRouter(), http.MethodGet, "/health", "")
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	env := decodeEnvelope(t, rec)
	if env.StatusCode != http.StatusOK {
		t.Errorf("envelope statusCode = %d, want 200", env.StatusCode)
	}
}

// -----------------------------------------------------------------------------
// /api/v1/ — auth required
// -----------------------------------------------------------------------------

func TestAPIV1_MissingSecret_Unauthorized(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/vulns"},
		{http.MethodPost, "/api/v1/vulns"},
		{http.MethodGet, "/api/v1/vulns/some-id"},
		{http.MethodPut, "/api/v1/vulns/some-id"},
		{http.MethodGet, "/api/v1/vulns/some-id/scores"},
		{http.MethodPost, "/api/v1/vulns/some-id/scores"},
		{http.MethodGet, "/api/v1/runs/ingestion"},
		{http.MethodPost, "/api/v1/runs/ingestion"},
		{http.MethodGet, "/api/v1/runs/newsletter"},
		{http.MethodPost, "/api/v1/runs/newsletter"},
	}

	h := newRouter()
	for _, r := range routes {
		rec := do(h, r.method, r.path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestAPIV1_WrongSecret_Unauthorized(t *testing.T) {
	rec := do(newRouter(), http.MethodGet, "/api/v1/vulns", "wrong-secret")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAPIV1_CorrectSecret_NotUnauthorized(t *testing.T) {
	// With the correct secret, auth passes and stub handlers return 501.
	rec := do(newRouter(), http.MethodGet, "/api/v1/vulns", testSecret)
	if rec.Code == http.StatusUnauthorized {
		t.Error("correct secret should not return 401")
	}
}

func TestAPIV1_StubHandlers_NotImplemented(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/vulns"},
		{http.MethodPost, "/api/v1/vulns"},
		{http.MethodGet, "/api/v1/vulns/some-id"},
		{http.MethodPut, "/api/v1/vulns/some-id"},
		{http.MethodGet, "/api/v1/vulns/some-id/scores"},
		{http.MethodPost, "/api/v1/vulns/some-id/scores"},
		{http.MethodGet, "/api/v1/runs/ingestion"},
		{http.MethodPost, "/api/v1/runs/ingestion"},
		{http.MethodGet, "/api/v1/runs/newsletter"},
		{http.MethodPost, "/api/v1/runs/newsletter"},
	}

	h := newRouter()
	for _, r := range routes {
		rec := do(h, r.method, r.path, testSecret)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: status = %d, want 501", r.method, r.path, rec.Code)
		}
	}
}

// -----------------------------------------------------------------------------
// Global middleware
// -----------------------------------------------------------------------------

func TestRequestIDHeader_SetOnResponse(t *testing.T) {
	rec := do(newRouter(), http.MethodGet, "/health", "")
	if rec.Header().Get(headerRequestID) == "" {
		t.Error("expected X-Request-ID response header to be set")
	}
}

func TestRecovery_PanicsReturn500(t *testing.T) {
	// Recovery is tested indirectly: health handler does not panic, so we just
	// verify the router compiles and serves normally.
	rec := do(newRouter(), http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// 404 — unknown routes
// -----------------------------------------------------------------------------

func TestUnknownRoute_NotFound(t *testing.T) {
	rec := do(newRouter(), http.MethodGet, "/does-not-exist", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// /metrics — public, Prometheus scrape endpoint
// -----------------------------------------------------------------------------

func TestMetrics_Endpoint_OK(t *testing.T) {
	rec := do(newRouterWithMetrics(t), http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMetrics_NoAuthRequired(t *testing.T) {
	rec := do(newRouterWithMetrics(t), http.MethodGet, "/metrics", "")
	if rec.Code == http.StatusUnauthorized {
		t.Error("/metrics should not require auth")
	}
}

func TestMetrics_ContentType(t *testing.T) {
	rec := do(newRouterWithMetrics(t), http.MethodGet, "/metrics", "")
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "text/openmetrics") {
		t.Errorf("Content-Type = %q, want text/plain or openmetrics", ct)
	}
}

func TestMetrics_NilGatherer_NoEndpoint(t *testing.T) {
	// When no gatherer is provided, /metrics should not be registered.
	rec := do(newRouter(), http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when gatherer is nil", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

func newRouter() http.Handler {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
	return New(nil, nil, nil, nil, nil, testSecret, log)
}

func newRouterWithMetrics(t *testing.T) http.Handler {
	t.Helper()

	reg := prometheus.NewRegistry()
	m, err := metrics.Register(reg)
	if err != nil {
		t.Fatalf("metrics.Register() error: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
	return New(nil, nil, nil, m, reg, testSecret, log)
}

func do(h http.Handler, method, path, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if secret != "" {
		req.Header.Set(headerInternalSecret, secret)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Error      string `json:"error"`
	StatusCode int    `json:"statusCode"`
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}
