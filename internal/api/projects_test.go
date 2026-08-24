package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestListGroups_ParsesFullPath(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		fmt.Fprint(w, `[{"id": 1, "name": "Core Services", "full_path": "backend/core-services"}]`)
	})

	groups, err := client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups returned error: %v", err)
	}
	if len(groups) != 1 || groups[0].FullPath != "backend/core-services" || groups[0].Name != "Core Services" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestListGroupProjects_ParsesAndPaginates(t *testing.T) {
	mux, client := setup(t)
	page := 0
	mux.HandleFunc("/api/v4/groups/backend/projects", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		page++
		if page == 1 {
			w.Header().Set("X-Next-Page", "2")
			w.Header().Set("Link", `<http://example.com?page=2>; rel="next"`)
			fmt.Fprint(w, `[{"id": 1, "name": "svc-a", "path_with_namespace": "backend/svc-a", "web_url": "https://gitlab.example.com/backend/svc-a", "default_branch": "main"}]`)
			return
		}
		fmt.Fprint(w, `[{"id": 2, "name": "svc-b", "path_with_namespace": "backend/svc-b", "web_url": "https://gitlab.example.com/backend/svc-b", "default_branch": "main"}]`)
	})

	projects, err := client.ListGroupProjects(context.Background(), "backend")
	if err != nil {
		t.Fatalf("ListGroupProjects returned error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(projects))
	}
	if projects[0].PathWithNamespace != "backend/svc-a" {
		t.Errorf("projects[0].PathWithNamespace = %q", projects[0].PathWithNamespace)
	}
	if projects[1].ID != 2 {
		t.Errorf("projects[1].ID = %d, want 2", projects[1].ID)
	}
}

func TestListBranches_ParsesCommitSHA(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/repository/branches", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name": "main", "commit": {"id": "abc123"}}]`)
	})

	refs, err := client.ListBranches(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListBranches returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].Name != "main" || refs[0].CommitSHA != "abc123" || refs[0].IsTag {
		t.Errorf("ListBranches() = %+v", refs)
	}
}

func TestListTags_ParsesCommitSHA(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/repository/tags", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name": "v1.2.3", "commit": {"id": "def456"}}]`)
	})

	refs, err := client.ListTags(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListTags returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].Name != "v1.2.3" || !refs[0].IsTag {
		t.Errorf("ListTags() = %+v", refs)
	}
}

func TestLatestSemVerTag_PicksHighestVersion(t *testing.T) {
	refs := []Ref{
		{Name: "v1.2.3", IsTag: true},
		{Name: "v2.0.0", IsTag: true},
		{Name: "v1.9.9", IsTag: true},
		{Name: "not-a-branch", IsTag: false},
	}

	best, ok := LatestSemVerTag(refs)
	if !ok {
		t.Fatal("LatestSemVerTag() ok = false, want true")
	}
	if best.Name != "v2.0.0" {
		t.Errorf("LatestSemVerTag() = %q, want v2.0.0", best.Name)
	}
}

func TestLatestSemVerTag_AcceptsTagsWithoutVPrefix(t *testing.T) {
	refs := []Ref{
		{Name: "1.0.0", IsTag: true},
		{Name: "1.4.0", IsTag: true},
	}

	best, ok := LatestSemVerTag(refs)
	if !ok {
		t.Fatal("LatestSemVerTag() ok = false, want true")
	}
	if best.Name != "1.4.0" {
		t.Errorf("LatestSemVerTag() = %q, want 1.4.0", best.Name)
	}
}

func TestLatestSemVerTag_IgnoresNonSemVerTags(t *testing.T) {
	refs := []Ref{
		{Name: "release-candidate", IsTag: true},
		{Name: "latest", IsTag: true},
	}

	if _, ok := LatestSemVerTag(refs); ok {
		t.Error("LatestSemVerTag() ok = true for tags with no valid SemVer")
	}
}

func TestLatestSemVerTag_NoTagsReturnsFalse(t *testing.T) {
	if _, ok := LatestSemVerTag(nil); ok {
		t.Error("LatestSemVerTag(nil) ok = true, want false")
	}
}
