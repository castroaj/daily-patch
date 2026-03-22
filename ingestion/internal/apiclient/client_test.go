// client_test.go — tests for the httpClient constructor
//
// Uses package apiclient (not apiclient_test) so the unexported httpClient
// struct is accessible for field-level assertions.

package apiclient

import (
	"testing"
	"time"
)

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

func TestNew_defaultTimeout(t *testing.T) {
	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want 30s", DefaultTimeout)
	}
}

func TestNew_implementsInterface(t *testing.T) {
	// Compile-time assertion — if New stops returning APIClient this fails.
	var _ APIClient
	var err error
	_, err = New("http://api:8080", "secret", DefaultTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
