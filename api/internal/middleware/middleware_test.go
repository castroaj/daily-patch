// middleware_test.go — tests for all middleware constructors

package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"daily-patch/api/internal/metrics"
)

// -----------------------------------------------------------------------------
// RequestID
// -----------------------------------------------------------------------------

func TestRequestID_SetsResponseHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := serve(RequestID(discardLogger()), okHandler, req)

	if rec.Header().Get(headerRequestID) == "" {
		t.Error("expected X-Request-ID response header to be set")
	}
}

func TestRequestID_PropagatesIncomingID(t *testing.T) {
	const incoming = "existing-id-123"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerRequestID, incoming)

	rec := serve(RequestID(discardLogger()), okHandler, req)

	if got := rec.Header().Get(headerRequestID); got != incoming {
		t.Errorf("X-Request-ID = %q, want %q", got, incoming)
	}
}

func TestRequestID_StoresOnContext(t *testing.T) {
	var capturedID string
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	serve(RequestID(discardLogger()), capture, req)

	if capturedID == "" {
		t.Error("RequestIDFromContext returned empty string inside handler")
	}
}

func TestRequestID_PropagatedIDOnContext(t *testing.T) {
	const incoming = "my-request-id"
	var capturedID string
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerRequestID, incoming)
	serve(RequestID(discardLogger()), capture, req)

	if capturedID != incoming {
		t.Errorf("context request ID = %q, want %q", capturedID, incoming)
	}
}

// -----------------------------------------------------------------------------
// Recovery
// -----------------------------------------------------------------------------

func TestRecovery_CatchesPanic(t *testing.T) {
	rec := serve(Recovery(discardLogger()), panicHandler,
		httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestRecovery_PassesThrough(t *testing.T) {
	rec := serve(Recovery(discardLogger()), okHandler,
		httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// Logger
// -----------------------------------------------------------------------------

func TestLogger_PassesThrough(t *testing.T) {
	rec := serve(Logger(discardLogger()), okHandler,
		httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// Auth
// -----------------------------------------------------------------------------

func TestAuth_MissingHeader(t *testing.T) {
	rec := serve(Auth("correct-secret"), okHandler,
		httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_WrongSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerInternalSecret, "wrong-secret")
	rec := serve(Auth("correct-secret"), okHandler, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_CorrectSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerInternalSecret, "correct-secret")
	rec := serve(Auth("correct-secret"), okHandler, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuth_ResponseIsJSON(t *testing.T) {
	rec := serve(Auth("secret"), okHandler,
		httptest.NewRequest(http.MethodGet, "/", nil))

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// -----------------------------------------------------------------------------
// Metrics
// -----------------------------------------------------------------------------

func TestMetrics_IncrementsRequestCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := testMetrics(t, reg)

	r := chiWithMetrics(m)
	r.Get("/test", okHandler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	count := counterValue(t, reg, "api_http_requests_total", "GET", "/test", "200")
	if count != 1 {
		t.Errorf("request counter = %v, want 1", count)
	}
}

func TestMetrics_RecordsDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := testMetrics(t, reg)

	r := chiWithMetrics(m)
	r.Get("/test", okHandler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "api_http_request_duration_seconds" {
			for _, metric := range fam.GetMetric() {
				if metric.GetHistogram().GetSampleCount() >= 1 {
					return
				}
			}
		}
	}

	t.Fatal("duration histogram has no samples")
}

func TestMetrics_InFlightGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := testMetrics(t, reg)

	var duringGauge float64
	var wg sync.WaitGroup
	wg.Add(1)

	blockingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duringGauge = gaugeValue(t, reg, "api_http_requests_in_flight")
		wg.Done()
		w.WriteHeader(http.StatusOK)
	})

	r := chiWithMetrics(m)
	r.Get("/test", blockingHandler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	wg.Wait()

	if duringGauge != 1 {
		t.Errorf("in-flight during request = %v, want 1", duringGauge)
	}

	afterGauge := gaugeValue(t, reg, "api_http_requests_in_flight")
	if afterGauge != 0 {
		t.Errorf("in-flight after request = %v, want 0", afterGauge)
	}
}

func TestMetrics_StatusLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := testMetrics(t, reg)

	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	r := chiWithMetrics(m)
	r.Get("/missing", notFoundHandler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	count := counterValue(t, reg, "api_http_requests_total", "GET", "/missing", "404")
	if count != 1 {
		t.Errorf("request counter for 404 = %v, want 1", count)
	}
}

func TestMetrics_PathUsesRoutePattern(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := testMetrics(t, reg)

	r := chiWithMetrics(m)
	r.Get("/test/{id}", okHandler.ServeHTTP)

	for _, id := range []string{"aaa", "bbb"} {
		req := httptest.NewRequest(http.MethodGet, "/test/"+id, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	}

	count := counterValue(t, reg, "api_http_requests_total", "GET", "/test/{id}", "200")
	if count != 2 {
		t.Errorf("request counter for pattern = %v, want 2", count)
	}
}

func TestMetrics_NilMetrics_PassesThrough(t *testing.T) {
	r := chiWithMetrics(nil)
	r.Get("/test", okHandler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// okHandler is a trivial handler that writes 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// panicHandler always panics.
var panicHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	panic("test panic")
})

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

func serve(mw func(http.Handler) http.Handler, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, req)
	return rec
}

// testMetrics creates a fresh *metrics.Metrics registered with reg.
func testMetrics(t *testing.T, reg *prometheus.Registry) *metrics.Metrics {
	t.Helper()

	m, err := metrics.Register(reg)
	if err != nil {
		t.Fatalf("metrics.Register() error: %v", err)
	}

	return m
}

// chiWithMetrics returns a chi.Mux with only the Metrics middleware applied.
func chiWithMetrics(m *metrics.Metrics) *chi.Mux {
	r := chi.NewRouter()
	r.Use(Metrics(m))
	return r
}

// counterValue reads the value of a counter metric with the given labels.
func counterValue(t *testing.T, reg *prometheus.Registry, name, method, path, status string) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}

		for _, metric := range fam.GetMetric() {
			labels := map[string]string{}
			for _, l := range metric.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}

			if labels["method"] == method && labels["path"] == path && labels["status"] == status {
				return metric.GetCounter().GetValue()
			}
		}
	}

	t.Fatalf("counter %s{method=%q,path=%q,status=%q} not found", name, method, path, status)
	return 0
}

// gaugeValue reads the current value of a gauge metric.
func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}

		metrics := fam.GetMetric()
		if len(metrics) > 0 {
			return metrics[0].GetGauge().GetValue()
		}
	}

	t.Fatalf("gauge %s not found", name)
	return 0
}
