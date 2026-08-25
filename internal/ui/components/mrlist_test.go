package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestMRList_SelectAllRespectsActiveFilter(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{
		{ID: 1, Title: "Fix login bug", SourceBranch: "fix/login"},
		{ID: 2, Title: "Unrelated change", SourceBranch: "chore/deps"},
	})
	updated, _ := l.Update(runeKey('/'))
	updated.filterInput.SetValue("login")
	updated.applyFilter()
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter}) // exit filter text entry

	updated, _ = updated.Update(runeKey('a'))
	if len(updated.Selected) != 1 || !updated.Selected[1] {
		t.Fatalf("expected only the filtered MR staged, got %+v", updated.Selected)
	}
}

func TestMRList_ToggleSelectDoesNotLeakCount(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{{ID: 1, Title: "Fix login"}})

	updated := l
	updated, _ = updated.Update(runeKey('x'))
	updated, _ = updated.Update(runeKey('x'))

	if len(updated.Selected) != 0 {
		t.Fatalf("expected Selected empty after toggle on/off, got %+v", updated.Selected)
	}
}

func TestMRList_EnterUsesStagedOrHighlightedFallback(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{
		{ID: 1, ProjectID: 10, Title: "Fix login"},
		{ID: 2, ProjectID: 11, Title: "Add feature"},
	})

	// Nothing staged: enter should act on the highlighted MR.
	updated, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an MRsChosenMsg cmd via highlighted fallback")
	}
	msg, ok := cmd().(MRsChosenMsg)
	if !ok || len(msg.MRs) != 1 || msg.MRs[0].ID != 1 {
		t.Fatalf("expected fallback to highlighted MR 1, got %+v", msg)
	}
	if updated.Active {
		t.Fatal("expected the modal to close on dispatch")
	}
}

func TestMRList_EnterUsesStagedWhenPresent(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{
		{ID: 1, ProjectID: 10, Title: "Fix login"},
		{ID: 2, ProjectID: 11, Title: "Add feature"},
	})
	l.Selected[2] = true

	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg, ok := cmd().(MRsChosenMsg)
	if !ok || len(msg.MRs) != 1 || msg.MRs[0].ID != 2 {
		t.Fatalf("expected only staged MR 2, got %+v", msg)
	}
}

func TestMRList_FilterMatchesTitleAndBranch(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{
		{ID: 1, Title: "Fix login bug", SourceBranch: "fix/login"},
		{ID: 2, Title: "Unrelated change", SourceBranch: "chore/deps"},
	})

	updated, _ := l.Update(runeKey('/'))
	updated.filterInput.SetValue("login")
	updated.applyFilter()

	if len(updated.filtered) != 1 || updated.filtered[0].ID != 1 {
		t.Fatalf("expected only the login MR to match, got %+v", updated.filtered)
	}
}

func TestMRList_AddMRsUpsertsByID(t *testing.T) {
	l := NewMRList()
	l.SetMRs(nil)

	l.AddMRs([]api.MergeRequest{{ID: 1, Title: "v1"}})
	l.AddMRs([]api.MergeRequest{{ID: 1, Title: "v2"}, {ID: 2, Title: "other"}})

	if len(l.all) != 2 {
		t.Fatalf("expected 2 MRs after upsert, got %d: %+v", len(l.all), l.all)
	}
	for _, mr := range l.all {
		if mr.ID == 1 && mr.Title != "v2" {
			t.Fatalf("expected MR 1 patched to v2, got %+v", mr)
		}
	}
}

func TestMRList_HasTextFocusWhileFiltering(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{{ID: 1}})

	if l.HasTextFocus() {
		t.Fatal("should not have text focus initially")
	}
	updated, _ := l.Update(runeKey('/'))
	if !updated.HasTextFocus() {
		t.Fatal("expected text focus after '/'")
	}
}

func TestMRList_EscClosesWithoutDispatching(t *testing.T) {
	l := NewMRList()
	l.SetMRs([]api.MergeRequest{{ID: 1}})

	updated, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.Active {
		t.Fatal("expected esc to close the modal")
	}
	if cmd != nil {
		t.Fatal("esc should not emit MRsChosenMsg")
	}
}
