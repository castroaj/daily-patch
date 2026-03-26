// registry_test.go — tests for Registry: NewRegistry, Register, and All

package source

import (
	"context"
	"testing"
	"time"

	"daily-patch/ingestion/internal/types"
)

// -----------------------------------------------------------------------------
// TestNewRegistry
// -----------------------------------------------------------------------------

func TestNewRegistry_returnsNonNil(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil Registry")
	}
}

func TestNewRegistry_sourcesMapInitialized(t *testing.T) {
	r := NewRegistry()
	if r.sources == nil {
		t.Fatal("expected sources map to be initialized")
	}
}

// -----------------------------------------------------------------------------
// TestRegister
// -----------------------------------------------------------------------------

func TestRegister_storesSingleSource(t *testing.T) {
	r := NewRegistry()
	s := newStub(types.SourceNVD)
	r.Register(s)

	if len(r.sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(r.sources))
	}
	if r.sources[types.SourceNVD] != s {
		t.Fatal("registered source not stored under its SourceType key")
	}
}

func TestRegister_storesMultipleSources(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub(types.SourceNVD))
	r.Register(newStub(types.SourceGHSA))

	if len(r.sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(r.sources))
	}
}

func TestRegister_panicOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub(types.SourceNVD))

	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
		msg, ok := v.(string)
		if !ok {
			t.Fatalf("expected panic value to be a string, got %T", v)
		}
		if !contains(msg, string(types.SourceNVD)) {
			t.Fatalf("panic message %q does not contain SourceType name %q", msg, types.SourceNVD)
		}
	}()

	r.Register(newStub(types.SourceNVD))
}

// -----------------------------------------------------------------------------
// TestAll
// -----------------------------------------------------------------------------

func TestAll_emptyRegistryReturnsEmptySlice(t *testing.T) {
	r := NewRegistry()
	got := r.All()
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got length %d", len(got))
	}
}

func TestAll_returnsSingleSource(t *testing.T) {
	r := NewRegistry()
	s := newStub(types.SourceNVD)
	r.Register(s)

	got := r.All()
	if len(got) != 1 {
		t.Fatalf("expected 1 source, got %d", len(got))
	}
	if got[0] != s {
		t.Fatal("returned source does not match registered source")
	}
}

func TestAll_returnsAllSources(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub(types.SourceNVD))
	r.Register(newStub(types.SourceGHSA))
	r.Register(newStub(types.SourceExploitDB))

	got := r.All()
	if len(got) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(got))
	}
}

func TestAll_sortedByName(t *testing.T) {
	r := NewRegistry()
	// Register in NVD → ExploitDB → GHSA order.
	r.Register(newStub(types.SourceNVD))
	r.Register(newStub(types.SourceExploitDB))
	r.Register(newStub(types.SourceGHSA))

	got := r.All()
	// Lexicographic ascending: exploitdb < ghsa < nvd
	want := []types.SourceType{types.SourceExploitDB, types.SourceGHSA, types.SourceNVD}
	for i, s := range got {
		if s.Name() != want[i] {
			t.Fatalf("position %d: expected %q, got %q", i, want[i], s.Name())
		}
	}
}

func TestAll_doesNotMutateRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub(types.SourceNVD))
	r.Register(newStub(types.SourceGHSA))

	first := r.All()
	second := r.All()

	if len(first) != len(second) {
		t.Fatalf("first call returned %d, second returned %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name() != second[i].Name() {
			t.Fatalf("position %d: first=%q second=%q", i, first[i].Name(), second[i].Name())
		}
	}
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

type stubSource struct{ name types.SourceType }

func (s *stubSource) Name() types.SourceType { return s.name }
func (s *stubSource) Fetch(_ context.Context, _ time.Time) ([]types.Vulnerability, error) {
	return nil, nil
}

func newStub(name types.SourceType) Source { return &stubSource{name: name} }

// contains reports whether substr is present in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
