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

// TestFilter_TrailingSlashMatchesDirectChildrenOnly covers the depth
// anchor: "backend/" should mean "direct children of backend", excluding
// anything nested under a subgroup.
func TestFilter_TrailingSlashMatchesDirectChildrenOnly(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-a"},
		{PathWithNamespace: "backend/subteam/svc-b"},
		{PathWithNamespace: "frontend/svc-c"},
	}}
	got := idx.Filter("backend/")
	if len(got) != 1 || got[0].PathWithNamespace != "backend/svc-a" {
		t.Fatalf("Filter(\"backend/\") = %+v, want only backend/svc-a", got)
	}
}

func TestFilter_TrailingSlashAnchorIsCaseInsensitive(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "Backend/svc-a"},
	}}
	got := idx.Filter("backend/")
	if len(got) != 1 {
		t.Fatalf("Filter(\"backend/\") = %+v, want 1 case-insensitive match", got)
	}
}

// TestFilter_ExclamationExcludesMatches covers the exclude token: "!term"
// drops any project whose path contains term.
func TestFilter_ExclamationExcludesMatches(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-a"},
		{PathWithNamespace: "backend/legacy-svc"},
	}}
	got := idx.Filter("svc !legacy")
	if len(got) != 1 || got[0].PathWithNamespace != "backend/svc-a" {
		t.Fatalf("Filter(\"svc !legacy\") = %+v, want only backend/svc-a", got)
	}
}

// TestFilter_MultipleFuzzyTermsAreANDed: space-separated plain terms must
// all match (in some order/position), not just the last one.
func TestFilter_MultipleFuzzyTermsAreANDed(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/core-services"},
		{PathWithNamespace: "backend/core-utils"},
		{PathWithNamespace: "frontend/core-services"},
	}}
	got := idx.Filter("backend services")
	if len(got) != 1 || got[0].PathWithNamespace != "backend/core-services" {
		t.Fatalf("Filter(\"backend services\") = %+v, want only backend/core-services", got)
	}
}

// TestFilter_CombinesAnchorAndExclude: the depth anchor and the exclude
// token compose in a single query.
func TestFilter_CombinesAnchorAndExclude(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-a"},
		{PathWithNamespace: "backend/legacy-svc"},
		{PathWithNamespace: "backend/subteam/svc-b"},
	}}
	got := idx.Filter("backend/ !legacy")
	if len(got) != 1 || got[0].PathWithNamespace != "backend/svc-a" {
		t.Fatalf("Filter(\"backend/ !legacy\") = %+v, want only backend/svc-a", got)
	}
}

func TestFilter_AnchorOrExcludeAloneWithNoFuzzyTermPreservesOrder(t *testing.T) {
	idx := &Index{Projects: []api.Project{
		{PathWithNamespace: "backend/svc-b"},
		{PathWithNamespace: "backend/svc-a"},
	}}
	got := idx.Filter("backend/")
	if len(got) != 2 || got[0].PathWithNamespace != "backend/svc-b" || got[1].PathWithNamespace != "backend/svc-a" {
		t.Fatalf("expected original cache order preserved with no fuzzy term, got %+v", got)
	}
}
