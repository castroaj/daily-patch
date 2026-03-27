// store.go — repository interfaces and domain types for the api service
//
// This is a leaf package: it imports nothing from within api/internal/.
// Handlers depend on these interfaces; concrete pgx implementations live in
// internal/store/postgres/ and are injected by router.New via main.go.

package store

import (
	"context"
	"encoding/json"
	"time"
)

// -----------------------------------------------------------------------------
// Domain types
// -----------------------------------------------------------------------------

// Vuln is a normalized vulnerability record.
type Vuln struct {
	ID          string
	CVEID       string
	GHSAID      string
	EDBID       string
	Source      string
	Title       string
	Description string
	CVSSScore   float64
	CVSSVector  string
	PublishedAt *time.Time
	UpdatedAt   *time.Time
	IngestedAt  time.Time
	RawJSON     json.RawMessage
}

// Score is an LLM-generated relevance score for a vulnerability.
type Score struct {
	ID              string
	VulnID          string
	Score           int
	Rationale       string
	ScoredAt        time.Time
	NewsletterRunID *string
}

// IngestionRun is a record of a completed ingestion pipeline run.
type IngestionRun struct {
	ID           string
	Source       string
	StartedAt    time.Time
	FinishedAt   *time.Time
	ItemsFetched int
	ItemsNew     int
}

// NewsletterRun is a record of a completed newsletter generation run.
type NewsletterRun struct {
	ID              string
	RunAt           time.Time
	ItemCount       int
	DeliveryTargets json.RawMessage
}

// ListFilters constrains the results returned by VulnStore.List.
// Zero values are ignored — no filter is applied for that field.
// Scored filters on whether the vuln has at least one score record.
type ListFilters struct {
	Source  string
	MinCVSS *float64
	MaxCVSS *float64
	Since   *time.Time
	Until   *time.Time
	Scored  *bool
}

// -----------------------------------------------------------------------------
// Repository interfaces
// -----------------------------------------------------------------------------

// VulnStore manages vulnerability records.
type VulnStore interface {
	// Create inserts a new vulnerability and returns the assigned UUID.
	Create(ctx context.Context, v Vuln) (id string, err error)

	// GetByID returns a single vulnerability by its database UUID.
	// Returns pgx.ErrNoRows if not found.
	GetByID(ctx context.Context, id string) (Vuln, error)

	// GetByCanonicalID looks up by cve_id, ghsa_id, or edb_id (UNIQUE columns).
	// Returns found=false (no error) when no record matches any of the IDs.
	GetByCanonicalID(ctx context.Context, cveID, ghsaID, edbID string) (Vuln, bool, error)

	// List returns vulnerabilities matching f.
	List(ctx context.Context, f ListFilters) ([]Vuln, error)

	// Update replaces the mutable fields of an existing record by UUID.
	// Returns pgx.ErrNoRows if id does not exist.
	Update(ctx context.Context, id string, v Vuln) error
}

// ScoreStore manages relevance score records.
type ScoreStore interface {
	// ListByVulnID returns all scores for the given vulnerability UUID.
	ListByVulnID(ctx context.Context, vulnID string) ([]Score, error)

	// Create inserts a new score record.
	Create(ctx context.Context, s Score) error
}

// RunStore manages ingestion and newsletter run records.
type RunStore interface {
	ListIngestion(ctx context.Context) ([]IngestionRun, error)
	CreateIngestion(ctx context.Context, r IngestionRun) error
	ListNewsletter(ctx context.Context) ([]NewsletterRun, error)
	CreateNewsletter(ctx context.Context, r NewsletterRun) error

	// LastSuccessfulIngestion returns finished_at for the most recent completed
	// ingestion run for source. Returns a zero time.Time if no run exists.
	LastSuccessfulIngestion(ctx context.Context, source string) (time.Time, error)
}
