package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestGroupPicker_OpenPreChecksAlreadyConfigured(t *testing.T) {
	g := NewGroupPicker()
	groups := []api.Group{
		{ID: 1, Name: "Core", FullPath: "backend/core"},
		{ID: 2, Name: "Infra", FullPath: "infrastructure"},
	}
	g.Open(groups, []string{"infrastructure"})

	if !g.Selected["infrastructure"] {
		t.Fatal("expected already-configured group pre-checked")
	}
	if g.Selected["backend/core"] {
		t.Fatal("expected non-configured group left unchecked")
	}
}

func TestGroupPicker_ToggleSelect(t *testing.T) {
	g := NewGroupPicker()
	g.Open([]api.Group{{ID: 1, Name: "Core", FullPath: "backend/core"}}, nil)

	updated, _ := g.Update(runeKey('x'))
	if !updated.Selected["backend/core"] {
		t.Fatal("expected group selected after 'x'")
	}
	updated, _ = updated.Update(runeKey('x'))
	if updated.Selected["backend/core"] {
		t.Fatal("expected group deselected after second 'x'")
	}
}

func TestGroupPicker_FilterNarrowsResults(t *testing.T) {
	g := NewGroupPicker()
	g.Open([]api.Group{
		{ID: 1, Name: "Core", FullPath: "backend/core"},
		{ID: 2, Name: "Unrelated", FullPath: "frontend/unrelated"},
	}, nil)

	updated, _ := g.Update(runeKey('/'))
	updated.filterInput.SetValue("backend")
	updated.applyFilter()

	if len(updated.filtered) != 1 || updated.filtered[0].FullPath != "backend/core" {
		t.Fatalf("expected only backend/core to match, got %+v", updated.filtered)
	}
}

func TestGroupPicker_SaveEmitsOnlySelectedPaths(t *testing.T) {
	g := NewGroupPicker()
	g.Open([]api.Group{
		{ID: 1, Name: "Core", FullPath: "backend/core"},
		{ID: 2, Name: "Infra", FullPath: "infrastructure"},
	}, nil)

	updated, _ := g.Update(runeKey('x')) // select backend/core (cursor starts at row 0)
	updated.table.SetCursor(len(updated.filtered))

	final, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if final.Active {
		t.Fatal("expected picker to close on save")
	}
	if cmd == nil {
		t.Fatal("expected a GroupsChosenMsg cmd")
	}
	msg, ok := cmd().(GroupsChosenMsg)
	if !ok {
		t.Fatalf("expected GroupsChosenMsg, got %T", msg)
	}
	if len(msg.FullPaths) != 1 || msg.FullPaths[0] != "backend/core" {
		t.Fatalf("unexpected saved paths: %+v", msg.FullPaths)
	}
}

func TestGroupPicker_HasTextFocusWhileFiltering(t *testing.T) {
	g := NewGroupPicker()
	g.Open([]api.Group{{ID: 1, FullPath: "a"}}, nil)

	if g.HasTextFocus() {
		t.Fatal("should not have text focus initially")
	}
	updated, _ := g.Update(runeKey('/'))
	if !updated.HasTextFocus() {
		t.Fatal("expected text focus after '/'")
	}
}

func TestGroupPicker_EscClosesWithoutSaving(t *testing.T) {
	g := NewGroupPicker()
	g.Open([]api.Group{{ID: 1, FullPath: "a"}}, nil)

	updated, cmd := g.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.Active {
		t.Fatal("expected esc to close the picker")
	}
	if cmd != nil {
		t.Fatal("esc should not emit a GroupsChosenMsg")
	}
}
