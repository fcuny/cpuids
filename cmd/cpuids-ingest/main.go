// Command cpuids-ingest runs the ingestion pipeline: fetch pinned upstream ID
// tables, parse them, normalize with overrides.yaml, validate, and write
// data/cpu_models.json plus build/cpu_models.{sql,db}.
//
// It is meant to run from the repository root, on a schedule in CI, with the
// resulting diff opened as a pull request for human review.
//
// Usage:
//
//	cpuids-ingest [flags]
//
//	-sources    path to sources.yaml         (default "sources.yaml")
//	-overrides  path to overrides.yaml        (default "overrides.yaml")
//	-state      path to pipeline state file   (default ".ingest/state.json")
//	-data       path to canonical JSON        (default "data/cpu_models.json")
//	-sql        path to SQL build output      (default "build/cpu_models.sql")
//	-db         path to SQLite build output   (default "build/cpu_models.db")
//	-snapshot   dir for fetched raw files     (default "build/snapshots")
//	-canary     count-drop floor as a fraction (default 0.5)
//
// Exit status is 0 on success (including a clean no-op), non-zero on any error
// including a tripped canary.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"fcuny.net/cpuids/internal/ingest/pipeline"
)

func main() {
	opt := pipeline.Options{}
	flag.StringVar(&opt.SourcesPath, "sources", "sources.yaml", "path to sources.yaml")
	flag.StringVar(&opt.OverridesPath, "overrides", "overrides.yaml", "path to overrides.yaml")
	flag.StringVar(&opt.StatePath, "state", ".ingest/state.json", "path to pipeline state file")
	flag.StringVar(&opt.DataPath, "data", "data/cpu_models.json", "path to canonical JSON")
	flag.StringVar(&opt.SQLPath, "sql", "build/cpu_models.sql", "path to SQL build output")
	flag.StringVar(&opt.DBPath, "db", "build/cpu_models.db", "path to SQLite build output")
	flag.StringVar(&opt.SnapshotDir, "snapshot", "build/snapshots", "dir for fetched raw files")
	flag.StringVar(&opt.RepoRoot, "root", ".", "repository root for curated in-repo sources")
	flag.Float64Var(&opt.CanaryFloor, "canary", 0.5, "count-drop floor as a fraction of the previous run")
	flag.Parse()

	opt.Now = time.Now()
	opt.GitHubToken = os.Getenv("GITHUB_TOKEN")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := pipeline.Run(ctx, opt)
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, m := range res.Messages {
		fmt.Fprintln(os.Stderr, m)
	}

	fmt.Printf("records: %d\n", res.RecordCount)
	for id, n := range res.Counts {
		fmt.Printf("  %-8s %d\n", id, n)
	}
	switch {
	case !res.Wrote:
		fmt.Println("result: no changes")
	case res.DBBuilt:
		fmt.Println("result: wrote JSON + SQL + SQLite")
	default:
		fmt.Println("result: wrote JSON + SQL (sqlite3 unavailable)")
	}

	// Signal to CI whether a PR should be opened.
	if gha := os.Getenv("GITHUB_OUTPUT"); gha != "" && res.Wrote {
		if f, e := os.OpenFile(gha, os.O_APPEND|os.O_WRONLY, 0o644); e == nil {
			fmt.Fprintln(f, "changed=true")
			f.Close()
		}
	}
}
