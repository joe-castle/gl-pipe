package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// DispatchMsg is emitted when the user confirms the trigger modal, asking
// the root model to create a pipeline on every staged project.
type DispatchMsg struct {
	Projects []api.Project
	Ref      string
	Vars     []api.Variable
}

// RefPickerRequestMsg is emitted when the user asks to browse available
// refs (ctrl+r) — the root model fetches branches+tags for the first
// staged project and calls OpenRefPicker with the result.
type RefPickerRequestMsg struct {
	ProjectID int
}

type varEditState int

const (
	varEditNone varEditState = iota
	varEditKey
	varEditValue
)

// Variables is the pipeline trigger modal: ref picker + an interactive
// variable table (key/value, env_var vs file, masked/protected flags),
// dispatching to every staged project at once.
type Variables struct {
	Active   bool
	Projects []api.Project
	Ref      string
	RefInput textinput.Model

	rows       []api.Variable
	table      table.Model
	editing    varEditState
	keyInput   textinput.Model
	valueInput textinput.Model

	focusRef bool

	refPickerActive bool
	refOptions      []api.Ref
	refCursor       int
}

func NewVariables() Variables {
	ref := textinput.New()
	ref.Prompt = "ref: "
	ref.Width = 40

	key := textinput.New()
	key.Prompt = "key: "
	key.Width = 30

	val := textinput.New()
	val.Prompt = "value: "
	val.Width = 40

	cols := []table.Column{
		{Title: "KEY", Width: 24},
		{Title: "VALUE", Width: 24},
		{Title: "TYPE", Width: 8},
		{Title: "MASKED", Width: 7},
		{Title: "PROTECTED", Width: 9},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(10), table.WithStyles(TableStyles()))

	return Variables{RefInput: ref, keyInput: key, valueInput: val, table: tbl}
}

// Open activates the modal for the given staged projects with a starting
// ref and optional preset variables.
func (v *Variables) Open(projects []api.Project, ref string, preset []api.Variable) {
	v.Active = true
	v.Projects = projects
	v.Ref = ref
	v.RefInput.SetValue(ref)
	v.rows = append([]api.Variable(nil), preset...)
	v.syncRows()
	v.focusRef = false
	v.editing = varEditNone
	v.refPickerActive = false
}

func (v *Variables) Close() { v.Active = false }

// FirstProjectID returns the first staged project's ID, or 0 if none —
// the ref picker fetches branches/tags for this one project, since the
// trigger modal shares a single ref field across every staged project
// (see the "known simplifications" note in CLAUDE.md).
func (v *Variables) FirstProjectID() int {
	if len(v.Projects) == 0 {
		return 0
	}
	return v.Projects[0].ID
}

// OpenRefPicker activates the ref browser overlay with a freshly-fetched
// branch+tag list.
func (v *Variables) OpenRefPicker(refs []api.Ref) {
	v.refPickerActive = true
	v.refOptions = refs
	v.refCursor = 0
}

func (v *Variables) syncRows() {
	rows := make([]table.Row, 0, len(v.rows)+1)
	for _, r := range v.rows {
		value := r.Value
		if r.Masked {
			value = "••••••"
		}
		rows = append(rows, table.Row{r.Key, value, string(r.Type), boolMark(r.Masked), boolMark(r.Protected)})
	}
	rows = append(rows, table.Row{"[ Dispatch → " + fmt.Sprint(len(v.Projects)) + " project(s) ]", "", "", "", ""})
	setRows(&v.table, rows)
}

func boolMark(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// HasTextFocus reports whether the ref input or an inline row editor
// currently owns keystrokes.
func (v *Variables) HasTextFocus() bool {
	return v.focusRef || v.editing != varEditNone
}

func (v Variables) isDispatchRow() bool {
	return v.table.Cursor() == len(v.rows)
}

func (v Variables) Update(msg tea.Msg) (Variables, tea.Cmd) {
	if v.refPickerActive {
		return v.updateRefPicker(msg)
	}
	if v.editing != varEditNone {
		return v.updateEditing(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			v.Active = false
			return v, nil
		case "ctrl+r":
			projectID := v.FirstProjectID()
			return v, func() tea.Msg { return RefPickerRequestMsg{ProjectID: projectID} }
		case "tab":
			v.focusRef = !v.focusRef
			if v.focusRef {
				v.RefInput.Focus()
			} else {
				v.RefInput.Blur()
			}
			return v, nil
		}
		if v.focusRef {
			var cmd tea.Cmd
			v.RefInput, cmd = v.RefInput.Update(msg)
			v.Ref = v.RefInput.Value()
			return v, cmd
		}
		switch km.String() {
		case "a":
			v.rows = append(v.rows, api.Variable{Type: api.VarTypeEnv})
			v.syncRows()
			v.table.SetCursor(len(v.rows) - 1)
			return v.startEdit(varEditKey), nil
		case "d":
			if !v.isDispatchRow() && len(v.rows) > 0 {
				i := v.table.Cursor()
				v.rows = append(v.rows[:i], v.rows[i+1:]...)
				v.syncRows()
			}
			return v, nil
		case "t":
			if !v.isDispatchRow() {
				i := v.table.Cursor()
				if v.rows[i].Type == api.VarTypeEnv {
					v.rows[i].Type = api.VarTypeFile
				} else {
					v.rows[i].Type = api.VarTypeEnv
				}
				v.syncRows()
			}
			return v, nil
		case "m":
			if !v.isDispatchRow() {
				i := v.table.Cursor()
				v.rows[i].Masked = !v.rows[i].Masked
				v.syncRows()
			}
			return v, nil
		case "p":
			if !v.isDispatchRow() {
				i := v.table.Cursor()
				v.rows[i].Protected = !v.rows[i].Protected
				v.syncRows()
			}
			return v, nil
		case "enter":
			if v.isDispatchRow() {
				v.Active = false
				return v, func() tea.Msg {
					return DispatchMsg{Projects: v.Projects, Ref: v.Ref, Vars: v.rows}
				}
			}
			return v.startEdit(varEditKey), nil
		}
	}

	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return v, cmd
}

func (v Variables) updateRefPicker(msg tea.Msg) (Variables, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return v, nil
	}
	switch km.String() {
	case "esc":
		v.refPickerActive = false
	case "j", "down":
		if v.refCursor < len(v.refOptions)-1 {
			v.refCursor++
		}
	case "k", "up":
		if v.refCursor > 0 {
			v.refCursor--
		}
	case "enter":
		if v.refCursor < len(v.refOptions) {
			v.Ref = v.refOptions[v.refCursor].Name
			v.RefInput.SetValue(v.Ref)
		}
		v.refPickerActive = false
	}
	return v, nil
}

func (v Variables) startEdit(field varEditState) Variables {
	v.editing = field
	i := v.table.Cursor()
	if field == varEditKey {
		v.keyInput.SetValue(v.rows[i].Key)
		v.keyInput.Focus()
	} else {
		v.valueInput.SetValue(v.rows[i].Value)
		v.valueInput.Focus()
	}
	return v
}

func (v Variables) updateEditing(msg tea.Msg) (Variables, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		i := v.table.Cursor()
		switch km.String() {
		case "esc":
			v.editing = varEditNone
			v.keyInput.Blur()
			v.valueInput.Blur()
			return v, nil
		case "enter", "tab":
			if v.editing == varEditKey {
				v.rows[i].Key = v.keyInput.Value()
				v.keyInput.Blur()
				v.syncRows()
				return v.startEdit(varEditValue), nil
			}
			v.rows[i].Value = v.valueInput.Value()
			v.valueInput.Blur()
			v.editing = varEditNone
			v.syncRows()
			return v, nil
		}
	}
	var cmd tea.Cmd
	if v.editing == varEditKey {
		v.keyInput, cmd = v.keyInput.Update(msg)
	} else {
		v.valueInput, cmd = v.valueInput.Update(msg)
	}
	return v, cmd
}

func (v Variables) View() string {
	if v.refPickerActive {
		return v.refPickerViewString()
	}

	var b string
	b += v.RefInput.View() + "\n\n"
	b += v.table.View() + "\n"
	if v.editing == varEditKey {
		b += "\n" + v.keyInput.View()
	} else if v.editing == varEditValue {
		b += "\n" + v.valueInput.View()
	}
	b += "\n\n" + RenderHelp(
		[2]string{"a", "add"},
		[2]string{"d", "delete"},
		[2]string{"t", "type"},
		[2]string{"m", "masked"},
		[2]string{"p", "protected"},
		[2]string{"tab", "ref/rows"},
		[2]string{"ctrl+r", "browse refs"},
		[2]string{"enter", "edit/dispatch"},
		[2]string{"esc", "cancel"},
	)
	return lipgloss.NewStyle().Render(b)
}

func (v Variables) refPickerViewString() string {
	var b string
	b += "Available refs (branches + tags):\n\n"
	for i, r := range v.refOptions {
		marker := "  "
		if i == v.refCursor {
			marker = "> "
		}
		kind := "branch"
		if r.IsTag {
			kind = "tag"
		}
		b += fmt.Sprintf("%s%s (%s)\n", marker, r.Name, kind)
	}
	if len(v.refOptions) == 0 {
		b += "(no refs found)\n"
	}
	b += "\n" + RenderHelp(
		[2]string{"j/k", "move"},
		[2]string{"enter", "select"},
		[2]string{"esc", "cancel"},
	)
	return lipgloss.NewStyle().Render(b)
}
