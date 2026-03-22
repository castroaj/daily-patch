// main.go — entry point for the api service
//
// Starts a minimal net/http server on :8080. The /health endpoint is used
// by Docker Compose for readiness checks.

package main

import (
	"log"
	"net/http"

	"daily-patch/api/internal/response"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response.Write(w, http.StatusOK, "", "", map[string]string{"status": "ok", "service": "api"})
	})

	log.Println("api listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
