// Package sources loads sources.yaml and fetches each pinned upstream file by
// raw content (no clone), resolving the commit SHA it was read at.
package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Source is one pinned upstream file.
type Source struct {
	ID     string `yaml:"id"`     // matches a parser: "intel" | "arm" | "amd"
	Repo   string `yaml:"repo"`   // "owner/name"; empty for in-repo curated sources
	Ref    string `yaml:"ref"`    // branch or tag to pin to
	Path   string `yaml:"path"`   // path within the repo, or within this repo for curated sources
	Parser string `yaml:"parser"` // parser id; defaults to ID

	// LastSeenCommit is the SHA recorded on the previous successful run. The
	// pipeline compares the freshly resolved SHA against it to decide whether
	// anything changed.
	LastSeenCommit string `yaml:"last_seen_commit"`

	// CrossCheck is an informational URL for reviewers (e.g. the official ARM
	// data catalogue); the pipeline does not fetch it.
	CrossCheck string `yaml:"cross_check,omitempty"`
}

// Config is the whole sources.yaml.
type Config struct {
	Sources []Source `yaml:"sources"`
}

// Fetched is the result of retrieving one source.
type Fetched struct {
	Source  Source
	Content []byte
	Commit  string // resolved SHA, or "" when it could not be resolved
	Changed bool   // Commit != Source.LastSeenCommit (true when either is unknown)
}

// Load reads sources.yaml from disk.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("sources: %w", err)
	}
	for i := range c.Sources {
		if c.Sources[i].Parser == "" {
			c.Sources[i].Parser = c.Sources[i].ID
		}
	}
	return c, nil
}

// Client fetches sources. The zero value is usable.
type Client struct {
	HTTP  *http.Client
	Token string // GitHub token for higher rate limits; optional
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// FetchAll retrieves every source with a non-empty Repo over the network. A
// source with an empty Repo is "curated in-repo": its Content is read from
// localDir/Path and its Commit is left empty (the pipeline treats curated
// sources as always eligible; their real change signal is the repo's own git
// history).
func (c *Client) FetchAll(ctx context.Context, cfg Config, localDir string) ([]Fetched, error) {
	out := make([]Fetched, 0, len(cfg.Sources))
	for _, s := range cfg.Sources {
		f, err := c.fetchOne(ctx, s, localDir)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", s.ID, err)
		}
		out = append(out, f)
	}
	return out, nil
}

func (c *Client) fetchOne(ctx context.Context, s Source, localDir string) (Fetched, error) {
	if s.Repo == "" {
		b, err := os.ReadFile(joinLocal(localDir, s.Path))
		if err != nil {
			return Fetched{}, err
		}
		return Fetched{Source: s, Content: b, Commit: "", Changed: true}, nil
	}

	ref := s.Ref
	if ref == "" {
		ref = "HEAD"
	}
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", s.Repo, ref, s.Path)
	body, err := c.get(ctx, rawURL, "")
	if err != nil {
		return Fetched{}, err
	}

	commit := c.resolveCommit(ctx, s, ref)
	return Fetched{
		Source:  s,
		Content: body,
		Commit:  commit,
		Changed: commit == "" || s.LastSeenCommit == "" || commit != s.LastSeenCommit,
	}, nil
}

// resolveCommit asks the GitHub API for the latest commit touching the pinned
// path at the pinned ref. Failure is non-fatal: it returns "".
func (c *Client) resolveCommit(ctx context.Context, s Source, ref string) string {
	api := fmt.Sprintf("https://api.github.com/repos/%s/commits?path=%s&sha=%s&per_page=1",
		s.Repo, s.Path, ref)
	body, err := c.get(ctx, api, "application/vnd.github+json")
	if err != nil {
		return ""
	}
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &commits); err != nil || len(commits) == 0 {
		return ""
	}
	return commits[0].SHA
}

func (c *Client) get(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}

func joinLocal(dir, path string) string {
	if dir == "" {
		return path
	}
	return strings.TrimRight(dir, "/") + "/" + strings.TrimLeft(path, "/")
}
