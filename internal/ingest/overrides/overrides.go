// Package overrides loads overrides.yaml — hand-maintained corrections merged
// on top of parsed data, keyed by row id. It is how a parser mistake gets
// fixed without fighting the automation on every re-ingestion.
//
// Every field is a pointer so "absent" and "set to zero value" are
// distinguishable: only fields actually present in the YAML are applied.
package overrides

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Override is the set of correctable fields for one row.
type Override struct {
	Name              *string  `yaml:"name"`
	MicroarchCodename *string  `yaml:"microarch_codename"`
	Segment           *string  `yaml:"segment"`
	GenerationRank    *int     `yaml:"generation_rank"`
	ReleaseYear       *int     `yaml:"release_year"`
	Aliases           []string `yaml:"aliases"`
	Notes             *string  `yaml:"notes"`
}

// Set maps row id -> Override.
type Set map[string]Override

type file struct {
	Overrides map[string]Override `yaml:"overrides"`
}

// Load reads and parses an overrides.yaml file. A missing file is not an error:
// it returns an empty Set.
func Load(path string) (Set, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Set{}, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes overrides.yaml bytes.
func Parse(b []byte) (Set, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("overrides: %w", err)
	}
	out := make(Set, len(f.Overrides))
	for id, o := range f.Overrides {
		if o.Segment != nil {
			switch *o.Segment {
			case "server", "client", "embedded", "unknown":
			default:
				return nil, fmt.Errorf("overrides: %s: invalid segment %q", id, *o.Segment)
			}
		}
		out[id] = o
	}
	return out, nil
}
