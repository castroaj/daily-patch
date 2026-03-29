// metrics.go — Prometheus metric collectors for the API service
//
// Register creates and registers three Prometheus collectors with the
// provided registerer: a request counter, a request duration histogram,
// and an in-flight gauge. The returned Metrics struct is passed to the
// Metrics middleware for instrumentation.

package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	// namespace prefixes all metric names (e.g. "api_http_requests_total").
	namespace = "api"

	// subsystem groups all metrics under "http".
	subsystem = "http"

	// labelMethod is the HTTP method label (GET, POST, etc.).
	labelMethod = "method"

	// labelPath is the matched route pattern label.
	labelPath = "path"

	// labelStatus is the HTTP response status code label.
	labelStatus = "status"
)

// -----------------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------------

// Metrics holds the three Prometheus collectors used by the HTTP
// middleware to instrument every request.
type Metrics struct {
	// Requests counts completed HTTP requests, partitioned by method,
	// matched route pattern, and response status code.
	Requests *prometheus.CounterVec

	// Duration observes request latency in seconds, partitioned by
	// method and matched route pattern.
	Duration *prometheus.HistogramVec

	// InFlight tracks the number of HTTP requests currently being served.
	InFlight prometheus.Gauge
}

// -----------------------------------------------------------------------------
// Public functions
// -----------------------------------------------------------------------------

// Register creates the three Prometheus collectors and registers them
// with reg. Returns an error if any collector is already registered.
func Register(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total number of completed HTTP requests.",
		}, []string{labelMethod, labelPath, labelStatus}),

		Duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "Duration of HTTP requests in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{labelMethod, labelPath}),

		InFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		}),
	}

	for _, c := range []prometheus.Collector{m.Requests, m.Duration, m.InFlight} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("register collector: %w", err)
		}
	}

	return m, nil
}
