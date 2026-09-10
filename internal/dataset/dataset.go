// Package dataset defines the on-disk shape of data/cpu_models.json and the
// row/manifest types shared by the lookup package and the ingestion pipeline.
//
// It is pure data: no file or network I/O, no OS-specific code. The lookup
// package embeds the JSON at compile time and parses it with [Parse].
package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// SchemaVersion is the current schema contract. A breaking change to the row
// shape bumps this (and the module major version).
const SchemaVersion = 1

// Arch values.
const (
	ArchX86 = "x86_64"
	ArchARM = "aarch64"
)

// Segment values. Segment is a filterable field, not an ingestion-scope filter.
const (
	SegmentServer   = "server"
	SegmentClient   = "client"
	SegmentEmbedded = "embedded"
	SegmentUnknown  = "unknown"
)

// Confidence records whether a row's (id, name) pair came verbatim from a
// vendor-assigned fact ("exact") or was derived/humanized during normalization
// ("inferred").
const (
	ConfidenceExact    = "exact"
	ConfidenceInferred = "inferred"
)

// Provenance is attached to every row: where the fact came from and how sure we
// are of it.
type Provenance struct {
	SourceRepo   string `json:"source_repo"`
	SourcePath   string `json:"source_path"`
	SourceCommit string `json:"source_commit"`
	IngestedAt   string `json:"ingested_at"`
	Confidence   string `json:"confidence"`
}

// Row is one entry in the core cpu_models table.
//
// Family/Model are x86-only and nil on aarch64 rows. ImplementerID/PartID are
// aarch64-only and nil on x86 rows.
type Row struct {
	ID                string     `json:"id"`
	Vendor            string     `json:"vendor"`
	Arch              string     `json:"arch"`
	Family            *int       `json:"family"`
	Model             *int       `json:"model"`
	ImplementerID     *int       `json:"implementer_id"`
	PartID            *int       `json:"part_id"`
	Name              string     `json:"name"`
	MicroarchCodename string     `json:"microarch_codename"`
	Segment           string     `json:"segment"`
	GenerationRank    int        `json:"generation_rank"`
	ReleaseYear       int        `json:"release_year"`
	Aliases           []string   `json:"aliases"`
	Notes             string     `json:"notes"`
	Provenance        Provenance `json:"provenance"`
}

// ManifestSource identifies one upstream source at the commit it was read from.
type ManifestSource struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
}

// Manifest is the per-snapshot header.
type Manifest struct {
	SchemaVersion int              `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Sources       []ManifestSource `json:"sources"`
}

// File is the top-level structure of data/cpu_models.json.
type File struct {
	Manifest Manifest `json:"manifest"`
	Models   []Row    `json:"models"`
}

// Parse decodes a data/cpu_models.json byte slice.
func Parse(b []byte) (File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("dataset: parse: %w", err)
	}
	return f, nil
}

// Marshal encodes a File as stable, human-diffable JSON: rows sorted by ID,
// two-space indent, trailing newline.
func Marshal(f File) ([]byte, error) {
	sort.SliceStable(f.Models, func(i, j int) bool { return f.Models[i].ID < f.Models[j].ID })
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dataset: marshal: %w", err)
	}
	return append(b, '\n'), nil
}

// IntPtr is a small helper for building rows.
func IntPtr(v int) *int { return &v }
