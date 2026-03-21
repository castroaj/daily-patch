// main.go — entry point for the api service
//
// Starts a minimal net/http server on :8080. The /health endpoint is used
// by Docker Compose for readiness checks.

package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "api"})
	})

	log.Println("api listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
