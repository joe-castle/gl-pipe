package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// field is one labeled input in a fieldForm, as (label, starting value).
type field struct {
	label string
	value string
}

// fieldForm is a small labeled-text-input form: tab/shift+tab (or up/down)
// to move between fields, enter to submit from anywhere, esc to cancel.
//
// It exists because the settings editor needs the same "edit these few
// strings" shape five times over (instance, preset, TTL, max age, and the
// add-variants of the first two) and hand-rolling focus juggling per screen
// is exactly the duplication that goes stale.
type fieldForm struct {
	title  string
	labels []string
	inputs []textinput.Model
	focus  int
}

func newFieldForm(title string, fields ...field) fieldForm {
	f := fieldForm{title: title}
	for i, fd := range fields {
		in := textinput.New()
		in.Prompt = ""
		in.Width = 48
		in.SetValue(fd.value)
		if i == 0 {
			in.Focus()
		}
		f.labels = append(f.labels, fd.label)
		f.inputs = append(f.inputs, in)
	}
	return f
}

func (f fieldForm) value(i int) string {
	if i < 0 || i >= len(f.inputs) {
		return ""
	}
	return f.inputs[i].Value()
}

func (f fieldForm) setFocus(i int) fieldForm {
	if len(f.inputs) == 0 {
		return f
	}
	i = (i + len(f.inputs)) % len(f.inputs)
	for j := range f.inputs {
		if j == i {
			f.inputs[j].Focus()
		} else {
			f.inputs[j].Blur()
		}
	}
	f.focus = i
	return f
}

func (f fieldForm) update(msg tea.Msg) (fieldForm, bool, bool, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			return f, true, false, nil
		case "esc":
			return f, false, true, nil
		case "tab", "down":
			return f.setFocus(f.focus + 1), false, false, nil
		case "shift+tab", "up":
			return f.setFocus(f.focus - 1), false, false, nil
		}
	}
	var cmd tea.Cmd
	if f.focus < len(f.inputs) {
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	}
	return f, false, false, cmd
}

func (f fieldForm) viewString() string {
	var b strings.Builder
	b.WriteString(helpKeyStyle.Render(f.title) + "\n\n")
	for i, in := range f.inputs {
		marker := "  "
		if i == f.focus {
			marker = "> "
		}
		b.WriteString(marker + helpDescStyle.Render(pad(f.labels[i], 30)) + in.View() + "\n")
	}
	return b.String()
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-len(s))
}
