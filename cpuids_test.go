package cpuids

import (
	"testing"

	"fcuny.net/cpuids/internal/dataset"
)

func TestResolveX86(t *testing.T) {
	m, ok := Resolve(X86Key{Vendor: "GenuineIntel", Family: 6, Model: 143})
	if !ok {
		t.Fatal("expected GenuineIntel 6/143 to resolve")
	}
	if m.Vendor != "Intel" {
		t.Errorf("Vendor = %q, want Intel", m.Vendor)
	}
	if m.Microarch != "Sapphire Rapids" {
		t.Errorf("Microarch = %q, want Sapphire Rapids", m.Microarch)
	}
	if m.Segment != dataset.SegmentServer {
		t.Errorf("Segment = %q, want server", m.Segment)
	}
}

func TestResolveARM(t *testing.T) {
	m, ok := Resolve(ARMKey{ImplementerID: 0x41, PartID: 0xd4f})
	if !ok {
		t.Fatal("expected ARM 0x41/0xd4f to resolve")
	}
	if m.Name != "Neoverse-V2" {
		t.Errorf("Name = %q, want Neoverse-V2", m.Name)
	}
	if m.Vendor != "ARM Ltd" {
		t.Errorf("Vendor = %q, want ARM Ltd", m.Vendor)
	}
}

func TestResolveNotFound(t *testing.T) {
	if m, ok := Resolve(X86Key{Vendor: "GenuineIntel", Family: 6, Model: 9999}); ok {
		t.Errorf("expected miss, got %+v", m)
	}
	if _, ok := Resolve(ARMKey{ImplementerID: 0x00, PartID: 0x000}); ok {
		t.Error("expected miss for zero ARM key")
	}
}

func TestResolveVendorIsolation(t *testing.T) {
	// Same family/model, different vendor string must not cross-match.
	if _, ok := Resolve(X86Key{Vendor: "AuthenticAMD", Family: 6, Model: 143}); ok {
		t.Error("AMD must not match an Intel family/model row")
	}
}

func TestAliasesAreCopied(t *testing.T) {
	m, _ := Resolve(X86Key{Vendor: "GenuineIntel", Family: 6, Model: 85})
	if len(m.Aliases) == 0 {
		t.Fatal("expected aliases on Skylake-SP row")
	}
	m.Aliases[0] = "mutated"
	m2, _ := Resolve(X86Key{Vendor: "GenuineIntel", Family: 6, Model: 85})
	if m2.Aliases[0] == "mutated" {
		t.Error("Resolve returned a shared aliases slice; callers can corrupt the dataset")
	}
}

func TestManifestAccessors(t *testing.T) {
	if got := SchemaVersion(); got != dataset.SchemaVersion {
		t.Errorf("SchemaVersion() = %d, want %d", got, dataset.SchemaVersion)
	}
	if GeneratedAt() == "" {
		t.Error("GeneratedAt() is empty")
	}
}

func TestEmbeddedDatasetValidates(t *testing.T) {
	f, err := dataset.Parse(rawData)
	if err != nil {
		t.Fatalf("parse embedded dataset: %v", err)
	}
	if err := dataset.Validate(f); err != nil {
		t.Fatalf("embedded dataset fails validation: %v", err)
	}
}

func TestEffectiveModel(t *testing.T) {
	cases := []struct {
		baseFamily, baseModel, extModel, want int
	}{
		{0x06, 0x0f, 0x8, 0x8f}, // family 6: ext model shifted into high nibble
		{0x0f, 0x02, 0x1, 0x12}, // family 15: same
		{0x17, 0x01, 0xf, 0x01}, // other family: ext model ignored
	}
	for _, c := range cases {
		if got := EffectiveModel(c.baseFamily, c.baseModel, c.extModel); got != c.want {
			t.Errorf("EffectiveModel(%#x,%#x,%#x) = %#x, want %#x",
				c.baseFamily, c.baseModel, c.extModel, got, c.want)
		}
	}
}

func TestEffectiveFamily(t *testing.T) {
	if got := EffectiveFamily(0x0f, 0x8); got != 0x17 {
		t.Errorf("EffectiveFamily(0xf,0x8) = %#x, want 0x17", got)
	}
	if got := EffectiveFamily(0x06, 0x8); got != 0x06 {
		t.Errorf("EffectiveFamily(0x6,0x8) = %#x, want 0x6", got)
	}
}
