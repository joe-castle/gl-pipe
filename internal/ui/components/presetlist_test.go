package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

func samplePresets() []PresetEntry {
	return []PresetEntry{
		{Name: "nightly", Ref: "main", Projects: []string{"backend/api", "backend/worker"},
			Vars: []api.Variable{{Key: "DEPLOY_ENV", Value: "dev", Type: api.VarTypeEnv}}},
		{Name: "vars-only", Vars: []api.Variable{{Key: "FOO", Value: "bar", Type: api.VarTypeEnv}}},
	}
}

func openPresetList(t *testing.T) PresetList {
	t.Helper()
	p := NewPresetList()
	p.Open(samplePresets())
	return p
}

// TestPresetList_EnterRunsARunnablePreset is the core of the one-tap ask:
// Enter on a preset that names its own projects fires it, with no
// intervening trigger modal.
func TestPresetList_EnterRunsARunnablePreset(t *testing.T) {
	p := openPresetList(t)

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a runnable preset produced no command")
	}
	msg, ok := cmd().(RunPresetMsg)
	if !ok {
		t.Fatalf("Enter emitted %T, want RunPresetMsg", cmd())
	}
	if msg.Name != "nightly" {
		t.Errorf("RunPresetMsg.Name = %q, want nightly", msg.Name)
	}
	if p.Active {
		t.Error("picker should close after firing a preset")
	}
}

// TestPresetList_EnterOnVariableOnlyPresetChoosesIt preserves the original
// <Space> v behavior for a preset with no projects: there is nothing to run
// it against, so it becomes the prefill for the next trigger modal instead.
func TestPresetList_EnterOnVariableOnlyPresetChoosesIt(t *testing.T) {
	p := openPresetList(t)

	p, _ = p.Update(runeKey('j'))
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter produced no command")
	}
	msg, ok := cmd().(PresetChosenMsg)
	if !ok {
		t.Fatalf("Enter on a variable-only preset emitted %T, want PresetChosenMsg", cmd())
	}
	if msg.Name != "vars-only" {
		t.Errorf("PresetChosenMsg.Name = %q, want vars-only", msg.Name)
	}
}

// TestPresetList_CChoosesEvenARunnablePreset gives a way to load a runnable
// preset into the trigger modal (to tweak a variable before firing) rather
// than dispatching it as-is.
func TestPresetList_CChoosesEvenARunnablePreset(t *testing.T) {
	p := openPresetList(t)

	_, cmd := p.Update(runeKey('c'))
	if cmd == nil {
		t.Fatal("c produced no command")
	}
	msg, ok := cmd().(PresetChosenMsg)
	if !ok {
		t.Fatalf("c emitted %T, want PresetChosenMsg", cmd())
	}
	if msg.Name != "nightly" {
		t.Errorf("PresetChosenMsg.Name = %q, want nightly", msg.Name)
	}
}

func TestPresetList_EscCloses(t *testing.T) {
	p := openPresetList(t)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Active {
		t.Error("esc should close the preset picker")
	}
}

// TestPresetList_EmptyDoesNothingOnEnter guards the no-presets-configured
// case: Enter on an empty list must not index out of range.
func TestPresetList_EmptyDoesNothingOnEnter(t *testing.T) {
	p := NewPresetList()
	p.Open(nil)

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("Enter on an empty preset list emitted %T, want nothing", cmd())
	}
	if !strings.Contains(p.View(), "no presets") {
		t.Errorf("empty preset list should say so, got:\n%s", p.View())
	}
}

// TestPresetList_ViewShowsProjectAndVariableCounts is what makes a one-tap
// fire safe to press: the row says how many projects it will hit before you
// hit it.
func TestPresetList_ViewShowsProjectAndVariableCounts(t *testing.T) {
	p := openPresetList(t)
	view := p.View()
	if !strings.Contains(view, "nightly") || !strings.Contains(view, "main") {
		t.Errorf("view missing preset name/ref:\n%s", view)
	}
	if !strings.Contains(view, "2") {
		t.Errorf("view missing the 2-project count:\n%s", view)
	}
}
