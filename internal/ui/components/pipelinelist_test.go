package components

import (
	"strings"
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

// TestPipelineList_NewBatchResetsStaleStaging is a real user-reported bug:
// staging pipelines in one batch, then loading an entirely different batch
// (e.g. jumping to a downstream pipeline from a bridge job) left the old
// Selected IDs in place. Since SelectedPipelines() only falls back to the
// highlighted row when Selected is *empty*, a new batch whose pipeline IDs
// don't overlap with the stale staged set silently returned nothing at
// all — "pressing enter doesn't do anything" for the new pipeline.
func TestPipelineList_NewBatchResetsStaleStaging(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.Selected[1] = true

	p.SetPipelines([]api.Pipeline{{ID: 99, ProjectID: 20}})

	if len(p.Selected) != 0 {
		t.Fatalf("expected Selected reset on a fresh batch, got %+v", p.Selected)
	}
	targets := p.SelectedPipelines()
	if len(targets) != 1 || targets[0].ID != 99 {
		t.Fatalf("expected SelectedPipelines to fall back to the new highlighted pipeline, got %+v", targets)
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

// TestPipelineList_EnterOnBridgeJobOpensDownstreamPipeline covers the
// user-reported gap: deploy jobs that trigger a downstream pipeline have
// no log to stream, so Enter must jump to the downstream pipeline instead
// of emitting OpenLogsMsg.
func TestPipelineList_EnterOnBridgeJobOpensDownstreamPipeline(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.AddJobs(api.Pipeline{ID: 1, ProjectID: 10}, []api.Job{
		{ID: 100, ProjectID: 10, PipelineID: 1, IsBridge: true, DownstreamProjectID: 20, DownstreamPipelineID: 555},
	})

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a Cmd")
	}
	msg, ok := cmd().(OpenDownstreamPipelineMsg)
	if !ok {
		t.Fatalf("expected OpenDownstreamPipelineMsg, got %T", cmd())
	}
	if msg.ProjectID != 20 || msg.PipelineID != 555 {
		t.Fatalf("unexpected downstream reference: %+v", msg)
	}
}

// TestPipelineList_EnterOnBridgeJobWithNoDownstreamYet still emits
// OpenDownstreamPipelineMsg (with PipelineID 0) rather than silently doing
// nothing or trying to stream a log that doesn't exist — the root model
// decides how to report "not started yet".
func TestPipelineList_EnterOnBridgeJobWithNoDownstreamYet(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.AddJobs(api.Pipeline{ID: 1, ProjectID: 10}, []api.Job{
		{ID: 100, ProjectID: 10, PipelineID: 1, IsBridge: true},
	})

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg, ok := cmd().(OpenDownstreamPipelineMsg)
	if !ok {
		t.Fatalf("expected OpenDownstreamPipelineMsg, got %T", cmd())
	}
	if msg.PipelineID != 0 {
		t.Fatalf("expected PipelineID 0, got %d", msg.PipelineID)
	}
}

// TestPipelineList_EnterOnRegularJobStillOpensLogs is the non-regression
// check: ordinary jobs must keep streaming logs, not be swept into the
// bridge-job path.
func TestPipelineList_EnterOnRegularJobStillOpensLogs(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.AddJobs(api.Pipeline{ID: 1, ProjectID: 10}, []api.Job{{ID: 100, ProjectID: 10, PipelineID: 1}})

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := cmd().(OpenLogsMsg); !ok {
		t.Fatalf("expected OpenLogsMsg for a regular job, got %T", cmd())
	}
}

// TestJobRunnerCell covers the RUNNER column's dual purpose: the runner
// tag for a regular job, or a visible pointer to the downstream pipeline
// (with its status) for a trigger job — the whole point being that a
// deploy-trigger job is no longer invisible in the matrix.
func TestJobRunnerCell(t *testing.T) {
	regular := api.Job{RunnerTag: "runner-a"}
	if got := jobRunnerCell(regular); got != "runner-a" {
		t.Errorf("regular job cell = %q, want runner-a", got)
	}

	pending := api.Job{IsBridge: true}
	if got := jobRunnerCell(pending); got != "→ (pending)" {
		t.Errorf("pending bridge cell = %q, want → (pending)", got)
	}

	running := api.Job{IsBridge: true, DownstreamPipelineID: 555, DownstreamPipelineIID: 42, DownstreamStatus: api.StatusRunning}
	if got := jobRunnerCell(running); !strings.Contains(got, "#42") {
		t.Errorf("running bridge cell = %q, want it to mention #42", got)
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

func TestPipelineList_LowercaseRRequestsRefreshInPipelineMode(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})

	_, cmd := p.Update(runeKey('r'))
	if cmd == nil {
		t.Fatal("expected a Cmd, got nil")
	}
	if _, ok := cmd().(RefreshRequestMsg); !ok {
		t.Fatalf("expected RefreshRequestMsg, got %T", cmd())
	}
}

func TestPipelineList_LowercaseRRequestsRefreshInJobMode(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100, ProjectID: 10}})

	_, cmd := p.Update(runeKey('r'))
	if cmd == nil {
		t.Fatal("expected a Cmd, got nil")
	}
	if _, ok := cmd().(RefreshRequestMsg); !ok {
		t.Fatalf("expected RefreshRequestMsg, got %T", cmd())
	}
}

func TestPipelineList_NeedsPoll_TrueWhileAnyPipelineNonTerminal(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Status: api.StatusSuccess}, {ID: 2, Status: api.StatusRunning}})
	if !p.NeedsPoll() {
		t.Fatal("expected NeedsPoll true with one running pipeline")
	}
}

func TestPipelineList_NeedsPoll_FalseOnceEverythingTerminal(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Status: api.StatusSuccess}, {ID: 2, Status: api.StatusFailed}})
	if p.NeedsPoll() {
		t.Fatal("expected NeedsPoll false once all pipelines are terminal")
	}
}

func TestPipelineList_NeedsPoll_ChecksJobsInJobMode(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Status: api.StatusRunning}})
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 100, Status: api.StatusSuccess}})
	if p.NeedsPoll() {
		t.Fatal("expected NeedsPoll false in job mode when all jobs are terminal, even though the pipeline itself is running")
	}

	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 101, Status: api.StatusRunning}})
	if !p.NeedsPoll() {
		t.Fatal("expected NeedsPoll true once a running job is present")
	}
}

func TestPipelineList_AllPipelinesReturnsUnfilteredSet(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, Ref: "main"}, {ID: 2, Ref: "dev"}})
	p.filterInput.SetValue("main")
	p.syncPipeRows()

	if len(p.filtered) != 1 {
		t.Fatalf("test setup: expected filter to narrow to 1, got %d", len(p.filtered))
	}
	if got := p.AllPipelines(); len(got) != 2 {
		t.Fatalf("AllPipelines() len = %d, want 2 (unfiltered)", len(got))
	}
}

// TestPipelineList_SlashOpensFilterInJobMode covers the user request: the
// job matrix had no '/' filter at all, unlike the pipeline matrix.
func TestPipelineList_SlashOpensFilterInJobMode(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()

	updated, _ := p.Update(runeKey('/'))
	if !updated.filtering {
		t.Fatal("expected '/' to enter filtering mode in the job matrix")
	}
	if !updated.HasTextFocus() {
		t.Fatal("expected HasTextFocus true while filtering the job matrix")
	}
}

func TestPipelineList_JobFilterMatchesName(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "deploy-prod"},
		{ID: 11, Name: "unit-tests"},
		{ID: 12, Name: "deploy-staging"},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("deploy")
	updated.syncJobRows()

	if len(updated.jobFiltered) != 2 {
		t.Fatalf("expected 2 jobs matching 'deploy', got %d", len(updated.jobFiltered))
	}
}

func TestPipelineList_JobFilterMatchesStageAndStatus(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Stage: "deploy", Status: api.StatusFailed},
		{ID: 11, Stage: "test", Status: api.StatusSuccess},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("failed")
	updated.syncJobRows()

	if len(updated.jobFiltered) != 1 || updated.jobFiltered[0].ID != 10 {
		t.Fatalf("expected only the failed job, got %v", updated.jobFiltered)
	}
}

func TestPipelineList_JobFilterHighlightedUsesFilteredIndex(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit-tests"},
		{ID: 11, Name: "deploy-prod"},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("deploy")
	updated.syncJobRows()

	j, ok := updated.HighlightedJob()
	if !ok || j.ID != 11 {
		t.Fatalf("expected the filtered job highlighted, got %+v ok=%v", j, ok)
	}
}

// TestPipelineList_JobStagingSurvivesFilterChanges mirrors the pipeline
// matrix's equivalent: staging a job stays staged even if a later filter
// hides it from the visible rows.
func TestPipelineList_JobStagingSurvivesFilterChanges(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit-tests"},
		{ID: 11, Name: "deploy-prod"},
	})
	p.SelectedJ[10] = true

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("deploy")
	updated.syncJobRows()

	if !updated.SelectedJ[10] {
		t.Fatal("expected job 10 to stay staged even though the filter now hides it")
	}
}

// TestPipelineList_ClearJobsResetsFilter mirrors SetPipelines' equivalent
// reset — a fresh job batch shouldn't inherit stale filter text from a
// previous job view or from browsing pipelines.
func TestPipelineList_ClearJobsResetsFilter(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("something")

	updated.ClearJobs()

	if updated.filtering || updated.filterInput.Value() != "" {
		t.Fatalf("expected ClearJobs to reset the filter, got filtering=%v value=%q", updated.filtering, updated.filterInput.Value())
	}
}

func TestPipelineList_SelectAllJobsRespectsActiveFilter(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit-tests"},
		{ID: 11, Name: "deploy-prod"},
	})

	updated, _ := p.Update(runeKey('/'))
	updated.filterInput.SetValue("deploy")
	updated.syncJobRows()
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter}) // stop filtering, keep the query
	updated, _ = updated.Update(runeKey('a'))

	if len(updated.SelectedJ) != 1 || !updated.SelectedJ[11] {
		t.Fatalf("expected 'a' to stage only the filtered job (11), got %+v", updated.SelectedJ)
	}
}

// --- failure digest (backlog 036) ---

func TestPipelineList_FailedJobsFiltersToFailuresOnly(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit", Status: api.StatusFailed},
		{ID: 11, Name: "build", Status: api.StatusSuccess},
		{ID: 12, Name: "e2e", Status: api.StatusFailed},
		{ID: 13, Name: "deploy", Status: api.StatusManual},
	})

	failed := p.FailedJobs()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed jobs, got %d: %+v", len(failed), failed)
	}
	if failed[0].ID != 10 || failed[1].ID != 12 {
		t.Errorf("unexpected failed jobs: %+v", failed)
	}
}

func TestPipelineList_FailedJobsIgnoresTheTextFilter(t *testing.T) {
	// The digest acts on everything loaded, not just what a '/' filter
	// happens to be showing — otherwise a filter narrows what you fetch and
	// the panel is silently empty for rows you scroll to afterwards.
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit", Status: api.StatusFailed},
		{ID: 11, Name: "e2e", Status: api.StatusFailed},
	})
	p.filterInput.SetValue("unit")
	p.syncJobRows()

	if len(p.FailedJobs()) != 2 {
		t.Fatalf("expected both failed jobs regardless of the filter, got %+v", p.FailedJobs())
	}
}

func TestPipelineList_JobDigestSurvivesAPollRefreshButNotANewJobView(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{Lines: []string{"FAILED: boom"}})

	// The 10s auto-poll re-issues AddJobs for every shown pipeline; wiping
	// the digest there would make it vanish seconds after it arrived.
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusFailed}})
	if d, ok := p.JobDigestFor(10); !ok || len(d.Lines) != 1 {
		t.Fatalf("digest lost across a poll refresh: %+v ok=%v", d, ok)
	}

	// A fresh job view is a different set of jobs entirely.
	p.ClearJobs()
	if _, ok := p.JobDigestFor(10); ok {
		t.Fatal("digest survived ClearJobs")
	}
}

func TestPipelineList_CapitalEInJobModeRequestsTheDigest(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusFailed}})

	_, cmd := p.Update(runeKey('E'))
	if cmd == nil {
		t.Fatal("expected a Cmd for 'E'")
	}
	if _, ok := cmd().(FailureDigestRequestMsg); !ok {
		t.Fatalf("expected FailureDigestRequestMsg, got %T", cmd())
	}
}

func TestPipelineList_CapitalEDoesNothingInPipelineMode(t *testing.T) {
	p := NewPipelineList()
	p.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})

	if _, cmd := p.Update(runeKey('E')); cmd != nil {
		if _, ok := cmd().(FailureDigestRequestMsg); ok {
			t.Fatal("digest must not be reachable from the pipeline matrix")
		}
	}
}

func TestPipelineList_ViewShowsTheDigestForTheHighlightedJob(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Name: "unit", Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{Lines: []string{"FAILED: token_test.go:88", "expected 200, got 401"}})

	view := p.View()
	if !strings.Contains(view, "token_test.go:88") || !strings.Contains(view, "expected 200, got 401") {
		t.Fatalf("digest missing from the job view:\n%s", view)
	}
}

func TestPipelineList_ViewSaysSoWhenATraceHadNoErrorLine(t *testing.T) {
	// "fetched, nothing matched" must read differently from "not fetched",
	// or a job with an unhelpful trace looks like the digest never ran.
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Name: "unit", Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{})

	if !strings.Contains(p.View(), "no error line") {
		t.Fatalf("expected an explicit no-match message:\n%s", p.View())
	}
}

func TestPipelineList_ViewShowsADigestFetchError(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Name: "unit", Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{Err: "403 forbidden"})

	if !strings.Contains(p.View(), "403 forbidden") {
		t.Fatalf("expected the fetch error in the panel:\n%s", p.View())
	}
}

func TestPipelineList_ViewHasNoDigestPanelBeforeTheDigestRuns(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Name: "unit", Status: api.StatusFailed}})

	if strings.Contains(p.View(), "no error line") {
		t.Fatal("un-fetched job must not render the no-match message")
	}
}

// The job table shrinks to make room for the panel, but only once there is
// a panel to make room for — the matrix must not be permanently shorter for
// a feature you are not using.
func TestPipelineList_JobTableHeightOnlyShrinksWhileADigestExists(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusFailed}})
	full := p.jobTable.Height()

	p.SetJobDigest(10, JobDigest{Lines: []string{"boom"}})
	shrunk := p.jobTable.Height()
	if shrunk >= full {
		t.Fatalf("expected the job table to shrink for the panel: %d -> %d", full, shrunk)
	}

	p.ClearJobs()
	if p.jobTable.Height() != full {
		t.Fatalf("expected the height restored after ClearJobs: got %d, want %d", p.jobTable.Height(), full)
	}
}

// A bridge is not a job: GitLab has no trace endpoint for one, so asking
// for it is a guaranteed 404 reported to the user as a failed fetch. Enter
// on a bridge already opens the downstream pipeline for the same reason.
func TestPipelineList_FailedJobsExcludesBridges(t *testing.T) {
	p := NewPipelineList()
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "unit", Status: api.StatusFailed},
		{ID: 11, Name: "deploy", Status: api.StatusFailed, IsBridge: true},
	})

	failed := p.FailedJobs()
	if len(failed) != 1 || failed[0].ID != 10 {
		t.Fatalf("expected only the real job, got %+v", failed)
	}
}

// Without this the feature looks broken: you press E, the status line says
// it worked, and then every row you happen to be sitting on shows nothing.
func TestPipelineList_ViewHintsWhenTheHighlightedJobHasNoSummary(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{
		{ID: 10, Name: "build", Status: api.StatusSuccess},
		{ID: 11, Name: "unit", Status: api.StatusFailed},
	})
	p.SetJobDigest(11, JobDigest{Lines: []string{"FAILED: boom"}})

	// Cursor is on the passing job, which has no summary of its own.
	view := p.View()
	if !strings.Contains(view, "no summary for this job") {
		t.Fatalf("expected a hint that the digest ran but not for this row:\n%s", view)
	}
}

func TestPipelineList_JobTableKeepsAUsableHeightOnATinyTerminal(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(80, 8) // h-3-digestPanelHeight would go to zero or below
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{Lines: []string{"boom"}})

	if p.jobTable.Height() < 1 {
		t.Fatalf("job table height = %d, want at least 1", p.jobTable.Height())
	}
}

// The panel used to be rendered in helpDescStyle — the exact style of the
// help line directly beneath it — with no separator, so a working digest
// read as more help text and was reported as "I see nothing underneath".
func TestPipelineList_DigestPanelIsVisuallySeparatedFromTheHelpLine(t *testing.T) {
	p := NewPipelineList()
	p.SetSize(120, 30)
	p.ClearJobs()
	p.AddJobs(api.Pipeline{ID: 1}, []api.Job{{ID: 10, Name: "unit", Stage: "test", Status: api.StatusFailed}})
	p.SetJobDigest(10, JobDigest{Lines: []string{"FAILED: boom"}})

	view := p.View()
	if !strings.Contains(view, "─") {
		t.Fatalf("expected a separator rule above the panel:\n%s", view)
	}
	// Colour can't be asserted here: lipgloss degrades to plain text with no
	// TTY, so every style renders identically under `go test`. The rule
	// above is the part that survives, and it's what actually breaks the
	// panel out of the help line visually.
	rule := strings.Index(view, "─")
	summary := strings.Index(view, "FAILED: boom")
	if summary < rule {
		t.Fatalf("expected the summary below the rule, not above it:\n%s", view)
	}
}
