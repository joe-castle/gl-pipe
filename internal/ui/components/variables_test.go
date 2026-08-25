package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestVariables_AddEditRow(t *testing.T) {
	v := NewVariables()
	v.Open([]api.Project{{ID: 1, PathWithNamespace: "a/b", DefaultBranch: "main"}}, "main", nil)

	updated, _ := v.Update(runeKey('a'))
	if updated.editing != varEditKey {
		t.Fatalf("expected editing key after 'a', got %v", updated.editing)
	}

	for _, r := range "DEPLOY_ENV" {
		updated, _ = updated.Update(runeKey(r))
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.editing != varEditValue {
		t.Fatalf("expected editing value after enter, got %v", updated.editing)
	}

	for _, r := range "qa" {
		updated, _ = updated.Update(runeKey(r))
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(updated.rows) != 1 || updated.rows[0].Key != "DEPLOY_ENV" || updated.rows[0].Value != "qa" {
		t.Fatalf("unexpected rows: %+v", updated.rows)
	}
}

func TestVariables_DeleteRow(t *testing.T) {
	v := NewVariables()
	v.Open(nil, "main", []api.Variable{{Key: "A"}, {Key: "B"}})

	updated, _ := v.Update(runeKey('d'))
	if len(updated.rows) != 1 || updated.rows[0].Key != "B" {
		t.Fatalf("expected only B left, got %+v", updated.rows)
	}
}

func TestVariables_ToggleFlags(t *testing.T) {
	v := NewVariables()
	v.Open(nil, "main", []api.Variable{{Key: "K", Value: "V", Type: api.VarTypeEnv}})

	updated, _ := v.Update(runeKey('m'))
	if !updated.rows[0].Masked {
		t.Fatal("expected Masked toggled true")
	}
	updated, _ = updated.Update(runeKey('p'))
	if !updated.rows[0].Protected {
		t.Fatal("expected Protected toggled true")
	}
	updated, _ = updated.Update(runeKey('t'))
	if updated.rows[0].Type != api.VarTypeFile {
		t.Fatalf("expected type toggled to file, got %v", updated.rows[0].Type)
	}
}

func TestVariables_DispatchEmitsProjectsRefAndVars(t *testing.T) {
	v := NewVariables()
	projects := []api.Project{{ID: 1, PathWithNamespace: "a/b"}}
	v.Open(projects, "main", []api.Variable{{Key: "K", Value: "V"}})
	v.table.SetCursor(len(v.rows)) // the "[ Dispatch ]" sentinel row

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.Active {
		t.Fatal("expected modal to close on dispatch")
	}
	if cmd == nil {
		t.Fatal("expected a DispatchMsg cmd")
	}
	msg, ok := cmd().(DispatchMsg)
	if !ok {
		t.Fatalf("expected DispatchMsg, got %T", msg)
	}
	if len(msg.Projects) != 1 || msg.Ref != "main" || len(msg.Vars) != 1 {
		t.Fatalf("unexpected dispatch msg: %+v", msg)
	}
}

func TestVariables_CtrlREmitsRefPickerRequestForFirstProject(t *testing.T) {
	v := NewVariables()
	v.Open([]api.Project{{ID: 42}, {ID: 43}}, "main", nil)

	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("expected a RefPickerRequestMsg cmd")
	}
	msg, ok := cmd().(RefPickerRequestMsg)
	if !ok {
		t.Fatalf("expected RefPickerRequestMsg, got %T", msg)
	}
	if msg.ProjectID != 42 {
		t.Fatalf("expected the first staged project's ID, got %d", msg.ProjectID)
	}
}

func TestVariables_OpenRefPickerThenSelectSetsRef(t *testing.T) {
	v := NewVariables()
	v.Open([]api.Project{{ID: 1}}, "main", nil)
	v.OpenRefPicker([]api.Ref{
		{Name: "main", IsTag: false},
		{Name: "v1.2.3", IsTag: true},
	})

	updated, _ := v.Update(runeKey('j')) // move to v1.2.3
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if updated.refPickerActive {
		t.Fatal("expected the picker to close after selecting")
	}
	if updated.Ref != "v1.2.3" {
		t.Fatalf("expected Ref set to the selected ref, got %q", updated.Ref)
	}
	if updated.RefInput.Value() != "v1.2.3" {
		t.Fatalf("expected RefInput updated too, got %q", updated.RefInput.Value())
	}
}

func TestVariables_RefPickerEscCancelsWithoutChangingRef(t *testing.T) {
	v := NewVariables()
	v.Open([]api.Project{{ID: 1}}, "main", nil)
	v.OpenRefPicker([]api.Ref{{Name: "other-branch"}})

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.refPickerActive {
		t.Fatal("expected esc to close the picker")
	}
	if updated.Ref != "main" {
		t.Fatalf("expected Ref unchanged by esc, got %q", updated.Ref)
	}
}

func TestVariables_EscClosesWithoutDispatching(t *testing.T) {
	v := NewVariables()
	v.Open([]api.Project{{ID: 1}}, "main", nil)

	updated, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.Active {
		t.Fatal("expected esc to close the modal")
	}
	if cmd != nil {
		t.Fatal("esc should not emit a DispatchMsg")
	}
}
