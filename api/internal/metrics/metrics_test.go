// metrics_test.go — tests for Prometheus metric registration
//
// All tests use a fresh prometheus.NewRegistry() to avoid collisions
// with the global default registry.

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	// expectedRequestsName is the fully qualified name for the request counter.
	expectedRequestsName = "api_http_requests_total"

	// expectedDurationName is the fully qualified name for the duration histogram.
	expectedDurationName = "api_http_request_duration_seconds"

	// expectedInFlightName is the fully qualified name for the in-flight gauge.
	expectedInFlightName = "api_http_requests_in_flight"
)

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestRegister_ReturnsMetrics(t *testing.T) {
	t.Parallel()

	m, err := Register(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if m == nil {
		t.Fatal("Register() returned nil Metrics")
	}
	if m.Requests == nil {
		t.Fatal("Requests counter is nil")
	}
	if m.Duration == nil {
		t.Fatal("Duration histogram is nil")
	}
	if m.InFlight == nil {
		t.Fatal("InFlight gauge is nil")
	}
}

func TestRegister_MetricNames(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()

	m, err := Register(reg)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	// Touch each collector so Gather returns them.
	m.Requests.WithLabelValues("GET", "/", "200").Inc()
	m.Duration.WithLabelValues("GET", "/").Observe(0.001)
	m.InFlight.Set(0)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() returned error: %v", err)
	}

	expected := map[string]bool{
		expectedRequestsName: false,
		expectedDurationName: false,
		expectedInFlightName: false,
	}

	for _, fam := range families {
		if _, ok := expected[fam.GetName()]; ok {
			expected[fam.GetName()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected metric %q not found in gathered families", name)
		}
	}
}

func TestRegister_DoubleRegistration(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()

	_, err := Register(reg)
	if err != nil {
		t.Fatalf("first Register() returned error: %v", err)
	}

	_, err = Register(reg)
	if err == nil {
		t.Fatal("second Register() should return error, got nil")
	}
}

func TestRegister_RequestsLabels(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()

	m, err := Register(reg)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	// Counter should accept method, path, and status labels without panicking.
	m.Requests.WithLabelValues("GET", "/health", "200").Inc()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() returned error: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != expectedRequestsName {
			continue
		}

		metrics := fam.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("expected 1 metric, got %d", len(metrics))
		}

		labels := metrics[0].GetLabel()
		if len(labels) != 3 {
			t.Fatalf("expected 3 labels, got %d", len(labels))
		}

		labelNames := map[string]string{}
		for _, l := range labels {
			labelNames[l.GetName()] = l.GetValue()
		}

		for _, name := range []string{labelMethod, labelPath, labelStatus} {
			if _, ok := labelNames[name]; !ok {
				t.Errorf("expected label %q not found", name)
			}
		}

		return
	}

	t.Fatal("requests metric family not found after Inc()")
}

func TestRegister_DurationLabels(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()

	m, err := Register(reg)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	// Histogram should accept method and path labels without panicking.
	m.Duration.WithLabelValues("POST", "/api/v1/vulns").Observe(0.123)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() returned error: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() != expectedDurationName {
			continue
		}

		metrics := fam.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("expected 1 metric, got %d", len(metrics))
		}

		labels := metrics[0].GetLabel()
		if len(labels) != 2 {
			t.Fatalf("expected 2 labels, got %d", len(labels))
		}

		labelNames := map[string]string{}
		for _, l := range labels {
			labelNames[l.GetName()] = l.GetValue()
		}

		for _, name := range []string{labelMethod, labelPath} {
			if _, ok := labelNames[name]; !ok {
				t.Errorf("expected label %q not found", name)
			}
		}

		return
	}

	t.Fatal("duration metric family not found after Observe()")
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------
