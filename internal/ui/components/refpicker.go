package components

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// refPicker is the shared "browse branches+tags, pick one" overlay: the
// trigger modal's ctrl+r (pick the ref to dispatch with) and the project
// explorer's ctrl+r (override a project's locked ref) show the same list
// and take the same keys, differing only in what the owning component does
// with the selection once made.
//
// Backed by table.Model (like every other list in the app) rather than a
// hand-rolled cursor + flat string: a project can easily have more refs
// than fit on screen, and bubbles/table already gives scrolling that
// follows the cursor for free — a manually-rendered list didn't, so with
// enough refs the view just showed a fixed window near the top with no
// way to reach anything past it.
type refPicker struct {
	active  bool
	options []api.Ref
	table   table.Model
}

// Open activates the overlay with a freshly-fetched ref list.
func (r *refPicker) Open(refs []api.Ref) {
	r.active = true
	r.options = refs

	rows := make([]table.Row, len(refs))
	for i, ref := range refs {
		kind := "branch"
		if ref.IsTag {
			kind = "tag"
		}
		rows[i] = table.Row{ref.Name, kind}
	}
	cols := []table.Column{
		{Title: "REF", Width: 40},
		{Title: "TYPE", Width: 8},
	}
	r.table = table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))
	setRows(&r.table, rows)
}

// update handles one key while the overlay is active. selected is true
// only on the frame the user confirms a ref with enter (and there was one
// to confirm) — refPicker has no way to call back into the owning
// component itself, so the caller acts on ref then. Movement keys (up/
// down/j/k/pgup/pgdown/g/G) fall through to the table itself, which is
// what gives scrolling for a list longer than the visible height.
func (r refPicker) update(msg tea.Msg) (next refPicker, selected bool, ref api.Ref) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			r.active = false
			return r, false, api.Ref{}
		case "enter":
			r.active = false
			if i := r.table.Cursor(); i >= 0 && i < len(r.options) {
				return r, true, r.options[i]
			}
			return r, false, api.Ref{}
		}
	}
	var cmd tea.Cmd
	r.table, cmd = r.table.Update(msg)
	_ = cmd // table.Update never returns a Cmd worth propagating here
	return r, false, api.Ref{}
}

func (r refPicker) viewString() string {
	b := "Available refs (branches + tags):\n\n" + r.table.View()
	if len(r.options) == 0 {
		b += "\n(no refs found)\n"
	}
	b += "\n\n" + RenderHelp(
		[2]string{"j/k", "move"},
		[2]string{"enter", "select"},
		[2]string{"esc", "cancel"},
	)
	return lipgloss.NewStyle().Render(b)
}
