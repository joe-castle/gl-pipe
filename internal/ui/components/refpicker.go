package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// refPicker is the shared "browse branches+tags, pick one" overlay: the
// trigger modal's ctrl+r (pick the ref to dispatch with) and the project
// explorer's ctrl+r (override a project's locked ref) show the same list
// and take the same keys, differing only in what the owning component does
// with the selection once made.
type refPicker struct {
	active  bool
	options []api.Ref
	cursor  int
}

// Open activates the overlay with a freshly-fetched ref list.
func (r *refPicker) Open(refs []api.Ref) {
	r.active = true
	r.options = refs
	r.cursor = 0
}

// update handles one key while the overlay is active. selected is true
// only on the frame the user confirms a ref with enter (and there was one
// to confirm) — refPicker has no way to call back into the owning
// component itself, so the caller acts on ref then.
func (r refPicker) update(msg tea.Msg) (next refPicker, selected bool, ref api.Ref) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return r, false, api.Ref{}
	}
	switch km.String() {
	case "esc":
		r.active = false
	case "j", "down":
		if r.cursor < len(r.options)-1 {
			r.cursor++
		}
	case "k", "up":
		if r.cursor > 0 {
			r.cursor--
		}
	case "enter":
		r.active = false
		if r.cursor < len(r.options) {
			return r, true, r.options[r.cursor]
		}
	}
	return r, false, api.Ref{}
}

func (r refPicker) viewString() string {
	var b string
	b += "Available refs (branches + tags):\n\n"
	for i, ref := range r.options {
		marker := "  "
		if i == r.cursor {
			marker = "> "
		}
		kind := "branch"
		if ref.IsTag {
			kind = "tag"
		}
		b += fmt.Sprintf("%s%s (%s)\n", marker, ref.Name, kind)
	}
	if len(r.options) == 0 {
		b += "(no refs found)\n"
	}
	b += "\n" + RenderHelp(
		[2]string{"j/k", "move"},
		[2]string{"enter", "select"},
		[2]string{"esc", "cancel"},
	)
	return lipgloss.NewStyle().Render(b)
}
