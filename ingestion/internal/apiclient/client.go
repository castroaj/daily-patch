// client.go — API client interface and HTTP implementation for the ingestion service
//
// Abstracts all REST calls to the api service. The unexported httpClient struct
// is the production implementation; tests may substitute any value satisfying
// the APIClient interface.

package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"daily-patch/ingestion/internal/types"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

// DefaultTimeout is used by New when no custom timeout is required.
const DefaultTimeout = 30 * time.Second

// maxRetries is the number of attempts before giving up on a transient error.
const maxRetries = 3

// -----------------------------------------------------------------------------
// Public types
// -----------------------------------------------------------------------------

// APIClient abstracts all REST calls made by the ingestion service to the
// api service.
type APIClient interface {
	// CheckExists queries for an existing vulnerability record by canonical ID.
	// Exactly one of cveID, ghsaID, edbID should be non-empty, though the
	// implementation passes all non-empty values so the API can match on any.
	// Returns the API-assigned UUID if a record is found.
	CheckExists(ctx context.Context, cveID, ghsaID, edbID string) (id string, found bool, err error)

	// CreateVuln posts a new vulnerability record and returns the assigned UUID.
	CreateVuln(ctx context.Context, v types.Vulnerability) (id string, err error)

	// UpdateVuln replaces an existing vulnerability record by its API UUID.
	UpdateVuln(ctx context.Context, id string, v types.Vulnerability) error

	// RecordRun posts a completed ingestion run to POST /api/v1/runs/ingestion.
	RecordRun(ctx context.Context, r types.RunRecord) error

	// LastSuccessfulRun returns finished_at for the most recent completed run
	// for the given source. Returns a zero time.Time if no prior run exists.
	LastSuccessfulRun(ctx context.Context, source types.SourceType) (time.Time, error)
}

// -----------------------------------------------------------------------------
// Public functions
// -----------------------------------------------------------------------------

// New returns a production APIClient backed by HTTP.
// baseURL should not have a trailing slash (e.g. "http://api:8080").
// secret is sent as the X-Internal-Secret header on every request.
// timeout sets the per-request deadline; pass DefaultTimeout if unsure.
// Returns an error if baseURL or secret fail validation.
func New(baseURL, secret string, timeout time.Duration) (APIClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid baseURL %q: must be an absolute http or https URL", baseURL)
	}
	if baseURL[len(baseURL)-1] == '/' {
		return nil, fmt.Errorf("invalid baseURL %q: must not have a trailing slash", baseURL)
	}

	if err := validateSecret(secret); err != nil {
		return nil, err
	}

	return &httpClient{
		baseURL: baseURL,
		secret:  secret,
		http: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// -----------------------------------------------------------------------------
// Private types and methods
// -----------------------------------------------------------------------------

// httpClient is the production implementation of APIClient.
type httpClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

// CheckExists queries GET /api/v1/vulns with canonical ID query parameters.
func (c *httpClient) CheckExists(ctx context.Context, cveID, ghsaID, edbID string) (string, bool, error) {
	q := url.Values{}
	if cveID != "" {
		q.Set("cve_id", cveID)
	}
	if ghsaID != "" {
		q.Set("ghsa_id", ghsaID)
	}
	if edbID != "" {
		q.Set("edb_id", edbID)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/vulns?"+q.Encode(), nil, &result)
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode == http.StatusNotFound || len(result.Data) == 0 {
		return "", false, nil
	}
	return result.Data[0].ID, true, nil
}

// CreateVuln posts a new vulnerability record to POST /api/v1/vulns.
func (c *httpClient) CreateVuln(ctx context.Context, v types.Vulnerability) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/vulns", v, &result)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	return result.ID, nil
}

// UpdateVuln replaces a vulnerability record via PUT /api/v1/vulns/{id}.
func (c *httpClient) UpdateVuln(ctx context.Context, id string, v types.Vulnerability) error {
	resp, err := c.do(ctx, http.MethodPut, "/api/v1/vulns/"+id, v, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}

// RecordRun posts a completed run record to POST /api/v1/runs/ingestion.
func (c *httpClient) RecordRun(ctx context.Context, r types.RunRecord) error {
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/runs/ingestion", r, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}

// LastSuccessfulRun queries GET /api/v1/runs/ingestion and returns the most
// recent finished_at for the given source. Returns zero time.Time if none.
func (c *httpClient) LastSuccessfulRun(ctx context.Context, source types.SourceType) (time.Time, error) {
	q := url.Values{}
	q.Set("source", string(source))
	q.Set("limit", "1")

	var result struct {
		Data []struct {
			FinishedAt time.Time `json:"finished_at"`
		} `json:"data"`
	}
	_, err := c.do(ctx, http.MethodGet, "/api/v1/runs/ingestion?"+q.Encode(), nil, &result)
	if err != nil {
		return time.Time{}, err
	}
	if len(result.Data) == 0 {
		return time.Time{}, nil
	}
	return result.Data[0].FinishedAt, nil
}

// do executes an HTTP request, retrying up to maxRetries times on network
// errors or 5xx responses using exponential backoff. It decodes a successful
// response body into dst when dst is non-nil.
func (c *httpClient) do(ctx context.Context, method, path string, body interface{}, dst interface{}) (*http.Response, error) {
	u := c.baseURL + path

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		var bodyReader *bytes.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}

		var req *http.Request
		var err error
		if bodyReader != nil {
			req, err = http.NewRequestWithContext(ctx, method, u, bodyReader)
		} else {
			req, err = http.NewRequestWithContext(ctx, method, u, nil)
		}
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("X-Internal-Secret", c.secret)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %s", resp.Status)
			continue
		}

		if dst != nil {
			defer resp.Body.Close()
			if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
				return resp, fmt.Errorf("decode response: %w", err)
			}
		}

		return resp, nil
	}

	return nil, fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

// -----------------------------------------------------------------------------
// Private functions
// -----------------------------------------------------------------------------

// validateSecret rejects empty strings and any character outside printable
// ASCII (0x21–0x7E). Whitespace is excluded to prevent silent trimming in
// config files and to guard against header-injection via newlines.
func validateSecret(secret string) error {
	if len(secret) == 0 {
		return fmt.Errorf("secret must not be empty")
	}
	for i := 0; i < len(secret); i++ {
		b := secret[i]
		if b < 0x21 || b > 0x7E {
			return fmt.Errorf("secret contains invalid character at position %d (0x%02X): must be printable ASCII with no whitespace", i, b)
		}
	}
	return nil
}
