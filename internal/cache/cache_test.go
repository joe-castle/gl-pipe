package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestLoad_MissingFileReturnsEmptyIndex(t *testing.T) {
	idx, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(idx.Projects) != 0 {
		t.Errorf("Projects = %v, want empty", idx.Projects)
	}
	if !idx.Stale(time.Hour) {
		t.Error("a never-synced index should be Stale")
	}
}

func TestLoad_CorruptFileReturnsEmptyIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("writing corrupt cache file: %v", err)
	}

	idx, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error for corrupt file: %v", err)
	}
	if len(idx.Projects) != 0 {
		t.Errorf("Projects = %v, want empty for corrupt cache", idx.Projects)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	idx := &Index{
		Instance: "work",
		SyncedAt: time.Now().UTC().Truncate(time.Second),
		Projects: []api.Project{
			{ID: 1, Name: "svc-a", PathWithNamespace: "backend/svc-a"},
		},
	}
	path := filepath.Join(t.TempDir(), "nested", "cache.json")

	if err := idx.Save(path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save returned error: %v", err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].PathWithNamespace != "backend/svc-a" {
		t.Errorf("Projects = %+v", loaded.Projects)
	}
	if !loaded.SyncedAt.Equal(idx.SyncedAt) {
		t.Errorf("SyncedAt = %v, want %v", loaded.SyncedAt, idx.SyncedAt)
	}
}

func TestStale_BoundaryConditions(t *testing.T) {
	fresh := &Index{SyncedAt: time.Now()}
	if fresh.Stale(time.Hour) {
		t.Error("a just-synced index should not be Stale")
	}

	old := &Index{SyncedAt: time.Now().Add(-2 * time.Hour)}
	if !old.Stale(time.Hour) {
		t.Error("a 2h-old index should be Stale against a 1h TTL")
	}
}

func TestFilter_EmptyQueryReturnsAll(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-a"},
		{PathWithNamespace: "frontend/svc-b"},
	}}
	got := idx.Filter("")
	if len(got) != 2 {
		t.Errorf("Filter(\"\") returned %d projects, want 2", len(got))
	}
}

func TestFilter_RanksBestMatchFirst(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "frontend/unrelated"},
		{PathWithNamespace: "backend/core-services"},
		{PathWithNamespace: "backend/core-utils"},
	}}

	got := idx.Filter("core-services")
	if len(got) == 0 {
		t.Fatal("Filter returned no matches")
	}
	if got[0].PathWithNamespace != "backend/core-services" {
		t.Errorf("best match = %q, want backend/core-services", got[0].PathWithNamespace)
	}
}

func TestFilter_NoMatchesReturnsEmpty(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-a"},
	}}
	got := idx.Filter("zzz-does-not-exist-anywhere")
	if len(got) != 0 {
		t.Errorf("Filter returned %d results, want 0", len(got))
	}
}
