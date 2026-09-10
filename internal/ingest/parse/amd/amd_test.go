package amd

import (
	"os"
	"path/filepath"
	"testing"

	"fcuny.net/cpuids/internal/dataset"
)

func TestParseSyntheticFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/synthetic-amd-families.json")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Parse(b, dataset.Provenance{SourcePath: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	r := recs[0]
	if r.Vendor != VendorString || *r.Family != 30 || *r.Model != 1 {
		t.Errorf("record 0 = %+v", r)
	}
	if r.Name != "Fictionium" || r.Microarch != "MythZen" || r.Segment != dataset.SegmentServer {
		t.Errorf("record 0 fields = %+v", r)
	}
	if r.ReleaseYear != 2099 || len(r.Aliases) != 1 || r.Confidence != dataset.ConfidenceExact {
		t.Errorf("record 0 extras = %+v", r)
	}
	if recs[1].Segment != dataset.SegmentClient {
		t.Errorf("record 1 segment = %q", recs[1].Segment)
	}
}

func TestParseRejectsDuplicate(t *testing.T) {
	src := `{"entries":[
		{"family":25,"model":1,"name":"A"},
		{"family":25,"model":1,"name":"B"}
	]}`
	if _, err := Parse([]byte(src), dataset.Provenance{}); err == nil {
		t.Error("expected duplicate family/model to be rejected")
	}
}

func TestParseRejectsBadRange(t *testing.T) {
	src := `{"entries":[{"family":25,"model":300,"name":"X"}]}`
	if _, err := Parse([]byte(src), dataset.Provenance{}); err == nil {
		t.Error("expected out-of-range model to be rejected")
	}
}

// The shipped curated table must always parse and stay non-trivial.
func TestCuratedTableParses(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("amd_families.json"))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Parse(b, dataset.Provenance{})
	if err != nil {
		t.Fatalf("curated amd_families.json does not parse: %v", err)
	}
	if len(recs) < 5 {
		t.Errorf("curated table has only %d entries; expected the pipeline canary floor", len(recs))
	}
}
