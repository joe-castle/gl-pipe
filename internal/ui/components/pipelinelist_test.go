package components

import (
	"testing"
	"time"

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

// TestPipelineList_HighlightedRecoversAfterEmptyToNonEmpty mirrors the same
// bubbles/table cursor bug for the pipeline matrix: e.g. drilling into a
// project with zero pipelines, then drilling into one that has some.
func TestPipelineList_HighlightedRecoversAfterEmptyToNonEmpty(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines(nil)

	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})

	pl, ok := p.HighlightedPipeline()
	if !ok || pl.ID != 1 {
		t.Fatalf("expected pipeline 1 highlighted after going from empty to non-empty, got %+v ok=%v", pl, ok)
	}
}

func TestPipelineList_DefaultSortIsDateDescending(t *testing.T) {
	now := time.Now()
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 2, CreatedAt: now},
		{ID: 3, CreatedAt: now.Add(-1 * time.Hour)},
	})

	if p.pipelines[0].ID != 2 || p.pipelines[1].ID != 3 || p.pipelines[2].ID != 1 {
		t.Fatalf("expected newest-first order [2,3,1], got %v", pipelineIDs(p.pipelines))
	}
}

func TestPipelineList_CycleSortChangesFieldAndResetsDescending(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "zzz"},
		{ID: 2, Ref: "aaa"},
	})

	p.CycleSort() // Date -> Project
	p.CycleSort() // Project -> Status
	p.CycleSort() // Status -> Ref
	if p.sortField != sortByRef || !p.sortDesc {
		t.Fatalf("expected sortField=Ref sortDesc=true, got field=%v desc=%v", p.sortField, p.sortDesc)
	}
	// descending by Ref: "zzz" before "aaa"
	if p.pipelines[0].ID != 1 || p.pipelines[1].ID != 2 {
		t.Fatalf("expected [1,2] sorted desc by Ref, got %v", pipelineIDs(p.pipelines))
	}
}

func TestPipelineList_ToggleSortDirectionReversesWithoutChangingField(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "aaa"},
		{ID: 2, Ref: "zzz"},
	})
	p.SortBy(sortByRef) // resets to desc: zzz, aaa

	p.ToggleSortDirection()
	if p.sortField != sortByRef {
		t.Fatal("ToggleSortDirection must not change the sort field")
	}
	if p.sortDesc {
		t.Fatal("expected sortDesc=false after toggling")
	}
	if p.pipelines[0].ID != 1 || p.pipelines[1].ID != 2 {
		t.Fatalf("expected [1,2] sorted asc by Ref, got %v", pipelineIDs(p.pipelines))
	}
}

func TestPipelineList_SortKeysDriveCycleAndToggle(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}, {ID: 2}})

	updated, _ := p.Update(runeKey('s'))
	if updated.sortField != sortByProject {
		t.Fatalf("expected 's' to cycle to Project, got %v", updated.sortField)
	}
	updated, _ = updated.Update(runeKey('S'))
	if updated.sortDesc {
		t.Fatal("expected 'S' to reverse to ascending")
	}
}

func pipelineIDs(pipelines []api.Pipeline) []int {
	ids := make([]int, len(pipelines))
	for i, p := range pipelines {
		ids[i] = p.ID
	}
	return ids
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
	if len(msg.Pipelines) != 1 || msg.Pipelines[0].ProjectID != 10 || msg.Pipelines[0].ID != 1 {
		t.Fatalf("unexpected OpenJobsMsg: %+v", msg)
	}

	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100}})
	if !p.InJobs() {
		t.Fatal("expected InJobs true after AddJobs")
	}

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.InJobs() {
		t.Fatal("expected esc to return to the pipeline matrix")
	}
}

func TestPipelineList_EnterOnStagedPipelinesOpensJobsForAll(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, ProjectID: 10},
		{ID: 2, ProjectID: 11},
		{ID: 3, ProjectID: 12},
	})
	p.Selected[1] = true
	p.Selected[2] = true

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg, ok := cmd().(OpenJobsMsg)
	if !ok {
		t.Fatalf("expected OpenJobsMsg, got %T", msg)
	}
	if len(msg.Pipelines) != 2 {
		t.Fatalf("expected the 2 staged pipelines, got %+v", msg.Pipelines)
	}
}

func TestPipelineList_AddJobsMergesAcrossMultiplePipelines(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10, IID: 100}, {ID: 2, ProjectID: 11, IID: 200}})
	p.ClearJobs()

	p.AddJobs(api.Pipeline{ID: 1, ProjectID: 10, IID: 100}, []api.Job{{ID: 10, ProjectID: 10, PipelineID: 1}})
	p.AddJobs(api.Pipeline{ID: 2, ProjectID: 11, IID: 200}, []api.Job{{ID: 20, ProjectID: 11, PipelineID: 2}})

	if len(p.jobs) != 2 {
		t.Fatalf("expected jobs from both pipelines merged, got %d", len(p.jobs))
	}
	if len(p.Pipelines) != 2 {
		t.Fatalf("expected both pipelines recorded, got %d", len(p.Pipelines))
	}
}

func TestPipelineList_AddJobsUpsertsByJobID(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()

	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusRunning}})
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusSuccess}})

	if len(p.jobs) != 1 || p.jobs[0].Status != api.StatusSuccess {
		t.Fatalf("expected job 10 patched in place, got %+v", p.jobs)
	}
}

func TestPipelineList_ClearJobsResetsBothJobsAndPipelines(t *testing.T) {
	p := NewPipelineList()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10}})

	p.ClearJobs()

	if len(p.jobs) != 0 || len(p.Pipelines) != 0 {
		t.Fatalf("expected ClearJobs to reset both, got jobs=%d pipelines=%d", len(p.jobs), len(p.Pipelines))
	}
}

func TestPipelineList_FindPipeline(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 5, Ref: "main"}})

	pl, ok := p.FindPipeline(5)
	if !ok || pl.Ref != "main" {
		t.Fatalf("expected to find pipeline 5, got %+v ok=%v", pl, ok)
	}
	if _, ok := p.FindPipeline(999); ok {
		t.Fatal("expected FindPipeline to fail for an unknown ID")
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
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100, ProjectID: 10}})

	_, cmd := p.Update(runeKey('K'))
	msg, ok := cmd().(BulkJobActionMsg)
	if !ok {
		t.Fatalf("expected BulkJobActionMsg, got %T", msg)
	}
	if msg.Retry || len(msg.Targets) != 1 || msg.Targets[0].ID != 100 {
		t.Fatalf("unexpected bulk job action msg: %+v", msg)
	}
}
