// Package pipeline wires the ingestion steps together: fetch pinned sources,
// run per-vendor parsers, normalize with overrides, validate (schema + canary),
// diff against the committed snapshot, and write data/cpu_models.json plus the
// SQLite build output.
package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest"
	"fcuny.net/cpuids/internal/ingest/normalize"
	"fcuny.net/cpuids/internal/ingest/overrides"
	amdparse "fcuny.net/cpuids/internal/ingest/parse/amd"
	armparse "fcuny.net/cpuids/internal/ingest/parse/arm"
	intelparse "fcuny.net/cpuids/internal/ingest/parse/intel"
	"fcuny.net/cpuids/internal/ingest/sources"
	"fcuny.net/cpuids/internal/ingest/sqlite"
)

// Options configures a run.
type Options struct {
	SourcesPath   string // sources.yaml
	OverridesPath string // overrides.yaml
	StatePath     string // .ingest/state.json
	DataPath      string // data/cpu_models.json (canonical, checked in)
	SQLPath       string // build/cpu_models.sql
	DBPath        string // build/cpu_models.db
	RepoRoot      string // base dir for curated in-repo sources
	SnapshotDir   string // where fetched raw files are written (audit trail; git-ignored)

	Offline     bool    // skip network; read curated sources only, fail on remote sources
	CanaryFloor float64 // fraction of previous count below which a source is treated as parser breakage (default 0.5)
	Now         time.Time
	GitHubToken string
}

// Result summarizes what a run did.
type Result struct {
	Changed     bool // canonical JSON content changed
	Wrote       bool // files were written (false in dry-run style no-op)
	DBBuilt     bool
	RecordCount int
	Counts      map[string]int
	Warnings    []string
	Messages    []string
}

// Parser is the signature every per-vendor parser satisfies.
type Parser func(content []byte, prov dataset.Provenance) ([]ingest.Record, error)

var parsers = map[string]Parser{
	"intel": intelparse.Parse,
	"arm":   armparse.Parse,
	"amd":   amdparse.Parse,
}

// Run executes the pipeline. It never writes partial output: validation must
// pass before anything is written.
func Run(ctx context.Context, opt Options) (Result, error) {
	if opt.CanaryFloor <= 0 {
		opt.CanaryFloor = 0.5
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}

	res := Result{Counts: map[string]int{}}

	cfg, err := sources.Load(opt.SourcesPath)
	if err != nil {
		return res, fmt.Errorf("load sources: %w", err)
	}
	ov, err := overrides.Load(opt.OverridesPath)
	if err != nil {
		return res, fmt.Errorf("load overrides: %w", err)
	}
	state, err := LoadState(opt.StatePath)
	if err != nil {
		return res, fmt.Errorf("load state: %w", err)
	}

	// Fetch.
	client := &sources.Client{Token: opt.GitHubToken}
	var fetched []sources.Fetched
	if opt.Offline {
		for _, s := range cfg.Sources {
			if s.Repo != "" {
				return res, fmt.Errorf("offline: remote source %q cannot be fetched", s.ID)
			}
			b, rerr := os.ReadFile(filepath.Join(opt.RepoRoot, s.Path))
			if rerr != nil {
				return res, fmt.Errorf("offline: read %q: %w", s.ID, rerr)
			}
			fetched = append(fetched, sources.Fetched{Source: s, Content: b, Changed: true})
		}
	} else {
		fetched, err = client.FetchAll(ctx, cfg, opt.RepoRoot)
		if err != nil {
			return res, fmt.Errorf("fetch: %w", err)
		}
	}

	if opt.SnapshotDir != "" {
		if err := os.MkdirAll(opt.SnapshotDir, 0o755); err != nil {
			return res, err
		}
	}

	// Parse.
	var all []ingest.Record
	anyChanged := false
	var manifestSources []dataset.ManifestSource
	for _, f := range fetched {
		if f.Changed {
			anyChanged = true
		}
		manifestSources = append(manifestSources, dataset.ManifestSource{Repo: f.Source.Repo, Commit: f.Commit})

		if opt.SnapshotDir != "" {
			_ = os.WriteFile(filepath.Join(opt.SnapshotDir, f.Source.ID+snapshotExt(f.Source.Path)), f.Content, 0o644)
		}

		parse, ok := parsers[f.Source.Parser]
		if !ok {
			return res, fmt.Errorf("source %q: no parser %q", f.Source.ID, f.Source.Parser)
		}
		prov := dataset.Provenance{
			SourceRepo:   f.Source.Repo,
			SourcePath:   f.Source.Path,
			SourceCommit: f.Commit,
			IngestedAt:   opt.Now.UTC().Format(time.RFC3339),
		}
		recs, perr := parse(f.Content, prov)
		if perr != nil {
			return res, fmt.Errorf("parse %q: %w", f.Source.ID, perr)
		}
		res.Counts[f.Source.ID] = len(recs)

		// Absolute floor: a source producing nothing is always parser
		// breakage — none of the configured sources is legitimately empty.
		if len(recs) == 0 {
			return res, fmt.Errorf("canary: source %q parsed 0 records; treating as parser breakage", f.Source.ID)
		}

		// Canary: a sharp drop vs last successful run means the parser broke.
		if prev, ok := state.LastCounts[f.Source.ID]; ok && prev > 0 {
			if float64(len(recs)) < float64(prev)*opt.CanaryFloor {
				return res, fmt.Errorf("canary: source %q parsed %d records, down from %d (floor %.0f%%); treating as parser breakage",
					f.Source.ID, len(recs), prev, opt.CanaryFloor*100)
			}
		}
		all = append(all, recs...)
	}

	// Normalize.
	built, err := normalize.Build(all, ov, opt.Now, manifestSources)
	if err != nil {
		return res, fmt.Errorf("normalize: %w", err)
	}
	res.Warnings = built.Warnings
	res.RecordCount = len(built.File.Models)

	// Validate (schema-level).
	if err := dataset.Validate(built.File); err != nil {
		return res, fmt.Errorf("validate: %w", err)
	}

	// Marshal and diff against the committed file.
	newJSON, err := dataset.Marshal(built.File)
	if err != nil {
		return res, err
	}
	oldJSON, _ := os.ReadFile(opt.DataPath)
	res.Changed = !bytes.Equal(normalizeManifestNoise(oldJSON), normalizeManifestNoise(newJSON))

	if !anyChanged {
		res.Messages = append(res.Messages, "no source commit changed since last run")
	}
	if !res.Changed {
		res.Messages = append(res.Messages, "canonical JSON is byte-identical (ignoring timestamps); nothing to write")
		return res, nil
	}

	// Write canonical JSON + SQLite.
	if err := os.MkdirAll(filepath.Dir(opt.DataPath), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(opt.DataPath, newJSON, 0o644); err != nil {
		return res, err
	}
	res.Wrote = true

	if opt.SQLPath != "" {
		if err := os.MkdirAll(filepath.Dir(opt.SQLPath), 0o755); err != nil {
			return res, err
		}
		builtDB, err := sqlite.Build(built.File, opt.SQLPath, opt.DBPath)
		if err != nil {
			return res, fmt.Errorf("sqlite build: %w", err)
		}
		res.DBBuilt = builtDB
		if !builtDB {
			res.Warnings = append(res.Warnings, "sqlite3 not found on PATH; wrote SQL script only")
		}
	}

	// Persist state.
	if err := os.MkdirAll(filepath.Dir(opt.StatePath), 0o755); err != nil {
		return res, err
	}
	for id, n := range res.Counts {
		state.LastCounts[id] = n
	}
	for _, f := range fetched {
		if f.Commit != "" {
			state.LastCommits[f.Source.ID] = f.Commit
		}
	}
	if err := state.Save(opt.StatePath); err != nil {
		return res, fmt.Errorf("save state: %w", err)
	}

	return res, nil
}

func snapshotExt(path string) string {
	if e := filepath.Ext(path); e != "" {
		return e
	}
	return ".txt"
}

// normalizeManifestNoise blanks the generated_at line so a rerun with no data
// change is not reported as a diff purely because time passed.
func normalizeManifestNoise(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	for _, line := range bytes.Split(b, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte(`"generated_at"`)) ||
			bytes.HasPrefix(trimmed, []byte(`"ingested_at"`)) {
			continue
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}
