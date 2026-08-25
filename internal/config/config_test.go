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

func TestExists(t *testing.T) {
	path := writeTemp(t, "config.yaml", validYAML)
	if !Exists(path) {
		t.Error("Exists() = false for a file that was just written")
	}
	if Exists(filepath.Join(t.TempDir(), "missing.yaml")) {
		t.Error("Exists() = true for a nonexistent file")
	}
}
