// response_test.go — tests for the Write helper in the response package
//
// Uses package response (whitebox) so Write is callable without a qualifier.
// Tests cover HTTP status propagation, Content-Type, success envelope shape,
// error envelope shape, and the invariant that body statusCode mirrors HTTP
// status.

package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite_StatusCode(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			Write(w, tc.code, "", "", nil)
			if w.Code != tc.code {
				t.Errorf("HTTP status = %d, want %d", w.Code, tc.code)
			}
		})
	}
}

func TestWrite_ContentType(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, http.StatusOK, "", "", nil)
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWrite_SuccessEnvelope(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	w := httptest.NewRecorder()
	Write(w, http.StatusCreated, "", "", payload{Name: "test"})

	var got struct {
		Error       string         `json:"error"`
		ErrorDetail string         `json:"errorDetail"`
		StatusCode  int            `json:"statusCode"`
		Result      map[string]any `json:"result"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got.Error != "" {
		t.Errorf("error = %q, want %q", got.Error, "")
	}
	if got.ErrorDetail != "" {
		t.Errorf("errorDetail = %q, want %q", got.ErrorDetail, "")
	}
	if got.StatusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", got.StatusCode, http.StatusCreated)
	}
	if got.Result["name"] != "test" {
		t.Errorf("result.name = %v, want %q", got.Result["name"], "test")
	}
}

func TestWrite_ErrorEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, http.StatusNotFound, "NOT_FOUND", "record does not exist", nil)

	var got struct {
		Error       string `json:"error"`
		ErrorDetail string `json:"errorDetail"`
		StatusCode  int    `json:"statusCode"`
		Result      any    `json:"result"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got.Error != "NOT_FOUND" {
		t.Errorf("error = %q, want %q", got.Error, "NOT_FOUND")
	}
	if got.ErrorDetail != "record does not exist" {
		t.Errorf("errorDetail = %q, want %q", got.ErrorDetail, "record does not exist")
	}
	if got.StatusCode != http.StatusNotFound {
		t.Errorf("statusCode = %d, want %d", got.StatusCode, http.StatusNotFound)
	}
	if got.Result != nil {
		t.Errorf("result = %v, want nil", got.Result)
	}
}

func TestWrite_StatusCodeInBodyMatchesHTTP(t *testing.T) {
	codes := []int{http.StatusOK, http.StatusCreated, http.StatusBadRequest, http.StatusNotFound}

	for _, code := range codes {
		w := httptest.NewRecorder()
		Write(w, code, "", "", map[string]string{"k": "v"})

		var got struct {
			StatusCode int `json:"statusCode"`
		}
		if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.StatusCode != code {
			t.Errorf("body statusCode = %d, want %d", got.StatusCode, code)
		}
		if w.Code != code {
			t.Errorf("HTTP status = %d, want %d", w.Code, code)
		}
	}
}
