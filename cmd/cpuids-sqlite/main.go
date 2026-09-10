// Command cpuids-sqlite builds the SQLite artifact from the committed
// data/cpu_models.json without re-running ingestion. Used by the release
// workflow and available locally as `just sqlite`.
//
// Usage:
//
//	cpuids-sqlite [-data data/cpu_models.json] [-sql build/cpu_models.sql] [-db build/cpu_models.db]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"fcuny.net/cpuids/internal/dataset"
	"fcuny.net/cpuids/internal/ingest/sqlite"
)

func main() {
	dataPath := flag.String("data", "data/cpu_models.json", "path to canonical JSON")
	sqlPath := flag.String("sql", "build/cpu_models.sql", "path to SQL output")
	dbPath := flag.String("db", "build/cpu_models.db", "path to SQLite output")
	flag.Parse()

	b, err := os.ReadFile(*dataPath)
	if err != nil {
		fail(err)
	}
	f, err := dataset.Parse(b)
	if err != nil {
		fail(err)
	}
	if err := dataset.Validate(f); err != nil {
		fail(fmt.Errorf("validate: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(*sqlPath), 0o755); err != nil {
		fail(err)
	}
	builtDB, err := sqlite.Build(f, *sqlPath, *dbPath)
	if err != nil {
		fail(err)
	}
	if builtDB {
		fmt.Printf("wrote %s and %s (%d models)\n", *sqlPath, *dbPath, len(f.Models))
	} else {
		fmt.Printf("wrote %s (%d models); sqlite3 not on PATH, skipped %s\n", *sqlPath, len(f.Models), *dbPath)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
