package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestBlobSearchInGroup_ParsesHits(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/groups/backend/-/search", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		q := r.URL.Query()
		if q.Get("scope") != "blobs" {
			t.Errorf("scope = %q, want blobs", q.Get("scope"))
		}
		if got, _ := url.QueryUnescape(q.Get("search")); got != "@SpringBootApplication" {
			t.Errorf("search = %q, want @SpringBootApplication", got)
		}
		fmt.Fprint(w, `[{"project_id": 5, "path": "src/main/java/App.java", "ref": "main", "startline": 12, "data": "@SpringBootApplication\nclass App {}"}]`)
	})

	hits, err := client.BlobSearchInGroup(context.Background(), "backend", "@SpringBootApplication")
	if err != nil {
		t.Fatalf("BlobSearchInGroup returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if hits[0].ProjectID != 5 || hits[0].Path != "src/main/java/App.java" || hits[0].StartLine != 12 {
		t.Errorf("hits[0] = %+v", hits[0])
	}
}
