// vuln.go — core domain types shared across all ingestion packages
//
// Defines SourceType, Vulnerability, and RunRecord. This package is a leaf
// with no internal imports; all other packages may import it freely.

package types

import (
	"encoding/json"
	"time"
)

// SourceType identifies which upstream feed a vulnerability came from.
// Values must match the CHECK constraint on the vulnerabilities and
// ingestion_runs tables.
type SourceType string

const (
	SourceNVD       SourceType = "nvd"
	SourceGHSA      SourceType = "ghsa"
	SourceExploitDB SourceType = "exploitdb"
)

// Vulnerability is the normalized representation of a single advisory record.
// Only fields with dedicated database columns are promoted to struct fields;
// the original source payload is preserved verbatim in RawJSON.
//
// id and ingested_at are absent — they are assigned by the database, not by
// the ingestion service.
type Vulnerability struct {

	// Canonical identifiers — at least one must be non-empty.
	// An Exploit-DB entry may carry a CVE-ID if the advisory references one,
	// but a standalone PoC may have only an EDB-ID.
	CVEID  string // e.g. "CVE-2024-1234"
	GHSAID string // e.g. "GHSA-xxxx-xxxx-xxxx"
	EDBID  string // e.g. "EDB-12345"

	Source      SourceType
	Title       string
	Description string

	// CVSSScore of 0.0 means the score is not available; always check
	// CVSSVector before treating 0.0 as a real score.
	CVSSScore  float64
	CVSSVector string // e.g. "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"

	PublishedAt time.Time // zero if unknown
	UpdatedAt   time.Time // zero if unknown

	RawJSON json.RawMessage // original source payload, stored verbatim
}

// RunRecord represents a completed ingestion run for a single source.
// It maps directly to a row in the ingestion_runs table.
type RunRecord struct {
	Source       SourceType
	StartedAt    time.Time
	FinishedAt   time.Time
	ItemsFetched int // total records returned by Fetch
	ItemsNew     int // CreateVuln calls that succeeded (not updates)
}
