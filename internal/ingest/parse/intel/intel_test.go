package intel

import (
	"os"
	"testing"

	"fcuny.net/cpuids/internal/dataset"
)

type recordView struct {
	name       string
	fam, model int
	seg        string
	conf       string
	repo       string
}

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
		out = append(out, recordView{
			name: r.Name, fam: *r.Family, model: *r.Model,
			seg: r.Segment, conf: r.Confidence, repo: r.Provenance.SourceRepo,
		})
	}
	return out
}

func TestParseCounts(t *testing.T) {
	got := load(t)
	// LAKE, LAKE_L, RIDGE_X, RIDGE_D, MESA_N, ANCIENT_QUARK (fam 5), LEGACY_PEAK_X.
	// INTEL_ANY is non-numeric; DUP repeats 6/0x10.
	if len(got) != 7 {
		t.Fatalf("got %d records, want 7: %+v", len(got), got)
	}
}

func TestParseBothGrammars(t *testing.T) {
	byKey := map[[2]int]recordView{}
	for _, r := range load(t) {
		byKey[[2]int{r.fam, r.model}] = r
	}

	if r := byKey[[2]int{6, 0x2A}]; r.seg != dataset.SegmentServer || r.name != "FICTIONAL_RIDGE_X" {
		t.Errorf("6/0x2A = %+v, want server / FICTIONAL_RIDGE_X", r)
	}
	if r := byKey[[2]int{6, 0x11}]; r.seg != dataset.SegmentClient {
		t.Errorf("6/0x11 segment = %q, want client", r.seg)
	}
	if r := byKey[[2]int{6, 0x10}]; r.seg != dataset.SegmentUnknown {
		t.Errorf("6/0x10 segment = %q, want unknown", r.seg)
	}
	if r := byKey[[2]int{6, 50}]; r.name != "MYTHIC_MESA_N" || r.seg != dataset.SegmentClient {
		t.Errorf("decimal IFM model = %+v, want MYTHIC_MESA_N / client", r)
	}
	if r := byKey[[2]int{5, 9}]; r.name != "ANCIENT_QUARK" {
		t.Errorf("family-5 IFM entry missing/wrong: %+v", r)
	}
	if r := byKey[[2]int{6, 0x3F}]; r.name != "LEGACY_PEAK_X" || r.seg != dataset.SegmentServer {
		t.Errorf("legacy bare-number form = %+v, want LEGACY_PEAK_X / server", r)
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
			t.Fatal("duplicate 6/0x10 should have been dropped")
		}
	}
}

// Guards against the header migrating to a form the parser no longer matches.
func TestParseCurrentUpstreamShape(t *testing.T) {
	sample := `#define IFM(_fam, _model)	VFM_MAKE(X86_VENDOR_INTEL, _fam, _model)
#define INTEL_SAPPHIRERAPIDS_X		IFM(6, 0x8F)
#define INTEL_EMERALDRAPIDS_X		IFM(6, 0xCF)
#define INTEL_LUNARLAKE_M		IFM(6, 0xBD)
`
	recs, err := Parse([]byte(sample), dataset.Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("got %d, want 3 from current-shape sample", len(recs))
	}
}
