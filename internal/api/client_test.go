package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestValidate_ReturnsUsernameOnSuccess(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		fmt.Fprint(w, `{"id": 1, "username": "octocat"}`)
	})

	username, err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if username != "octocat" {
		t.Errorf("Validate() = %q, want %q", username, "octocat")
	}
}

func TestValidate_ReturnsErrorOnUnauthorized(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message": "401 Unauthorized"}`)
	})

	if _, err := client.Validate(context.Background()); err == nil {
		t.Fatal("expected error for unauthorized response, got nil")
	}
}
