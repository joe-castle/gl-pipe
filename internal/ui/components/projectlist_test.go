package components

import (
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

func TestProjectList_BlobSearchParsesGroupPrefix(t *testing.T) {
	p := NewProjectList()
	p.OpenBlobSearch()
	p.blobInput.SetValue("backend: @SpringBootApplication")

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

func TestProjectList_BlobSearchFallsBackToDefaultGroup(t *testing.T) {
	p := NewProjectList()
	p.DefaultGroup = "core-services"
	p.OpenBlobSearch()
	p.blobInput.SetValue("TODO")

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(BlobSearchRequestMsg)
	if msg.Group != "core-services" || msg.Query != "TODO" {
		t.Fatalf("got Group=%q Query=%q", msg.Group, msg.Query)
	}
}
