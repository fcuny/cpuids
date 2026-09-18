package confirmppr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest/parse/amd"
)

func TestParseCommand(t *testing.T) {
	body := "/confirm-ppr\n" +
		"url: https://docs.amd.com/v/u/en-US/57238\n" +
		"name: Turin\n" +
		"microarch: Zen 5\n" +
		"segment: server\n" +
		"release_year: 2024\n" +
		"aliases:\n" +
		"  - EPYC 9005\n"
	c, err := ParseCommand([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://docs.amd.com/v/u/en-US/57238" || c.Name != "Turin" || c.Microarch != "Zen 5" {
		t.Errorf("parsed = %+v", c)
	}
	if c.Segment != "server" || c.ReleaseYear != 2024 || len(c.Aliases) != 1 || c.Aliases[0] != "EPYC 9005" {
		t.Errorf("parsed = %+v", c)
	}
}

func TestParseCommandRequiresMarker(t *testing.T) {
	if _, err := ParseCommand([]byte("url: https://example.com\nname: X\n")); err == nil {
		t.Error("expected an error for a body missing the marker line")
	}
}

func TestParseCommandRequiresURLAndName(t *testing.T) {
	if _, err := ParseCommand([]byte("/confirm-ppr\nname: X\n")); err == nil {
		t.Error("expected an error for a missing url")
	}
	if _, err := ParseCommand([]byte("/confirm-ppr\nurl: https://example.com\n")); err == nil {
		t.Error("expected an error for a missing name")
	}
}

func TestParseIssueReport(t *testing.T) {
	b, err := os.ReadFile("testdata/issue-8-body.txt")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := ParseIssueReport(b)
	if err != nil {
		t.Fatal(err)
	}
	if fields["vendor_id"] != "AuthenticAMD" || fields["cpu family"] != "26" || fields["model"] != "36" {
		t.Errorf("fields = %+v", fields)
	}
}

func TestReportedFamilyModel(t *testing.T) {
	fam, mod, err := ReportedFamilyModel(map[string]string{
		"vendor_id": "AuthenticAMD", "cpu family": "26", "model": "36",
	})
	if err != nil || fam != 26 || mod != 36 {
		t.Fatalf("got %d/%d, %v", fam, mod, err)
	}
}

func TestReportedFamilyModelRejectsARM(t *testing.T) {
	_, _, err := ReportedFamilyModel(map[string]string{"cpu implementer": "0x41", "cpu part": "0xd07"})
	if err == nil {
		t.Error("expected ARM report to be rejected")
	}
}

func TestReportedFamilyModelRejectsNonAMD(t *testing.T) {
	_, _, err := ReportedFamilyModel(map[string]string{
		"vendor_id": "GenuineIntel", "cpu family": "6", "model": "143",
	})
	if err == nil {
		t.Error("expected non-AMD vendor to be rejected")
	}
}

func TestExtractFamilyModel(t *testing.T) {
	page := []byte("57238 Rev 0.24 - Sep 29, 2024   PPR Vol 1 for AMD Family 1Ah Model 02h C1\n")
	fam, mod, ok := ExtractFamilyModel(page)
	if !ok || fam != 26 || mod != 2 {
		t.Fatalf("got %d/%d ok=%v", fam, mod, ok)
	}
}

func TestExtractFamilyModelNoMatch(t *testing.T) {
	if _, _, ok := ExtractFamilyModel([]byte("nothing relevant here")); ok {
		t.Error("expected no match")
	}
}

const sampleAMDFamilies = `{
  "_comment": "test fixture",
  "entries": [
    { "family": 23, "model": 1, "name": "Naples", "microarch": "Zen", "segment": "server", "release_year": 2017, "aliases": ["EPYC 7001"] },
    { "family": 25, "model": 1, "name": "Milan", "microarch": "Zen 3", "segment": "server", "release_year": 2021, "aliases": ["EPYC 7003"] }
  ]
}
`

func TestInsertEntry(t *testing.T) {
	out, err := InsertEntry([]byte(sampleAMDFamilies), Entry{
		Family: 26, Model: 2, Name: "Turin", Microarch: "Zen 5",
		Segment: "server", ReleaseYear: 2024, Aliases: []string{"EPYC 9005"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, `"family": 26, "model": 2, "name": "Turin"`) {
		t.Errorf("missing new entry:\n%s", got)
	}
	if strings.Count(got, "{") != 4 { // 1 top-level object + 3 entries
		t.Errorf("expected 3 entries after insert, got:\n%s", got)
	}
	// Must still parse as valid JSON via the real AMD parser.
	if _, err := amd.Parse(out, dataset.Provenance{}); err != nil {
		t.Fatalf("inserted result does not parse: %v\n%s", err, got)
	}
}

func TestAlreadyPresent(t *testing.T) {
	name, ok := AlreadyPresent([]byte(sampleAMDFamilies), 25, 1)
	if !ok || name != "Milan" {
		t.Errorf("got %q, %v", name, ok)
	}
	if _, ok := AlreadyPresent([]byte(sampleAMDFamilies), 26, 2); ok {
		t.Error("expected 26/2 to be absent")
	}
}

func TestValidate(t *testing.T) {
	inserted, err := InsertEntry([]byte(sampleAMDFamilies), Entry{
		Family: 26, Model: 2, Name: "Turin", Microarch: "Zen 5", Segment: "server", ReleaseYear: 2024,
	})
	if err != nil {
		t.Fatal(err)
	}
	missingOverrides := filepath.Join(t.TempDir(), "missing-overrides.yaml")
	f, err := Validate(inserted, missingOverrides)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Models) != 3 {
		t.Errorf("got %d models, want 3", len(f.Models))
	}
}
