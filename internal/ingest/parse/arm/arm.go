// Package arm parses the util-linux table sys-utils/lscpu-arm.c.
//
// That file declares, per implementer, an array of {part id, "name"} pairs:
//
//	static const struct id_part arm_part[] = {
//		{ 0xd03, "Cortex-A53" },
//		{ 0xd4f, "Neoverse-V2" },
//		{ -1, "unknown" },
//	};
//
// and a table joining implementer id -> part array + implementer display name:
//
//	static const struct hw_impl hw_implementer[] = {
//		{ 0x41, arm_part, "ARM" },
//		{ 0x4e, nvidia_part, "NVIDIA" },
//	};
//
// The parser extracts the (implementer id, part id) -> (part name, implementer
// name) facts and nothing else. Sentinel { -1, ... } rows are dropped.
//
// The ARM part names should be cross-checked against the official
// github.com/arm-software/data catalogue; that cross-check is a review step,
// not something this parser performs.
package arm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
)

var (
	partArrayRE = regexp.MustCompile(`(?s)struct\s+id_part\s+([A-Za-z0-9_]+)\s*\[\]\s*=\s*\{(.*?)\}\s*;`)
	partEntryRE = regexp.MustCompile(`\{\s*(-?0[xX][0-9A-Fa-f]+|-?\d+)\s*,\s*"([^"]*)"\s*\}`)
	implTableRE = regexp.MustCompile(`(?s)struct\s+hw_impl\s+hw_implementer\s*\[\]\s*=\s*\{(.*?)\}\s*;`)
	implEntryRE = regexp.MustCompile(`\{\s*(0[xX][0-9A-Fa-f]+|\d+)\s*,\s*([A-Za-z0-9_]+)\s*,\s*"([^"]*)"\s*\}`)
)

// ImplementerNames maps a handful of raw implementer display strings from the
// source to the more descriptive form used elsewhere. It is intentionally
// small; anything not listed is stored as-is.
var ImplementerNames = map[string]string{
	"ARM": "ARM Ltd",
}

// Parse reads lscpu-arm.c content and returns one Record per resolvable
// (implementer, part) pair. Part names are taken verbatim, so Confidence is
// "exact".
func Parse(content []byte, prov dataset.Provenance) ([]ingest.Record, error) {
	prov.Confidence = dataset.ConfidenceExact
	src := string(content)

	// 1. Collect every part array by its C identifier.
	partArrays := map[string][]partEntry{}
	for _, m := range partArrayRE.FindAllStringSubmatch(src, -1) {
		name, body := m[1], m[2]
		var entries []partEntry
		for _, e := range partEntryRE.FindAllStringSubmatch(body, -1) {
			id, err := parseSigned(e[1])
			if err != nil || id < 0 {
				continue // sentinel or garbage
			}
			entries = append(entries, partEntry{id: id, name: strings.TrimSpace(e[2])})
		}
		partArrays[name] = entries
	}

	// 2. Walk the implementer table and join.
	tm := implTableRE.FindStringSubmatch(src)
	if tm == nil {
		return nil, fmt.Errorf("arm: hw_implementer table not found")
	}

	var out []ingest.Record
	for _, e := range implEntryRE.FindAllStringSubmatch(tm[1], -1) {
		implID, err := parseSigned(e[1])
		if err != nil || implID < 0 || implID > 0xFF {
			continue
		}
		arrayName := e[2]
		implName := strings.TrimSpace(e[3])
		if mapped, ok := ImplementerNames[implName]; ok {
			implName = mapped
		}
		for _, pe := range partArrays[arrayName] {
			if pe.id > 0xFFF {
				continue
			}
			impl := implID
			part := pe.id
			out = append(out, ingest.Record{
				Arch:          dataset.ArchARM,
				Vendor:        implName,
				ImplementerID: &impl,
				PartID:        &part,
				Name:          pe.name,
				Segment:       dataset.SegmentUnknown,
				Confidence:    dataset.ConfidenceExact,
				Provenance:    prov,
			})
		}
	}
	return out, nil
}

type partEntry struct {
	id   int
	name string
}

func parseSigned(s string) (int, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var v int64
	var err error
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err = strconv.ParseInt(s[2:], 16, 64)
	} else {
		v, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		return 0, err
	}
	if neg {
		v = -v
	}
	return int(v), nil
}
