package ui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/config"
)

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
