package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestProjectList_FilterRanksBestMatch(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 1, PathWithNamespace: "frontend/unrelated"},
		{ID: 2, PathWithNamespace: "backend/core-services"},
	})
	p.filterInput.SetValue("core-services")
	p.applyFilter()

	highlighted, ok := p.Highlighted()
	if !ok || highlighted.ID != 2 {
		t.Fatalf("expected project 2 highlighted first, got %+v ok=%v", highlighted, ok)
	}
}

func TestProjectList_ToggleSelect(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: "a/b"}})

	updated, _ := p.Update(runeKey('x'))
	if !updated.Selected[1] {
		t.Fatal("expected project 1 selected after 'x'")
	}

	updated, _ = updated.Update(runeKey('x'))
	if updated.Selected[1] {
		t.Fatal("expected project 1 deselected after second 'x'")
	}
}

// TestProjectList_HighlightedRecoversAfterEmptyToNonEmpty is a regression
// test for a bubbles/table quirk: SetRows only clamps the cursor downward,
// so a table that starts with 0 rows (cursor forced to -1) and later gets
// its first real rows is left with an invalid cursor forever — Highlighted
// (and therefore every "Enter on the highlighted row" action) silently
// fails from then on. This is exactly what "Enter does nothing" looked
// like: the project list starts empty before the first sync completes.
func TestProjectList_HighlightedRecoversAfterEmptyToNonEmpty(t *testing.T) {
	p := NewProjectList()
	p.SetProjects(nil) // simulates the pre-sync empty state

	p.SetProjects([]api.Project{{ID: 42, PathWithNamespace: "backend/svc-a"}})

	proj, ok := p.Highlighted()
	if !ok || proj.ID != 42 {
		t.Fatalf("expected project 42 highlighted after going from empty to non-empty, got %+v ok=%v", proj, ok)
	}
}

// TestProjectList_SetLockedRefUpdatesVisibleRefColumn is a regression test:
// LockedRef used to be mutated directly from the root model (a plain map
// write on an exported field), which updated the data but never rebuilt
// the table's cached row strings — so the Ref column never visibly
// changed on screen even though the lock had genuinely taken. SetLockedRef/
// ClearLockedRef fix this by re-syncing rows as part of the mutation.
func TestProjectList_SetLockedRefUpdatesVisibleRefColumn(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: "a/b"}})

	p.SetLockedRef(1, "v2.3.4")
	if !strings.Contains(p.View(), "v2.3.4") {
		t.Fatalf("expected the locked ref visible in the rendered view, got:\n%s", p.View())
	}

	p.ClearLockedRef(1)
	if strings.Contains(p.View(), "v2.3.4") {
		t.Fatalf("expected the ref cleared from the rendered view, got:\n%s", p.View())
	}
}

// TestProjectList_SelectAllRespectsActiveFilter is the core of the
// select-all feature: 'a' must stage only what's currently visible after a
// '/' filter, not the whole underlying project list.
func TestProjectList_SelectAllRespectsActiveFilter(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 1, PathWithNamespace: "backend/svc-a"},
		{ID: 2, PathWithNamespace: "backend/svc-b"},
		{ID: 3, PathWithNamespace: "frontend/app"},
	})
	p.filterInput.SetValue("backend")
	p.applyFilter()

	updated, _ := p.Update(runeKey('a'))
	if len(updated.Selected) != 2 || !updated.Selected[1] || !updated.Selected[2] {
		t.Fatalf("expected only the 2 filtered (backend) projects staged, got %+v", updated.Selected)
	}
	if updated.Selected[3] {
		t.Fatal("expected the filtered-out project to remain unstaged")
	}
}

// TestProjectList_SelectAllTogglesOffWhenAllAlreadyStaged mirrors a
// checkbox's tri-state behavior: pressing 'a' again when everything
// visible is already staged unstages it instead of being a no-op.
func TestProjectList_SelectAllTogglesOffWhenAllAlreadyStaged(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 1, PathWithNamespace: "a/b"},
		{ID: 2, PathWithNamespace: "c/d"},
	})

	updated, _ := p.Update(runeKey('a'))
	if len(updated.Selected) != 2 {
		t.Fatalf("expected both staged after first 'a', got %+v", updated.Selected)
	}
	updated, _ = updated.Update(runeKey('a'))
	if len(updated.Selected) != 0 {
		t.Fatalf("expected both unstaged after second 'a', got %+v", updated.Selected)
	}
}

// TestProjectList_ToggleSelectDoesNotLeakCount is a regression test: 'x'
// used to set Selected[id] = false rather than delete the key, so the map
// entry (and therefore len(Selected), the "staged" count shown to the
// user) never went back down after toggling the same project off.
func TestProjectList_ToggleSelectDoesNotLeakCount(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: "a/b"}})

	updated := p
	for i := 0; i < 5; i++ {
		updated, _ = updated.Update(runeKey('x')) // on
		updated, _ = updated.Update(runeKey('x')) // off
	}

	if len(updated.Selected) != 0 {
		t.Fatalf("expected Selected empty after repeated toggle on/off, got %d entries: %+v", len(updated.Selected), updated.Selected)
	}
}

// TestProjectList_SelectedProjectsFallsBackAfterFullUnstage guards the
// knock-on effect of the same bug: SelectedProjects()'s "staged, or
// highlighted fallback" convention relies on len(Selected) == 0 meaning
// "nothing staged" — with the leaked-key bug, unstaging the only staged
// project left a stale false entry, len(Selected) stayed 1, the fallback
// never engaged, and the filtered result was silently empty.
func TestProjectList_SelectedProjectsFallsBackAfterFullUnstage(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: "a/b"}})

	updated, _ := p.Update(runeKey('x'))      // stage
	updated, _ = updated.Update(runeKey('x')) // unstage

	got := updated.SelectedProjects()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected fallback to the highlighted project after full unstage, got %+v", got)
	}
}

func TestProjectList_SelectedProjectsFallsBackToHighlighted(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 1, PathWithNamespace: "a/b"},
		{ID: 2, PathWithNamespace: "c/d"},
	})

	got := p.SelectedProjects()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected fallback to highlighted project 1, got %+v", got)
	}
}

func TestProjectList_SelectedProjectsUsesStagedWhenPresent(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 1, PathWithNamespace: "a/b"},
		{ID: 2, PathWithNamespace: "c/d"},
	})
	p.Selected[2] = true

	got := p.SelectedProjects()
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("expected staged project 2, got %+v", got)
	}
}

func TestProjectList_HasTextFocus(t *testing.T) {
	p := NewProjectList()
	if p.HasTextFocus() {
		t.Fatal("should not have text focus initially")
	}
	updated, _ := p.Update(runeKey('/'))
	if !updated.HasTextFocus() {
		t.Fatal("should have text focus after '/'")
	}
}

func TestProjectList_BlobSearchUsesSeparateGroupAndQueryFields(t *testing.T) {
	p := NewProjectList()
	p.OpenBlobSearch()
	p.groupInput.SetValue("backend")
	p.queryInput.SetValue("@SpringBootApplication")

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a BlobSearchRequestMsg cmd")
	}
	msg, ok := cmd().(BlobSearchRequestMsg)
	if !ok {
		t.Fatalf("expected BlobSearchRequestMsg, got %T", msg)
	}
	if msg.Group != "backend" || msg.Query != "@SpringBootApplication" {
		t.Fatalf("got Group=%q Query=%q", msg.Group, msg.Query)
	}
}

// TestProjectList_BlobSearchQueryWithPathQualifierIsNotMisreadAsGroup is a
// regression test for the original single-field design: splitting on the
// first ':' broke as soon as the query itself contained GitLab's own
// path:/filename:/extension: search qualifiers, since those also use ':'.
// Two separate fields sidestep the ambiguity entirely.
func TestProjectList_BlobSearchQueryWithPathQualifierIsNotMisreadAsGroup(t *testing.T) {
	p := NewProjectList()
	p.OpenBlobSearch()
	p.groupInput.SetValue("backend")
	p.queryInput.SetValue("path:src/main extension:java @SpringBootApplication")

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(BlobSearchRequestMsg)
	if msg.Group != "backend" {
		t.Fatalf("expected Group=backend (not misread from the query's ':'), got %q", msg.Group)
	}
	if msg.Query != "path:src/main extension:java @SpringBootApplication" {
		t.Fatalf("expected the query passed through unmodified, got %q", msg.Query)
	}
}

func TestProjectList_BlobSearchTabSwitchesFieldFocus(t *testing.T) {
	p := NewProjectList()
	p.DefaultGroup = "backend" // starts focus on the query field
	p.OpenBlobSearch()
	if p.blobOnGroup {
		t.Fatal("expected focus to start on the query field when DefaultGroup is set")
	}

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !updated.blobOnGroup {
		t.Fatal("expected tab to switch focus to the group field")
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.blobOnGroup {
		t.Fatal("expected a second tab to switch focus back to the query field")
	}
}

func TestProjectList_BlobSearchOpenPrefillsDefaultGroup(t *testing.T) {
	p := NewProjectList()
	p.DefaultGroup = "core-services"
	p.OpenBlobSearch()
	p.queryInput.SetValue("TODO")

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(BlobSearchRequestMsg)
	if msg.Group != "core-services" || msg.Query != "TODO" {
		t.Fatalf("got Group=%q Query=%q", msg.Group, msg.Query)
	}
}

// TestProjectList_RefOverrideAppliesToEveryPreparedTarget covers the
// per-project ref override: PrepareRefOverride records the batch (staged,
// or highlighted fallback — set by the caller, mirroring T/t), then
// OpenRefOverridePicker + a selection locks every one of them to the
// chosen ref in a single pick.
func TestProjectList_RefOverrideAppliesToEveryPreparedTarget(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{
		{ID: 10, PathWithNamespace: "a/b"},
		{ID: 11, PathWithNamespace: "c/d"},
	})
	p.PrepareRefOverride([]int{10, 11})
	p.OpenRefOverridePicker([]api.Ref{
		{Name: "main", IsTag: false},
		{Name: "hotfix/urgent", IsTag: false},
	})

	updated, _ := p.Update(runeKey('j')) // move to hotfix/urgent
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if updated.LockedRef[10] != "hotfix/urgent" || updated.LockedRef[11] != "hotfix/urgent" {
		t.Fatalf("expected both targets locked to hotfix/urgent, got %+v", updated.LockedRef)
	}
	if updated.mode != modeBrowse {
		t.Fatalf("expected picker to close back to browse mode, got mode=%v", updated.mode)
	}
}

func TestProjectList_RefOverrideEscCancelsWithoutLocking(t *testing.T) {
	p := NewProjectList()
	p.SetProjects([]api.Project{{ID: 10, PathWithNamespace: "a/b"}})
	p.PrepareRefOverride([]int{10})
	p.OpenRefOverridePicker([]api.Ref{{Name: "main"}})

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := updated.LockedRef[10]; ok {
		t.Fatalf("expected no lock after esc, got %+v", updated.LockedRef)
	}
	if updated.mode != modeBrowse {
		t.Fatalf("expected esc to return to browse mode, got mode=%v", updated.mode)
	}
}

// TestProjectList_HasTextFocusDuringRefPicker: <Space> must not escape the
// picker to open the leader menu, even though the picker takes j/k/enter,
// not literal text.
func TestProjectList_HasTextFocusDuringRefPicker(t *testing.T) {
	p := NewProjectList()
	p.OpenRefOverridePicker([]api.Ref{{Name: "main"}})
	if !p.HasTextFocus() {
		t.Fatal("expected HasTextFocus true while the ref picker overlay is open")
	}
}
