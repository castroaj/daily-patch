// response.go — standard JSON response envelope for all API handlers
//
// Every handler must call Write instead of encoding JSON directly. This
// guarantees a consistent shape across all routes: error code, human-readable
// detail, HTTP status mirror, and the result payload.

package response

import (
	"encoding/json"
	"net/http"
)

// Response is the envelope written by every API endpoint.
type Response struct {
	Error       string `json:"error"`
	ErrorDetail string `json:"errorDetail"`
	StatusCode  int    `json:"statusCode"`
	Result      any    `json:"result"`
}

// Write marshals a Response envelope and writes it to w. On marshal failure
// it falls back to a plain 500 text response.
func Write(w http.ResponseWriter, statusCode int, errType string, errDetail string, result any) {
	env := Response{
		Error:       errType,
		ErrorDetail: errDetail,
		StatusCode:  statusCode,
		Result:      result,
	}

	b, err := json.Marshal(env)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(b)
}
