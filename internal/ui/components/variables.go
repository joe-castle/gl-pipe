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
		{Title: "Key", Width: 24},
		{Title: "Value", Width: 24},
		{Title: "Type", Width: 8},
		{Title: "Masked", Width: 7},
		{Title: "Protected", Width: 9},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(10))

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
}

func (v *Variables) Close() { v.Active = false }

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
	if v.editing != varEditNone {
		return v.updateEditing(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			v.Active = false
			return v, nil
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
	var b string
	b += v.RefInput.View() + "\n\n"
	b += v.table.View() + "\n"
	if v.editing == varEditKey {
		b += "\n" + v.keyInput.View()
	} else if v.editing == varEditValue {
		b += "\n" + v.valueInput.View()
	}
	b += "\n\na: add · d: delete · t: type · m: masked · p: protected · tab: ref/rows · enter: edit/dispatch · esc: cancel"
	return lipgloss.NewStyle().Render(b)
}
