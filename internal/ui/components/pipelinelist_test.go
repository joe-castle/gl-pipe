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
