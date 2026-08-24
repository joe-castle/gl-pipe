package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestListMyMergeRequests_MergesScopesAndDedupes(t *testing.T) {
	mux, client := setup(t)
	calls := map[string]int{}
	mux.HandleFunc("/api/v4/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		scope := r.URL.Query().Get("scope")
		calls[scope]++
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("state = %q, want opened", r.URL.Query().Get("state"))
		}
		switch scope {
		case "assigned_to_me":
			fmt.Fprint(w, `[{"id": 1, "iid": 5, "project_id": 10, "title": "Fix login", "source_branch": "fix/login", "target_branch": "main", "author": {"username": "alice"}}]`)
		case "created_by_me":
			// MR 1 appears in both scopes; MR 2 is new.
			fmt.Fprint(w, `[{"id": 1, "iid": 5, "project_id": 10, "title": "Fix login"}, {"id": 2, "iid": 6, "project_id": 11, "title": "Add feature"}]`)
		default:
			t.Fatalf("unexpected scope %q", scope)
		}
	})

	mrs, err := client.ListMyMergeRequests(context.Background())
	if err != nil {
		t.Fatalf("ListMyMergeRequests returned error: %v", err)
	}
	if calls["assigned_to_me"] != 1 || calls["created_by_me"] != 1 {
		t.Fatalf("expected both scopes queried once, got %+v", calls)
	}
	if len(mrs) != 2 {
		t.Fatalf("expected 2 deduplicated MRs, got %d: %+v", len(mrs), mrs)
	}
	if mrs[0].Author != "alice" || mrs[0].SourceBranch != "fix/login" {
		t.Errorf("unexpected first MR: %+v", mrs[0])
	}
}

func TestListProjectMergeRequests_ParsesFields(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/10/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("state = %q, want opened", r.URL.Query().Get("state"))
		}
		fmt.Fprint(w, `[{"id": 3, "iid": 7, "project_id": 10, "title": "Bump deps", "draft": true, "web_url": "https://x/mr/7"}]`)
	})

	mrs, err := client.ListProjectMergeRequests(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListProjectMergeRequests returned error: %v", err)
	}
	if len(mrs) != 1 || !mrs[0].Draft || mrs[0].WebURL != "https://x/mr/7" {
		t.Fatalf("unexpected MRs: %+v", mrs)
	}
}

func TestListMergeRequestPipelines_ParsesPipelines(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/10/merge_requests/7/pipelines", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		fmt.Fprint(w, `[{"id": 99, "project_id": 10, "status": "failed", "ref": "refs/merge-requests/7/head", "sha": "abc123"}]`)
	})

	pipelines, err := client.ListMergeRequestPipelines(context.Background(), 10, 7)
	if err != nil {
		t.Fatalf("ListMergeRequestPipelines returned error: %v", err)
	}
	if len(pipelines) != 1 || pipelines[0].Status != StatusFailed {
		t.Fatalf("unexpected pipelines: %+v", pipelines)
	}
}
