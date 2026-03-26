// runner_test.go — tests for Runner: New and Run
//
// Uses package runner (same-package) so that Runner struct fields are
// accessible for direct assertions.

package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"daily-patch/ingestion/internal/source"
	"daily-patch/ingestion/internal/types"
)

// -----------------------------------------------------------------------------
// TestNew
// -----------------------------------------------------------------------------

func TestNew_storesClient(t *testing.T) {
	client := newStubClient()
	r := New(client, nil)
	if r.client != client {
		t.Error("client field not set on Runner")
	}
}

func TestNew_storesSources(t *testing.T) {
	src := newStubSource(types.SourceNVD, nil, nil)
	srcs := []source.Source{src}
	r := New(newStubClient(), srcs)
	if len(r.sources) != 1 || r.sources[0] != src {
		t.Error("sources field not set on Runner")
	}
}

// -----------------------------------------------------------------------------
// TestRun_noSources
// -----------------------------------------------------------------------------

func TestRun_noSources_returnsNil(t *testing.T) {
	r := New(newStubClient(), nil)
	if err := r.Run(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// TestRun_happyPath
// -----------------------------------------------------------------------------

func TestRun_happyPath(t *testing.T) {
	cases := []struct {
		name        string
		srcs        []*stubSource
		checkExists func(context.Context, string, string, string) (string, bool, error)
		wantRuns    int
		wantNew     []int
	}{
		{
			name:        "single source all new",
			srcs:        []*stubSource{newStubSource(types.SourceNVD, []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2")}, nil)},
			checkExists: func(_ context.Context, _, _, _ string) (string, bool, error) { return "", false, nil },
			wantRuns:    1,
			wantNew:     []int{2},
		},
		{
			name:        "single source all existing",
			srcs:        []*stubSource{newStubSource(types.SourceNVD, []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2")}, nil)},
			checkExists: func(_ context.Context, _, _, _ string) (string, bool, error) { return "id", true, nil },
			wantRuns:    1,
			wantNew:     []int{0},
		},
		{
			name: "single source mixed",
			srcs: []*stubSource{newStubSource(types.SourceNVD, []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2"), testVuln("CVE-3")}, nil)},
			checkExists: func() func(context.Context, string, string, string) (string, bool, error) {
				calls := 0
				return func(_ context.Context, _, _, _ string) (string, bool, error) {
					calls++
					if calls%2 == 0 {
						return "id", true, nil
					}
					return "", false, nil
				}
			}(),
			wantRuns: 1,
			wantNew:  []int{2}, // calls 1 and 3 are new; call 2 is existing
		},
		{
			name: "two sources both succeed",
			srcs: []*stubSource{
				newStubSource(types.SourceNVD, []types.Vulnerability{testVuln("CVE-1")}, nil),
				newStubSource(types.SourceGHSA, []types.Vulnerability{testVuln("CVE-2")}, nil),
			},
			checkExists: func(_ context.Context, _, _, _ string) (string, bool, error) { return "", false, nil },
			wantRuns:    2,
			wantNew:     []int{1, 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newStubClient()
			client.checkExistsFn = tc.checkExists

			srcs := make([]source.Source, len(tc.srcs))
			for i, s := range tc.srcs {
				srcs[i] = s
			}

			r := New(client, srcs)
			if err := r.Run(context.Background()); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if len(client.recordedRuns) != tc.wantRuns {
				t.Fatalf("RecordRun called %d times, want %d", len(client.recordedRuns), tc.wantRuns)
			}
			for i, wantNew := range tc.wantNew {
				if client.recordedRuns[i].ItemsNew != wantNew {
					t.Errorf("run[%d].ItemsNew = %d, want %d", i, client.recordedRuns[i].ItemsNew, wantNew)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestRun_lastSuccessfulRunError
// -----------------------------------------------------------------------------

func TestRun_lastSuccessfulRunError_fetchCalledWithZeroTime(t *testing.T) {
	client := newStubClient()
	client.lastSuccessfulRunFn = func(_ context.Context, _ types.SourceType) (time.Time, error) {
		return time.Time{}, errors.New("lookup failed")
	}

	var gotSince time.Time
	src := &stubSource{
		name: types.SourceNVD,
		fetchFn: func(_ context.Context, since time.Time) ([]types.Vulnerability, error) {
			gotSince = since
			return nil, nil
		},
	}

	r := New(client, []source.Source{src})
	if err := r.Run(context.Background()); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !gotSince.IsZero() {
		t.Errorf("Fetch called with non-zero time %v, want zero", gotSince)
	}
}

// -----------------------------------------------------------------------------
// TestRun_fetchCalledWithLastSuccessfulRunTime
// -----------------------------------------------------------------------------

func TestRun_fetchCalledWithLastSuccessfulRunTime(t *testing.T) {
	wantSince := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	client := newStubClient()
	client.lastSuccessfulRunFn = func(_ context.Context, _ types.SourceType) (time.Time, error) {
		return wantSince, nil
	}

	var gotSince time.Time
	src := &stubSource{
		name: types.SourceNVD,
		fetchFn: func(_ context.Context, since time.Time) ([]types.Vulnerability, error) {
			gotSince = since
			return nil, nil
		},
	}

	r := New(client, []source.Source{src})
	r.Run(context.Background()) //nolint:errcheck

	if !gotSince.Equal(wantSince) {
		t.Errorf("Fetch called with since=%v, want %v", gotSince, wantSince)
	}
}

// -----------------------------------------------------------------------------
// TestRun_fetchError
// -----------------------------------------------------------------------------

func TestRun_fetchError(t *testing.T) {
	fetchErr := errors.New("fetch failed")

	cases := []struct {
		name     string
		srcs     []*stubSource
		wantErr  bool
		wantRuns int
	}{
		{
			name:     "single source fails",
			srcs:     []*stubSource{newStubSource(types.SourceNVD, nil, fetchErr)},
			wantErr:  true,
			wantRuns: 1,
		},
		{
			name: "first of two fails second succeeds",
			srcs: []*stubSource{
				newStubSource(types.SourceNVD, nil, fetchErr),
				newStubSource(types.SourceGHSA, nil, nil),
			},
			wantErr:  true,
			wantRuns: 2,
		},
		{
			name: "both fail",
			srcs: []*stubSource{
				newStubSource(types.SourceNVD, nil, fetchErr),
				newStubSource(types.SourceGHSA, nil, fetchErr),
			},
			wantErr:  true,
			wantRuns: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newStubClient()
			srcs := make([]source.Source, len(tc.srcs))
			for i, s := range tc.srcs {
				srcs[i] = s
			}
			r := New(client, srcs)
			err := r.Run(context.Background())

			if tc.wantErr && err == nil {
				t.Error("expected non-nil error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
			if len(client.recordedRuns) != tc.wantRuns {
				t.Errorf("RecordRun called %d times, want %d", len(client.recordedRuns), tc.wantRuns)
			}
		})
	}
}

func TestRun_fetchError_recordRunCalledWithZeroCounts(t *testing.T) {
	client := newStubClient()
	src := newStubSource(types.SourceNVD, nil, errors.New("fetch error"))
	r := New(client, []source.Source{src})
	r.Run(context.Background()) //nolint:errcheck

	if len(client.recordedRuns) != 1 {
		t.Fatalf("expected 1 RecordRun call, got %d", len(client.recordedRuns))
	}
	rr := client.recordedRuns[0]
	if rr.ItemsFetched != 0 {
		t.Errorf("ItemsFetched = %d, want 0", rr.ItemsFetched)
	}
	if rr.ItemsNew != 0 {
		t.Errorf("ItemsNew = %d, want 0", rr.ItemsNew)
	}
}

// -----------------------------------------------------------------------------
// TestRun_perRecordError
// -----------------------------------------------------------------------------

func TestRun_perRecordError(t *testing.T) {
	apiErr := errors.New("api error")
	vulns := []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2"), testVuln("CVE-3")}

	cases := []struct {
		name        string
		setupClient func(*stubAPIClient)
		wantItemsNew int
	}{
		{
			name: "CheckExists errors every record",
			setupClient: func(c *stubAPIClient) {
				c.checkExistsFn = func(_ context.Context, _, _, _ string) (string, bool, error) {
					return "", false, apiErr
				}
			},
			wantItemsNew: 0,
		},
		{
			name: "CreateVuln errors every record",
			setupClient: func(c *stubAPIClient) {
				c.createVulnFn = func(_ context.Context, _ types.Vulnerability) (string, error) {
					return "", apiErr
				}
			},
			wantItemsNew: 0,
		},
		{
			name: "UpdateVuln errors",
			setupClient: func(c *stubAPIClient) {
				c.checkExistsFn = func(_ context.Context, _, _, _ string) (string, bool, error) {
					return "id", true, nil
				}
				c.updateVulnFn = func(_ context.Context, _ string, _ types.Vulnerability) error {
					return apiErr
				}
			},
			wantItemsNew: 0,
		},
		{
			name: "partial CreateVuln success",
			setupClient: func(c *stubAPIClient) {
				calls := 0
				c.createVulnFn = func(_ context.Context, _ types.Vulnerability) (string, error) {
					calls++
					if calls%2 == 0 {
						return "", apiErr
					}
					return "new-id", nil
				}
			},
			wantItemsNew: 2, // calls 1 and 3 succeed; call 2 fails
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newStubClient()
			tc.setupClient(client)

			src := newStubSource(types.SourceNVD, vulns, nil)
			r := New(client, []source.Source{src})
			if err := r.Run(context.Background()); err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
			if len(client.recordedRuns) != 1 {
				t.Fatalf("expected 1 RecordRun call, got %d", len(client.recordedRuns))
			}
			if client.recordedRuns[0].ItemsNew != tc.wantItemsNew {
				t.Errorf("ItemsNew = %d, want %d", client.recordedRuns[0].ItemsNew, tc.wantItemsNew)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestRun_recordRunError
// -----------------------------------------------------------------------------

func TestRun_recordRunError_notPropagated(t *testing.T) {
	client := newStubClient()
	client.recordRunFn = func(_ context.Context, _ types.RunRecord) error {
		return errors.New("record failed")
	}

	src := newStubSource(types.SourceNVD, nil, nil)
	r := New(client, []source.Source{src})
	if err := r.Run(context.Background()); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// TestRun_contextCancellation
// -----------------------------------------------------------------------------

func TestRun_contextAlreadyCancelled_returnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newStubClient()
	fetchCalled := false
	src := &stubSource{
		name: types.SourceNVD,
		fetchFn: func(_ context.Context, _ time.Time) ([]types.Vulnerability, error) {
			fetchCalled = true
			return nil, nil
		},
	}

	r := New(client, []source.Source{src})
	err := r.Run(ctx)

	if err == nil {
		t.Error("expected non-nil error for cancelled context")
	}
	if fetchCalled {
		t.Error("Fetch should not have been called with pre-cancelled context")
	}
}

func TestRun_contextCancelledAfterFetch_noRunRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	client := newStubClient()
	src := &stubSource{
		name: types.SourceNVD,
		fetchFn: func(_ context.Context, _ time.Time) ([]types.Vulnerability, error) {
			cancel()
			return nil, nil
		},
	}

	r := New(client, []source.Source{src})
	r.Run(ctx) //nolint:errcheck

	if len(client.recordedRuns) != 0 {
		t.Errorf("RecordRun should not have been called, got %d calls", len(client.recordedRuns))
	}
}

func TestRun_contextCancelledMidLoop_noRunRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	client := newStubClient()
	calls := 0
	client.checkExistsFn = func(_ context.Context, _, _, _ string) (string, bool, error) {
		calls++
		if calls == 1 {
			cancel()
			return "", false, errors.New("context cancelled")
		}
		return "", false, nil
	}

	vulns := []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2")}
	src := newStubSource(types.SourceNVD, vulns, nil)
	r := New(client, []source.Source{src})
	r.Run(ctx) //nolint:errcheck

	if len(client.recordedRuns) != 0 {
		t.Errorf("RecordRun should not have been called, got %d calls", len(client.recordedRuns))
	}
}

// -----------------------------------------------------------------------------
// TestRun_contextTimeout
// -----------------------------------------------------------------------------

// TestRun_contextTimeout_completesBeforeDeadline asserts that a context with a
// generous timeout does not interfere with normal execution: Run returns nil
// and RecordRun is called with the correct counts.
func TestRun_contextTimeout_completesBeforeDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := newStubClient()
	vulns := []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2")}
	src := newStubSource(types.SourceNVD, vulns, nil)

	r := New(client, []source.Source{src})
	if err := r.Run(ctx); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(client.recordedRuns) != 1 {
		t.Fatalf("expected 1 RecordRun call, got %d", len(client.recordedRuns))
	}
	if client.recordedRuns[0].ItemsFetched != len(vulns) {
		t.Errorf("ItemsFetched = %d, want %d", client.recordedRuns[0].ItemsFetched, len(vulns))
	}
}

// TestRun_contextTimeout_deadlineExceededDuringFetch asserts that when the
// context deadline fires while Fetch is blocked, Run returns
// context.DeadlineExceeded and RecordRun is never called.
func TestRun_contextTimeout_deadlineExceededDuringFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	client := newStubClient()
	src := &stubSource{
		name: types.SourceNVD,
		fetchFn: func(ctx context.Context, _ time.Time) ([]types.Vulnerability, error) {
			<-ctx.Done() // block until the deadline fires
			return nil, ctx.Err()
		},
	}

	r := New(client, []source.Source{src})
	err := r.Run(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if len(client.recordedRuns) != 0 {
		t.Errorf("RecordRun should not have been called, got %d calls", len(client.recordedRuns))
	}
}

// -----------------------------------------------------------------------------
// TestRun_sourcesExecutedInProvidedOrder
// -----------------------------------------------------------------------------

func TestRun_sourcesExecutedInProvidedOrder(t *testing.T) {
	client := newStubClient()
	srcs := []source.Source{
		newStubSource(types.SourceGHSA, nil, nil),
		newStubSource(types.SourceNVD, nil, nil),
		newStubSource(types.SourceExploitDB, nil, nil),
	}

	r := New(client, srcs)
	r.Run(context.Background()) //nolint:errcheck

	if len(client.recordedRuns) != 3 {
		t.Fatalf("expected 3 RecordRun calls, got %d", len(client.recordedRuns))
	}
	wantOrder := []types.SourceType{types.SourceGHSA, types.SourceNVD, types.SourceExploitDB}
	for i, want := range wantOrder {
		if client.recordedRuns[i].Source != want {
			t.Errorf("run[%d].Source = %q, want %q", i, client.recordedRuns[i].Source, want)
		}
	}
}

// -----------------------------------------------------------------------------
// TestRun_itemsFetchedMatchesFetchResult
// -----------------------------------------------------------------------------

func TestRun_itemsFetchedMatchesFetchResult(t *testing.T) {
	client := newStubClient()
	vulns := []types.Vulnerability{testVuln("CVE-1"), testVuln("CVE-2"), testVuln("CVE-3")}
	src := newStubSource(types.SourceNVD, vulns, nil)

	r := New(client, []source.Source{src})
	r.Run(context.Background()) //nolint:errcheck

	if len(client.recordedRuns) != 1 {
		t.Fatalf("expected 1 RecordRun, got %d", len(client.recordedRuns))
	}
	if client.recordedRuns[0].ItemsFetched != len(vulns) {
		t.Errorf("ItemsFetched = %d, want %d", client.recordedRuns[0].ItemsFetched, len(vulns))
	}
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

type stubAPIClient struct {
	lastSuccessfulRunFn func(context.Context, types.SourceType) (time.Time, error)
	checkExistsFn       func(context.Context, string, string, string) (string, bool, error)
	createVulnFn        func(context.Context, types.Vulnerability) (string, error)
	updateVulnFn        func(context.Context, string, types.Vulnerability) error
	recordRunFn         func(context.Context, types.RunRecord) error
	recordedRuns        []types.RunRecord
}

func (c *stubAPIClient) LastSuccessfulRun(ctx context.Context, src types.SourceType) (time.Time, error) {
	if c.lastSuccessfulRunFn != nil {
		return c.lastSuccessfulRunFn(ctx, src)
	}
	return time.Time{}, nil
}

func (c *stubAPIClient) CheckExists(ctx context.Context, cveID, ghsaID, edbID string) (string, bool, error) {
	if c.checkExistsFn != nil {
		return c.checkExistsFn(ctx, cveID, ghsaID, edbID)
	}
	return "", false, nil
}

func (c *stubAPIClient) CreateVuln(ctx context.Context, v types.Vulnerability) (string, error) {
	if c.createVulnFn != nil {
		return c.createVulnFn(ctx, v)
	}
	return "new-id", nil
}

func (c *stubAPIClient) UpdateVuln(ctx context.Context, id string, v types.Vulnerability) error {
	if c.updateVulnFn != nil {
		return c.updateVulnFn(ctx, id, v)
	}
	return nil
}

func (c *stubAPIClient) RecordRun(ctx context.Context, r types.RunRecord) error {
	c.recordedRuns = append(c.recordedRuns, r)
	if c.recordRunFn != nil {
		return c.recordRunFn(ctx, r)
	}
	return nil
}

type stubSource struct {
	name    types.SourceType
	fetchFn func(context.Context, time.Time) ([]types.Vulnerability, error)
}

func (s *stubSource) Name() types.SourceType { return s.name }

func (s *stubSource) Fetch(ctx context.Context, since time.Time) ([]types.Vulnerability, error) {
	if s.fetchFn != nil {
		return s.fetchFn(ctx, since)
	}
	return nil, nil
}

func newStubClient() *stubAPIClient {
	return &stubAPIClient{}
}

func newStubSource(name types.SourceType, vulns []types.Vulnerability, err error) *stubSource {
	return &stubSource{
		name: name,
		fetchFn: func(_ context.Context, _ time.Time) ([]types.Vulnerability, error) {
			return vulns, err
		},
	}
}

func testVuln(cveID string) types.Vulnerability {
	return types.Vulnerability{CVEID: cveID}
}
