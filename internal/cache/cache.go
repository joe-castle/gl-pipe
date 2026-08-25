// Package cache maintains a local JSON index of GitLab projects so the
// project explorer can fuzzy-filter instantly instead of hitting the API
// on every keystroke.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"

	"github.com/joeca/gl-pipe/internal/api"
)

// Index is the on-disk project cache for one instance.
type Index struct {
	Instance string        `json:"instance"`
	SyncedAt time.Time     `json:"synced_at"`
	Projects []api.Project `json:"projects"`
}

// Load reads the cache file at path. A missing file is not an error: it
// returns a zero-value Index so first-run behaves like an empty, expired cache.
func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Index{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cache: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		// A corrupt cache file should not block startup: treat it as empty.
		return &Index{}, nil
	}
	return &idx, nil
}

// Save writes the index to path as JSON, creating parent directories as needed.
func (idx *Index) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing cache: %w", err)
	}
	return nil
}

// Stale reports whether the index was synced longer ago than ttl, or has
// never been synced.
func (idx *Index) Stale(ttl time.Duration) bool {
	if idx.SyncedAt.IsZero() {
		return true
	}
	return time.Since(idx.SyncedAt) > ttl
}

// Filter matches the cached projects against query by path-with-namespace,
// best match first. An empty query returns every project in cache order.
//
// query is split on whitespace into space-separated tokens, all of which
// must hold (AND). Most tokens are plain fuzzy-subsequence terms (ranked,
// same as a bare query always was), but two prefixes carry special
// meaning, borrowed from fzf's extended-search syntax:
//
//   - "!term" excludes any project whose path contains term.
//   - "group/" (a token ending in "/") keeps only direct children of that
//     group — it must start with the prefix and have no further "/" after
//     it, so subgroup projects are excluded. This is a hard filter, not a
//     fuzzy one: it answers "this group, not its subgroups," which no
//     amount of fuzzy-matching the group name can express, since fuzzy
//     matching has no notion of path depth.
//
// A query using only exclude/anchor tokens (no plain term) returns
// matches in cache order, since there's nothing to rank by.
func (idx *Index) Filter(query string) []api.Project {
	if query == "" {
		return idx.Projects
	}

	var excludes, anchors, fuzzyTerms []string
	for _, tok := range strings.Fields(query) {
		switch {
		case len(tok) > 1 && strings.HasPrefix(tok, "!"):
			excludes = append(excludes, strings.ToLower(tok[1:]))
		case len(tok) > 1 && strings.HasSuffix(tok, "/"):
			anchors = append(anchors, strings.ToLower(tok))
		default:
			fuzzyTerms = append(fuzzyTerms, tok)
		}
	}

	candidates := idx.Projects
	if len(excludes) > 0 || len(anchors) > 0 {
		candidates = filterByAnchorsAndExcludes(candidates, anchors, excludes)
	}

	if len(fuzzyTerms) == 0 {
		return candidates
	}
	return fuzzyRankAND(candidates, fuzzyTerms)
}

func filterByAnchorsAndExcludes(projects []api.Project, anchors, excludes []string) []api.Project {
	out := make([]api.Project, 0, len(projects))
	for _, p := range projects {
		path := strings.ToLower(p.PathWithNamespace)

		excluded := false
		for _, ex := range excludes {
			if strings.Contains(path, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		matchesAllAnchors := true
		for _, prefix := range anchors {
			rest, ok := strings.CutPrefix(path, prefix)
			if !ok || strings.Contains(rest, "/") {
				matchesAllAnchors = false
				break
			}
		}
		if matchesAllAnchors {
			out = append(out, p)
		}
	}
	return out
}

// fuzzyRankAND requires every term to fuzzy-match (AND, not just the
// last term), ranking survivors by their summed score across terms —
// with one term this is identical to a plain fuzzy.Find.
func fuzzyRankAND(projects []api.Project, terms []string) []api.Project {
	liveIdx := make([]int, len(projects))
	liveNames := make([]string, len(projects))
	for i, p := range projects {
		liveIdx[i] = i
		liveNames[i] = p.PathWithNamespace
	}
	scoreSum := make([]int, len(projects))

	for _, term := range terms {
		matches := fuzzy.Find(term, liveNames)
		nextIdx := make([]int, len(matches))
		nextNames := make([]string, len(matches))
		for i, m := range matches {
			origIdx := liveIdx[m.Index]
			scoreSum[origIdx] += m.Score
			nextIdx[i] = origIdx
			nextNames[i] = liveNames[m.Index]
		}
		liveIdx, liveNames = nextIdx, nextNames
	}

	sort.SliceStable(liveIdx, func(i, j int) bool { return scoreSum[liveIdx[i]] > scoreSum[liveIdx[j]] })

	out := make([]api.Project, len(liveIdx))
	for i, origIdx := range liveIdx {
		out[i] = projects[origIdx]
	}
	return out
}
