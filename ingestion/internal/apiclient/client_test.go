// client_test.go — tests for the httpClient struct and all of its methods
//
// Uses package apiclient (not apiclient_test) so the unexported httpClient
// struct is accessible for field-level assertions and test configuration
// (e.g. setting retryDelay to 0 for fast retry-path tests).

package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"daily-patch/ingestion/internal/types"
)

// -----------------------------------------------------------------------------
// Constructor (New)
// -----------------------------------------------------------------------------

// TestNew_storesBaseURL asserts that New stores the provided base URL on the httpClient.
func TestNew_storesBaseURL(t *testing.T) {
	c, err := New("http://api:8080", "secret", DefaultTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := c.(*httpClient).baseURL
	if got != "http://api:8080" {
		t.Errorf("baseURL = %q, want %q", got, "http://api:8080")
	}
}

// TestNew_storesSecret asserts that New stores the provided secret on the httpClient.
func TestNew_storesSecret(t *testing.T) {
	c, err := New("http://api:8080", "my-secret", DefaultTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := c.(*httpClient).secret
	if got != "my-secret" {
		t.Errorf("secret = %q, want %q", got, "my-secret")
	}
}

// TestNew_appliesTimeout asserts that New wires the timeout duration into the underlying http.Client.
func TestNew_appliesTimeout(t *testing.T) {
	timeout := 10 * time.Second
	c, err := New("http://api:8080", "secret", timeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := c.(*httpClient).http.Timeout
	if got != timeout {
		t.Errorf("http.Timeout = %v, want %v", got, timeout)
	}
}

// TestNew_defaultTimeout asserts that DefaultTimeout is set to 30 seconds.
func TestNew_defaultTimeout(t *testing.T) {
	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want 30s", DefaultTimeout)
	}
}

// TestNew_implementsInterface asserts that New returns a value satisfying APIClient.
func TestNew_implementsInterface(t *testing.T) {
	var _ APIClient
	var err error
	_, err = New("http://api:8080", "secret", DefaultTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestNew_invalidSecret asserts that New rejects secrets containing whitespace, non-ASCII, or control characters.
func TestNew_invalidSecret(t *testing.T) {
	cases := []struct {
		name   string
		secret string
	}{
		{"empty", ""},
		{"space", "has space"},
		{"tab", "has\ttab"},
		{"newline", "has\nnewline"},
		{"carriage return", "has\rreturn"},
		{"non-ascii", "caf\u00e9"},
		{"del character", "sec\x7fret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New("http://api:8080", tc.secret, DefaultTimeout)
			if err == nil {
				t.Errorf("expected error for secret %q, got nil (client=%v)", tc.secret, c)
			}
		})
	}
}

// TestNew_validSecret asserts that New accepts any non-empty string of printable ASCII characters as a secret.
func TestNew_validSecret(t *testing.T) {
	cases := []struct {
		name   string
		secret string
	}{
		{"alphanumeric", "abc123"},
		{"with hyphens", "my-internal-secret"},
		{"with special chars", "s3cr3t!@#$%^&*()"},
		{"min printable ascii", "!"},
		{"max printable ascii", "~"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New("http://api:8080", tc.secret, DefaultTimeout)
			if err != nil {
				t.Errorf("unexpected error for secret %q: %v", tc.secret, err)
			}
		})
	}
}

// TestNew_invalidURL asserts that New rejects non-absolute URLs, non-http(s) schemes, and trailing slashes.
func TestNew_invalidURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty string", ""},
		{"no scheme", "api:8080"},
		{"non-http scheme", "ftp://api:8080"},
		{"path only", "/api/v1"},
		{"relative url", "//api:8080"},
		{"trailing slash", "http://api:8080/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.url, "secret", DefaultTimeout)
			if err == nil {
				t.Errorf("expected error for URL %q, got nil (client=%v)", tc.url, c)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// CheckExists
// -----------------------------------------------------------------------------

// TestCheckExists_StatusCodes asserts that CheckExists maps server status codes to the correct (id, found, err) return values.
func TestCheckExists_StatusCodes(t *testing.T) {
	const secret = "test-secret"

	cases := []struct {
		name      string
		handler   http.HandlerFunc
		wantID    string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "200 with record returns found and id",
			handler:   requireSecret(secret, vulnFound("uuid-abc123")),
			wantID:    "uuid-abc123",
			wantFound: true,
		},
		{
			name:      "404 returns not found without error",
			handler:   requireSecret(secret, vulnNotFound()),
			wantFound: false,
		},
		{
			name:    "401 returns error",
			handler: requireSecret("other-secret", vulnFound("should-not-reach")),
			wantErr: true,
		},
		{
			name: "500 returns error after retries",
			handler: requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				respond(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startServer(t, secret, tc.handler)
			id, found, err := c.CheckExists(context.Background(), "CVE-2024-1234", "", "")

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got id=%q found=%v", id, found)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tc.wantFound {
				t.Errorf("found = %v, want %v", found, tc.wantFound)
			}
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

// TestCheckExists_Payload asserts that the ID is decoded from the envelope's result field, not the envelope root.
func TestCheckExists_Payload(t *testing.T) {
	const secret = "test-secret"

	t.Run("malformed JSON returns error", func(t *testing.T) {
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`not-valid-json`)); err != nil {
				panic("write: " + err.Error())
			}
		})
		c := startServer(t, secret, handler)
		_, _, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", "")
		if err == nil {
			t.Error("expected error for malformed JSON, got nil")
		}
	})

	t.Run("id in result field is decoded", func(t *testing.T) {
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			respondEnvelope(w, http.StatusOK, map[string]any{"id": "result-id"})
		})
		c := startServer(t, secret, handler)
		id, found, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Error("expected found=true")
		}
		if id != "result-id" {
			t.Errorf("id = %q, want %q", id, "result-id")
		}
	})

	t.Run("id outside result field is not decoded", func(t *testing.T) {
		// id is at the envelope root rather than inside result — must be ignored.
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusOK, map[string]any{
				"error": "", "errorDetail": "", "statusCode": http.StatusOK,
				"result": map[string]any{},
				"id":     "should-not-be-decoded",
			})
		})
		c := startServer(t, secret, handler)
		id, found, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Error("expected found=true (200 response)")
		}
		if id != "" {
			t.Errorf("id = %q, want empty (id must come from result field)", id)
		}
	})
}

// TestCheckExists_QueryParams asserts that only non-empty canonical IDs are forwarded as query parameters and absent IDs are omitted.
func TestCheckExists_QueryParams(t *testing.T) {
	const secret = "test-secret"

	cases := []struct {
		name       string
		cveID      string
		ghsaID     string
		edbID      string
		wantParams map[string]string
		absentKeys []string
	}{
		{
			name:       "cve_id only",
			cveID:      "CVE-2024-1234",
			wantParams: map[string]string{paramCVEID: "CVE-2024-1234"},
			absentKeys: []string{paramGHSAID, paramEDBID},
		},
		{
			name:       "ghsa_id only",
			ghsaID:     "GHSA-xxxx-yyyy-zzzz",
			wantParams: map[string]string{paramGHSAID: "GHSA-xxxx-yyyy-zzzz"},
			absentKeys: []string{paramCVEID, paramEDBID},
		},
		{
			name:       "edb_id only",
			edbID:      "12345",
			wantParams: map[string]string{paramEDBID: "12345"},
			absentKeys: []string{paramCVEID, paramGHSAID},
		},
		{
			name:   "all ids set",
			cveID:  "CVE-2024-9999",
			ghsaID: "GHSA-aaaa-bbbb-cccc",
			edbID:  "99999",
			wantParams: map[string]string{
				paramCVEID:  "CVE-2024-9999",
				paramGHSAID: "GHSA-aaaa-bbbb-cccc",
				paramEDBID:  "99999",
			},
		},
		{
			name:       "all ids empty sends no id params",
			absentKeys: []string{paramCVEID, paramGHSAID, paramEDBID},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedQuery map[string][]string

			handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = map[string][]string(r.URL.Query())
				w.WriteHeader(http.StatusNotFound)
			})

			c := startServer(t, secret, handler)
			_, _, err := c.CheckExists(context.Background(), tc.cveID, tc.ghsaID, tc.edbID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for key, want := range tc.wantParams {
				if vals := capturedQuery[key]; len(vals) == 0 || vals[0] != want {
					t.Errorf("query param %s = %v, want %q", key, vals, want)
				}
			}
			for _, key := range tc.absentKeys {
				if vals := capturedQuery[key]; len(vals) > 0 {
					t.Errorf("unexpected query param %s = %v", key, vals)
				}
			}
		})
	}
}

// TestCheckExists_Authorization asserts that the configured secret is sent as X-Internal-Secret and that wrong secrets are rejected.
func TestCheckExists_Authorization(t *testing.T) {
	t.Run("valid secret is accepted", func(t *testing.T) {
		const secret = "valid-secret"
		c := startServer(t, secret, requireSecret(secret, vulnFound("found-id")))
		id, found, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || id != "found-id" {
			t.Errorf("found=%v id=%q, want found=true id=%q", found, id, "found-id")
		}
	})

	t.Run("wrong secret returns error", func(t *testing.T) {
		// Server accepts "server-secret"; client is configured with "wrong-secret".
		c := startServer(t, "wrong-secret", requireSecret("server-secret", vulnFound("found-id")))
		_, _, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", "")
		if err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
	})

	t.Run("X-Internal-Secret header carries the configured secret", func(t *testing.T) {
		const secret = "inspect-me"
		var capturedSecret string

		handler := func(w http.ResponseWriter, r *http.Request) {
			capturedSecret = r.Header.Get("X-Internal-Secret")
			w.WriteHeader(http.StatusNotFound)
		}

		c := startServer(t, secret, handler)
		if _, _, err := c.CheckExists(context.Background(), "CVE-2024-0001", "", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedSecret != secret {
			t.Errorf("X-Internal-Secret = %q, want %q", capturedSecret, secret)
		}
	})
}

// TestCheckExists_AllIDsEmpty asserts that empty IDs are forwarded to the server without a client-side error; validation is the server's responsibility.
func TestCheckExists_AllIDsEmpty(t *testing.T) {
	const secret = "test-secret"

	t.Run("server 404 is passed through as not found", func(t *testing.T) {
		c := startServer(t, secret, requireSecret(secret, vulnNotFound()))
		id, found, err := c.CheckExists(context.Background(), "", "", "")
		if err != nil {
			t.Fatalf("client must not error on empty IDs, got: %v", err)
		}
		if found || id != "" {
			t.Errorf("found=%v id=%q, want found=false id=%q", found, id, "")
		}
	})

	t.Run("server error response is passed through", func(t *testing.T) {
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusBadRequest, map[string]string{"error": "at least one id required"})
		})
		c := startServer(t, secret, handler)
		// 400 is not retried and not treated specially by do(); the client does
		// not surface it as an error.
		_, _, err := c.CheckExists(context.Background(), "", "", "")
		if err != nil {
			t.Fatalf("client must not error on empty IDs, got: %v", err)
		}
	})
}

// -----------------------------------------------------------------------------
// CreateVuln
// -----------------------------------------------------------------------------

// TestCreateVuln_StatusCodes asserts that CreateVuln returns the assigned ID on 201 and an error for all other status codes.
func TestCreateVuln_StatusCodes(t *testing.T) {
	const secret = "test-secret"

	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantID  string
		wantErr bool
	}{
		{
			name:    "201 returns id and no error",
			handler: requireSecret(secret, vulnCreated("new-uuid-123")),
			wantID:  "new-uuid-123",
		},
		{
			name: "200 returns unexpected status error",
			handler: requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				respondEnvelope(w, http.StatusOK, map[string]any{"id": "some-id"})
			}),
			wantErr: true,
		},
		{
			name: "400 returns unexpected status error",
			handler: requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				respondEnvelope(w, http.StatusBadRequest, map[string]string{"error": "invalid"})
			}),
			wantErr: true,
		},
		{
			name: "409 returns unexpected status error",
			handler: requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				respondEnvelope(w, http.StatusConflict, map[string]string{"error": "already exists"})
			}),
			wantErr: true,
		},
		{
			name:    "401 returns error",
			handler: requireSecret("other-secret", vulnCreated("should-not-reach")),
			wantErr: true,
		},
		{
			name: "500 returns error after retries",
			handler: requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
				respondEnvelope(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startServer(t, secret, tc.handler)
			id, err := c.CreateVuln(context.Background(), testVuln())

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got id=%q", id)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

// TestCreateVuln_RequestMethod asserts that CreateVuln sends a POST request.
func TestCreateVuln_RequestMethod(t *testing.T) {
	const secret = "test-secret"
	var capturedMethod string

	handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		respondEnvelope(w, http.StatusCreated, map[string]any{"id": "new-id"})
	})

	c := startServer(t, secret, handler)
	if _, err := c.CreateVuln(context.Background(), testVuln()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", capturedMethod, http.MethodPost)
	}
}

// TestCreateVuln_RequestPath asserts that CreateVuln targets the correct API path.
func TestCreateVuln_RequestPath(t *testing.T) {
	const secret = "test-secret"
	var capturedPath string

	handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		respondEnvelope(w, http.StatusCreated, map[string]any{"id": "new-id"})
	})

	c := startServer(t, secret, handler)
	if _, err := c.CreateVuln(context.Background(), testVuln()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != pathVulns {
		t.Errorf("path = %q, want %q", capturedPath, pathVulns)
	}
}

// TestCreateVuln_RequestPayload asserts that vulnerability fields are serialized into the request body.
func TestCreateVuln_RequestPayload(t *testing.T) {
	const secret = "test-secret"
	var capturedBody map[string]any

	handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		respondEnvelope(w, http.StatusCreated, map[string]any{"id": "new-id"})
	})

	v := testVuln()
	c := startServer(t, secret, handler)
	if _, err := c.CreateVuln(context.Background(), v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := map[string]any{
		"CVEID":  v.CVEID,
		"Source": string(v.Source),
		"Title":  v.Title,
	}
	for field, want := range checks {
		if got := capturedBody[field]; got != want {
			t.Errorf("body[%q] = %v, want %v", field, got, want)
		}
	}
}

// TestCreateVuln_ContentType asserts that CreateVuln sends Content-Type: application/json on the request.
func TestCreateVuln_ContentType(t *testing.T) {
	const secret = "test-secret"
	var capturedContentType string

	handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		respondEnvelope(w, http.StatusCreated, map[string]any{"id": "new-id"})
	})

	c := startServer(t, secret, handler)
	if _, err := c.CreateVuln(context.Background(), testVuln()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}
}

// TestCreateVuln_Authorization asserts that the configured secret is sent as X-Internal-Secret and that wrong secrets are rejected.
func TestCreateVuln_Authorization(t *testing.T) {
	t.Run("valid secret is accepted", func(t *testing.T) {
		const secret = "valid-secret"
		c := startServer(t, secret, requireSecret(secret, vulnCreated("new-id")))
		id, err := c.CreateVuln(context.Background(), testVuln())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "new-id" {
			t.Errorf("id = %q, want %q", id, "new-id")
		}
	})

	t.Run("wrong secret returns error", func(t *testing.T) {
		c := startServer(t, "wrong-secret", requireSecret("server-secret", vulnCreated("new-id")))
		_, err := c.CreateVuln(context.Background(), testVuln())
		if err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
	})

	t.Run("X-Internal-Secret header carries the configured secret", func(t *testing.T) {
		const secret = "inspect-me"
		var capturedSecret string

		handler := func(w http.ResponseWriter, r *http.Request) {
			capturedSecret = r.Header.Get("X-Internal-Secret")
			respondEnvelope(w, http.StatusCreated, map[string]any{"id": "new-id"})
		}

		c := startServer(t, secret, handler)
		if _, err := c.CreateVuln(context.Background(), testVuln()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedSecret != secret {
			t.Errorf("X-Internal-Secret = %q, want %q", capturedSecret, secret)
		}
	})
}

// TestCreateVuln_MalformedJSON asserts that the ID is decoded from the envelope's result field and that malformed responses return an error.
func TestCreateVuln_MalformedJSON(t *testing.T) {
	const secret = "test-secret"

	t.Run("malformed envelope returns error", func(t *testing.T) {
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`not-valid-json`)); err != nil {
				panic("write: " + err.Error())
			}
		})
		c := startServer(t, secret, handler)
		_, err := c.CreateVuln(context.Background(), testVuln())
		if err == nil {
			t.Error("expected error for malformed JSON, got nil")
		}
	})

	t.Run("id in result field is decoded", func(t *testing.T) {
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			respondEnvelope(w, http.StatusCreated, map[string]any{"id": "result-id"})
		})
		c := startServer(t, secret, handler)
		id, err := c.CreateVuln(context.Background(), testVuln())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "result-id" {
			t.Errorf("id = %q, want %q", id, "result-id")
		}
	})

	t.Run("id outside result field is not decoded", func(t *testing.T) {
		// id is at the envelope root rather than inside result — must be ignored.
		handler := requireSecret(secret, func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusCreated, map[string]any{
				"error": "", "errorDetail": "", "statusCode": http.StatusCreated,
				"result": map[string]any{},
				"id":     "should-not-be-decoded",
			})
		})
		c := startServer(t, secret, handler)
		id, err := c.CreateVuln(context.Background(), testVuln())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "" {
			t.Errorf("id = %q, want empty (id must come from result field)", id)
		}
	})
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// startServer registers handler on a new httptest.Server and returns an
// httpClient pointed at it. retryDelay is set to zero so retry-path tests
// complete without sleeping.
func startServer(t *testing.T, secret string, handler http.HandlerFunc) *httpClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, secret, DefaultTimeout)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hc := c.(*httpClient)
	hc.retryDelay = 0
	return hc
}

// respond marshals body to JSON, sets Content-Type, and writes status. Panics
// on marshal failure (should never happen in tests with known types).
func respond(w http.ResponseWriter, status int, body any) {
	b, err := json.Marshal(body)
	if err != nil {
		panic("respond: marshal: " + err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		panic("respond: write: " + err.Error())
	}
}

// requireSecret wraps next, returning 401 when X-Internal-Secret != secret.
func requireSecret(secret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// respondEnvelope writes a standard API response envelope with the given
// status code and result payload, matching the shape produced by
// api/internal/response.Write.
func respondEnvelope(w http.ResponseWriter, statusCode int, result any) {
	respond(w, statusCode, map[string]any{
		"error": "", "errorDetail": "", "statusCode": statusCode, "result": result,
	})
}

// vulnFound returns a handler that replies 200 with an enveloped vuln object.
func vulnFound(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondEnvelope(w, http.StatusOK, map[string]any{"id": id})
	}
}

// vulnNotFound returns a handler that replies 404, signalling no matching record.
func vulnNotFound() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
}

// vulnCreated returns a handler that replies 201 with an enveloped vuln object.
func vulnCreated(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondEnvelope(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// testVuln returns a sample Vulnerability for use in CreateVuln tests.
func testVuln() types.Vulnerability {
	return types.Vulnerability{
		CVEID:       "CVE-2024-1234",
		Source:      types.SourceNVD,
		Title:       "Test vulnerability",
		Description: "A test vulnerability for unit tests",
		CVSSScore:   7.5,
	}
}
