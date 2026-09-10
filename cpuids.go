package cpuids

import (
	"fmt"
	"sync"

	"fcuny.net/cpuids/internal/dataset"
)

// Key is a sealed interface implemented by [X86Key] and [ARMKey].
type Key interface{ isKey() }

// X86Key identifies an x86 part by the raw CPUID vendor string plus the
// effective family and model numbers.
//
// Vendor takes the raw CPUID vendor string as-is — "GenuineIntel",
// "AuthenticAMD" — exactly what a caller already has from /proc/cpuinfo or a
// raw CPUID call. There is no caller-side translation table; the human-readable
// vendor name is a field on the returned [Model].
//
// Family and Model must be the effective values, i.e. with the extended-model
// and extended-family bits already folded in. /proc/cpuinfo already reports
// effective values; a caller reading raw CPUID leaves must fold them itself
// (see [EffectiveFamily] and [EffectiveModel]).
type X86Key struct {
	Vendor string
	Family int
	Model  int
}

func (X86Key) isKey() {}

// ARMKey identifies an ARM part by its MIDR implementer and part fields.
type ARMKey struct {
	ImplementerID uint8  // MIDR implementer field, 8 bits
	PartID        uint16 // MIDR part field, 12 bits
}

func (ARMKey) isKey() {}

// Model is the resolved, caller-facing view of a CPU model. It is a projection
// of the fuller database row: provenance and raw ID fields are omitted.
type Model struct {
	Vendor         string // human-readable: "Intel", "AMD", "NVIDIA", "ARM Ltd"
	Name           string
	Microarch      string
	Segment        string // "server" | "client" | "embedded" | "unknown"
	GenerationRank int
	ReleaseYear    int
	Aliases        []string
}

// Resolve looks up k and reports whether it was found, with map-style
// not-found semantics: the zero [Model] and false when there is no match.
func Resolve(k Key) (Model, bool) {
	idx := index()
	switch key := k.(type) {
	case X86Key:
		r, ok := idx.x86[x86k{key.Vendor, key.Family, key.Model}]
		if !ok {
			return Model{}, false
		}
		return toModel(r), true
	case ARMKey:
		r, ok := idx.arm[armk{key.ImplementerID, key.PartID}]
		if !ok {
			return Model{}, false
		}
		return toModel(r), true
	default:
		return Model{}, false
	}
}

// SchemaVersion reports the schema_version of the embedded dataset.
func SchemaVersion() int { return index().manifest.SchemaVersion }

// GeneratedAt reports the generated_at timestamp of the embedded dataset, an
// RFC 3339 string, as a freshness signal independent of the module version.
func GeneratedAt() string { return index().manifest.GeneratedAt }

type x86k struct {
	vendor string
	family int
	model  int
}

type armk struct {
	impl uint8
	part uint16
}

type datasetIndex struct {
	x86      map[x86k]dataset.Row
	arm      map[armk]dataset.Row
	manifest dataset.Manifest
}

var (
	idxOnce sync.Once
	idxVal  *datasetIndex
)

func index() *datasetIndex {
	idxOnce.Do(func() {
		f, err := dataset.Parse(rawData)
		if err != nil {
			panic(fmt.Sprintf("cpuids: embedded dataset is invalid: %v", err))
		}
		di := &datasetIndex{
			x86:      make(map[x86k]dataset.Row),
			arm:      make(map[armk]dataset.Row),
			manifest: f.Manifest,
		}
		for _, r := range f.Models {
			switch r.Arch {
			case dataset.ArchX86:
				if r.Family == nil || r.Model == nil {
					continue
				}
				di.x86[x86k{r.Vendor, *r.Family, *r.Model}] = r
			case dataset.ArchARM:
				if r.ImplementerID == nil || r.PartID == nil {
					continue
				}
				di.arm[armk{uint8(*r.ImplementerID), uint16(*r.PartID)}] = r
			}
		}
		idxVal = di
	})
	return idxVal
}

func toModel(r dataset.Row) Model {
	var aliases []string
	if len(r.Aliases) > 0 {
		aliases = append(aliases, r.Aliases...)
	}
	name := r.Name
	micro := r.MicroarchCodename
	return Model{
		Vendor:         humanVendor(r.Vendor, r.Arch),
		Name:           name,
		Microarch:      micro,
		Segment:        r.Segment,
		GenerationRank: r.GenerationRank,
		ReleaseYear:    r.ReleaseYear,
		Aliases:        aliases,
	}
}

// humanVendor maps the stored vendor value to a display string. For x86 the
// stored value is the raw CPUID vendor string; for ARM it is already
// human-readable.
func humanVendor(stored, arch string) string {
	if arch == dataset.ArchX86 {
		switch stored {
		case "GenuineIntel":
			return "Intel"
		case "AuthenticAMD":
			return "AMD"
		case "HygonGenuine":
			return "Hygon"
		case "CentaurHauls":
			return "Centaur"
		}
	}
	return stored
}
