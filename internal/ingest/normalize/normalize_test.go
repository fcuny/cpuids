package normalize

import (
	"testing"
	"time"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
	"fcuny.net/cpuids/internal/ingest/overrides"
)

func x86(vendor string, fam, mod int, name, seg string, conf string) ingest.Record {
	return ingest.Record{
		Arch: dataset.ArchX86, Vendor: vendor,
		Family: &fam, Model: &mod, Name: name, Segment: seg, Confidence: conf,
		Provenance: dataset.Provenance{Confidence: conf},
	}
}

func arm(impl, part int, name string) ingest.Record {
	return ingest.Record{
		Arch: dataset.ArchARM, Vendor: "ARM Ltd",
		ImplementerID: &impl, PartID: &part, Name: name,
		Confidence: dataset.ConfidenceExact,
		Provenance: dataset.Provenance{Confidence: dataset.ConfidenceExact},
	}
}

func build(t *testing.T, recs []ingest.Record, ov overrides.Set) dataset.File {
	t.Helper()
	if ov == nil {
		ov = overrides.Set{}
	}
	r, err := Build(recs, ov, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := dataset.Validate(r.File); err != nil {
		t.Fatalf("built file fails validation: %v", err)
	}
	return r.File
}

func find(f dataset.File, id string) (dataset.Row, bool) {
	for _, r := range f.Models {
		if r.ID == id {
			return r, true
		}
	}
	return dataset.Row{}, false
}

func TestIDAndHumanization(t *testing.T) {
	f := build(t, []ingest.Record{
		x86("GenuineIntel", 6, 143, "SAPPHIRERAPIDS_X", dataset.SegmentServer, dataset.ConfidenceInferred),
		x86("AuthenticAMD", 25, 17, "Genoa", dataset.SegmentServer, dataset.ConfidenceExact),
		arm(0x41, 0xd4f, "Neoverse-V2"),
	}, nil)

	if _, ok := find(f, "intel-6-143"); !ok {
		t.Error("missing id intel-6-143")
	}
	if r, _ := find(f, "amd-25-17"); r.Name != "Genoa" {
		t.Errorf("amd name = %q, want Genoa (clean names untouched)", r.Name)
	}
	if r, _ := find(f, "arm-0x41-0xd4f"); r.Name != "Neoverse-V2" {
		t.Errorf("arm id/name wrong: %+v", r)
	}
	r, _ := find(f, "intel-6-143")
	if r.Name != "Sapphirerapids" {
		t.Errorf("humanized name = %q, want Sapphirerapids (crude; overrides refine)", r.Name)
	}
}

func TestGenerationRankMonotonic(t *testing.T) {
	f := build(t, []ingest.Record{
		{Arch: dataset.ArchX86, Vendor: "GenuineIntel", Family: iptr(6), Model: iptr(85), Name: "Skylake", ReleaseYear: 2017, Confidence: dataset.ConfidenceInferred, Provenance: dataset.Provenance{Confidence: dataset.ConfidenceInferred}},
		{Arch: dataset.ArchX86, Vendor: "GenuineIntel", Family: iptr(6), Model: iptr(143), Name: "SPR", ReleaseYear: 2023, Confidence: dataset.ConfidenceInferred, Provenance: dataset.Provenance{Confidence: dataset.ConfidenceInferred}},
		{Arch: dataset.ArchX86, Vendor: "GenuineIntel", Family: iptr(6), Model: iptr(63), Name: "HSW", ReleaseYear: 2014, Confidence: dataset.ConfidenceInferred, Provenance: dataset.Provenance{Confidence: dataset.ConfidenceInferred}},
	}, nil)

	hsw, _ := find(f, "intel-6-63")
	sky, _ := find(f, "intel-6-85")
	spr, _ := find(f, "intel-6-143")
	if !(hsw.GenerationRank < sky.GenerationRank && sky.GenerationRank < spr.GenerationRank) {
		t.Errorf("ranks not ordered by release year: hsw=%d sky=%d spr=%d",
			hsw.GenerationRank, sky.GenerationRank, spr.GenerationRank)
	}
}

func TestOverridesApplied(t *testing.T) {
	name := "Sapphire Rapids-SP"
	micro := "Sapphire Rapids"
	rank := 99
	f := build(t, []ingest.Record{
		x86("GenuineIntel", 6, 143, "SAPPHIRERAPIDS_X", dataset.SegmentServer, dataset.ConfidenceInferred),
	}, overrides.Set{
		"intel-6-143": {
			Name:              &name,
			MicroarchCodename: &micro,
			GenerationRank:    &rank,
			Aliases:           []string{"4th Gen Xeon Scalable"},
		},
	})

	r, _ := find(f, "intel-6-143")
	if r.Name != name || r.MicroarchCodename != micro || r.GenerationRank != 99 {
		t.Errorf("override not applied: %+v", r)
	}
	if len(r.Aliases) != 1 || r.Aliases[0] != "4th Gen Xeon Scalable" {
		t.Errorf("alias override not applied: %+v", r.Aliases)
	}
}

func TestDuplicatePrefersExact(t *testing.T) {
	r, err := Build([]ingest.Record{
		x86("GenuineIntel", 6, 143, "FROM_INFERRED", "", dataset.ConfidenceInferred),
		x86("GenuineIntel", 6, 143, "From Exact", dataset.SegmentServer, dataset.ConfidenceExact),
	}, overrides.Set{}, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := find(r.File, "intel-6-143")
	if row.Name != "From Exact" {
		t.Errorf("expected exact-confidence row to win, got %q", row.Name)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a duplicate-id warning")
	}
}

func TestOverrideMatchingNothingWarns(t *testing.T) {
	r, err := Build([]ingest.Record{x86("GenuineIntel", 6, 143, "X", "", dataset.ConfidenceInferred)},
		overrides.Set{"intel-6-999": {}}, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range r.Warnings {
		if w == `override for "intel-6-999" matched no row` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unmatched-override warning, got %v", r.Warnings)
	}
}

func iptr(v int) *int { return &v }
