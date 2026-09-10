package pipeline

import (
	"encoding/json"
	"os"
	"time"
)

// State is the small amount of run-to-run memory the pipeline keeps, checked
// into the repo. It drives the "did anything change?" and count-drop canary
// checks.
type State struct {
	LastRun     string            `json:"last_run"`
	LastCounts  map[string]int    `json:"last_counts"`  // source id -> parsed record count
	LastCommits map[string]string `json:"last_commits"` // source id -> resolved SHA
}

// LoadState reads a state file. A missing file yields a zero-valued State.
func LoadState(path string) (State, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{LastCounts: map[string]int{}, LastCommits: map[string]string{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	if s.LastCounts == nil {
		s.LastCounts = map[string]int{}
	}
	if s.LastCommits == nil {
		s.LastCommits = map[string]string{}
	}
	return s, nil
}

// Save writes the state file with a trailing newline.
func (s State) Save(path string) error {
	s.LastRun = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
