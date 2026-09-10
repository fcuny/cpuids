package intel

import (
	"os"
	"testing"

	"fcuny.net/cpuids/internal/dataset"
)

func load(t *testing.T) []recordView {
	t.Helper()
	b, err := os.ReadFile("testdata/synthetic-intel-family.h")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Parse(b, dataset.Provenance{SourceRepo: "x", SourcePath: "y", SourceCommit: "z"})
	if err != nil {
		t.Fatal(err)
	}
	var out []recordView
	for _, r := range recs {
		out = append(out, recordView{name: r.Name, model: *r.Model, seg: r.Segment, conf: r.Confidence, repo: r.Provenance.SourceRepo})
	}
	return out
}

type recordView struct {
	name  string
	model int
	seg   string
	conf  string
	repo  string
}

func TestParseCounts(t *testing.T) {
	got := load(t)
	// FICTIONAL_LAKE, FICTIONAL_LAKE_L, FICTIONAL_RIDGE_X, FICTIONAL_RIDGE_D,
	// MYTHIC_MESA_N. ANY is non-numeric; the DUP repeats model 0x10.
	if len(got) != 5 {
		t.Fatalf("got %d records, want 5: %+v", len(got), got)
	}
}

func TestParseFields(t *testing.T) {
	byModel := map[int]recordView{}
	for _, r := range load(t) {
		byModel[r.model] = r
	}

	if r := byModel[0x2A]; r.seg != dataset.SegmentServer || r.name != "FICTIONAL_RIDGE_X" {
		t.Errorf("0x2A = %+v, want server / FICTIONAL_RIDGE_X", r)
	}
	if r := byModel[0x11]; r.seg != dataset.SegmentClient {
		t.Errorf("0x11 segment = %q, want client", r.seg)
	}
	if r := byModel[0x10]; r.seg != dataset.SegmentUnknown {
		t.Errorf("0x10 segment = %q, want unknown", r.seg)
	}
	if r := byModel[50]; r.name != "MYTHIC_MESA_N" || r.seg != dataset.SegmentClient {
		t.Errorf("decimal define = %+v, want MYTHIC_MESA_N / client", r)
	}
	for _, r := range load(t) {
		if r.conf != dataset.ConfidenceInferred {
			t.Errorf("%s confidence = %q, want inferred", r.name, r.conf)
		}
		if r.repo != "x" {
			t.Errorf("%s provenance not propagated: %q", r.name, r.repo)
		}
	}
}

func TestParseFirstDefinitionWins(t *testing.T) {
	for _, r := range load(t) {
		if r.name == "FICTIONAL_LAKE_DUP" {
			t.Fatal("duplicate model 0x10 should have been dropped")
		}
	}
}
