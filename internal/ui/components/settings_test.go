package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		CurrentInstance: "work",
		Instances: map[string]config.Instance{
			"work":     {URL: "https://gitlab.internal", Token: "glpat-secret", DefaultGroups: []string{"backend"}},
			"personal": {URL: "https://gitlab.com", Token: "glpat-other"},
		},
		Cache:     config.CacheConfig{TTLMinutes: 60},
		Pipelines: config.PipelinesConfig{MaxAgeDays: 30},
		Presets: map[string]config.Preset{
			"nightly": {Ref: "main", Projects: []string{"backend/api"}, Variables: map[string]string{"DEPLOY_ENV": "dev"}},
		},
	}
}

func openSettings(t *testing.T, cfg *config.Config) Settings {
	t.Helper()
	s := NewSettings()
	s.Open(cfg)
	return s
}

// focusRow points the settings cursor at the first row of the given kind
// (optionally matching key), so tests don't depend on how many j presses a
// particular config shape happens to need.
func focusRow(t *testing.T, s *Settings, kind settingsRowKind, key string) {
	t.Helper()
	for i, r := range s.rows {
		if r.kind == kind && (key == "" || r.key == key) {
			s.table.SetCursor(i)
			return
		}
	}
	t.Fatalf("no row of kind %d with key %q in %+v", kind, key, s.rows)
}

func typeString(s Settings, text string) Settings {
	for _, r := range text {
		s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return s
}

func enter(s Settings) (Settings, tea.Cmd) {
	return s.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// TestSettings_EditsCacheTTLInPlace is the headline of backlog 031: a value
// that previously required editing config.yaml and restarting is now
// editable in the app, and reported back for persisting.
func TestSettings_EditsCacheTTLInPlace(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowTTL, "")

	s, _ = enter(s)
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU}) // clear the prefilled value
	s = typeString(s, "15")
	s, cmd := enter(s)

	if cfg.Cache.TTLMinutes != 15 {
		t.Fatalf("TTLMinutes = %d, want 15", cfg.Cache.TTLMinutes)
	}
	if cmd == nil {
		t.Fatal("editing TTL emitted no command")
	}
	if _, ok := cmd().(ConfigChangedMsg); !ok {
		t.Fatalf("emitted %T, want ConfigChangedMsg", cmd())
	}
	if s.mode != settingsBrowse {
		t.Error("expected to return to the browse list after saving a field")
	}
}

func TestSettings_RejectsNonNumericTTL(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowTTL, "")

	s, _ = enter(s)
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	s = typeString(s, "soon")
	s, cmd := enter(s)

	if cfg.Cache.TTLMinutes != 60 {
		t.Errorf("TTLMinutes = %d, want the original 60 to survive a bad edit", cfg.Cache.TTLMinutes)
	}
	if cmd != nil {
		t.Error("a rejected edit should not report a config change")
	}
	if !strings.Contains(s.View(), "number") {
		t.Errorf("expected an explanation in the view, got:\n%s", s.View())
	}
}

func TestSettings_EditsPipelineMaxAge(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowMaxAge, "")

	s, _ = enter(s)
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	s = typeString(s, "7")
	_, cmd := enter(s)

	if cfg.Pipelines.MaxAgeDays != 7 {
		t.Fatalf("MaxAgeDays = %d, want 7", cfg.Pipelines.MaxAgeDays)
	}
	if cmd == nil {
		t.Fatal("editing max age emitted no command")
	}
}

func TestSettings_EnterOnInstanceSwitchesActive(t *testing.T) {
	s := openSettings(t, testConfig())
	focusRow(t, &s, rowInstance, "personal")

	_, cmd := enter(s)
	if cmd == nil {
		t.Fatal("enter on an instance emitted no command")
	}
	msg, ok := cmd().(SwitchInstanceMsg)
	if !ok {
		t.Fatalf("emitted %T, want SwitchInstanceMsg", cmd())
	}
	if msg.Name != "personal" {
		t.Errorf("SwitchInstanceMsg.Name = %q, want personal", msg.Name)
	}
}

func TestSettings_EditsInstanceURL(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowInstance, "work")

	s = typeString(s, "e")                          // open the instance form
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})   // name -> url
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU}) // clear url
	s = typeString(s, "https://gitlab.new")
	s, cmd := enter(s)

	if cfg.Instances["work"].URL != "https://gitlab.new" {
		t.Fatalf("URL = %q, want the edited value", cfg.Instances["work"].URL)
	}
	if cfg.Instances["work"].Token != "glpat-secret" {
		t.Errorf("token was lost editing an unrelated field: %q", cfg.Instances["work"].Token)
	}
	if cmd == nil {
		t.Fatal("no ConfigChangedMsg after editing an instance")
	}
	msg, ok := cmd().(ConfigChangedMsg)
	if !ok {
		t.Fatalf("emitted %T, want ConfigChangedMsg", cmd())
	}
	if !msg.ReloadInstance {
		t.Error("editing an instance URL must ask the root model to rebuild the API client")
	}
	if s.mode != settingsBrowse {
		t.Error("expected to return to the browse list")
	}
}

// TestSettings_RenamingAnInstanceMovesIt covers the rename path: the old key
// must not linger, and current_instance has to follow the rename or the
// config no longer validates.
func TestSettings_RenamingAnInstanceMovesIt(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowInstance, "work")

	s = typeString(s, "e")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	s = typeString(s, "office")
	_, _ = enter(s)

	if _, stale := cfg.Instances["work"]; stale {
		t.Error("old instance key survived the rename")
	}
	if cfg.Instances["office"].URL != "https://gitlab.internal" {
		t.Fatalf("renamed instance = %+v", cfg.Instances["office"])
	}
	if cfg.CurrentInstance != "office" {
		t.Errorf("CurrentInstance = %q, want it to follow the rename", cfg.CurrentInstance)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("config invalid after rename: %v", err)
	}
}

func TestSettings_AddsAnInstance(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowAddInstance, "")

	s, _ = enter(s)
	s = typeString(s, "staging")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeString(s, "https://gitlab.staging")
	_, cmd := enter(s)

	if cfg.Instances["staging"].URL != "https://gitlab.staging" {
		t.Fatalf("Instances = %+v", cfg.Instances)
	}
	if cmd == nil {
		t.Fatal("adding an instance emitted no command")
	}
}

func TestSettings_RejectsInstanceWithoutURL(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowAddInstance, "")

	s, _ = enter(s)
	s = typeString(s, "broken")
	s, cmd := enter(s)

	if _, ok := cfg.Instances["broken"]; ok {
		t.Error("an instance with no url must not be saved — Load would reject the file")
	}
	if cmd != nil {
		t.Error("a rejected form should not report a config change")
	}
	if !strings.Contains(s.View(), "url") {
		t.Errorf("expected the view to explain the missing url, got:\n%s", s.View())
	}
}

func TestSettings_DeleteInstanceAsksForConfirmation(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowInstance, "personal")

	s = typeString(s, "d")
	if _, gone := cfg.Instances["personal"]; !gone {
		t.Fatal("d must not delete before confirmation")
	}
	s = typeString(s, "n")
	if _, gone := cfg.Instances["personal"]; !gone {
		t.Fatal("declining the confirmation must keep the instance")
	}

	s = typeString(s, "d")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if _, still := cfg.Instances["personal"]; still {
		t.Fatal("confirming should have deleted the instance")
	}
	if cmd == nil {
		t.Fatal("deleting an instance emitted no command")
	}
}

func TestSettings_DeleteRefusesTheLastInstance(t *testing.T) {
	cfg := &config.Config{
		CurrentInstance: "work",
		Instances:       map[string]config.Instance{"work": {URL: "https://gitlab.com"}},
	}
	s := openSettings(t, cfg)
	focusRow(t, &s, rowInstance, "work")

	s = typeString(s, "d")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if len(cfg.Instances) != 1 {
		t.Fatal("the only instance must not be deletable")
	}
	if !strings.Contains(s.View(), "only instance") {
		t.Errorf("expected the refusal to be explained, got:\n%s", s.View())
	}
}

// TestSettings_AddsAPresetWithProjectsAndVariables is the other half of the
// one-tap ask: a runnable preset can be created without leaving the app.
func TestSettings_AddsAPresetWithProjectsAndVariables(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowAddPreset, "")

	s, _ = enter(s)
	s = typeString(s, "release")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeString(s, "v1.2.3")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeString(s, "backend/api, backend/worker")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = typeString(s, "DEPLOY_ENV=prod, DRY_RUN=false")
	_, cmd := enter(s)

	p, ok := cfg.Presets["release"]
	if !ok {
		t.Fatalf("preset not created: %+v", cfg.Presets)
	}
	if p.Ref != "v1.2.3" {
		t.Errorf("Ref = %q", p.Ref)
	}
	if len(p.Projects) != 2 || p.Projects[0] != "backend/api" || p.Projects[1] != "backend/worker" {
		t.Errorf("Projects = %+v", p.Projects)
	}
	if p.Variables["DEPLOY_ENV"] != "prod" || p.Variables["DRY_RUN"] != "false" {
		t.Errorf("Variables = %+v", p.Variables)
	}
	if !p.Runnable() {
		t.Error("a preset with projects should be runnable")
	}
	if cmd == nil {
		t.Fatal("adding a preset emitted no command")
	}
}

// TestSettings_EditPresetRoundTrips proves the list<->text conversion is
// lossless: opening an existing preset and saving it unchanged must not
// mangle its projects or variables.
func TestSettings_EditPresetRoundTrips(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowPreset, "nightly")

	s, _ = enter(s)
	_, _ = enter(s)

	p := cfg.Presets["nightly"]
	if p.Ref != "main" || len(p.Projects) != 1 || p.Projects[0] != "backend/api" {
		t.Fatalf("preset changed on a no-op edit: %+v", p)
	}
	if p.Variables["DEPLOY_ENV"] != "dev" {
		t.Fatalf("variables changed on a no-op edit: %+v", p.Variables)
	}
}

func TestSettings_DeletesAPreset(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowPreset, "nightly")

	s = typeString(s, "d")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if _, still := cfg.Presets["nightly"]; still {
		t.Fatal("preset was not deleted")
	}
	if cmd == nil {
		t.Fatal("deleting a preset emitted no command")
	}
}

// TestSettings_TokenIsMasked keeps a PAT off the screen in the browse list,
// where it would otherwise sit visible for the whole session.
func TestSettings_TokenIsMasked(t *testing.T) {
	s := openSettings(t, testConfig())
	focusRow(t, &s, rowInstance, "work")

	if strings.Contains(s.View(), "glpat-secret") {
		t.Errorf("token rendered in the clear:\n%s", s.View())
	}
}

func TestSettings_EscFromFormReturnsToListWithoutSaving(t *testing.T) {
	cfg := testConfig()
	s := openSettings(t, cfg)
	focusRow(t, &s, rowInstance, "work")

	s = typeString(s, "e")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	s = typeString(s, "renamed")
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if _, ok := cfg.Instances["work"]; !ok {
		t.Error("esc in the form should have discarded the edit")
	}
	if s.mode != settingsBrowse {
		t.Error("esc should return to the browse list")
	}
	if !s.Active {
		t.Error("esc from a form should not close the whole settings screen")
	}
}

func TestSettings_EscFromBrowseClosesTheScreen(t *testing.T) {
	s := openSettings(t, testConfig())
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if s.Active {
		t.Error("esc from the browse list should close settings")
	}
}

// TestSettings_HasTextFocusWhileEditing keeps <Space> from opening the
// leader menu mid-edit — the same invariant every other text-input
// component in the app upholds.
func TestSettings_HasTextFocusWhileEditing(t *testing.T) {
	s := openSettings(t, testConfig())
	if s.HasTextFocus() {
		t.Error("browse mode should not claim text focus")
	}
	focusRow(t, &s, rowTTL, "")
	s, _ = enter(s)
	if !s.HasTextFocus() {
		t.Error("an open editor must claim text focus")
	}
}
