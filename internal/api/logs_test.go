package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestStreamJobTrace_EmitsIncrementalChunksThenDone(t *testing.T) {
	mux, client := setup(t)

	traceCalls := 0
	mux.HandleFunc("/api/v4/projects/1/jobs/7/trace", func(w http.ResponseWriter, r *http.Request) {
		traceCalls++
		if traceCalls == 1 {
			fmt.Fprint(w, "hello")
			return
		}
		fmt.Fprint(w, "hello world")
	})

	jobCalls := 0
	mux.HandleFunc("/api/v4/projects/1/jobs/7", func(w http.ResponseWriter, r *http.Request) {
		jobCalls++
		if jobCalls == 1 {
			fmt.Fprint(w, `{"id": 7, "status": "running"}`)
			return
		}
		fmt.Fprint(w, `{"id": 7, "status": "success"}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunks := make(chan LogChunk, 10)
	go client.StreamJobTrace(ctx, 1, 7, chunks, time.Millisecond)

	var got []LogChunk
	for c := range chunks {
		got = append(got, c)
		if c.Done || c.Err != nil {
			break
		}
	}

	if len(got) != 3 {
		t.Fatalf("received %d chunks, want 3: %+v", len(got), got)
	}
	if got[0].Content != "hello" || got[0].Offset != 0 {
		t.Errorf("chunk 0 = %+v", got[0])
	}
	if got[1].Content != " world" || got[1].Offset != 5 {
		t.Errorf("chunk 1 = %+v", got[1])
	}
	if !got[2].Done {
		t.Errorf("chunk 2 Done = false, want true")
	}
}

func TestStreamJobTrace_PropagatesError(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/jobs/9/trace", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chunks := make(chan LogChunk, 10)
	go client.StreamJobTrace(ctx, 1, 9, chunks, time.Millisecond)

	c, ok := <-chunks
	if !ok {
		t.Fatal("channel closed with no chunk")
	}
	if c.Err == nil {
		t.Error("expected non-nil Err on the emitted chunk")
	}
}
