package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validYAML = `
current_instance: work
instances:
  work:
    url: "https://gitlab.internal.example.com"
    token: "glpat-xxxx"
    default_groups: ["core-services"]
  personal:
    url: "https://gitlab.com"
    token: "glpat-yyyy"
cache:
  ttl_minutes: 30
pipelines:
  max_age_days: 30
presets:
  deploy_dev:
    variables:
      DEPLOY_ENV: "development"
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func TestLoad_ParsesValidConfig(t *testing.T) {
	path := writeTemp(t, "config.yaml", validYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.CurrentInstance != "work" {
		t.Errorf("CurrentInstance = %q, want %q", cfg.CurrentInstance, "work")
	}
	if len(cfg.Instances) != 2 {
		t.Errorf("len(Instances) = %d, want 2", len(cfg.Instances))
	}
	if cfg.Instances["work"].URL != "https://gitlab.internal.example.com" {
		t.Errorf("work.URL = %q", cfg.Instances["work"].URL)
	}
	if cfg.Presets["deploy_dev"].Variables["DEPLOY_ENV"] != "development" {
		t.Errorf("preset variable not parsed correctly")
	}
	if cfg.PipelineMaxAge() != 30*24*time.Hour {
		t.Errorf("PipelineMaxAge() = %v, want 30 days", cfg.PipelineMaxAge())
	}
}

func TestLoad_RejectsMissingCurrentInstance(t *testing.T) {
	bad := `
instances:
  work:
    url: "https://gitlab.com"
    token: "x"
`
	path := writeTemp(t, "config.yaml", bad)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing current_instance, got nil")
	}
}

func TestLoad_RejectsUnknownCurrentInstance(t *testing.T) {
	bad := `
current_instance: nope
instances:
  work:
    url: "https://gitlab.com"
    token: "x"
`
	path := writeTemp(t, "config.yaml", bad)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown current_instance, got nil")
	}
}

func TestLoad_RejectsInstanceWithoutURL(t *testing.T) {
	bad := `
current_instance: work
instances:
  work:
    token: "x"
`
	path := writeTemp(t, "config.yaml", bad)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for instance without url, got nil")
	}
}

func TestSave_RoundTrip(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "personal",
		Instances: map[string]Instance{
			"personal": {URL: "https://gitlab.com", Token: "glpat-zzzz"},
		},
		Cache: CacheConfig{TTLMinutes: 15},
	}
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save returned error: %v", err)
	}
	if loaded.CurrentInstance != cfg.CurrentInstance {
		t.Errorf("CurrentInstance = %q, want %q", loaded.CurrentInstance, cfg.CurrentInstance)
	}
	if loaded.Instances["personal"].Token != "glpat-zzzz" {
		t.Errorf("Token round-trip mismatch")
	}
}

func TestActive_ReturnsSelectedInstance(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "work",
		Instances: map[string]Instance{
			"work": {URL: "https://gitlab.internal.example.com"},
		},
	}
	inst, err := cfg.Active()
	if err != nil {
		t.Fatalf("Active returned error: %v", err)
	}
	if inst.URL != "https://gitlab.internal.example.com" {
		t.Errorf("Active().URL = %q", inst.URL)
	}
}

func TestSetActive_ErrorsOnUnknownInstance(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "work",
		Instances: map[string]Instance{
			"work": {URL: "https://gitlab.com"},
		},
	}
	if err := cfg.SetActive("does-not-exist"); err == nil {
		t.Fatal("expected error switching to unknown instance, got nil")
	}
	if cfg.CurrentInstance != "work" {
		t.Errorf("CurrentInstance mutated on failed SetActive: %q", cfg.CurrentInstance)
	}
}

func TestTTL_DefaultsWhenUnset(t *testing.T) {
	cfg := &Config{Cache: CacheConfig{}}
	if got := cfg.TTL(); got != 60*time.Minute {
		t.Errorf("TTL() = %v, want 60m default", got)
	}
}

func TestTTL_UsesConfiguredMinutes(t *testing.T) {
	cfg := &Config{Cache: CacheConfig{TTLMinutes: 5}}
	if got := cfg.TTL(); got != 5*time.Minute {
		t.Errorf("TTL() = %v, want 5m", got)
	}
}

// TestPipelineMaxAge_DefaultsToZeroWhenUnset covers the user request for a
// configurable pipeline age cap: unset must mean "no cap" (0), not some
// implicit default that would silently hide pipelines for existing users
// who never configured it.
func TestPipelineMaxAge_DefaultsToZeroWhenUnset(t *testing.T) {
	cfg := &Config{}
	if got := cfg.PipelineMaxAge(); got != 0 {
		t.Errorf("PipelineMaxAge() = %v, want 0 (no cap) when unset", got)
	}
}

func TestPipelineMaxAge_UsesConfiguredDays(t *testing.T) {
	cfg := &Config{Pipelines: PipelinesConfig{MaxAgeDays: 30}}
	if got := cfg.PipelineMaxAge(); got != 30*24*time.Hour {
		t.Errorf("PipelineMaxAge() = %v, want 30 days", got)
	}
}

// TestLoad_ParsesRunPreset covers backlog 032: a preset may now carry the
// projects and ref to run against, not just variables, so it can be fired
// in one keystroke.
func TestLoad_ParsesRunPreset(t *testing.T) {
	withRun := `
current_instance: work
instances:
  work:
    url: "https://gitlab.com"
    token: "x"
presets:
  nightly:
    ref: "main"
    projects: ["backend/api", "backend/worker"]
    variables:
      DEPLOY_ENV: "dev"
`
	cfg, err := Load(writeTemp(t, "config.yaml", withRun))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	p := cfg.Presets["nightly"]
	if p.Ref != "main" {
		t.Errorf("Ref = %q, want main", p.Ref)
	}
	if len(p.Projects) != 2 || p.Projects[0] != "backend/api" {
		t.Errorf("Projects = %+v", p.Projects)
	}
	if p.Variables["DEPLOY_ENV"] != "dev" {
		t.Errorf("Variables = %+v", p.Variables)
	}
}

// TestLoad_VariableOnlyPresetStillParses guards backward compatibility: the
// pre-032 preset shape (variables only, no projects/ref) must keep working
// for anyone with an existing config.yaml.
func TestLoad_VariableOnlyPresetStillParses(t *testing.T) {
	cfg, err := Load(writeTemp(t, "config.yaml", validYAML))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	p := cfg.Presets["deploy_dev"]
	if p.Variables["DEPLOY_ENV"] != "development" {
		t.Errorf("Variables = %+v", p.Variables)
	}
	if p.Ref != "" || len(p.Projects) != 0 {
		t.Errorf("variable-only preset should have empty Ref/Projects, got %+v", p)
	}
	if p.Runnable() {
		t.Error("Runnable() = true for a preset with no projects")
	}
}

func TestPreset_RunnableRequiresProjects(t *testing.T) {
	if !(Preset{Projects: []string{"a/b"}}).Runnable() {
		t.Error("Runnable() = false for a preset with projects")
	}
}

func TestSetPreset_CreatesMapAndRoundTrips(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "work",
		Instances:       map[string]Instance{"work": {URL: "https://gitlab.com"}},
	}
	cfg.SetPreset("nightly", Preset{Ref: "main", Projects: []string{"a/b"}})

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Presets["nightly"].Ref != "main" {
		t.Errorf("preset did not round-trip: %+v", loaded.Presets)
	}
}

func TestDeletePreset(t *testing.T) {
	cfg := &Config{Presets: map[string]Preset{"a": {}, "b": {}}}
	cfg.DeletePreset("a")
	if _, ok := cfg.Presets["a"]; ok {
		t.Error("preset a still present after DeletePreset")
	}
	if len(cfg.Presets) != 1 {
		t.Errorf("len(Presets) = %d, want 1", len(cfg.Presets))
	}
}

func TestPresetNames_Sorted(t *testing.T) {
	cfg := &Config{Presets: map[string]Preset{"zeta": {}, "alpha": {}, "mid": {}}}
	got := cfg.PresetNames()
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("PresetNames() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PresetNames() = %+v, want %+v", got, want)
		}
	}
}

// TestDeleteInstance_RepointsCurrentInstance covers the in-app settings
// editor (031): deleting the active instance must leave the config valid,
// not dangling at a current_instance that no longer exists.
func TestDeleteInstance_RepointsCurrentInstance(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "work",
		Instances: map[string]Instance{
			"work":     {URL: "https://gitlab.internal"},
			"personal": {URL: "https://gitlab.com"},
		},
	}
	if err := cfg.DeleteInstance("work"); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if cfg.CurrentInstance != "personal" {
		t.Errorf("CurrentInstance = %q, want personal", cfg.CurrentInstance)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("config invalid after DeleteInstance: %v", err)
	}
}

func TestDeleteInstance_RefusesLastInstance(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "work",
		Instances:       map[string]Instance{"work": {URL: "https://gitlab.com"}},
	}
	if err := cfg.DeleteInstance("work"); err == nil {
		t.Fatal("expected error deleting the only instance, got nil")
	}
	if len(cfg.Instances) != 1 {
		t.Errorf("instance was deleted despite the error: %+v", cfg.Instances)
	}
}

func TestSetInstance_CreatesMap(t *testing.T) {
	cfg := &Config{}
	cfg.SetInstance("work", Instance{URL: "https://gitlab.com"})
	if cfg.Instances["work"].URL != "https://gitlab.com" {
		t.Fatalf("Instances = %+v", cfg.Instances)
	}
	if cfg.CurrentInstance != "work" {
		t.Errorf("CurrentInstance = %q, want the first instance added to become current", cfg.CurrentInstance)
	}
}

func TestExists(t *testing.T) {
	path := writeTemp(t, "config.yaml", validYAML)
	if !Exists(path) {
		t.Error("Exists() = false for a file that was just written")
	}
	if Exists(filepath.Join(t.TempDir(), "missing.yaml")) {
		t.Error("Exists() = true for a nonexistent file")
	}
}
