package arm

import (
	"os"
	"testing"

	"fcuny.net/cpuids/internal/dataset"
)

func TestParse(t *testing.T) {
	b, err := os.ReadFile("testdata/synthetic-lscpu-arm.c")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Parse(b, dataset.Provenance{SourceCommit: "abc123"})
	if err != nil {
		t.Fatal(err)
	}

	type key struct{ impl, part int }
	got := map[key]string{}
	for _, r := range recs {
		if r.Arch != dataset.ArchARM || r.Confidence != dataset.ConfidenceExact {
			t.Errorf("bad record shape: %+v", r)
		}
		if r.Provenance.SourceCommit != "abc123" {
			t.Errorf("provenance not propagated: %+v", r.Provenance)
		}
		got[key{*r.ImplementerID, *r.PartID}] = r.Name + "|" + r.Vendor
	}

	if len(got) != 3 {
		t.Fatalf("got %d records, want 3: %v", len(got), got)
	}
	if got[key{0x99, 0x001}] != "Acme-One|Acme" {
		t.Errorf("0x99/0x001 = %q", got[key{0x99, 0x001}])
	}
	if got[key{0x99, 0xabc}] != "Acme-Nova|Acme" {
		t.Errorf("0x99/0xabc = %q", got[key{0x99, 0xabc}])
	}
	if got[key{0x42, 0x0f0}] != "Globex-Zero|Globex" {
		t.Errorf("0x42/0x0f0 = %q", got[key{0x42, 0x0f0}])
	}
}

func TestParseImplementerRename(t *testing.T) {
	src := `static const struct id_part arm_part[] = {
		{ 0xd4f, "Neoverse-V2" },
		{ -1, "unknown" },
	};
	static const struct hw_impl hw_implementer[] = {
		{ 0x41, arm_part, "ARM" },
	};`
	recs, err := Parse([]byte(src), dataset.Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Vendor != "ARM Ltd" {
		t.Fatalf("want 1 record vendor 'ARM Ltd', got %+v", recs)
	}
}

func TestParseMissingTable(t *testing.T) {
	if _, err := Parse([]byte("static const struct id_part x[] = { { 0x1, \"a\" } };"), dataset.Provenance{}); err == nil {
		t.Error("expected error when hw_implementer table is absent")
	}
}
