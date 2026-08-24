package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/joeca/gl-pipe/internal/api"
)

// GroupsChosenMsg is emitted when the user saves their group selection from
// the discovery picker (<Space> g). The root model merges FullPaths into
// the active instance's default_groups and triggers a resync.
type GroupsChosenMsg struct {
	FullPaths []string
}

// GroupPicker lists every group the authenticated user belongs to and lets
// them multi-select which ones to sync projects from, so default_groups
// never has to be hand-typed into config.yaml.
type GroupPicker struct {
	Active bool

	all      []api.Group
	filtered []api.Group

	filtering   bool
	filterInput textinput.Model
	table       table.Model

	Selected map[string]bool // keyed by Group.FullPath
}

func NewGroupPicker() GroupPicker {
	filter := textinput.New()
	filter.Placeholder = "fuzzy filter..."
	filter.Prompt = "/ "

	cols := []table.Column{
		{Title: "", Width: 3},
		{Title: "Group", Width: 30},
		{Title: "Path", Width: 40},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15))

	return GroupPicker{filterInput: filter, table: tbl, Selected: map[string]bool{}}
}

// Open activates the picker with the freshly-fetched group list, pre-checking
// any group already present in the instance's default_groups.
func (g *GroupPicker) Open(groups []api.Group, alreadyConfigured []string) {
	g.Active = true
	g.all = groups
	g.filtering = false
	g.filterInput.SetValue("")
	g.filterInput.Blur()
	g.Selected = map[string]bool{}
	for _, path := range alreadyConfigured {
		g.Selected[path] = true
	}
	g.applyFilter()
}

func (g *GroupPicker) applyFilter() {
	query := g.filterInput.Value()
	if query == "" {
		g.filtered = g.all
	} else {
		paths := make([]string, len(g.all))
		for i, grp := range g.all {
			paths[i] = grp.FullPath
		}
		matches := fuzzy.Find(query, paths)
		g.filtered = make([]api.Group, len(matches))
		for i, m := range matches {
			g.filtered[i] = g.all[m.Index]
		}
	}
	g.syncRows()
}

func (g *GroupPicker) syncRows() {
	rows := make([]table.Row, 0, len(g.filtered)+1)
	for _, grp := range g.filtered {
		check := " "
		if g.Selected[grp.FullPath] {
			check = "x"
		}
		rows = append(rows, table.Row{check, grp.Name, grp.FullPath})
	}
	rows = append(rows, table.Row{fmt.Sprintf("[ Save %d group(s) ]", len(g.Selected)), "", ""})
	g.table.SetRows(rows)
}

func (g *GroupPicker) isSaveRow() bool {
	return g.table.Cursor() == len(g.filtered)
}

// HasTextFocus reports whether the fuzzy-filter input owns keystrokes.
func (g *GroupPicker) HasTextFocus() bool { return g.filtering }

func (g GroupPicker) Update(msg tea.Msg) (GroupPicker, tea.Cmd) {
	if g.filtering {
		return g.updateFilter(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			g.Active = false
			return g, nil
		case "/":
			g.filtering = true
			g.filterInput.Focus()
			return g, nil
		case "x":
			if !g.isSaveRow() {
				grp := g.filtered[g.table.Cursor()]
				g.Selected[grp.FullPath] = !g.Selected[grp.FullPath]
				g.syncRows()
			}
			return g, nil
		case "enter":
			if g.isSaveRow() {
				paths := make([]string, 0, len(g.Selected))
				for p, on := range g.Selected {
					if on {
						paths = append(paths, p)
					}
				}
				g.Active = false
				return g, func() tea.Msg { return GroupsChosenMsg{FullPaths: paths} }
			}
			grp := g.filtered[g.table.Cursor()]
			g.Selected[grp.FullPath] = !g.Selected[grp.FullPath]
			g.syncRows()
			return g, nil
		}
	}

	var cmd tea.Cmd
	g.table, cmd = g.table.Update(msg)
	return g, cmd
}

func (g GroupPicker) updateFilter(msg tea.Msg) (GroupPicker, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			g.filterInput.SetValue("")
			g.filterInput.Blur()
			g.filtering = false
			g.applyFilter()
			return g, nil
		case "enter":
			g.filterInput.Blur()
			g.filtering = false
			return g, nil
		}
	}
	var cmd tea.Cmd
	g.filterInput, cmd = g.filterInput.Update(msg)
	g.applyFilter()
	return g, cmd
}

func (g GroupPicker) View() string {
	var b strings.Builder
	b.WriteString("Discover groups — projects sync from whichever you select here.\n\n")
	if g.filtering {
		b.WriteString(g.filterInput.View() + "\n")
	} else {
		b.WriteString(fmt.Sprintf("%d group(s), %d selected\n", len(g.filtered), len(g.Selected)))
	}
	b.WriteString(g.table.View())
	if len(g.all) == 0 {
		b.WriteString("\n(no groups found — you may not be a member of any)")
	}
	b.WriteString("\n\n/: filter · x/enter: toggle · enter on last row: save · esc: cancel")
	return lipgloss.NewStyle().Render(b.String())
}
