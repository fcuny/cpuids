// Package intel parses the Linux kernel header
// arch/x86/include/asm/intel-family.h.
//
// The header is a flat list of family-6 model numbers:
//
//	#define INTEL_FAM6_SAPPHIRERAPIDS_X	0x8F
//
// The parser extracts only the (model number, macro token) fact pairs. The
// macro token is the vendor's own short codename; humanization into a display
// name happens in the normalizer. Comments and any prose are discarded.
//
// A trailing segment suffix on the token is a well-known kernel convention and
// is mapped to a segment hint:
//
//	_X, _D            -> server
//	_L, _H, _N, _S    -> client
//	(none)            -> unknown
package intel

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
)

// VendorString is the raw CPUID vendor string stored on every Intel row.
const VendorString = "GenuineIntel"

var defineRE = regexp.MustCompile(`^\s*#define\s+INTEL_FAM6_([A-Z0-9_]+)\s+(0[xX][0-9A-Fa-f]+|\d+)\b`)

// Parse reads intel-family.h content and returns one Record per #define. prov
// is copied onto every Record (the caller sets source_repo/path/commit and
// ingested_at); Confidence is forced to "inferred" because the display name is
// derived from the macro token rather than taken verbatim.
func Parse(content []byte, prov dataset.Provenance) ([]ingest.Record, error) {
	prov.Confidence = dataset.ConfidenceInferred

	var out []ingest.Record
	seen := make(map[int]bool)

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		m := defineRE.FindSubmatch(sc.Bytes())
		if m == nil {
			continue
		}
		token := string(m[1])
		model, err := parseInt(string(m[2]))
		if err != nil {
			continue
		}
		// Kernel defines a few non-model helper macros (e.g. ANY); those do
		// not have a plausible model byte. Guard on range.
		if model < 0 || model > 0xFF {
			continue
		}
		if seen[model] {
			continue // first definition wins; later ones are usually aliases
		}
		seen[model] = true

		fam := 6
		mod := model
		out = append(out, ingest.Record{
			Arch:       dataset.ArchX86,
			Vendor:     VendorString,
			Family:     &fam,
			Model:      &mod,
			Name:       token, // raw fact; normalizer humanizes
			Segment:    segmentFromToken(token),
			Confidence: dataset.ConfidenceInferred,
			Provenance: prov,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func segmentFromToken(token string) string {
	switch {
	case strings.HasSuffix(token, "_X"), strings.HasSuffix(token, "_D"):
		return dataset.SegmentServer
	case strings.HasSuffix(token, "_L"), strings.HasSuffix(token, "_H"),
		strings.HasSuffix(token, "_N"), strings.HasSuffix(token, "_S"):
		return dataset.SegmentClient
	default:
		return dataset.SegmentUnknown
	}
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseInt(s[2:], 16, 64)
		return int(v), err
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return int(v), err
}
