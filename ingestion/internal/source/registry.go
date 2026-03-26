// registry.go — Registry holds all registered Source implementations
//
// Callers register sources once at startup via Register, then retrieve an
// ordered slice via All for the runner to iterate.

package source

import (
	"fmt"
	"sort"

	"daily-patch/ingestion/internal/types"
)

// Registry holds all registered Source implementations keyed by SourceType.
type Registry struct {
	sources map[types.SourceType]Source
}

// NewRegistry returns an empty, ready-to-use Registry.
func NewRegistry() *Registry {
	return &Registry{sources: make(map[types.SourceType]Source)}
}

// Register adds s to the registry. Panics if a source with the same Name is
// already registered.
func (r *Registry) Register(s Source) {
	if _, exists := r.sources[s.Name()]; exists {
		panic(fmt.Sprintf("source already registered: %s", s.Name()))
	}
	r.sources[s.Name()] = s
}

// All returns a snapshot of all registered sources sorted by name ascending.
func (r *Registry) All() []Source {
	out := make([]Source, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}
