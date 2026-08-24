package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestPipelineList_AddOrUpdateInsertsThenPatches(t *testing.T) {
	p := NewPipelineList()
	p.AddOrUpdate(api.Pipeline{ID: 1, ProjectID: 10, Status: api.StatusRunning})
	if len(p.pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(p.pipelines))
	}

	p.AddOrUpdate(api.Pipeline{ID: 1, ProjectID: 10, Status: api.StatusSuccess})
	if len(p.pipelines) != 1 || p.pipelines[0].Status != api.StatusSuccess {
		t.Fatalf("expected in-place patch, got %+v", p.pipelines)
	}

	p.AddOrUpdate(api.Pipeline{ID: 2, ProjectID: 10, Status: api.StatusFailed})
	if len(p.pipelines) != 2 {
		t.Fatalf("expected 2 pipelines after insert, got %d", len(p.pipelines))
	}
}

func TestPipelineList_SelectedPipelinesFallsBackToHighlighted(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}, {ID: 2}})

	got := p.SelectedPipelines()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected fallback to highlighted pipeline 1, got %+v", got)
	}
}

func TestPipelineList_EnterOpensJobsThenEscReturns(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected OpenJobsMsg cmd")
	}
	msg, ok := cmd().(OpenJobsMsg)
	if !ok {
		t.Fatalf("expected OpenJobsMsg, got %T", msg)
	}
	if msg.ProjectID != 10 || msg.PipelineID != 1 {
		t.Fatalf("unexpected OpenJobsMsg: %+v", msg)
	}

	p.SetJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100}})
	if !p.InJobs() {
		t.Fatal("expected InJobs true after SetJobs")
	}

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.InJobs() {
		t.Fatal("expected esc to return to the pipeline matrix")
	}
}

func TestPipelineList_BulkRetryUsesStagedPipelines(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}, {ID: 2, ProjectID: 11}})
	p.Selected[2] = true

	_, cmd := p.Update(runeKey('R'))
	msg, ok := cmd().(BulkPipelineActionMsg)
	if !ok {
		t.Fatalf("expected BulkPipelineActionMsg, got %T", msg)
	}
	if !msg.Retry || len(msg.Targets) != 1 || msg.Targets[0].ID != 2 {
		t.Fatalf("unexpected bulk action msg: %+v", msg)
	}
}

func TestPipelineList_BulkCancelInJobsUsesHighlightedJob(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.SetJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100, ProjectID: 10}})

	_, cmd := p.Update(runeKey('K'))
	msg, ok := cmd().(BulkJobActionMsg)
	if !ok {
		t.Fatalf("expected BulkJobActionMsg, got %T", msg)
	}
	if msg.Retry || len(msg.Targets) != 1 || msg.Targets[0].ID != 100 {
		t.Fatalf("unexpected bulk job action msg: %+v", msg)
	}
}
