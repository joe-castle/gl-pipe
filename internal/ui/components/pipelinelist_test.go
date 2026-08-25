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

	if p.filtered[0].ID != 2 || p.filtered[1].ID != 3 || p.filtered[2].ID != 1 {
		t.Fatalf("expected newest-first order [2,3,1], got %v", pipelineIDs(p.filtered))
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
	if p.filtered[0].ID != 1 || p.filtered[1].ID != 2 {
		t.Fatalf("expected [1,2] sorted desc by Ref, got %v", pipelineIDs(p.filtered))
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
	if p.filtered[0].ID != 1 || p.filtered[1].ID != 2 {
		t.Fatalf("expected [1,2] sorted asc by Ref, got %v", pipelineIDs(p.filtered))
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

func TestPipelineList_FilterMatchesRef(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "feature/login-fix"},
		{ID: 2, Ref: "main"},
		{ID: 3, Ref: "feature/login-fix"}, // e.g. two MRs sharing a source branch
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("login-fix")
	updated.syncPipeRows()

	if len(updated.filtered) != 2 {
		t.Fatalf("expected 2 pipelines matching the ref, got %d: %v", len(updated.filtered), pipelineIDs(updated.filtered))
	}
}

func TestPipelineList_FilterMatchesStatus(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Status: api.StatusFailed},
		{ID: 2, Status: api.StatusSuccess},
		{ID: 3, Status: api.StatusFailed},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("failed")
	updated.syncPipeRows()

	if len(updated.filtered) != 2 {
		t.Fatalf("expected 2 failed pipelines, got %d: %v", len(updated.filtered), pipelineIDs(updated.filtered))
	}
}

func TestPipelineList_FilterMatchesProjectName(t *testing.T) {
	p := NewPipelineList()
	p.SetProjectNames(map[int]string{10: "backend/core-services", 11: "frontend/app"})
	p.SetPipelines([]api.Pipeline{
		{ID: 1, ProjectID: 10},
		{ID: 2, ProjectID: 11},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("backend")
	updated.syncPipeRows()

	if len(updated.filtered) != 1 || updated.filtered[0].ID != 1 {
		t.Fatalf("expected only the backend project's pipeline, got %v", pipelineIDs(updated.filtered))
	}
}

func TestPipelineList_HighlightedUsesFilteredIndex(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "main"},
		{ID: 2, Ref: "feature-x"},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("feature")
	updated.syncPipeRows()

	pl, ok := updated.HighlightedPipeline()
	if !ok || pl.ID != 2 {
		t.Fatalf("expected the filtered pipeline highlighted, got %+v ok=%v", pl, ok)
	}
}

func TestPipelineList_StagingSurvivesFilterChanges(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "main"},
		{ID: 2, Ref: "feature-x"},
	})
	p.Selected[1] = true

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("feature") // filters project 1 out of view
	updated.syncPipeRows()

	got := updated.SelectedPipelines()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected staged pipeline 1 to still be staged despite being filtered out of view, got %+v", got)
	}
}

func TestPipelineList_EscClearsFilterAndShowsEverythingAgain(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Ref: "main"}, {ID: 2, Ref: "feature-x"}})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("feature")
	updated.syncPipeRows()
	if len(updated.filtered) != 1 {
		t.Fatalf("expected filter to narrow to 1, got %d", len(updated.filtered))
	}

	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.filtering {
		t.Fatal("expected esc to exit filtering mode")
	}
	if len(updated.filtered) != 2 {
		t.Fatalf("expected esc to clear the filter and show both pipelines again, got %d", len(updated.filtered))
	}
}

func TestPipelineList_HasTextFocusWhileFiltering(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}})

	if p.HasTextFocus() {
		t.Fatal("should not have text focus initially")
	}
	updated, _ := p.Update(runeKey('/'))
	if !updated.HasTextFocus() {
		t.Fatal("expected text focus after '/'")
	}
}

func TestPipelineList_NewBatchResetsFilter(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Ref: "main"}})
	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("something")

	updated.SetPipelines([]api.Pipeline{{ID: 2, Ref: "main"}})

	if updated.filtering || updated.filterInput.Value() != "" {
		t.Fatalf("expected a fresh SetPipelines to reset the filter, got filtering=%v value=%q", updated.filtering, updated.filterInput.Value())
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

// TestPipelineList_SelectAllRespectsActiveFilter mirrors ProjectList's
// select-all-respects-filter test, one level down.
func TestPipelineList_SelectAllRespectsActiveFilter(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{
		{ID: 1, Ref: "main"},
		{ID: 2, Ref: "main"},
		{ID: 3, Ref: "feature-x"},
	})
	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("main")
	updated.syncPipeRows()
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter}) // exit filter text entry

	updated, _ = updated.Update(runeKey('a'))
	if len(updated.Selected) != 2 || !updated.Selected[1] || !updated.Selected[2] {
		t.Fatalf("expected only the 2 filtered pipelines staged, got %+v", updated.Selected)
	}
	if updated.Selected[3] {
		t.Fatal("expected the filtered-out pipeline to remain unstaged")
	}
}

// TestPipelineList_SelectAllJobsCoversFullJobSet covers the job-matrix
// variant, which has no filter of its own — 'a' should stage every job
// currently loaded.
func TestPipelineList_SelectAllJobsCoversFullJobSet(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}})
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10}, {ID: 20}, {ID: 30}})

	updated, _ := p.Update(runeKey('a'))
	if len(updated.SelectedJ) != 3 {
		t.Fatalf("expected all 3 jobs staged, got %+v", updated.SelectedJ)
	}
	updated, _ = updated.Update(runeKey('a'))
	if len(updated.SelectedJ) != 0 {
		t.Fatalf("expected all 3 jobs unstaged on second 'a', got %+v", updated.SelectedJ)
	}
}

// TestPipelineList_ToggleSelectDoesNotLeakCount and
// TestPipelineList_ToggleJobDoesNotLeakCount guard the same toggleSet fix
// at the pipeline-matrix and job-matrix call sites.
func TestPipelineList_ToggleSelectDoesNotLeakCount(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}})

	updated := p
	updated, _ = updated.Update(runeKey('x'))
	updated, _ = updated.Update(runeKey('x'))

	if len(updated.Selected) != 0 {
		t.Fatalf("expected Selected empty after toggle on/off, got %+v", updated.Selected)
	}
}

func TestPipelineList_ToggleJobDoesNotLeakCount(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1}})
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100}})

	updated := p
	updated, _ = updated.Update(runeKey('x'))
	updated, _ = updated.Update(runeKey('x'))

	if len(updated.SelectedJ) != 0 {
		t.Fatalf("expected SelectedJ empty after toggle on/off, got %+v", updated.SelectedJ)
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
