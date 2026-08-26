package components

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/config"
)

// SwitchInstanceMsg asks the root model to switch the active GitLab profile.
type SwitchInstanceMsg struct {
	Name string
}

// ConfigChangedMsg tells the root model the settings screen has mutated the
// live config in place and it should be persisted. ReloadInstance is set
// when the change affects the active connection (URL, token, or the
// instance set itself), so the API client and project cache are rebuilt
// rather than left pointing at the old profile.
type ConfigChangedMsg struct {
	Reason         string
	ReloadInstance bool
}

type settingsRowKind int

const (
	rowHeader settingsRowKind = iota
	rowInstance
	rowAddInstance
	rowTTL
	rowMaxAge
	rowPreset
	rowAddPreset
)

// settingsRow is one line of the settings screen. Headers are rendered as
// ordinary (inert) rows: bubbles/table drives the cursor and has no notion
// of a skippable row, and every action below is guarded on row kind anyway.
type settingsRow struct {
	kind  settingsRowKind
	key   string // instance or preset name
	label string
	value string
}

type settingsMode int

const (
	settingsBrowse settingsMode = iota
	settingsForm
	settingsConfirmDelete
)

// Settings is the in-app config editor opened by <Space> s: instances
// (switch, add, edit, delete), cache TTL, the pipeline age cap, and presets
// — everything in config.yaml except default_groups, which the group
// discovery picker (<Space> g) owns.
//
// It edits the live *config.Config in place and reports each committed
// change with a ConfigChangedMsg; the root model persists it through the
// same saveConfigCmd every other config write already uses. Importing
// internal/config here is deliberate: this component's entire subject *is*
// the config document, and a parallel view-model would only be the same
// fields under different names.
type Settings struct {
	Active bool

	cfg   *config.Config
	rows  []settingsRow
	table table.Model

	mode    settingsMode
	form    fieldForm
	target  settingsRow // the row the open form or confirmation applies to
	errMsg  string
	pending settingsRow
}

func NewSettings() Settings {
	cols := []table.Column{
		{Title: "SETTING", Width: 28},
		{Title: "VALUE", Width: 44},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))
	return Settings{table: tbl}
}

// Open activates the editor against the live config.
func (s *Settings) Open(cfg *config.Config) {
	s.Active = true
	s.cfg = cfg
	s.mode = settingsBrowse
	s.errMsg = ""
	s.rebuild()
	s.table.SetCursor(0)
}

// HasTextFocus reports whether an open editor form owns keystrokes.
func (s *Settings) HasTextFocus() bool { return s.mode == settingsForm }

func (s *Settings) rebuild() {
	rows := []settingsRow{{kind: rowHeader, label: "INSTANCES"}}
	for _, name := range s.cfg.InstanceNames() {
		inst := s.cfg.Instances[name]
		label := name
		if name == s.cfg.CurrentInstance {
			label = name + "  (active)"
		}
		rows = append(rows, settingsRow{kind: rowInstance, key: name, label: label, value: inst.URL})
	}
	rows = append(rows,
		settingsRow{kind: rowAddInstance, label: "+ add instance"},
		settingsRow{kind: rowHeader, label: "GENERAL"},
		settingsRow{kind: rowTTL, label: "project cache TTL (minutes)", value: strconv.Itoa(s.cfg.Cache.TTLMinutes)},
		settingsRow{kind: rowMaxAge, label: "pipeline max age (days)", value: maxAgeValue(s.cfg.Pipelines.MaxAgeDays)},
		settingsRow{kind: rowHeader, label: "PRESETS"},
	)
	for _, name := range s.cfg.PresetNames() {
		p := s.cfg.Presets[name]
		rows = append(rows, settingsRow{kind: rowPreset, key: name, label: name, value: presetSummary(p)})
	}
	rows = append(rows, settingsRow{kind: rowAddPreset, label: "+ add preset"})

	s.rows = rows
	tableRows := make([]table.Row, len(rows))
	for i, r := range rows {
		tableRows[i] = table.Row{r.label, r.value}
	}
	setRows(&s.table, tableRows)
}

func maxAgeValue(days int) string {
	if days <= 0 {
		return "0  (no cap)"
	}
	return strconv.Itoa(days)
}

func presetSummary(p config.Preset) string {
	ref := p.Ref
	if ref == "" {
		ref = "(default branch)"
	}
	if !p.Runnable() {
		return fmt.Sprintf("variables only · %d var(s)", len(p.Variables))
	}
	return fmt.Sprintf("%s · %d project(s) · %d var(s)", ref, len(p.Projects), len(p.Variables))
}

func (s Settings) highlighted() settingsRow {
	i := s.table.Cursor()
	if i < 0 || i >= len(s.rows) {
		return settingsRow{}
	}
	return s.rows[i]
}

func (s Settings) Update(msg tea.Msg) (Settings, tea.Cmd) {
	switch s.mode {
	case settingsForm:
		return s.updateForm(msg)
	case settingsConfirmDelete:
		return s.updateConfirm(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		row := s.highlighted()
		switch km.String() {
		case "esc":
			s.Active = false
			return s, nil
		case "enter":
			return s.activate(row)
		case "e":
			switch row.kind {
			case rowInstance, rowTTL, rowMaxAge, rowPreset:
				return s.openEditor(row), nil
			}
			return s, nil
		case "d":
			if row.kind == rowInstance || row.kind == rowPreset {
				s.mode = settingsConfirmDelete
				s.pending = row
				s.errMsg = ""
			}
			return s, nil
		}
	}

	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return s, cmd
}

// activate is Enter's behavior per row: an instance switches profile (the
// most common reason to be here), everything else opens its editor.
func (s Settings) activate(row settingsRow) (Settings, tea.Cmd) {
	switch row.kind {
	case rowInstance:
		if row.key == s.cfg.CurrentInstance {
			return s.openEditor(row), nil
		}
		name := row.key
		s.Active = false
		return s, func() tea.Msg { return SwitchInstanceMsg{Name: name} }
	case rowHeader:
		return s, nil
	default:
		return s.openEditor(row), nil
	}
}

func (s Settings) openEditor(row settingsRow) Settings {
	s.errMsg = ""
	s.target = row
	s.mode = settingsForm
	switch row.kind {
	case rowTTL:
		s.form = newFieldForm("Project cache TTL", field{"minutes", strconv.Itoa(s.cfg.Cache.TTLMinutes)})
	case rowMaxAge:
		s.form = newFieldForm("Pipeline max age (0 = no cap)", field{"days", strconv.Itoa(s.cfg.Pipelines.MaxAgeDays)})
	case rowInstance:
		inst := s.cfg.Instances[row.key]
		s.form = newFieldForm("Edit instance",
			field{"name", row.key},
			field{"url", inst.URL},
			field{"token", inst.Token},
			field{"token command", inst.TokenCommand},
			field{"default groups", strings.Join(inst.DefaultGroups, ", ")},
		)
	case rowAddInstance:
		s.form = newFieldForm("New instance",
			field{"name", ""},
			field{"url", ""},
			field{"token", ""},
			field{"token command", ""},
			field{"default groups", ""},
		)
	case rowPreset:
		p := s.cfg.Presets[row.key]
		s.form = newFieldForm("Edit preset",
			field{"name", row.key},
			field{"ref (blank = default branch)", p.Ref},
			field{"projects", strings.Join(p.Projects, ", ")},
			field{"variables", formatVars(p.Variables)},
		)
	case rowAddPreset:
		s.form = newFieldForm("New preset",
			field{"name", ""},
			field{"ref (blank = default branch)", ""},
			field{"projects", ""},
			field{"variables", ""},
		)
	default:
		s.mode = settingsBrowse
	}
	return s
}

func (s Settings) updateForm(msg tea.Msg) (Settings, tea.Cmd) {
	form, submitted, canceled, cmd := s.form.update(msg)
	s.form = form
	switch {
	case canceled:
		s.mode = settingsBrowse
		s.errMsg = ""
		return s, nil
	case submitted:
		return s.commit()
	}
	return s, cmd
}

// commit validates the open form and writes it into the live config. A
// rejected form stays open with an explanation rather than silently
// discarding what was typed.
func (s Settings) commit() (Settings, tea.Cmd) {
	switch s.target.kind {
	case rowTTL:
		n, err := strconv.Atoi(strings.TrimSpace(s.form.value(0)))
		if err != nil || n < 0 {
			s.errMsg = "cache TTL must be a number of minutes (0 = use the 60m default)"
			return s, nil
		}
		s.cfg.Cache.TTLMinutes = n
		return s.done("cache TTL set to "+strconv.Itoa(n)+"m", false)

	case rowMaxAge:
		n, err := strconv.Atoi(strings.TrimSpace(s.form.value(0)))
		if err != nil || n < 0 {
			s.errMsg = "pipeline max age must be a number of days (0 = no cap)"
			return s, nil
		}
		s.cfg.Pipelines.MaxAgeDays = n
		return s.done("pipeline max age set to "+maxAgeValue(n), false)

	case rowInstance, rowAddInstance:
		name := strings.TrimSpace(s.form.value(0))
		url := strings.TrimSpace(s.form.value(1))
		if name == "" || url == "" {
			s.errMsg = "an instance needs both a name and a url"
			return s, nil
		}
		inst := config.Instance{
			URL:           url,
			Token:         strings.TrimSpace(s.form.value(2)),
			TokenCommand:  strings.TrimSpace(s.form.value(3)),
			DefaultGroups: splitList(s.form.value(4)),
		}
		// A rename is a move: drop the old key, and carry
		// current_instance across so the config still validates.
		if s.target.kind == rowInstance && name != s.target.key {
			wasCurrent := s.cfg.CurrentInstance == s.target.key
			delete(s.cfg.Instances, s.target.key)
			s.cfg.SetInstance(name, inst)
			if wasCurrent {
				s.cfg.CurrentInstance = name
			}
		} else {
			s.cfg.SetInstance(name, inst)
		}
		return s.done("saved instance "+name, true)

	case rowPreset, rowAddPreset:
		name := strings.TrimSpace(s.form.value(0))
		if name == "" {
			s.errMsg = "a preset needs a name"
			return s, nil
		}
		vars, err := parseVars(s.form.value(3))
		if err != nil {
			s.errMsg = err.Error()
			return s, nil
		}
		if s.target.kind == rowPreset && name != s.target.key {
			s.cfg.DeletePreset(s.target.key)
		}
		s.cfg.SetPreset(name, config.Preset{
			Ref:       strings.TrimSpace(s.form.value(1)),
			Projects:  splitList(s.form.value(2)),
			Variables: vars,
		})
		return s.done("saved preset "+name, false)
	}

	s.mode = settingsBrowse
	return s, nil
}

func (s Settings) done(reason string, reload bool) (Settings, tea.Cmd) {
	s.mode = settingsBrowse
	s.errMsg = ""
	s.rebuild()
	return s, func() tea.Msg { return ConfigChangedMsg{Reason: reason, ReloadInstance: reload} }
}

func (s Settings) updateConfirm(msg tea.Msg) (Settings, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch km.String() {
	case "y":
		row := s.pending
		s.mode = settingsBrowse
		if row.kind == rowInstance {
			if err := s.cfg.DeleteInstance(row.key); err != nil {
				s.errMsg = err.Error()
				return s, nil
			}
			return s.done("deleted instance "+row.key, true)
		}
		s.cfg.DeletePreset(row.key)
		return s.done("deleted preset "+row.key, false)
	case "n", "esc":
		s.mode = settingsBrowse
		return s, nil
	}
	return s, nil
}

// splitList parses a comma-separated field ("backend/api, backend/worker")
// into a trimmed slice, dropping empties.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// formatVars renders a preset's variable map as the comma-separated
// KEY=VALUE text the form edits, in sorted order so a no-op edit round-trips
// byte-for-byte instead of reshuffling on Go's random map iteration.
func formatVars(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + vars[k]
	}
	return strings.Join(pairs, ", ")
}

func parseVars(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("variables must be KEY=VALUE pairs, got %q", part)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (s Settings) View() string {
	var b strings.Builder
	switch s.mode {
	case settingsForm:
		b.WriteString(s.form.viewString())
		if s.errMsg != "" {
			b.WriteString("\n\n" + errorTextStyle.Render(s.errMsg))
		}
		b.WriteString("\n\n" + RenderHelp(
			[2]string{"tab", "next field"},
			[2]string{"enter", "save"},
			[2]string{"esc", "cancel"},
		))
		return lipgloss.NewStyle().Render(b.String())

	case settingsConfirmDelete:
		b.WriteString(fmt.Sprintf("Delete %s %q?\n\n", settingsKindLabel(s.pending.kind), s.pending.key))
		b.WriteString(RenderHelp([2]string{"y", "delete"}, [2]string{"n/esc", "keep"}))
		return lipgloss.NewStyle().Render(b.String())
	}

	b.WriteString("Settings — changes are written to config.yaml as you make them.\n\n")
	b.WriteString(s.table.View())
	if detail := s.detail(); detail != "" {
		b.WriteString("\n\n" + detail)
	}
	if s.errMsg != "" {
		b.WriteString("\n\n" + errorTextStyle.Render(s.errMsg))
	}
	b.WriteString("\n\n" + RenderHelp(
		[2]string{"enter", "switch instance / edit"},
		[2]string{"e", "edit"},
		[2]string{"d", "delete"},
		[2]string{"esc", "close"},
	))
	return lipgloss.NewStyle().Render(b.String())
}

func settingsKindLabel(k settingsRowKind) string {
	if k == rowInstance {
		return "instance"
	}
	return "preset"
}

// detail expands the highlighted row below the table — the token (masked),
// an instance's groups, a preset's actual projects and variables — so the
// list stays scannable without hiding what a row really holds.
func (s Settings) detail() string {
	row := s.highlighted()
	switch row.kind {
	case rowInstance:
		inst := s.cfg.Instances[row.key]
		parts := []string{maskToken(inst)}
		if len(inst.DefaultGroups) > 0 {
			parts = append(parts, "groups: "+strings.Join(inst.DefaultGroups, ", "))
		} else {
			parts = append(parts, "groups: none (<space> g to pick some)")
		}
		return helpDescStyle.Render(strings.Join(parts, " · "))
	case rowPreset:
		p := s.cfg.Presets[row.key]
		var parts []string
		if p.Runnable() {
			parts = append(parts, "→ "+strings.Join(p.Projects, ", "))
		}
		if len(p.Variables) > 0 {
			parts = append(parts, formatVars(p.Variables))
		}
		return helpDescStyle.Render(strings.Join(parts, "\n"))
	}
	return ""
}

// maskToken keeps a literal PAT off the screen while still showing how the
// token is sourced. ${ENV_VAR} and token_command aren't secrets — they're
// the indirection itself — so those are shown as written.
func maskToken(inst config.Instance) string {
	switch {
	case inst.TokenCommand != "":
		return "token_command: " + inst.TokenCommand
	case inst.Token == "":
		return "token: (unset)"
	case strings.HasPrefix(inst.Token, "${"):
		return "token: " + inst.Token
	default:
		return "token: •••••• (shown in full when editing)"
	}
}
