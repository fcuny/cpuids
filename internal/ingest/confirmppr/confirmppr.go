// Package confirmppr implements the offline logic behind the /confirm-ppr
// issue-comment workflow: parse a maintainer's confirmation comment and the
// reporter's issue body, check a cited AMD document's family/model against
// the report, and — only on a match — produce an updated amd_families.json.
//
// Nothing here talks to GitHub or the network; cmd/cpuids-confirm-ppr wires
// this package to the triggering issue_comment event, an HTTP fetch of the
// cited URL, and $GITHUB_OUTPUT.
package confirmppr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest/normalize"
	"fcuny.net/cpuids/internal/ingest/overrides"
	"fcuny.net/cpuids/internal/ingest/parse/amd"
	"gopkg.in/yaml.v3"
)

// Marker is the literal first line a triggering comment must start with.
const Marker = "/confirm-ppr"

// Command is a maintainer's /confirm-ppr comment, decoded from the YAML body
// that follows the marker line. Only URL and Name are required — everything
// else is passthrough to the new amd_families.json entry, matching
// amd.Entry's own bar (amd.Parse rejects only an empty Name).
type Command struct {
	URL         string   `yaml:"url"`
	Name        string   `yaml:"name"`
	Microarch   string   `yaml:"microarch"`
	Segment     string   `yaml:"segment"`
	ReleaseYear int      `yaml:"release_year"`
	Aliases     []string `yaml:"aliases"`
}

// ParseCommand strips the marker line and decodes the rest as YAML.
func ParseCommand(body []byte) (Command, error) {
	first, rest, ok := cutFirstLine(body)
	if !ok || first != Marker {
		return Command{}, fmt.Errorf("confirmppr: comment does not start with the %q marker", Marker)
	}
	var c Command
	if err := yaml.Unmarshal(rest, &c); err != nil {
		return Command{}, fmt.Errorf("confirmppr: decode command: %w", err)
	}
	if c.URL == "" {
		return Command{}, errors.New(`confirmppr: command missing required field "url"`)
	}
	if c.Name == "" {
		return Command{}, errors.New(`confirmppr: command missing required field "name"`)
	}
	return c, nil
}

func cutFirstLine(b []byte) (first string, rest []byte, ok bool) {
	before, after, found := bytes.Cut(b, []byte("\n"))
	if !found {
		trimmed := strings.TrimSpace(string(b))
		return trimmed, nil, trimmed != ""
	}
	return strings.TrimSpace(string(before)), after, true
}

var headingRe = regexp.MustCompile(`(?m)^### (.+)$`)

// ParseIssueReport extracts the "/proc/cpuinfo" field from a rendered
// unknown-cpu.yml issue-form body and parses it into key/value pairs — the
// same shape produced by firstBlock() in linuxcpuinfo.go and site/app.js.
//
// Issue forms render each field as "### <Label>\n\n<value>\n\n", with a
// render:text textarea fenced as ```text ... ```. Confirmed against a real
// rendered body via `gh issue view 8 --json body` (see testdata).
func ParseIssueReport(body []byte) (map[string]string, error) {
	block, err := issueField(body, "/proc/cpuinfo")
	if err != nil {
		return nil, err
	}
	fields := parseCPUInfoBlock(stripFence(block))
	if len(fields) == 0 {
		return nil, errors.New("confirmppr: issue's /proc/cpuinfo field is empty or unparseable")
	}
	return fields, nil
}

// issueField returns the raw text between a "### <label>" heading and the
// next heading (or end of body).
func issueField(body []byte, label string) ([]byte, error) {
	locs := headingRe.FindAllSubmatchIndex(body, -1)
	for i, loc := range locs {
		heading := strings.TrimSpace(string(body[loc[2]:loc[3]]))
		if heading != label {
			continue
		}
		start := loc[1]
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return bytes.TrimSpace(body[start:end]), nil
	}
	return nil, fmt.Errorf("confirmppr: issue body has no %q field", label)
}

var fenceRe = regexp.MustCompile("(?s)^```[a-zA-Z]*\n(.*?)\n?```$")

// stripFence removes a fenced-code-block wrapper, if present.
func stripFence(b []byte) []byte {
	if m := fenceRe.FindSubmatch(b); m != nil {
		return m[1]
	}
	return b
}

// parseCPUInfoBlock mirrors firstBlock in linuxcpuinfo.go / site/app.js:
// "key\t: value" lines, lowercased trimmed keys, trimmed values.
func parseCPUInfoBlock(b []byte) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

// ReportedFamilyModel reads parsed /proc/cpuinfo fields and returns the x86
// family/model. It errors clearly for shapes this automation does not
// support yet: ARM reports (no writable landing spot — see issue #5), and
// x86 reports from a vendor other than AMD (amd_families.json is
// AMD-specific).
func ReportedFamilyModel(fields map[string]string) (family, model int, err error) {
	if _, isARM := fields["cpu implementer"]; isARM {
		return 0, 0, errors.New("confirmppr: reported CPU is ARM; this automation only supports AMD reports today")
	}
	vendor := fields["vendor_id"]
	if vendor != amd.VendorString {
		return 0, 0, fmt.Errorf("confirmppr: reported vendor %q is not AMD; this automation only supports AMD reports today", vendor)
	}
	famStr, ok1 := fields["cpu family"]
	modStr, ok2 := fields["model"]
	if !ok1 || !ok2 {
		return 0, 0, errors.New(`confirmppr: reported CPU is missing "cpu family" or "model"`)
	}
	fam, err1 := strconv.Atoi(strings.TrimSpace(famStr))
	mod, err2 := strconv.Atoi(strings.TrimSpace(modStr))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("confirmppr: cpu family/model did not parse as integers (%q/%q)", famStr, modStr)
	}
	return fam, mod, nil
}

var familyModelRe = regexp.MustCompile(`(?i)Family\s+([0-9A-Fa-f]+)h\s+Model\s+([0-9A-Fa-f]+)h`)

// ExtractFamilyModel finds AMD's "Family <hex>h Model <hex>h" identifier —
// the string AMD stamps on every PPR page footer and its docs.amd.com
// catalog page title — in arbitrary fetched text, and returns it in decimal.
func ExtractFamilyModel(pageText []byte) (family, model int, ok bool) {
	m := familyModelRe.FindSubmatch(pageText)
	if m == nil {
		return 0, 0, false
	}
	fam, err1 := strconv.ParseInt(string(m[1]), 16, 32)
	mod, err2 := strconv.ParseInt(string(m[2]), 16, 32)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return int(fam), int(mod), true
}

// Entry is a new amd_families.json row, ready to be inserted verbatim.
type Entry struct {
	Family      int
	Model       int
	Name        string
	Microarch   string
	Segment     string
	ReleaseYear int
	Aliases     []string
}

// RowID returns the dataset row id normalize.Build would assign this entry,
// e.g. "amd-26-2".
func RowID(family, model int) string {
	return fmt.Sprintf("amd-%d-%d", family, model)
}

// InsertEntry appends e as a new single-line entry to an amd_families.json
// document, immediately before the closing "]" of the entries array. It
// edits the file as text rather than round-tripping through encoding/json,
// so every existing row's hand-aligned formatting is left untouched and the
// diff is exactly one added line — matching how this file is edited by
// hand today.
func InsertEntry(current []byte, e Entry) ([]byte, error) {
	lines := strings.Split(string(current), "\n")

	closeIdx := -1
	for i, line := range slices.Backward(lines) {
		if strings.TrimSpace(line) == "]" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 1 {
		return nil, errors.New(`confirmppr: could not find the entries array's closing "]"`)
	}

	lastEntryIdx := -1
	for i, line := range slices.Backward(lines[:closeIdx]) {
		if strings.TrimSpace(line) != "" {
			lastEntryIdx = i
			break
		}
	}
	if lastEntryIdx < 0 || !strings.Contains(lines[lastEntryIdx], "{") {
		return nil, errors.New("confirmppr: could not find the entries array's last entry")
	}

	last := strings.TrimRight(lines[lastEntryIdx], " \t")
	if !strings.HasSuffix(last, ",") {
		last += ","
	}
	lines[lastEntryIdx] = last

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:lastEntryIdx+1]...)
	newLines = append(newLines, formatEntry(e))
	newLines = append(newLines, lines[lastEntryIdx+1:]...)
	return []byte(strings.Join(newLines, "\n")), nil
}

// formatEntry renders e in amd_families.json's existing single-line style.
func formatEntry(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, `    { "family": %d, "model": %d, "name": %s`, e.Family, e.Model, quote(e.Name))
	if e.Microarch != "" {
		fmt.Fprintf(&b, `, "microarch": %s`, quote(e.Microarch))
	}
	if e.Segment != "" {
		fmt.Fprintf(&b, `, "segment": %s`, quote(e.Segment))
	}
	if e.ReleaseYear != 0 {
		fmt.Fprintf(&b, `, "release_year": %d`, e.ReleaseYear)
	}
	if len(e.Aliases) > 0 {
		b.WriteString(`, "aliases": [`)
		for i, a := range e.Aliases {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quote(a))
		}
		b.WriteString(`]`)
	}
	b.WriteString(" }")
	return b.String()
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// AlreadyPresent reports whether current already has an entry for
// (family, model), so a duplicate report — or a re-run of this automation —
// gets a clean answer instead of amd.Parse's raw duplicate-key error.
func AlreadyPresent(current []byte, family, model int) (name string, ok bool) {
	recs, err := amd.Parse(current, dataset.Provenance{})
	if err != nil {
		return "", false
	}
	for _, r := range recs {
		if r.Family != nil && r.Model != nil && *r.Family == family && *r.Model == model {
			return r.Name, true
		}
	}
	return "", false
}

// Validate runs the same offline checks the ingestion pipeline runs for the
// AMD source, in isolation: parse candidate as amd_families.json, normalize
// it (with overrides.yaml applied, as the real pipeline does), and run
// dataset.Validate. Because AuthenticAMD is the only vendor amd.Parse ever
// produces, this is authoritative for the AMD group's generation_rank, not
// an approximation of a full pipeline run.
func Validate(candidate []byte, overridesPath string) (dataset.File, error) {
	recs, err := amd.Parse(candidate, dataset.Provenance{
		SourcePath: "internal/ingest/parse/amd/amd_families.json",
	})
	if err != nil {
		return dataset.File{}, fmt.Errorf("confirmppr: candidate amd_families.json does not parse: %w", err)
	}
	ov, err := overrides.Load(overridesPath)
	if err != nil {
		return dataset.File{}, fmt.Errorf("confirmppr: load overrides: %w", err)
	}
	res, err := normalize.Build(recs, ov, time.Now(), nil)
	if err != nil {
		return dataset.File{}, fmt.Errorf("confirmppr: normalize: %w", err)
	}
	if err := dataset.Validate(res.File); err != nil {
		return dataset.File{}, fmt.Errorf("confirmppr: validate: %w", err)
	}
	return res.File, nil
}
