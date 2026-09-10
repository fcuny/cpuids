// Package intel parses the Linux kernel header
// arch/x86/include/asm/intel-family.h.
//
// The header lists x86 model numbers, one per #define, in two forms:
//
//	#define INTEL_SAPPHIRERAPIDS_X		IFM(6, 0x8F)   // current
//	#define INTEL_FAM6_SKYLAKE_X		0x55           // legacy
//
// IFM(fam, model) is the kernel's family/model constructor. Either form may
// carry a trailing segment suffix on the token; the legacy form always implies
// family 6.
//
// The parser extracts only the (family, model, macro token) fact pairs. The
// macro token is the vendor's own short codename; turning it into a display
// name happens in the normalizer. Comments and prose are discarded.
//
// Segment suffix heuristic:
//
//	_X, _D                          -> server
//	_L, _H, _N, _S, _P, _M, _G,
//	_U, _Y                          -> client
//	(none / unknown suffix)         -> unknown
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

var (
	// #define INTEL_<TOKEN>  IFM(<fam>, <model>)   — <fam> may be non-numeric
	// (X86_FAMILY_ANY), in which case the entry is skipped.
	ifmRE = regexp.MustCompile(`^\s*#define\s+INTEL_(?:FAM6_)?([A-Z0-9_]+)\s+IFM\(\s*([0-9A-Za-zx_]+)\s*,\s*(0[xX][0-9A-Fa-f]+|\d+)\s*\)`)

	// #define INTEL_FAM6_<TOKEN>  0x<hex>|<dec>   — family 6 implied.
	legacyRE = regexp.MustCompile(`^\s*#define\s+INTEL_FAM6_([A-Z0-9_]+)\s+(0[xX][0-9A-Fa-f]+|\d+)\b`)
)

// Parse reads intel-family.h content and returns one Record per model #define.
// prov is copied onto every Record; Confidence is "inferred" because the
// display name is derived from the macro token rather than taken verbatim.
func Parse(content []byte, prov dataset.Provenance) ([]ingest.Record, error) {
	prov.Confidence = dataset.ConfidenceInferred

	var out []ingest.Record
	type fm struct{ fam, model int }
	seen := make(map[fm]bool)

	add := func(token string, fam, model int) {
		if fam <= 0 || fam > 0xFF || model < 0 || model > 0xFF {
			return
		}
		if seen[fm{fam, model}] {
			return // first definition wins; later ones are usually aliases
		}
		seen[fm{fam, model}] = true
		f, m := fam, model
		out = append(out, ingest.Record{
			Arch:       dataset.ArchX86,
			Vendor:     VendorString,
			Family:     &f,
			Model:      &m,
			Name:       token, // raw fact; normalizer humanizes
			Segment:    segmentFromToken(token),
			Confidence: dataset.ConfidenceInferred,
			Provenance: prov,
		})
	}

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()

		if m := ifmRE.FindSubmatch(line); m != nil {
			fam, err := parseInt(string(m[2]))
			if err != nil {
				continue // non-numeric family (X86_FAMILY_ANY): skip
			}
			model, err := parseInt(string(m[3]))
			if err != nil {
				continue
			}
			add(string(m[1]), fam, model)
			continue
		}

		if m := legacyRE.FindSubmatch(line); m != nil {
			model, err := parseInt(string(m[2]))
			if err != nil {
				continue
			}
			add(string(m[1]), 6, model)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

var (
	serverSuffixes = []string{"_X", "_D"}
	clientSuffixes = []string{"_L", "_H", "_N", "_S", "_P", "_M", "_G", "_U", "_Y"}
)

func segmentFromToken(token string) string {
	for _, s := range serverSuffixes {
		if strings.HasSuffix(token, s) {
			return dataset.SegmentServer
		}
	}
	for _, s := range clientSuffixes {
		if strings.HasSuffix(token, s) {
			return dataset.SegmentClient
		}
	}
	return dataset.SegmentUnknown
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
