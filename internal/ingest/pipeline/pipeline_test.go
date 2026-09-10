package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fcuny.net/cpuids/internal/dataset"
)

const curatedAMD = `{"entries":[
	{"family":25,"model":1,"name":"Milan","microarch":"Zen 3","segment":"server","release_year":2021},
	{"family":25,"model":17,"name":"Genoa","microarch":"Zen 4","segment":"server","release_year":2022},
	{"family":25,"model":33,"name":"Vermeer","microarch":"Zen 3","segment":"client","release_year":2020}
]}`

const sourcesYAML = `sources:
  - id: amd
    repo: ""
    ref: ""
    path: amd_families.json
    parser: amd
    last_seen_commit: ""
`

func setup(t *testing.T) (root string, opt Options) {
	t.Helper()
	root = t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("amd_families.json", curatedAMD)
	write("sources.yaml", sourcesYAML)
	write("overrides.yaml", "overrides: {}\n")

	opt = Options{
		SourcesPath:   filepath.Join(root, "sources.yaml"),
		OverridesPath: filepath.Join(root, "overrides.yaml"),
		StatePath:     filepath.Join(root, ".ingest", "state.json"),
		DataPath:      filepath.Join(root, "data", "cpu_models.json"),
		SQLPath:       filepath.Join(root, "build", "cpu_models.sql"),
		DBPath:        filepath.Join(root, "build", "cpu_models.db"),
		RepoRoot:      root,
		Offline:       true,
		Now:           time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
	}
	return root, opt
}

func TestRunOfflineWritesValidDataset(t *testing.T) {
	_, opt := setup(t)

	res, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Wrote || res.RecordCount != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}

	b, err := os.ReadFile(opt.DataPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := dataset.Parse(b)
	if err != nil {
		t.Fatalf("output JSON does not parse: %v", err)
	}
	if err := dataset.Validate(f); err != nil {
		t.Fatalf("output JSON invalid: %v", err)
	}

	sql, err := os.ReadFile(opt.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sql), "CREATE TABLE cpu_models") || !strings.Contains(string(sql), "amd-25-17") {
		t.Error("SQL script missing expected content")
	}

	// Second run with no changes is a clean no-op.
	res2, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if res2.Wrote {
		t.Errorf("second run rewrote unchanged data: %+v", res2)
	}
}

func TestCanaryTripsOnCountDrop(t *testing.T) {
	_, opt := setup(t)
	if err := os.MkdirAll(filepath.Dir(opt.StatePath), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := State{LastCounts: map[string]int{"amd": 100}, LastCommits: map[string]string{}}
	if err := seed.Save(opt.StatePath); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), opt)
	if err == nil || !strings.Contains(err.Error(), "canary") {
		t.Fatalf("expected canary error, got %v", err)
	}
	if _, statErr := os.Stat(opt.DataPath); statErr == nil {
		t.Error("canary failure must not write output")
	}
}

func TestOfflineRejectsRemoteSource(t *testing.T) {
	root, opt := setup(t)
	remote := `sources:
  - id: intel
    repo: torvalds/linux
    ref: master
    path: arch/x86/include/asm/intel-family.h
    parser: intel
    last_seen_commit: ""
`
	if err := os.WriteFile(filepath.Join(root, "sources.yaml"), []byte(remote), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), opt); err == nil {
		t.Error("offline run should refuse a remote source")
	}
}
