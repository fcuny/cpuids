package overrides

import "testing"

func TestParse(t *testing.T) {
	y := `
overrides:
  intel-6-143:
    name: "Sapphire Rapids-SP"
    microarch_codename: "Sapphire Rapids"
    segment: server
    generation_rank: 90
    release_year: 2023
    aliases:
      - "4th Gen Intel Xeon Scalable"
    notes: "example"
  arm-0x41-0xd4f:
    segment: server
`
	set, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 2 {
		t.Fatalf("got %d overrides, want 2", len(set))
	}
	o := set["intel-6-143"]
	if o.Name == nil || *o.Name != "Sapphire Rapids-SP" {
		t.Errorf("name = %v", o.Name)
	}
	if o.GenerationRank == nil || *o.GenerationRank != 90 {
		t.Errorf("generation_rank = %v", o.GenerationRank)
	}
	if len(o.Aliases) != 1 {
		t.Errorf("aliases = %v", o.Aliases)
	}
	if set["arm-0x41-0xd4f"].Segment == nil || *set["arm-0x41-0xd4f"].Segment != "server" {
		t.Error("arm segment override missing")
	}
	// A field not mentioned stays nil so it is not applied.
	if set["arm-0x41-0xd4f"].Name != nil {
		t.Error("absent field should be nil")
	}
}

func TestParseRejectsBadSegment(t *testing.T) {
	if _, err := Parse([]byte("overrides:\n  x:\n    segment: laptop\n")); err == nil {
		t.Error("expected invalid segment to be rejected")
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	if _, err := Parse([]byte("overrides:\n  x:\n    stepping: 4\n")); err == nil {
		t.Error("expected unknown field to be rejected")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	set, err := Load("/no/such/overrides.yaml")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(set) != 0 {
		t.Errorf("want empty set, got %v", set)
	}
}
