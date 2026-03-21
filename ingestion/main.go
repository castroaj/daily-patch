// main.go — entry point for the ingestion service
//
// One-shot binary: fetches vulnerability data from configured sources,
// normalizes records, and persists them via the REST API, then exits.

package main

import "fmt"

func main() {
	fmt.Println("hello from ingestion")
}
