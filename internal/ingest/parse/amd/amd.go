// Package amd produces AMD records from a curated, in-repo table.
//
// Unlike Intel and ARM, there is no clean upstream (id, name) table for AMD:
// the mapping in Todd Allen's cpuid tool is embedded in conditional logic and
// its strings may include the author's own composition, which the project's
// licensing rules say not to copy. So the AMD "source" is
// amd_families.json in this package — a hand-maintained table re-derived from
// AMD Processor Programming Reference (PPR) documents. Each entry is a bare
// (family, model) -> product-name fact.
//
// Keep this table conservative: add an entry only when the family/model pair
// is confirmed from a PPR or an equally primary AMD document.
package amd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
)

// VendorString is the raw CPUID vendor string stored on every AMD row.
const VendorString = "AuthenticAMD"

// Table is the JSON shape of amd_families.json.
type Table struct {
	Comment string  `json:"_comment"`
	Entries []Entry `json:"entries"`
}

// Entry is one curated (family, model) -> name fact.
type Entry struct {
	Family      int      `json:"family"`
	Model       int      `json:"model"`
	Name        string   `json:"name"`
	Microarch   string   `json:"microarch"`
	Segment     string   `json:"segment"`
	ReleaseYear int      `json:"release_year"`
	Aliases     []string `json:"aliases"`
	Notes       string   `json:"notes"`
}

// Parse decodes a curated AMD table and returns one Record per entry. Names are
// re-derived from primary AMD documentation, so Confidence is "exact".
func Parse(content []byte, prov dataset.Provenance) ([]ingest.Record, error) {
	prov.Confidence = dataset.ConfidenceExact

	var t Table
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("amd: decode curated table: %w", err)
	}

	seen := make(map[[2]int]bool)
	var out []ingest.Record
	for i, e := range t.Entries {
		if e.Name == "" {
			return nil, fmt.Errorf("amd: entries[%d]: empty name", i)
		}
		if e.Family <= 0 || e.Model < 0 || e.Model > 0xFF {
			return nil, fmt.Errorf("amd: entries[%d] (%s): family/model out of range", i, e.Name)
		}
		k := [2]int{e.Family, e.Model}
		if seen[k] {
			return nil, fmt.Errorf("amd: entries[%d]: duplicate family/model %d/%d", i, e.Family, e.Model)
		}
		seen[k] = true

		seg := e.Segment
		if seg == "" {
			seg = dataset.SegmentUnknown
		}
		fam := e.Family
		mod := e.Model
		out = append(out, ingest.Record{
			Arch:        dataset.ArchX86,
			Vendor:      VendorString,
			Family:      &fam,
			Model:       &mod,
			Name:        e.Name,
			Microarch:   e.Microarch,
			Segment:     seg,
			ReleaseYear: e.ReleaseYear,
			Aliases:     e.Aliases,
			Notes:       e.Notes,
			Confidence:  dataset.ConfidenceExact,
			Provenance:  prov,
		})
	}
	return out, nil
}
