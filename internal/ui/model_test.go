package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui/components"
)

var errBoom = errors.New("boom")

func newTestModel(t *testing.T) Model {
	t.Helper()
	return newTestModelWithGroups(t, []string{"backend"})
}

func newTestModelWithGroups(t *testing.T, groups []string) Model {
	t.Helper()
	cfg := &config.Config{
		CurrentInstance: "test",
		Instances: map[string]config.Instance{
			"test": {URL: "http://127.0.0.1:1", Token: "glpat-test", DefaultGroups: groups},
		},
		Cache: config.CacheConfig{TTLMinutes: 60},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dir := t.TempDir()
	return New(ctx, cancel, cfg, filepath.Join(dir, "config.yaml"), dir)
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// TestUpdate_DropsStaleProjectsSyncedMsg exercises invariant #2: a response
// tagged with an older reqID than the pane's current generation must not
// clobber state.
func TestUpdate_DropsStaleProjectsSyncedMsg(t *testing.T) {
	m := newTestModel(t)
	m.genProjects = 5

	updated, _ := m.Update(projectsSyncedMsg{
		reqID: 3, instance: "test",
		projects: []api.Project{{ID: 1, Name: "stale"}},
	})
	mm := updated.(Model)
	if len(mm.cacheIdx.Projects) != 0 {
		t.Fatalf("stale message should not have updated cacheIdx, got %+v", mm.cacheIdx.Projects)
	}

	updated, _ = mm.Update(projectsSyncedMsg{
		reqID: 5, instance: "test",
		projects: []api.Project{{ID: 2, Name: "fresh"}},
	})
	mm = updated.(Model)
	if len(mm.cacheIdx.Projects) != 1 || mm.cacheIdx.Projects[0].Name != "fresh" {
		t.Fatalf("matching-generation message should have updated cacheIdx, got %+v", mm.cacheIdx.Projects)
	}
}

// TestSaveChosenGroups_MergesAdditivelyWithoutDuplicates exercises the
// discovery picker's save path: newly-chosen groups are appended to
// default_groups, existing ones are left alone and not duplicated.
func TestSaveChosenGroups_MergesAdditivelyWithoutDuplicates(t *testing.T) {
	m := newTestModelWithGroups(t, []string{"backend/core"})

	updated, _ := m.Update(components.GroupsChosenMsg{FullPaths: []string{"backend/core", "infrastructure"}})
	mm := updated.(Model)

	inst := mm.cfg.Instances["test"]
	if len(inst.DefaultGroups) != 2 {
		t.Fatalf("expected 2 default_groups (no duplicate), got %+v", inst.DefaultGroups)
	}
	found := map[string]bool{}
	for _, g := range inst.DefaultGroups {
		found[g] = true
	}
	if !found["backend/core"] || !found["infrastructure"] {
		t.Fatalf("expected both groups present, got %+v", inst.DefaultGroups)
	}
}

func TestHandleLeaderAction_GTriggersGroupsLoad(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(components.LeaderActionMsg{Key: "g"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd to load groups")
	}
	if mm.genGroups == 0 {
		t.Fatal("expected genGroups to be set")
	}
}

// TestUpdate_BlobSearchRequestActuallyFiresTheSearch is a regression test:
// components.BlobSearchRequestMsg was emitted by ProjectList but the root
// model had no case for it at all, so the message was silently dropped —
// pressing enter on the blob search form did nothing, with no error and no
// feedback, exactly as reported.
func TestUpdate_BlobSearchRequestActuallyFiresTheSearch(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(components.BlobSearchRequestMsg{Group: "backend", Query: "@SpringBootApplication"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd that actually runs the blob search")
	}
	if mm.genBlob == 0 {
		t.Fatal("expected genBlob to be set")
	}
	if !mm.loading {
		t.Fatal("expected loading=true while the search runs")
	}
}

// TestUpdate_BlobSearchResultsDropsStaleResponse guards genBlob's own
// generation check (it used to piggyback on genProjects, which a
// concurrent project sync could bump and silently swallow blob results).
func TestUpdate_BlobSearchResultsDropsStaleResponse(t *testing.T) {
	m := newTestModel(t)
	m.genBlob = 5

	updated, _ := m.Update(blobSearchResultsMsg{reqID: 3, hits: []api.BlobHit{{Path: "a.go"}}})
	mm := updated.(Model)
	if len(mm.projectList.BlobHits) != 0 {
		t.Fatalf("expected a stale-generation response to be dropped, got %+v", mm.projectList.BlobHits)
	}

	updated, _ = mm.Update(blobSearchResultsMsg{reqID: 5, hits: []api.BlobHit{{Path: "b.go"}}})
	mm = updated.(Model)
	if len(mm.projectList.BlobHits) != 1 || mm.projectList.BlobHits[0].Path != "b.go" {
		t.Fatalf("expected the matching-generation response to apply, got %+v", mm.projectList.BlobHits)
	}
}

// TestUpdate_RefPickerRequestFiresLoadAndOpensPicker covers the full
// round trip: Variables emits RefPickerRequestMsg, the model fires
// loadAllRefsCmd, and the resulting refPickerLoadedMsg opens the picker
// with the fetched refs.
func TestUpdate_RefPickerRequestFiresLoadAndOpensPicker(t *testing.T) {
	m := newTestModel(t)
	m.variables.Open([]api.Project{{ID: 7}}, "main", nil)

	updated, cmd := m.Update(components.RefPickerRequestMsg{ProjectID: 7})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd fetching branches+tags")
	}
	if mm.genRefPicker == 0 {
		t.Fatal("expected genRefPicker to be set")
	}

	updated, _ = mm.Update(refPickerLoadedMsg{
		reqID: mm.genRefPicker, projectID: 7,
		refs: []api.Ref{{Name: "main"}, {Name: "v1.0.0", IsTag: true}},
	})
	mm = updated.(Model)
	if !strings.Contains(mm.variables.View(), "v1.0.0") {
		t.Fatalf("expected the fetched refs to appear in the picker view, got:\n%s", mm.variables.View())
	}
}

func TestUpdate_RefPickerRequestWithNoProjectShowsStatus(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(components.RefPickerRequestMsg{ProjectID: 0})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no fetch Cmd when there's no project to browse refs for")
	}
	if mm.statusMsg == "" {
		t.Fatal("expected a status message explaining why nothing happened")
	}
}

// TestHandleKey_CtrlROpensExplorerRefOverride covers the explorer's own
// ctrl+r: unlike the trigger modal's ctrl+r (which opens the picker inside
// Variables), this one must route the fetched refs into ProjectList, and
// must apply the chosen ref to every staged project, not just the first.
func TestHandleKey_CtrlROpensExplorerRefOverride(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
	})
	m.projectList.Selected[10] = true
	m.projectList.Selected[11] = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd fetching branches+tags for the first staged project")
	}
	if mm.genRefPicker == 0 {
		t.Fatal("expected genRefPicker to be set")
	}
	if mm.refPickerFor != refPickerForExplorer {
		t.Fatalf("expected refPickerFor=refPickerForExplorer, got %v", mm.refPickerFor)
	}

	updated, _ = mm.Update(refPickerLoadedMsg{
		reqID: mm.genRefPicker, projectID: 10,
		refs: []api.Ref{{Name: "main"}, {Name: "hotfix/urgent"}},
	})
	mm = updated.(Model)

	// The trigger modal must NOT have been touched by this flow.
	if mm.variables.Active {
		t.Fatal("explorer ctrl+r should not open the trigger modal's picker")
	}

	pl, _ := mm.projectList.Update(runeKey('j')) // move to hotfix/urgent
	pl, _ = pl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pl.LockedRef[10] != "hotfix/urgent" || pl.LockedRef[11] != "hotfix/urgent" {
		t.Fatalf("expected both staged projects locked to hotfix/urgent, got %+v", pl.LockedRef)
	}
}

func TestHandleKey_CtrlRWithNoProjectSelectedShowsStatus(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no fetch Cmd with nothing to target")
	}
	if mm.statusMsg == "" {
		t.Fatal("expected a status message explaining why nothing happened")
	}
}

func TestHandleLeaderAction_BOpensRefSearch(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(components.LeaderActionMsg{Key: "b"})
	mm := updated.(Model)
	if !mm.refSearch.Active {
		t.Fatal("expected 'b' to open the ref search modal")
	}
}

func TestHandleLeaderAction_MTriggersMyMRsLoad(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(components.LeaderActionMsg{Key: "m"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd to load 'my MRs'")
	}
	if mm.genMRs == 0 {
		t.Fatal("expected genMRs to be set")
	}
}

// TestHandleKey_CapitalTLocksStagedProjectsToLatestTag is a regression
// test both for "T does nothing visible" (LockedRef used to be mutated
// directly, bypassing the table's row re-sync) and for the new
// staged-projects batch behavior.
func TestHandleKey_CapitalTLocksStagedProjectsToLatestTag(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
	})
	m.projectList.Selected[10] = true
	m.projectList.Selected[11] = true

	updated, cmd := m.Update(runeKey('T'))
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch loading tags for the staged projects")
	}
	if mm.refsPending != 2 {
		t.Fatalf("expected both staged projects queried, got refsPending=%d", mm.refsPending)
	}

	updated, _ = mm.Update(refsLoadedMsg{
		reqID: mm.genRefs, projectID: 10,
		refs: []api.Ref{{Name: "v1.2.3", IsTag: true}},
	})
	mm = updated.(Model)
	updated, _ = mm.Update(refsLoadedMsg{
		reqID: mm.genRefs, projectID: 11,
		refs: []api.Ref{{Name: "v2.0.0", IsTag: true}},
	})
	mm = updated.(Model)

	if mm.projectList.LockedRef[10] != "v1.2.3" || mm.projectList.LockedRef[11] != "v2.0.0" {
		t.Fatalf("expected both projects locked to their latest tag, got %+v", mm.projectList.LockedRef)
	}
	// The regression this guards: the Ref column must actually reflect it.
	if !strings.Contains(mm.projectList.View(), "v1.2.3") || !strings.Contains(mm.projectList.View(), "v2.0.0") {
		t.Fatalf("expected locked tags visible in the rendered view, got:\n%s", mm.projectList.View())
	}
}

// TestHandleKey_CapitalTUnlocksWhenAllStagedAlreadyLocked covers the
// toggle half: pressing T again when everything targeted is already
// locked unlocks it instead of re-fetching.
func TestHandleKey_CapitalTUnlocksWhenAllStagedAlreadyLocked(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{{ID: 10, PathWithNamespace: "a/b"}})
	m.projectList.SetLockedRef(10, "v1.0.0")

	updated, cmd := m.Update(runeKey('T'))
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no fetch Cmd when unlocking (no network call needed)")
	}
	if _, ok := mm.projectList.LockedRef[10]; ok {
		t.Fatalf("expected project 10 unlocked, got LockedRef=%+v", mm.projectList.LockedRef)
	}
}

// TestHandleKey_CapitalTUnlocksOnlyTheLockedOnesInAPartialBatch is a
// regression test for the exact bug reported: with a large staged batch,
// it's common for one or more projects to have no qualifying tag and so
// never get locked. Requiring every single targeted project to be locked
// before T would ever unlock meant such a batch could never be unlocked
// again via T. It should now unlock whatever *is* locked and leave the
// rest alone.
func TestHandleKey_CapitalTUnlocksOnlyTheLockedOnesInAPartialBatch(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"}, // never got a tag locked (e.g. no semver tags)
		{ID: 12, PathWithNamespace: "e/f"},
	})
	m.projectList.Selected[10] = true
	m.projectList.Selected[11] = true
	m.projectList.Selected[12] = true
	m.projectList.SetLockedRef(10, "v1.0.0")
	m.projectList.SetLockedRef(12, "v2.0.0")
	// project 11 deliberately left unlocked

	updated, cmd := m.Update(runeKey('T'))
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no fetch Cmd — with any project already locked, T should unlock, not re-fetch")
	}
	if _, ok := mm.projectList.LockedRef[10]; ok {
		t.Error("expected project 10 unlocked")
	}
	if _, ok := mm.projectList.LockedRef[12]; ok {
		t.Error("expected project 12 unlocked")
	}
	if _, ok := mm.projectList.LockedRef[11]; ok {
		t.Error("project 11 was never locked; it should not have gained a lock")
	}
}

// TestHandleKey_LowercaseTUsesLatestCreatedStrategy covers the second
// ask: a way to choose "latest created" over "latest SemVer" when picking
// which tag to lock to.
func TestHandleKey_LowercaseTUsesLatestCreatedStrategy(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{{ID: 10, PathWithNamespace: "a/b"}})

	updated, cmd := m.Update(runeKey('t'))
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd loading tags")
	}
	if mm.refLockMode != refLockLatestCreated {
		t.Fatalf("expected refLockMode=refLockLatestCreated, got %v", mm.refLockMode)
	}

	now := time.Now()
	updated, _ = mm.Update(refsLoadedMsg{
		reqID: mm.genRefs, projectID: 10,
		refs: []api.Ref{
			{Name: "not-semver-but-newest", IsTag: true, CreatedAt: now},
			{Name: "v9.9.9", IsTag: true, CreatedAt: now.Add(-time.Hour)},
		},
	})
	mm = updated.(Model)
	if mm.projectList.LockedRef[10] != "not-semver-but-newest" {
		t.Fatalf("expected the most-recently-created tag chosen regardless of SemVer validity, got %q", mm.projectList.LockedRef[10])
	}
}

// TestShouldPoll covers the periodic-refresh gate: only poll while the
// pipeline/job matrix is on screen, nothing else is mid-fetch, and
// something shown hasn't settled into a terminal status yet.
func TestShouldPoll(t *testing.T) {
	m := newTestModel(t)
	m.pane = panePipelines
	m.pipelineList.SetPipelines([]api.Pipeline{{ID: 1, Status: api.StatusRunning}})

	if !m.shouldPoll() {
		t.Error("expected shouldPoll true: on the pipelines pane with a running pipeline")
	}

	m.pane = paneExplorer
	if m.shouldPoll() {
		t.Error("expected shouldPoll false: not on the pipelines pane")
	}
	m.pane = panePipelines

	m.loading = true
	if m.shouldPoll() {
		t.Error("expected shouldPoll false: a fetch is already in flight")
	}
	m.loading = false

	m.pipelineList.SetPipelines([]api.Pipeline{{ID: 1, Status: api.StatusSuccess}})
	if m.shouldPoll() {
		t.Error("expected shouldPoll false: everything shown is terminal")
	}
}

// TestPollTick_AlwaysReschedules covers the self-rescheduling half of the
// poll loop (invariant #3's pattern): it must return a Cmd regardless of
// whether anything actually needed refreshing, or the loop dies silently.
func TestPollTick_AlwaysReschedules(t *testing.T) {
	m := newTestModel(t)
	m.pane = paneExplorer // shouldPoll() false

	_, cmd := m.pollTick()
	if cmd == nil {
		t.Fatal("expected pollTick to always return a reschedule Cmd, got nil")
	}
}

// TestRefreshActiveMatrix covers the shared refresh helper behind both the
// periodic poll and the manual 'r' key: nil when there's nothing loaded,
// non-nil once there is, in either pipeline or job mode.
func TestRefreshActiveMatrix(t *testing.T) {
	m := newTestModel(t)

	if cmd := m.refreshActiveMatrix(); cmd != nil {
		t.Error("expected nil with no pipelines loaded")
	}

	m.pipelineList.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})
	if cmd := m.refreshActiveMatrix(); cmd == nil {
		t.Error("expected a Cmd once pipelines are loaded")
	}

	m.pipelineList.ClearJobs()
	if cmd := m.refreshActiveMatrix(); cmd != nil {
		t.Error("expected nil in job mode before any jobs/pipelines are attached")
	}
	m.pipelineList.AddJobs(api.Pipeline{ID: 1, ProjectID: 10}, []api.Job{{ID: 100, ProjectID: 10}})
	if cmd := m.refreshActiveMatrix(); cmd == nil {
		t.Error("expected a Cmd once the job matrix has a pipeline attached")
	}
}

// TestUpdate_RefreshRequestMsgSetsStatusWhenSomethingToRefresh covers the
// manual 'r' key's end-to-end wiring through Update.
func TestUpdate_RefreshRequestMsgSetsStatusWhenSomethingToRefresh(t *testing.T) {
	m := newTestModel(t)
	m.pipelineList.SetPipelines([]api.Pipeline{{ID: 1, ProjectID: 10}})

	updated, cmd := m.Update(components.RefreshRequestMsg{})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a refresh Cmd")
	}
	if mm.statusMsg != "refreshing..." {
		t.Fatalf("statusMsg = %q, want %q", mm.statusMsg, "refreshing...")
	}
}

// TestUpdate_RefreshRequestMsgNoOpWhenNothingLoaded ensures pressing 'r'
// before anything's in the matrix doesn't fire a pointless request or
// clobber the status line.
func TestUpdate_RefreshRequestMsgNoOpWhenNothingLoaded(t *testing.T) {
	m := newTestModel(t)
	m.setStatus("previous status")

	updated, cmd := m.Update(components.RefreshRequestMsg{})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no Cmd with nothing loaded")
	}
	if mm.statusMsg != "previous status" {
		t.Fatalf("statusMsg = %q, want unchanged %q", mm.statusMsg, "previous status")
	}
}

// TestHandleKey_CapitalMOpensMRsForStagedProjects covers the per-project
// MR fetch: staging projects and pressing M should batch-fetch each
// project's MRs into the same modal, not just the highlighted one.
func TestHandleKey_CapitalMOpensMRsForStagedProjects(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
	})
	m.projectList.Selected[10] = true
	m.projectList.Selected[11] = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch loading MRs for the staged projects")
	}
	if mm.mrsPending != 2 {
		t.Fatalf("expected both staged projects queried, got mrsPending=%d", mm.mrsPending)
	}
}

// TestUpdate_ProjectMRsLoadedAccumulatesAndSummarizes mirrors the pipeline
// batch-completion test: merges across projects, one summary at the end.
func TestUpdate_ProjectMRsLoadedAccumulatesAndSummarizes(t *testing.T) {
	m := newTestModel(t)
	m.genMRs = 9
	m.mrsPending = 2

	updated, _ := m.Update(projectMRsLoadedMsg{reqID: 9, projectID: 10, mrs: []api.MergeRequest{{ID: 1, ProjectID: 10}}})
	mm := updated.(Model)
	if mm.statusMsg != "" {
		t.Fatalf("expected no status yet with a response outstanding, got %q", mm.statusMsg)
	}

	updated, _ = mm.Update(projectMRsLoadedMsg{reqID: 9, projectID: 11, mrs: []api.MergeRequest{{ID: 2, ProjectID: 11}}})
	mm = updated.(Model)
	if mm.mrsPending != 0 {
		t.Fatalf("expected mrsPending drained to 0, got %d", mm.mrsPending)
	}
	if mm.mrList.Count() != 2 {
		t.Fatalf("expected both projects' MRs merged, got %d", mm.mrList.Count())
	}
	if mm.statusMsg == "" {
		t.Fatal("expected a summary status once the batch completes")
	}
}

// TestUpdate_MRsChosenJumpsToPipelines is the payoff: choosing MR(s)
// reuses the exact same pipeline batch machinery as staged-project and
// ref-search pipeline views.
func TestUpdate_MRsChosenJumpsToPipelines(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.Update(components.MRsChosenMsg{MRs: []api.MergeRequest{
		{ID: 1, IID: 5, ProjectID: 10},
		{ID: 2, IID: 6, ProjectID: 11},
	}})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch loading the MRs' pipelines")
	}
	if mm.pipelinesPending != 2 {
		t.Fatalf("expected both MRs' pipelines queried, got pipelinesPending=%d", mm.pipelinesPending)
	}
}

// TestRefSearchSubmit_SearchesEveryCachedProjectNotJustStaged is the core
// of this feature: unlike Enter (which only looks at staged/highlighted
// projects), submitting a ref search must query every project currently in
// the cache, since the whole point is not having to already know which
// repo the ref belongs to.
func TestRefSearchSubmit_SearchesEveryCachedProjectNotJustStaged(t *testing.T) {
	m := newTestModel(t)
	m.cacheIdx.Projects = []api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
		{ID: 12, PathWithNamespace: "e/f"},
	}
	// deliberately nothing staged and nothing highlighted matters here

	updated, cmd := m.Update(refSearchSubmitMsg{ref: "feature/login-fix"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch searching every cached project")
	}
	if mm.pipelinesPending != 3 {
		t.Fatalf("expected all 3 cached projects queried, got pipelinesPending=%d", mm.pipelinesPending)
	}
	if mm.pipelineList.Count() != 0 {
		t.Fatalf("expected the matrix cleared at search start, got %d rows", mm.pipelineList.Count())
	}
}

func TestRefSearchSubmit_NoCachedProjectsShowsStatusInsteadOfSearching(t *testing.T) {
	m := newTestModel(t)
	m.cacheIdx.Projects = nil

	updated, cmd := m.Update(refSearchSubmitMsg{ref: "main"})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no search Cmd when there are no cached projects")
	}
	if mm.statusMsg == "" {
		t.Fatal("expected a status explaining nothing was searched")
	}
}

// TestUpdate_PipelinesLoadedSummarizesOnceBatchCompletes guards the
// per-response-flicker fix: with several projects in flight, the status
// line should only report a summary once every response has arrived, not
// on each individual (mostly-empty, for a ref search) response.
func TestUpdate_PipelinesLoadedSummarizesOnceBatchCompletes(t *testing.T) {
	m := newTestModel(t)
	m.genPipelines = 9
	m.pipelinesPending = 3

	updated, _ := m.Update(pipelinesLoadedMsg{reqID: 9, projectID: 10, pipelines: nil})
	mm := updated.(Model)
	if mm.statusMsg != "" {
		t.Fatalf("expected no status yet with responses still outstanding, got %q", mm.statusMsg)
	}

	updated, _ = mm.Update(pipelinesLoadedMsg{reqID: 9, projectID: 11, pipelines: []api.Pipeline{{ID: 1, ProjectID: 11}}})
	mm = updated.(Model)
	if mm.statusMsg != "" {
		t.Fatalf("expected still no status with 1 response outstanding, got %q", mm.statusMsg)
	}

	updated, _ = mm.Update(pipelinesLoadedMsg{reqID: 9, projectID: 12, err: errBoom})
	mm = updated.(Model)
	if mm.pipelinesPending != 0 {
		t.Fatalf("expected pipelinesPending drained to 0, got %d", mm.pipelinesPending)
	}
	if mm.statusMsg == "" {
		t.Fatal("expected a summary status once the batch completes")
	}
	if mm.pipelineList.Count() != 1 {
		t.Fatalf("expected the 1 successful project's pipeline in the matrix, got %d", mm.pipelineList.Count())
	}
}

// TestUpdate_DropsStaleJobsLoadedMsg mirrors the above for the jobs pane.
func TestUpdate_DropsStaleJobsLoadedMsg(t *testing.T) {
	m := newTestModel(t)
	m.genJobs = 9

	updated, _ := m.Update(jobsLoadedMsg{reqID: 1, pipelineID: 1, jobs: []api.Job{{ID: 1}}})
	mm := updated.(Model)
	if mm.loading {
		t.Fatal("a stale jobsLoadedMsg should be dropped before touching loading state")
	}
}

// TestHandleKey_EnterOnProjectLoadsPipelines is a regression test: Enter on
// a highlighted project in the explorer must actually drill into its
// pipelines. pipelinesForProjectCmd existed but was never wired to any key
// until this was reported as "nothing happens."
func TestHandleKey_EnterOnProjectLoadsPipelines(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{{ID: 42, PathWithNamespace: "backend/svc-a"}})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected Enter on a project to return a Cmd that loads its pipelines")
	}
	if mm.genPipelines == 0 {
		t.Fatal("expected genPipelines to be set")
	}
	if !mm.loading {
		t.Fatal("expected loading=true while pipelines load")
	}
}

// TestHandleKey_EnterOnStagedProjectsShowsAllTogether covers viewing
// several projects' pipelines in one matrix: staging with 'x' and pressing
// Enter should fetch and merge all of them, not just the highlighted one.
func TestHandleKey_EnterOnStagedProjectsShowsAllTogether(t *testing.T) {
	m := newTestModel(t)
	m.projectList.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
		{ID: 12, PathWithNamespace: "e/f"},
	})
	m.projectList.Selected[10] = true
	m.projectList.Selected[11] = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch loading pipelines for the staged projects")
	}
	if mm.pipelineList.Count() != 0 {
		t.Fatalf("expected the matrix cleared at batch start, got %d rows", mm.pipelineList.Count())
	}

	// Both staged projects' responses arrive (order-independent) and merge
	// into the same matrix; project 12 was never staged and never fetched.
	updated, _ = mm.Update(pipelinesLoadedMsg{
		reqID: mm.genPipelines, projectID: 11,
		pipelines: []api.Pipeline{{ID: 2, ProjectID: 11}},
	})
	mm = updated.(Model)
	updated, _ = mm.Update(pipelinesLoadedMsg{
		reqID: mm.genPipelines, projectID: 10,
		pipelines: []api.Pipeline{{ID: 1, ProjectID: 10}},
	})
	mm = updated.(Model)

	if got := mm.pipelineList.Count(); got != 2 {
		t.Fatalf("expected 2 pipelines merged from the 2 staged projects, got %d", got)
	}
}

// TestHandleKey_EnterOnStagedPipelinesShowsJobsForAll mirrors the
// multi-project pipeline view, one level down: staging pipelines (possibly
// from different projects) and pressing Enter should show all their jobs
// together in one matrix.
func TestHandleKey_EnterOnStagedPipelinesShowsJobsForAll(t *testing.T) {
	m := newTestModel(t)
	m.pane = panePipelines
	m.pipelineList.SetPipelines([]api.Pipeline{
		{ID: 1, ProjectID: 10},
		{ID: 2, ProjectID: 11},
	})
	m.pipelineList.Selected[1] = true
	m.pipelineList.Selected[2] = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a Cmd batch loading jobs for the staged pipelines")
	}

	updated, _ = mm.Update(jobsLoadedMsg{
		reqID: mm.genJobs, pipelineID: 1,
		jobs: []api.Job{{ID: 100, ProjectID: 10, PipelineID: 1}},
	})
	mm = updated.(Model)
	updated, _ = mm.Update(jobsLoadedMsg{
		reqID: mm.genJobs, pipelineID: 2,
		jobs: []api.Job{{ID: 200, ProjectID: 11, PipelineID: 2}},
	})
	mm = updated.(Model)

	if !mm.pipelineList.InJobs() {
		t.Fatal("expected job matrix mode after both responses")
	}
	if len(mm.pipelineList.Pipelines) != 2 {
		t.Fatalf("expected jobs merged from both staged pipelines, got %d pipelines recorded", len(mm.pipelineList.Pipelines))
	}
}

// TestUpdate_JobActionMsgReturnsRefreshCmd is a light regression check that
// pipelineID now threads through job retry/cancel responses (needed so the
// post-action refresh targets the right pipeline in a multi-pipeline view).
func TestUpdate_JobActionMsgReturnsRefreshCmd(t *testing.T) {
	m := newTestModel(t)
	m.genJobs = 5

	updated, cmd := m.Update(jobActionMsg{reqID: 5, projectID: 10, pipelineID: 1, jobID: 100})
	if cmd == nil {
		t.Fatal("expected a Cmd to refresh that pipeline's jobs")
	}
	_ = updated
}

// TestUpdate_PipelinesLoadedClearsOnANewBatchButAccumulatesWithinOne
// guards the two halves of the fix together: a fresh Enter must not show
// leftovers from an earlier, unrelated view, but responses belonging to
// the same batch (e.g. multiple staged projects) must merge, not clobber
// each other.
func TestUpdate_PipelinesLoadedClearsOnANewBatchButAccumulatesWithinOne(t *testing.T) {
	m := newTestModel(t)
	m.pipelineList.AddOrUpdate(api.Pipeline{ID: 1, ProjectID: 99}) // leftover from an earlier view
	m.genPipelines = 7

	updated, _ := m.Update(pipelinesLoadedMsg{
		reqID: 7, projectID: 42,
		pipelines: []api.Pipeline{{ID: 2, ProjectID: 42}},
	})
	mm := updated.(Model)

	// The stale project-99 row is still here: the message handler itself
	// doesn't clear anything, only starting a new batch does (see the
	// staged-projects test above and openPipelinesForSelected).
	if got := mm.pipelineList.Count(); got != 2 {
		t.Fatalf("expected the leftover row plus the new one to accumulate within this batch, got %d", got)
	}

	// A second response in the SAME batch (e.g. another staged project)
	// merges in rather than replacing.
	updated, _ = mm.Update(pipelinesLoadedMsg{
		reqID: 7, projectID: 43,
		pipelines: []api.Pipeline{{ID: 3, ProjectID: 43}},
	})
	mm = updated.(Model)
	if got := mm.pipelineList.Count(); got != 3 {
		t.Fatalf("expected 3 pipelines accumulated within the batch, got %d", got)
	}
}

// TestUpdate_NoDefaultGroupsShowsStatusAndDoesNotCache is a regression test:
// syncing with no default_groups configured must surface a clear status
// message and must NOT stamp cacheIdx.SyncedAt, or a subsequent launch
// would see a "fresh" empty cache and silently show nothing — no projects,
// no error, no explanation — exactly the bug this guards against.
func TestUpdate_NoDefaultGroupsShowsStatusAndDoesNotCache(t *testing.T) {
	m := newTestModelWithGroups(t, nil)
	m.genProjects = 1

	updated, _ := m.Update(projectsSyncedMsg{reqID: 1, instance: "test", projects: nil})
	mm := updated.(Model)

	if mm.statusMsg == "" || mm.statusErr {
		t.Fatalf("expected a neutral, non-error status message, got %q (err=%v)", mm.statusMsg, mm.statusErr)
	}
	if !mm.cacheIdx.SyncedAt.IsZero() {
		t.Fatal("an unconfigured sync must not stamp SyncedAt, or the empty cache looks permanently fresh")
	}
}

// TestInit_ResyncsWhenCachedProjectListIsEmpty is a regression test for the
// same bug from the other direction: even a cache file with a recent
// SyncedAt (e.g. written before this fix, or before default_groups was
// configured) must not block a resync while it holds zero projects.
func TestInit_ResyncsWhenCachedProjectListIsEmpty(t *testing.T) {
	m := newTestModel(t)
	m.cacheIdx.SyncedAt = time.Now() // looks maximally fresh
	m.cacheIdx.Projects = nil

	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init should resync when the cached project list is empty, regardless of TTL freshness")
	}
}

func TestUpdate_CtrlCReturnsQuitCmd(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a quit Cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

// TestHandleKey_SpaceOpensLeaderMenu exercises invariant #4's default case:
// with no modal active and no text input focused, <Space> opens the leader
// menu.
func TestHandleKey_SpaceOpensLeaderMenu(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(runeKey(' '))
	mm := updated.(Model)
	if !mm.leaderMenu.Active {
		t.Fatal("space should open the leader menu when nothing else has focus")
	}
}

// TestHandleKey_SpaceGoesToFilterInputWhenFocused exercises invariant #4's
// carve-out: <Space> must not open the leader menu while a text input
// (here, the explorer's fuzzy filter) has focus.
func TestHandleKey_SpaceGoesToFilterInputWhenFocused(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(runeKey('/')) // enters filter mode
	mm := updated.(Model)

	updated, _ = mm.Update(runeKey(' '))
	mm = updated.(Model)
	if mm.leaderMenu.Active {
		t.Fatal("space should not open the leader menu while the filter input is focused")
	}
}

// TestHandleKey_SpaceGoesToPipelineFilterInputWhenFocused mirrors the
// explorer carve-out for the pipeline matrix's own filter input.
func TestHandleKey_SpaceGoesToPipelineFilterInputWhenFocused(t *testing.T) {
	m := newTestModel(t)
	m.pane = panePipelines
	m.pipelineList.SetPipelines([]api.Pipeline{{ID: 1}})

	updated, _ := m.Update(runeKey('/'))
	mm := updated.(Model)

	updated, _ = mm.Update(runeKey(' '))
	mm = updated.(Model)
	if mm.leaderMenu.Active {
		t.Fatal("space should not open the leader menu while the pipeline filter input is focused")
	}
}

// TestHandleKey_ModalTakesPrecedenceOverLeaderMenu exercises invariant #4's
// top rule: an active modal is dispatched to before the leader menu, even
// if the leader menu also happens to be open.
func TestHandleKey_ModalTakesPrecedenceOverLeaderMenu(t *testing.T) {
	m := newTestModel(t)
	m.leaderMenu.Open()
	m.variables.Active = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.variables.Active {
		t.Fatal("esc should have closed the variables modal")
	}
	if !mm.leaderMenu.Active {
		t.Fatal("leader menu should be untouched since the modal had dispatch precedence")
	}
}

func TestBreadcrumb_ReflectsCurrentView(t *testing.T) {
	m := newTestModel(t)
	if got := m.breadcrumb(); got != "EXPLORER" {
		t.Errorf("breadcrumb() = %q, want EXPLORER", got)
	}

	m.pane = panePipelines
	if got := m.breadcrumb(); got != "PIPELINES" {
		t.Errorf("breadcrumb() = %q, want PIPELINES", got)
	}

	m.pipelineList.AddJobs(api.Pipeline{ID: 1}, nil)
	if got := m.breadcrumb(); got != "JOBS" {
		t.Errorf("breadcrumb() = %q, want JOBS", got)
	}

	m.pane = paneExplorer
	m.mrList.Active = true
	if got := m.breadcrumb(); got != "MERGE REQUESTS" {
		t.Errorf("breadcrumb() = %q, want MERGE REQUESTS", got)
	}
}

func TestHandleKey_TabSwitchesPane(t *testing.T) {
	m := newTestModel(t)
	if m.pane != paneExplorer {
		t.Fatalf("expected to start in paneExplorer, got %v", m.pane)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm := updated.(Model)
	if mm.pane != panePipelines {
		t.Fatalf("expected tab to switch to panePipelines, got %v", mm.pane)
	}
}
