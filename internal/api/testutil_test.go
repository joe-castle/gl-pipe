package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// setup starts a mock GitLab API server and returns a mux to register
// handlers on plus a Client wired to hit it.
func setup(t *testing.T) (*http.ServeMux, *Client) {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	gl, err := gitlab.NewClient("test-token", gitlab.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("constructing test gitlab client: %v", err)
	}
	return mux, &Client{gl: gl, baseURL: server.URL}
}

func testMethod(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.Method != want {
		t.Errorf("request method = %s, want %s", r.Method, want)
	}
}
