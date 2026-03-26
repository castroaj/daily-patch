// source.go — Source interface implemented by each vulnerability data source
//
// Each source handles its own pagination, retries, and rate limiting. The
// runner calls Fetch once per source and receives the full result set.

package source

import (
	"context"
	"time"

	"daily-patch/ingestion/internal/types"
)

// Source is implemented by each upstream vulnerability feed.
type Source interface {
	Name() types.SourceType

	// Fetch returns all records published or modified after since.
	// A zero-value since means full backfill.
	// Pagination, retries, and rate limiting are handled internally.
	// Returns a non-nil error only if the fetch cannot proceed at all.
	Fetch(ctx context.Context, since time.Time) ([]types.Vulnerability, error)
}
