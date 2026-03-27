// middleware_test.go — tests for all middleware constructors

package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Helpers
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
