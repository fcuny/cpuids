package dataset

import (
	"errors"
	"fmt"
	"sort"
)

// Validate runs the schema-level checks that must hold for any published
// snapshot:
//
//   - schema_version matches this build
//   - ids are unique and non-empty
//   - required fields are present (vendor, arch, name, segment)
//   - arch is a known value, and the arch-specific ID fields are populated
//     (family+model for x86, implementer+part for aarch64) and mutually
//     exclusive
//   - segment and confidence are known values
//   - generation_rank is strictly increasing (distinct) within each
//     vendor+arch group
//
// The count-drop "canary" check is not here: it needs the previous run's
// counts and lives in the ingestion pipeline.
func Validate(f File) error {
	var errs []error

	if f.Manifest.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("manifest schema_version = %d, want %d", f.Manifest.SchemaVersion, SchemaVersion))
	}

	seen := make(map[string]int, len(f.Models))
	type grp struct{ vendor, arch string }
	ranks := make(map[grp][]int)

	for i, r := range f.Models {
		where := fmt.Sprintf("models[%d] id=%q", i, r.ID)

		if r.ID == "" {
			errs = append(errs, fmt.Errorf("%s: empty id", where))
		} else if prev, dup := seen[r.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate id (also models[%d])", where, prev))
		} else {
			seen[r.ID] = i
		}

		if r.Vendor == "" {
			errs = append(errs, fmt.Errorf("%s: empty vendor", where))
		}
		if r.Name == "" {
			errs = append(errs, fmt.Errorf("%s: empty name", where))
		}

		switch r.Arch {
		case ArchX86:
			if r.Family == nil || r.Model == nil {
				errs = append(errs, fmt.Errorf("%s: x86_64 row missing family/model", where))
			}
			if r.ImplementerID != nil || r.PartID != nil {
				errs = append(errs, fmt.Errorf("%s: x86_64 row has ARM implementer/part set", where))
			}
		case ArchARM:
			if r.ImplementerID == nil || r.PartID == nil {
				errs = append(errs, fmt.Errorf("%s: aarch64 row missing implementer_id/part_id", where))
			}
			if r.Family != nil || r.Model != nil {
				errs = append(errs, fmt.Errorf("%s: aarch64 row has x86 family/model set", where))
			}
		default:
			errs = append(errs, fmt.Errorf("%s: unknown arch %q", where, r.Arch))
		}

		switch r.Segment {
		case SegmentServer, SegmentClient, SegmentEmbedded, SegmentUnknown:
		default:
			errs = append(errs, fmt.Errorf("%s: unknown segment %q", where, r.Segment))
		}

		switch r.Provenance.Confidence {
		case ConfidenceExact, ConfidenceInferred:
		default:
			errs = append(errs, fmt.Errorf("%s: unknown confidence %q", where, r.Provenance.Confidence))
		}

		g := grp{r.Vendor, r.Arch}
		ranks[g] = append(ranks[g], r.GenerationRank)
	}

	for g, rs := range ranks {
		sorted := append([]int(nil), rs...)
		sort.Ints(sorted)
		for i := 1; i < len(sorted); i++ {
			if sorted[i] == sorted[i-1] {
				errs = append(errs, fmt.Errorf("vendor=%q arch=%q: duplicate generation_rank %d (must be distinct/monotonic per group)", g.vendor, g.arch, sorted[i]))
				break
			}
		}
	}

	return errors.Join(errs...)
}
