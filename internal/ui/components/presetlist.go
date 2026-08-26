package components

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// RunPresetMsg asks the root model to fire a runnable preset immediately:
// its own projects, its own ref, its own variables, no trigger modal.
type RunPresetMsg struct {
	Name string
}

// PresetChosenMsg asks the root model to remember a preset as the prefill
// for the next trigger modal (<Space> p) rather than firing it. This is the
// only thing a variables-only preset can do — it names no projects — and is
// also how you tweak a runnable preset before dispatching it.
type PresetChosenMsg struct {
	Name string
}

// PresetEntry is one config preset flattened for display. The component
// deliberately doesn't import internal/config: it needs a name, a ref, and
// two counts, and keeping it to plain fields keeps it a pure view model.
type PresetEntry struct {
	Name     string
	Ref      string
	Projects []string
	Vars     []api.Variable
}

// Runnable mirrors config.Preset.Runnable: a preset that names its own
// projects can be dispatched directly.
func (e PresetEntry) Runnable() bool { return len(e.Projects) > 0 }

// PresetList is the <Space> v modal: every configured preset, with Enter
// firing a runnable one in a single keystroke.
type PresetList struct {
	Active bool

	entries []PresetEntry
	table   table.Model
}

func NewPresetList() PresetList {
	cols := []table.Column{
		{Title: "PRESET", Width: 22},
		{Title: "REF", Width: 20},
		{Title: "PROJECTS", Width: 9},
		{Title: "VARS", Width: 5},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))
	return PresetList{table: tbl}
}

// Open activates the picker with the current config's presets.
func (p *PresetList) Open(entries []PresetEntry) {
	p.Active = true
	p.entries = entries
	p.syncRows()
	p.table.SetCursor(0)
}

func (p *PresetList) syncRows() {
	rows := make([]table.Row, 0, len(p.entries))
	for _, e := range p.entries {
		ref := e.Ref
		if ref == "" {
			ref = "(default branch)"
		}
		projects := strconv.Itoa(len(e.Projects))
		if !e.Runnable() {
			projects = "—"
		}
		rows = append(rows, table.Row{e.Name, ref, projects, strconv.Itoa(len(e.Vars))})
	}
	setRows(&p.table, rows)
}

// Highlighted returns the preset under the cursor, if any.
func (p PresetList) Highlighted() (PresetEntry, bool) {
	i := p.table.Cursor()
	if i < 0 || i >= len(p.entries) {
		return PresetEntry{}, false
	}
	return p.entries[i], true
}

func (p PresetList) Update(msg tea.Msg) (PresetList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			p.Active = false
			return p, nil
		case "enter":
			e, ok := p.Highlighted()
			if !ok {
				return p, nil
			}
			p.Active = false
			if e.Runnable() {
				return p, func() tea.Msg { return RunPresetMsg{Name: e.Name} }
			}
			return p, func() tea.Msg { return PresetChosenMsg{Name: e.Name} }
		case "c":
			e, ok := p.Highlighted()
			if !ok {
				return p, nil
			}
			p.Active = false
			return p, func() tea.Msg { return PresetChosenMsg{Name: e.Name} }
		}
	}

	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	return p, cmd
}

func (p PresetList) View() string {
	var b strings.Builder
	b.WriteString("Presets — enter fires a runnable preset at its own projects.\n\n")
	b.WriteString(p.table.View())
	if len(p.entries) == 0 {
		b.WriteString("\n\n(no presets configured — add one in <space> s, or save one from the trigger modal)")
	} else if e, ok := p.Highlighted(); ok {
		b.WriteString("\n\n" + presetDetail(e))
	}
	b.WriteString("\n\n" + RenderHelp(
		[2]string{"enter", "run (or select, if it names no projects)"},
		[2]string{"c", "select for next trigger"},
		[2]string{"esc", "cancel"},
	))
	return lipgloss.NewStyle().Render(b.String())
}

// presetDetail spells out exactly what the highlighted preset will do, so a
// one-keystroke fire is never a guess about which repos it hits.
func presetDetail(e PresetEntry) string {
	var b strings.Builder
	if e.Runnable() {
		b.WriteString(fmt.Sprintf("→ %s\n", strings.Join(e.Projects, ", ")))
	} else {
		b.WriteString("→ variables only; select it, then <space> p to pick projects\n")
	}
	if len(e.Vars) > 0 {
		pairs := make([]string, len(e.Vars))
		for i, v := range e.Vars {
			pairs[i] = v.Key + "=" + v.Value
		}
		b.WriteString("  " + strings.Join(pairs, "  "))
	}
	return helpDescStyle.Render(b.String())
}
