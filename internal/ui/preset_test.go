package ui

import (
	"strings"
	"testing"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/cache"
	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui/components"
)

// modelWithProjects builds a model whose project cache is already populated,
// which is what a preset's path_with_namespace entries resolve against.
func modelWithProjects(t *testing.T, projects ...api.Project) Model {
	t.Helper()
	m := newTestModel(t)
	m.cacheIdx = &cache.Index{Instance: "test", Projects: projects}
	m.projectList.SetProjects(projects)
	m.rebuildProjectNames()
	return m
}

var presetProjects = []api.Project{
	{ID: 1, Name: "api", PathWithNamespace: "backend/api", DefaultBranch: "develop"},
	{ID: 2, Name: "worker", PathWithNamespace: "backend/worker", DefaultBranch: "main"},
}

// TestPresetTargets_ResolvesPathsAndReportsMisses covers the chosen storage
// strategy: presets name projects by path, resolved against the synced
// cache, and a path that no longer resolves is reported rather than
// silently dropped or allowed to block the run.
func TestPresetTargets_ResolvesPathsAndReportsMisses(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)

	found, missing := m.presetTargets(config.Preset{
		Projects: []string{"backend/api", "backend/gone", "backend/worker"},
	})
	if len(found) != 2 || found[0].ID != 1 || found[1].ID != 2 {
		t.Fatalf("found = %+v, want the two cached projects in preset order", found)
	}
	if len(missing) != 1 || missing[0] != "backend/gone" {
		t.Fatalf("missing = %+v, want [backend/gone]", missing)
	}
}

func TestPresetRefFor_FallsBackToProjectDefaultBranch(t *testing.T) {
	explicit := presetRefFor(config.Preset{Ref: "release/1.2"}, presetProjects[0])
	if explicit != "release/1.2" {
		t.Errorf("ref = %q, want the preset's own ref to win", explicit)
	}
	fallback := presetRefFor(config.Preset{}, presetProjects[0])
	if fallback != "develop" {
		t.Errorf("ref = %q, want the project's default branch when the preset names none", fallback)
	}
	last := presetRefFor(config.Preset{}, api.Project{ID: 9})
	if last != "main" {
		t.Errorf("ref = %q, want main as the last-resort fallback", last)
	}
}

// TestRunPreset_FiresWithoutOpeningTheTriggerModal is the one-tap
// requirement: a runnable preset dispatches straight through, no modal.
func TestRunPreset_FiresWithoutOpeningTheTriggerModal(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	m.cfg.SetPreset("nightly", config.Preset{
		Ref:       "main",
		Projects:  []string{"backend/api", "backend/worker"},
		Variables: map[string]string{"DEPLOY_ENV": "dev"},
	})

	updated, cmd := m.Update(components.RunPresetMsg{Name: "nightly"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("running a preset produced no command")
	}
	if mm.variables.Active {
		t.Error("one-tap run must not open the trigger modal")
	}
	if !mm.loading {
		t.Error("expected loading to be set while the batch is in flight")
	}
	if !strings.Contains(mm.statusMsg, "nightly") || !strings.Contains(mm.statusMsg, "2") {
		t.Errorf("status = %q, want it to name the preset and the project count", mm.statusMsg)
	}
}

// TestRunPreset_WarnsOnMissingProjectsButStillRuns is the "paths, warn on
// miss" decision: one renamed repo must not block the rest of the preset.
func TestRunPreset_WarnsOnMissingProjectsButStillRuns(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	m.cfg.SetPreset("nightly", config.Preset{Projects: []string{"backend/api", "backend/gone"}})

	updated, cmd := m.Update(components.RunPresetMsg{Name: "nightly"})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("expected the resolvable project to still be triggered")
	}
	if !strings.Contains(mm.statusMsg, "backend/gone") {
		t.Errorf("status = %q, want it to name the unresolved project", mm.statusMsg)
	}
}

func TestRunPreset_NothingResolvableDoesNotDispatch(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	m.cfg.SetPreset("nightly", config.Preset{Projects: []string{"nope/one", "nope/two"}})

	updated, cmd := m.Update(components.RunPresetMsg{Name: "nightly"})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no dispatch when no project resolves")
	}
	if !mm.statusErr {
		t.Errorf("status = %q, want an error status when nothing resolves", mm.statusMsg)
	}
}

func TestRunPreset_UnknownPresetIsAnError(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	updated, cmd := m.Update(components.RunPresetMsg{Name: "does-not-exist"})
	mm := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no dispatch for an unknown preset")
	}
	if !mm.statusErr {
		t.Error("expected an error status for an unknown preset")
	}
}

// TestPresetChosen_PrefillsTheNextTriggerModal preserves the pre-existing
// <Space> v → <Space> p flow for variables-only presets.
func TestPresetChosen_PrefillsTheNextTriggerModal(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	m.cfg.SetPreset("vars", config.Preset{Variables: map[string]string{"DEPLOY_ENV": "dev"}})

	updated, _ := m.Update(components.PresetChosenMsg{Name: "vars"})
	mm := updated.(Model)

	updated, _ = mm.handleLeaderAction("p")
	mm = updated.(Model)
	if !mm.variables.Active {
		t.Fatal("<space> p should have opened the trigger modal")
	}
	if !strings.Contains(mm.variables.View(), "DEPLOY_ENV") {
		t.Errorf("trigger modal did not prefill from the chosen preset:\n%s", mm.variables.View())
	}
}

// TestLeaderV_OpensThePresetListWithConfiguredPresets wires the leader menu
// to the new picker.
func TestLeaderV_OpensThePresetListWithConfiguredPresets(t *testing.T) {
	m := modelWithProjects(t, presetProjects...)
	m.cfg.SetPreset("nightly", config.Preset{Ref: "main", Projects: []string{"backend/api"}})

	updated, _ := m.handleLeaderAction("v")
	mm := updated.(Model)
	if !mm.presets.Active {
		t.Fatal("<space> v should open the preset list")
	}
	if !strings.Contains(mm.presets.View(), "nightly") {
		t.Errorf("preset list missing the configured preset:\n%s", mm.presets.View())
	}
}
