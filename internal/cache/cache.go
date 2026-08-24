// Package cache maintains a local JSON index of GitLab projects so the
// project explorer can fuzzy-filter instantly instead of hitting the API
// on every keystroke.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Filter fuzzy-ranks the cached projects against query by path-with-namespace,
// best match first. An empty query returns every project in cache order.
func (idx *Index) Filter(query string) []api.Project {
	if query == "" {
		return idx.Projects
	}
	names := make([]string, len(idx.Projects))
	for i, p := range idx.Projects {
		names[i] = p.PathWithNamespace
	}
	matches := fuzzy.Find(query, names)
	out := make([]api.Project, len(matches))
	for i, m := range matches {
		out[i] = idx.Projects[m.Index]
	}
	return out
}
