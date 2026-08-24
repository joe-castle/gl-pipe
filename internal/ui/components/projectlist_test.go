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
