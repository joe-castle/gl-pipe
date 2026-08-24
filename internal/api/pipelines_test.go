package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestListPipelines_ParsesStatusAndRef(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		fmt.Fprint(w, `[{"id": 10, "iid": 1, "project_id": 1, "status": "running", "ref": "main", "sha": "abc123", "web_url": "https://x/1"}]`)
	})

	pipelines, err := client.ListPipelines(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPipelines returned error: %v", err)
	}
	if len(pipelines) != 1 {
		t.Fatalf("len(pipelines) = %d, want 1", len(pipelines))
	}
	if pipelines[0].Status != StatusRunning {
		t.Errorf("Status = %q, want running", pipelines[0].Status)
	}
	if pipelines[0].SHA != "abc123" {
		t.Errorf("SHA = %q", pipelines[0].SHA)
	}
}

func TestListPipelinesByRef_SendsRefFilter(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		if got := r.URL.Query().Get("ref"); got != "feature/login-fix" {
			t.Errorf("ref query param = %q, want feature/login-fix", got)
		}
		fmt.Fprint(w, `[{"id": 20, "project_id": 1, "status": "success", "ref": "feature/login-fix", "sha": "def456"}]`)
	})

	pipelines, err := client.ListPipelinesByRef(context.Background(), 1, "feature/login-fix")
	if err != nil {
		t.Fatalf("ListPipelinesByRef returned error: %v", err)
	}
	if len(pipelines) != 1 || pipelines[0].Ref != "feature/login-fix" {
		t.Fatalf("unexpected pipelines: %+v", pipelines)
	}
}

func TestGetPipeline_IncludesUserAndDuration(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines/10", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id": 10, "project_id": 1, "status": "success", "ref": "main", "sha": "abc123", "user": {"username": "alice"}, "duration": 90}`)
	})

	p, err := client.GetPipeline(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetPipeline returned error: %v", err)
	}
	if p.User != "alice" {
		t.Errorf("User = %q, want alice", p.User)
	}
	if p.Duration.Seconds() != 90 {
		t.Errorf("Duration = %v, want 90s", p.Duration)
	}
}

func TestCreatePipeline_SendsVariables(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipeline", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		var body struct {
			Ref       string `json:"ref"`
			Variables []struct {
				Key          string `json:"key"`
				Value        string `json:"value"`
				VariableType string `json:"variable_type"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body.Ref != "main" {
			t.Errorf("Ref = %q, want main", body.Ref)
		}
		if len(body.Variables) != 1 || body.Variables[0].Key != "DEPLOY_ENV" || body.Variables[0].Value != "qa" {
			t.Errorf("Variables = %+v", body.Variables)
		}
		fmt.Fprint(w, `{"id": 99, "project_id": 1, "status": "created", "ref": "main"}`)
	})

	p, err := client.CreatePipeline(context.Background(), 1, "main", []Variable{
		{Key: "DEPLOY_ENV", Value: "qa", Type: VarTypeEnv},
	})
	if err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if p.ID != 99 {
		t.Errorf("ID = %d, want 99", p.ID)
	}
}

func TestRetryPipeline_HitsRetryEndpoint(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines/10/retry", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		fmt.Fprint(w, `{"id": 10, "project_id": 1, "status": "pending", "ref": "main"}`)
	})

	p, err := client.RetryPipeline(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("RetryPipeline returned error: %v", err)
	}
	if p.Status != StatusPending {
		t.Errorf("Status = %q, want pending", p.Status)
	}
}

func TestCancelPipeline_HitsCancelEndpoint(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines/10/cancel", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodPost)
		fmt.Fprint(w, `{"id": 10, "project_id": 1, "status": "canceled", "ref": "main"}`)
	})

	p, err := client.CancelPipeline(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("CancelPipeline returned error: %v", err)
	}
	if p.Status != StatusCanceled {
		t.Errorf("Status = %q, want canceled", p.Status)
	}
}

func TestListJobs_ComputesRetryCountPerStageAndName(t *testing.T) {
	mux, client := setup(t)
	mux.HandleFunc("/api/v4/projects/1/pipelines/10/jobs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"id": 1, "name": "test", "stage": "test", "status": "failed", "runner": {"description": "runner-a"}},
			{"id": 2, "name": "test", "stage": "test", "status": "success", "runner": {"description": "runner-a"}},
			{"id": 3, "name": "build", "stage": "build", "status": "success", "runner": {"description": "runner-b"}}
		]`)
	})

	jobs, err := client.ListJobs(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	if jobs[0].RetryCount != 0 || jobs[1].RetryCount != 1 {
		t.Errorf("retry counts = [%d, %d], want [0, 1]", jobs[0].RetryCount, jobs[1].RetryCount)
	}
	if jobs[2].RunnerTag != "runner-b" {
		t.Errorf("RunnerTag = %q, want runner-b", jobs[2].RunnerTag)
	}
}

func TestRetryJob_HitsRetryEndpoint(t *testing.T) {
	mux, client := setup(t)
	hit := false
	mux.HandleFunc("/api/v4/projects/1/jobs/5/retry", func(w http.ResponseWriter, r *http.Request) {
		hit = true
		testMethod(t, r, http.MethodPost)
		fmt.Fprint(w, `{"id": 5, "status": "pending"}`)
	})

	if err := client.RetryJob(context.Background(), 1, 5); err != nil {
		t.Fatalf("RetryJob returned error: %v", err)
	}
	if !hit {
		t.Error("retry endpoint was not called")
	}
}

func TestCancelJob_HitsCancelEndpoint(t *testing.T) {
	mux, client := setup(t)
	hit := false
	mux.HandleFunc("/api/v4/projects/1/jobs/5/cancel", func(w http.ResponseWriter, r *http.Request) {
		hit = true
		testMethod(t, r, http.MethodPost)
		fmt.Fprint(w, `{"id": 5, "status": "canceled"}`)
	})

	if err := client.CancelJob(context.Background(), 1, 5); err != nil {
		t.Fatalf("CancelJob returned error: %v", err)
	}
	if !hit {
		t.Error("cancel endpoint was not called")
	}
}
