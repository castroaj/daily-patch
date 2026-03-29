// middleware.go — HTTP middleware for the api service
//
// Provides five middleware constructors: RequestID, Recovery, Logger, Metrics,
// and Auth. Each returns a func(http.Handler) http.Handler compatible with
// chi's Use() method. Auth is applied only to the /api/v1/ sub-router.

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"daily-patch/api/internal/metrics"
	"daily-patch/api/internal/response"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	headerInternalSecret   = "X-Internal-Secret"
	headerRequestID        = "X-Request-ID"
	errUnauthorized        = "unauthorized"
	errDetailMissingSecret = "X-Internal-Secret header is missing or invalid"
)

// -----------------------------------------------------------------------------
// Context key
// -----------------------------------------------------------------------------

type contextKey int

const requestIDKey contextKey = iota

// -----------------------------------------------------------------------------
// wrappedResponseWriter
// -----------------------------------------------------------------------------

// wrappedResponseWriter wraps http.ResponseWriter to capture the status code
// written by the handler, used by Logger and Metrics.
type wrappedResponseWriter struct {
	http.ResponseWriter
	status int
}

func newWrapped(w http.ResponseWriter) *wrappedResponseWriter {
	return &wrappedResponseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *wrappedResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// -----------------------------------------------------------------------------
// Exported helpers
// -----------------------------------------------------------------------------

// RequestIDFromContext returns the request ID stored by the RequestID
// middleware, or an empty string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// -----------------------------------------------------------------------------
// Middleware constructors
// -----------------------------------------------------------------------------

// RequestID generates or propagates an X-Request-ID, stores it on the context,
// and echoes it in the response header.
func RequestID(_ *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(headerRequestID)
			if id == "" {
				id = generateID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			w.Header().Set(headerRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recovery catches panics, logs the stack trace, and writes a 500 response.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(r.Context(), "panic recovered",
						"error", rec,
						"stack", string(debug.Stack()),
						"request_id", RequestIDFromContext(r.Context()),
					)
					response.Write(w, http.StatusInternalServerError,
						"internal_error", "an unexpected error occurred", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logger logs each completed request: method, path, status, duration, request ID.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := newWrapped(w)
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFromContext(r.Context()),
			)
		})
	}
}

// Metrics records Prometheus request count, duration, and in-flight gauge.
// When m is nil the middleware is a pass-through no-op, which preserves
// backward compatibility for tests that do not need metrics.
func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.InFlight.Inc()
			defer m.InFlight.Dec()

			start := time.Now()
			ww := newWrapped(w)

			next.ServeHTTP(ww, r)

			// chi resolves the route pattern during ServeHTTP, so we
			// read it after the handler returns.
			pattern := chi.RouteContext(r.Context()).RoutePattern()

			m.Requests.WithLabelValues(r.Method, pattern, strconv.Itoa(ww.status)).Inc()
			m.Duration.WithLabelValues(r.Method, pattern).Observe(time.Since(start).Seconds())
		})
	}
}

// Auth validates the X-Internal-Secret header. Returns 401 if absent or wrong.
// Applied only to the /api/v1/ sub-router.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(headerInternalSecret) != secret {
				response.Write(w, http.StatusUnauthorized,
					errUnauthorized, errDetailMissingSecret, nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// -----------------------------------------------------------------------------
// Private helpers
// -----------------------------------------------------------------------------

// generateID returns a random 16-byte hex string for use as a request ID.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
