// runner.go — orchestrates sequential execution of all registered sources
//
// Run iterates sources in the order provided by New. Source-level Fetch
// failures are logged and skipped; Run returns non-nil only when every
// source errors.

package runner

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"daily-patch/ingestion/internal/apiclient"
	"daily-patch/ingestion/internal/source"
	"daily-patch/ingestion/internal/types"
)

// -----------------------------------------------------------------------------
// Public types
// -----------------------------------------------------------------------------

// Runner drives each registered source through the full fetch → dedup →
// persist → record cycle sequentially.
type Runner struct {
	client  apiclient.APIClient
	sources []source.Source
}

// New returns a Runner that will execute the provided sources in order.
func New(client apiclient.APIClient, sources []source.Source) *Runner {
	return &Runner{
		client:  client,
		sources: sources,
	}
}

// Run iterates over all registered sources in order, calling runSource for
// each. It checks for context cancellation before each source and returns
// ctx.Err() immediately if the context is done. Fetch errors from individual
// sources are accumulated and returned as a joined error after all sources
// have been attempted.
func (r *Runner) Run(ctx context.Context) error {
	var errs []error
	for _, src := range r.sources {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := r.runSource(ctx, src); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// -----------------------------------------------------------------------------
// Private methods
// -----------------------------------------------------------------------------

// runSource drives a single source through its full cycle: resolve the last
// successful run time, fetch new records, persist them, and record the run
// outcome. It returns a non-nil error only when Fetch fails; per-record errors
// and RecordRun errors are logged and swallowed. Context cancellation causes
// an immediate return of ctx.Err() without recording a partial run.
func (r *Runner) runSource(ctx context.Context, src source.Source) error {
	startedAt := time.Now()
	slog.InfoContext(ctx, "starting source", "source", src.Name())

	since := r.resolveLastRun(ctx, src.Name())

	vulns, err := src.Fetch(ctx, since)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.WarnContext(ctx, "fetch failed", "source", src.Name(), "error", err)
		r.recordRun(ctx, src.Name(), startedAt, 0, 0)
		return err
	}

	itemsNew := r.persistVulns(ctx, vulns)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r.recordRun(ctx, src.Name(), startedAt, len(vulns), itemsNew)
	slog.InfoContext(ctx, "source complete", "source", src.Name(), "fetched", len(vulns), "new", itemsNew)
	return nil
}

// resolveLastRun returns the finished_at timestamp of the most recent
// successful run for src. If no prior run exists or the API call fails, it
// logs a warning and returns a zero time.Time so the caller performs a full
// backfill.
func (r *Runner) resolveLastRun(ctx context.Context, src types.SourceType) time.Time {
	since, err := r.client.LastSuccessfulRun(ctx, src)
	if err != nil {
		slog.WarnContext(ctx, "last successful run lookup failed, using zero time for full backfill",
			"source", src, "error", err)
		return time.Time{}
	}
	return since
}

// persistVulns iterates over a slice of vulnerabilities, checking whether
// each already exists in the API by canonical ID. Existing records are updated
// via UpdateVuln; new records are created via CreateVuln. It returns the count
// of successfully created (new) records. Per-record errors are logged at WARN
// and skipped without aborting the loop. Successful creates and updates are
// logged at DEBUG.
func (r *Runner) persistVulns(ctx context.Context, vulns []types.Vulnerability) int {
	itemsNew := 0
	for _, v := range vulns {
		id, found, err := r.client.CheckExists(ctx, v.CVEID, v.GHSAID, v.EDBID)
		if err != nil {
			slog.WarnContext(ctx, "check exists failed, skipping record", "error", err)
			continue
		}
		if found {
			if err := r.client.UpdateVuln(ctx, id, v); err != nil {
				slog.WarnContext(ctx, "update vuln failed, skipping record", "id", id, "error", err)
			} else {
				slog.DebugContext(ctx, "updated vuln", "id", id)
			}
		} else {
			if _, err := r.client.CreateVuln(ctx, v); err != nil {
				slog.WarnContext(ctx, "create vuln failed, skipping record", "error", err)
			} else {
				slog.DebugContext(ctx, "created vuln", "cve_id", v.CVEID, "ghsa_id", v.GHSAID, "edb_id", v.EDBID)
				itemsNew++
			}
		}
	}
	return itemsNew
}

// recordRun posts a completed run record to the API. Errors are logged at
// WARN only and do not propagate; a RecordRun failure is not considered a
// source failure.
func (r *Runner) recordRun(ctx context.Context, src types.SourceType, startedAt time.Time, itemsFetched int, itemsNew int) {
	err := r.client.RecordRun(ctx, types.RunRecord{
		Source:       src,
		StartedAt:    startedAt,
		FinishedAt:   time.Now(),
		ItemsFetched: itemsFetched,
		ItemsNew:     itemsNew,
	})
	if err != nil {
		slog.WarnContext(ctx, "record run failed", "source", src, "error", err)
	}
}
