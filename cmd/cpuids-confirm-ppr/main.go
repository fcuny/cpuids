// Command cpuids-confirm-ppr implements the /confirm-ppr issue-comment
// automation: given a maintainer's confirmation comment and the reporting
// issue's body, fetch the cited document, check its family/model against
// the report, and on a match, append a new amd_families.json entry.
//
// It is meant to run from .github/workflows/confirm-ppr.yml, triggered by an
// issue_comment event. It never merges anything — the workflow opens a PR
// from whatever this command writes, for a human to review.
//
// Usage:
//
//	cpuids-confirm-ppr [flags]
//
//	-comment-file     path to the triggering comment body        (required)
//	-issue-body-file  path to the issue's body                   (required)
//	-amd-families     path to amd_families.json  (default "internal/ingest/parse/amd/amd_families.json")
//	-overrides        path to overrides.yaml     (default "overrides.yaml")
//
// Outputs (written to $GITHUB_OUTPUT when set):
//
//	status  added | already_present | mismatch | unsupported | fetch_error | command_error
//	family  reported family, when known
//	model   reported model, when known
//	row_id  e.g. "amd-26-2", when known
//	name    the confirmed product name, when status is added or already_present
//
// Exit status is non-zero only for infra failures (bad flags, unreadable
// files, a write failure). A mismatched, unsupported, or unparseable report
// is a normal "status" output, not a crash — the workflow needs to tell
// those apart to reply usefully on the issue either way.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"fcuny.net/cpuids/internal/ingest/confirmppr"
)

func main() {
	commentFile := flag.String("comment-file", "", "path to the triggering comment body")
	issueBodyFile := flag.String("issue-body-file", "", "path to the issue's body")
	amdFamiliesPath := flag.String("amd-families", "internal/ingest/parse/amd/amd_families.json", "path to amd_families.json")
	overridesPath := flag.String("overrides", "overrides.yaml", "path to overrides.yaml")
	flag.Parse()

	if *commentFile == "" || *issueBodyFile == "" {
		fmt.Fprintln(os.Stderr, "error: -comment-file and -issue-body-file are required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	commentBody, err := os.ReadFile(*commentFile)
	if err != nil {
		fail(err)
	}
	issueBody, err := os.ReadFile(*issueBodyFile)
	if err != nil {
		fail(err)
	}

	cmd, err := confirmppr.ParseCommand(commentBody)
	if err != nil {
		fmt.Println(err)
		report("command_error", nil)
		return
	}

	fields, err := confirmppr.ParseIssueReport(issueBody)
	if err != nil {
		fmt.Println(err)
		report("command_error", nil)
		return
	}

	reportedFamily, reportedModel, err := confirmppr.ReportedFamilyModel(fields)
	if err != nil {
		fmt.Println(err)
		report("unsupported", nil)
		return
	}

	pageText, err := fetch(ctx, cmd.URL)
	if err != nil {
		fmt.Println("fetch error:", err)
		report("fetch_error", nil)
		return
	}

	fetchedFamily, fetchedModel, ok := confirmppr.ExtractFamilyModel(pageText)
	if !ok {
		fmt.Printf("could not find a \"Family <hex>h Model <hex>h\" identifier at %s\n", cmd.URL)
		report("fetch_error", nil)
		return
	}

	if fetchedFamily != reportedFamily || fetchedModel != reportedModel {
		fmt.Printf("mismatch: issue reports family=%d model=%d, but %s confirms family=%d model=%d\n",
			reportedFamily, reportedModel, cmd.URL, fetchedFamily, fetchedModel)
		report("mismatch", map[string]string{
			"family": strconv.Itoa(reportedFamily), "model": strconv.Itoa(reportedModel),
		})
		return
	}

	current, err := os.ReadFile(*amdFamiliesPath)
	if err != nil {
		fail(err)
	}

	rowID := confirmppr.RowID(reportedFamily, reportedModel)
	if name, ok := confirmppr.AlreadyPresent(current, reportedFamily, reportedModel); ok {
		fmt.Printf("%s is already present as %q\n", rowID, name)
		report("already_present", map[string]string{
			"family": strconv.Itoa(reportedFamily), "model": strconv.Itoa(reportedModel),
			"row_id": rowID, "name": name,
		})
		return
	}

	candidate, err := confirmppr.InsertEntry(current, confirmppr.Entry{
		Family: reportedFamily, Model: reportedModel, Name: cmd.Name,
		Microarch: cmd.Microarch, Segment: cmd.Segment, ReleaseYear: cmd.ReleaseYear,
		Aliases: cmd.Aliases,
	})
	if err != nil {
		fail(err)
	}

	if _, err := confirmppr.Validate(candidate, *overridesPath); err != nil {
		fmt.Println("validation error:", err)
		report("command_error", nil)
		fail(err)
	}

	if err := os.WriteFile(*amdFamiliesPath, candidate, 0o644); err != nil {
		fail(err)
	}

	fmt.Printf("added %s (%s), confirmed against %s\n", rowID, cmd.Name, cmd.URL)
	report("added", map[string]string{
		"family": strconv.Itoa(reportedFamily), "model": strconv.Itoa(reportedModel),
		"row_id": rowID, "name": cmd.Name,
	})
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}

// report writes status (plus any extra key/value pairs) to $GITHUB_OUTPUT,
// mirroring cpuids-ingest's existing changed=true pattern. A no-op outside
// Actions, where GITHUB_OUTPUT is unset.
func report(status string, extra map[string]string) {
	gha := os.Getenv("GITHUB_OUTPUT")
	if gha == "" {
		return
	}
	f, err := os.OpenFile(gha, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "status=%s\n", status)
	for k, v := range extra {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
