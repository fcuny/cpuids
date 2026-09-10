// Package ingest and its subpackages implement the offline pipeline that turns
// upstream ID tables into data/cpu_models.json plus the SQLite build output.
//
// Layout:
//
//	sources/    sources.yaml: pin repo/path/ref, fetch raw content, resolve commit
//	parse/      per-vendor parsers (intel, arm, amd), each producing []Record
//	overrides/  overrides.yaml: hand-maintained corrections keyed by row id
//	normalize/  Record + overrides -> dataset.File (slugs, generation_rank)
//	sqlite/     dataset.File -> schema.sql + cpu_models.db
//
// Nothing here is imported by the core cpuids package; it stays pure.
package ingest

import "fcuny.net/cpuids/internal/dataset"

// Record is a normalized-but-not-yet-keyed row emitted by a parser. The
// normalizer assigns the stable id and generation_rank and applies overrides.
//
// A parser fills exactly one of the x86 (Family+Model) or ARM
// (ImplementerID+PartID) field pairs, and sets Name to the vendor-assigned
// fact as extracted from the source — no upstream prose, comments or
// editorializing text.
type Record struct {
	Arch string // dataset.ArchX86 | dataset.ArchARM

	// Vendor is the value stored in the row: the raw CPUID vendor string for
	// x86 ("GenuineIntel", "AuthenticAMD"), a human-readable implementer name
	// for ARM ("ARM Ltd", "NVIDIA").
	Vendor string

	Family *int
	Model  *int

	ImplementerID *int
	PartID        *int

	Name        string
	Microarch   string
	Segment     string // "" until a heuristic or override fills it; normalizer defaults to "unknown"
	ReleaseYear int
	Aliases     []string
	Notes       string

	Confidence string // dataset.ConfidenceExact | dataset.ConfidenceInferred
	Provenance dataset.Provenance
}
