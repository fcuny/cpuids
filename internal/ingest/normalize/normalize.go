// Package normalize turns parser Records plus overrides into a dataset.File:
// it assigns the stable id and generation_rank, humanizes bare Intel codename
// tokens, applies overrides.yaml on top, and defaults missing fields.
package normalize

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
	"fcuny.net/cpuids/internal/ingest/overrides"
)

// Result carries the built file plus non-fatal warnings worth surfacing in the
// pipeline log (id collisions, overrides that matched nothing).
type Result struct {
	File     dataset.File
	Warnings []string
}

// Build assembles the dataset. generatedAt fills the manifest; sources is the
// resolved list of upstream repos/commits for the manifest.
func Build(recs []ingest.Record, ov overrides.Set, generatedAt time.Time, sources []dataset.ManifestSource) (Result, error) {
	var warns []string

	rows := make(map[string]dataset.Row)
	order := make([]string, 0, len(recs))

	for i, r := range recs {
		row, err := toRow(r)
		if err != nil {
			return Result{}, fmt.Errorf("record %d (%s): %w", i, r.Name, err)
		}
		if existing, dup := rows[row.ID]; dup {
			if preferNew(existing, row) {
				rows[row.ID] = merged(row, existing)
			} else {
				rows[row.ID] = merged(existing, row)
			}
			warns = append(warns, fmt.Sprintf("duplicate id %q from multiple records; merged", row.ID))
			continue
		}
		rows[row.ID] = row
		order = append(order, row.ID)
	}

	// Apply overrides.
	for id, o := range ov {
		row, ok := rows[id]
		if !ok {
			warns = append(warns, fmt.Sprintf("override for %q matched no row", id))
			continue
		}
		applyOverride(&row, o)
		rows[id] = row
	}

	assignGenerationRank(rows, ov, &warns)

	out := make([]dataset.Row, 0, len(rows))
	for _, id := range order {
		out = append(out, rows[id])
	}

	f := dataset.File{
		Manifest: dataset.Manifest{
			SchemaVersion: dataset.SchemaVersion,
			GeneratedAt:   generatedAt.UTC().Format(time.RFC3339),
			Sources:       sources,
		},
		Models: out,
	}
	// dataset.Marshal sorts; sort here too so validation errors reference the
	// final order.
	sort.SliceStable(f.Models, func(i, j int) bool { return f.Models[i].ID < f.Models[j].ID })
	return Result{File: f, Warnings: warns}, nil
}

func toRow(r ingest.Record) (dataset.Row, error) {
	seg := r.Segment
	if seg == "" {
		seg = dataset.SegmentUnknown
	}
	row := dataset.Row{
		Vendor:            r.Vendor,
		Arch:              r.Arch,
		Family:            r.Family,
		Model:             r.Model,
		ImplementerID:     r.ImplementerID,
		PartID:            r.PartID,
		Name:              r.Name,
		MicroarchCodename: r.Microarch,
		Segment:           seg,
		ReleaseYear:       r.ReleaseYear,
		Aliases:           append([]string(nil), r.Aliases...),
		Notes:             r.Notes,
		Provenance:        r.Provenance,
	}

	switch r.Arch {
	case dataset.ArchX86:
		if r.Family == nil || r.Model == nil {
			return dataset.Row{}, fmt.Errorf("x86 record missing family/model")
		}
		row.ID = fmt.Sprintf("%s-%d-%d", vendorPrefix(r.Vendor), *r.Family, *r.Model)
		if looksLikeToken(r.Name) {
			display := humanizeToken(r.Name)
			row.Name = display
			if row.MicroarchCodename == "" {
				row.MicroarchCodename = display
			}
		}
	case dataset.ArchARM:
		if r.ImplementerID == nil || r.PartID == nil {
			return dataset.Row{}, fmt.Errorf("arm record missing implementer/part")
		}
		row.ID = fmt.Sprintf("arm-0x%02x-0x%03x", *r.ImplementerID, *r.PartID)
	default:
		return dataset.Row{}, fmt.Errorf("unknown arch %q", r.Arch)
	}
	if row.MicroarchCodename == "" {
		row.MicroarchCodename = row.Name
	}
	return row, nil
}

func vendorPrefix(vendor string) string {
	switch vendor {
	case "GenuineIntel":
		return "intel"
	case "AuthenticAMD":
		return "amd"
	case "HygonGenuine":
		return "hygon"
	case "CentaurHauls":
		return "centaur"
	default:
		return sanitize(strings.ToLower(vendor))
	}
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

// looksLikeToken reports whether name is a bare kernel macro token
// (ALL_CAPS_WITH_UNDERSCORES) rather than an already-clean display string.
func looksLikeToken(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return strings.ContainsRune(name, '_') || strings.ToUpper(name) == name
}

var tokenSegmentSuffixes = []string{"_X", "_D", "_L", "_H", "_N", "_S"}

// humanizeToken makes a best-effort display name from a macro token. It is
// deliberately crude — real cleanup is the job of overrides.yaml.
func humanizeToken(token string) string {
	for _, suf := range tokenSegmentSuffixes {
		if strings.HasSuffix(token, suf) {
			token = strings.TrimSuffix(token, suf)
			break
		}
	}
	parts := strings.Split(token, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if isAllDigits(p) {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, " ")
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func preferNew(existing, candidate dataset.Row) bool {
	// Prefer an exact-confidence row over an inferred one.
	if existing.Provenance.Confidence != dataset.ConfidenceExact &&
		candidate.Provenance.Confidence == dataset.ConfidenceExact {
		return true
	}
	return false
}

// merged returns primary with any empty fields filled from secondary.
func merged(primary, secondary dataset.Row) dataset.Row {
	if primary.Name == "" {
		primary.Name = secondary.Name
	}
	if primary.MicroarchCodename == "" {
		primary.MicroarchCodename = secondary.MicroarchCodename
	}
	if primary.Segment == "" || primary.Segment == dataset.SegmentUnknown {
		if secondary.Segment != "" && secondary.Segment != dataset.SegmentUnknown {
			primary.Segment = secondary.Segment
		}
	}
	if primary.ReleaseYear == 0 {
		primary.ReleaseYear = secondary.ReleaseYear
	}
	if len(primary.Aliases) == 0 {
		primary.Aliases = secondary.Aliases
	}
	if primary.Notes == "" {
		primary.Notes = secondary.Notes
	}
	return primary
}

func applyOverride(row *dataset.Row, o overrides.Override) {
	if o.Name != nil {
		row.Name = *o.Name
	}
	if o.MicroarchCodename != nil {
		row.MicroarchCodename = *o.MicroarchCodename
	}
	if o.Segment != nil {
		row.Segment = *o.Segment
	}
	if o.GenerationRank != nil {
		row.GenerationRank = *o.GenerationRank
	}
	if o.ReleaseYear != nil {
		row.ReleaseYear = *o.ReleaseYear
	}
	if o.Aliases != nil {
		row.Aliases = append([]string(nil), o.Aliases...)
	}
	if o.Notes != nil {
		row.Notes = *o.Notes
	}
}

type groupKey struct{ vendor, arch string }

// assignGenerationRank fills GenerationRank for every row that an override did
// not pin. Rows are grouped by vendor+arch and ordered by (release year, model
// or part id, id); each group is numbered from 1, skipping any rank an override
// already claimed in that group.
func assignGenerationRank(rows map[string]dataset.Row, ov overrides.Set, warns *[]string) {
	groups := make(map[groupKey][]string)
	claimed := make(map[groupKey]map[int]bool)

	for id, row := range rows {
		g := groupKey{row.Vendor, row.Arch}
		groups[g] = append(groups[g], id)
		if o, ok := ov[id]; ok && o.GenerationRank != nil {
			if claimed[g] == nil {
				claimed[g] = make(map[int]bool)
			}
			claimed[g][*o.GenerationRank] = true
		}
	}

	for g, ids := range groups {
		sort.Slice(ids, func(i, j int) bool {
			ri, rj := rows[ids[i]], rows[ids[j]]
			if ri.ReleaseYear != rj.ReleaseYear {
				return ri.ReleaseYear < rj.ReleaseYear
			}
			ki, kj := sortSubKey(ri), sortSubKey(rj)
			if ki != kj {
				return ki < kj
			}
			return ids[i] < ids[j]
		})

		next := 1
		for _, id := range ids {
			row := rows[id]
			if o, ok := ov[id]; ok && o.GenerationRank != nil {
				continue // already pinned
			}
			for claimed[g][next] {
				next++
			}
			row.GenerationRank = next
			rows[id] = row
			next++
		}
		if len(claimed[g]) > 0 {
			*warns = append(*warns, fmt.Sprintf(
				"vendor=%q arch=%q: generation_rank pinned by overrides for %d row(s); check monotonicity of neighbours",
				g.vendor, g.arch, len(claimed[g])))
		}
	}
}

func sortSubKey(r dataset.Row) int {
	if r.Model != nil {
		return *r.Model
	}
	if r.PartID != nil {
		return *r.PartID
	}
	return 0
}
