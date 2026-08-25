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

// MRsChosenMsg is emitted when the user picks merge request(s) to jump to
// the pipelines of — the staged (or highlighted-fallback) MR(s), same
// convention as everywhere else in the app.
type MRsChosenMsg struct {
	MRs []api.MergeRequest
}

// MRList is the merge-request picker modal (<Space> m for "my MRs" across
// every synced project, M on the explorer for one project's MRs): fuzzy
// filter, multi-select, Enter jumps the selection's pipelines into the
// existing pipeline matrix. It never shows pipelines itself — that reuses
// PipelineList entirely, so sort/filter/bulk-retry all just work.
type MRList struct {
	Active bool

	all      []api.MergeRequest
	filtered []api.MergeRequest

	table       table.Model
	filterInput textinput.Model
	filtering   bool

	Selected map[int]bool // MR ID -> staged

	projectNames map[int]string
}

func NewMRList() MRList {
	filter := textinput.New()
	filter.Placeholder = "fuzzy filter..."
	filter.Prompt = "/ "

	cols := []table.Column{
		{Title: "", Width: 3},
		{Title: "", Width: 5},
		{Title: "PROJECT", Width: 20},
		{Title: "TITLE", Width: 40},
		{Title: "BRANCH", Width: 24},
		{Title: "AUTHOR", Width: 12},
		{Title: "UPDATED", Width: 10},
	}
	tbl := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))

	return MRList{table: tbl, filterInput: filter, Selected: map[int]bool{}, projectNames: map[int]string{}}
}

// SetProjectNames supplies a project ID -> path lookup for the Project column.
func (l *MRList) SetProjectNames(names map[int]string) { l.projectNames = names }

func (l *MRList) SetSize(w, h int) {
	const checkW, draftW, branchW, authorW, updatedW = 3, 5, 24, 12, 10
	projectW, titleW := splitFlexWidth(w, []int{checkW, draftW, branchW, authorW, updatedW}, 0.35, 16, 24)
	l.table.SetColumns([]table.Column{
		{Title: "", Width: checkW},
		{Title: "", Width: draftW},
		{Title: "PROJECT", Width: projectW},
		{Title: "TITLE", Width: titleW},
		{Title: "BRANCH", Width: branchW},
		{Title: "AUTHOR", Width: authorW},
		{Title: "UPDATED", Width: updatedW},
	})
	l.table.SetWidth(w)
	l.table.SetHeight(h - 4)
}

// SetMRs replaces the list contents and (re)activates the modal — used to
// start a fresh "my MRs" or "this project's MRs" fetch.
func (l *MRList) SetMRs(mrs []api.MergeRequest) {
	l.Active = true
	l.all = mrs
	l.Selected = map[int]bool{}
	l.filtering = false
	l.filterInput.SetValue("")
	l.filterInput.Blur()
	l.applyFilter()
}

// AddMRs merges one project's MRs into an in-progress fetch (insert-or-
// update by ID), for batching "this project's MRs" across staged projects
// the same way the pipeline matrix batches staged projects' pipelines.
func (l *MRList) AddMRs(mrs []api.MergeRequest) {
	l.Active = true
	for _, mr := range mrs {
		l.upsert(mr)
	}
	l.applyFilter()
}

func (l *MRList) upsert(mr api.MergeRequest) {
	for i, existing := range l.all {
		if existing.ID == mr.ID {
			l.all[i] = mr
			return
		}
	}
	l.all = append(l.all, mr)
}

func (l *MRList) applyFilter() {
	query := l.filterInput.Value()
	if query == "" {
		l.filtered = l.all
		l.syncRows()
		return
	}
	haystacks := make([]string, len(l.all))
	for i, mr := range l.all {
		haystacks[i] = strings.Join([]string{mr.Title, mr.SourceBranch, l.projectNames[mr.ProjectID], mr.Author}, " ")
	}
	matches := fuzzy.Find(query, haystacks)
	l.filtered = make([]api.MergeRequest, len(matches))
	for i, m := range matches {
		l.filtered[i] = l.all[m.Index]
	}
	l.syncRows()
}

func (l *MRList) syncRows() {
	rows := make([]table.Row, len(l.filtered))
	for i, mr := range l.filtered {
		check := " "
		if l.Selected[mr.ID] {
			check = "x"
		}
		draft := ""
		if mr.Draft {
			draft = "draft"
		}
		rows[i] = table.Row{
			check,
			draft,
			l.projectNames[mr.ProjectID],
			mr.Title,
			mr.SourceBranch + " → " + mr.TargetBranch,
			mr.Author,
			formatAge(mr.UpdatedAt),
		}
	}
	setRows(&l.table, rows)
}

// Highlighted returns the MR under the cursor, if any.
func (l *MRList) Highlighted() (api.MergeRequest, bool) {
	i := l.table.Cursor()
	if i < 0 || i >= len(l.filtered) {
		return api.MergeRequest{}, false
	}
	return l.filtered[i], true
}

// SelectedMRs returns staged MRs, or the highlighted one if none staged.
func (l *MRList) SelectedMRs() []api.MergeRequest {
	if len(l.Selected) == 0 {
		if mr, ok := l.Highlighted(); ok {
			return []api.MergeRequest{mr}
		}
		return nil
	}
	out := make([]api.MergeRequest, 0, len(l.Selected))
	for _, mr := range l.all {
		if l.Selected[mr.ID] {
			out = append(out, mr)
		}
	}
	return out
}

// HasTextFocus reports whether the fuzzy-filter input owns keystrokes.
func (l *MRList) HasTextFocus() bool { return l.filtering }

// Count returns how many MRs are currently loaded (unfiltered).
func (l *MRList) Count() int { return len(l.all) }

func (l MRList) Update(msg tea.Msg) (MRList, tea.Cmd) {
	if l.filtering {
		return l.updateFilter(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			l.Active = false
			return l, nil
		case "/":
			l.filtering = true
			l.filterInput.Focus()
			return l, nil
		case "x":
			if mr, ok := l.Highlighted(); ok {
				toggleSet(l.Selected, mr.ID)
				l.syncRows()
			}
			return l, nil
		case "a":
			ids := make([]int, len(l.filtered))
			for i, mr := range l.filtered {
				ids[i] = mr.ID
			}
			toggleSetAll(l.Selected, ids)
			l.syncRows()
			return l, nil
		case "enter":
			targets := l.SelectedMRs()
			if len(targets) == 0 {
				return l, nil
			}
			l.Active = false
			return l, func() tea.Msg { return MRsChosenMsg{MRs: targets} }
		}
	}

	var cmd tea.Cmd
	l.table, cmd = l.table.Update(msg)
	return l, cmd
}

func (l MRList) updateFilter(msg tea.Msg) (MRList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			l.filterInput.SetValue("")
			l.filterInput.Blur()
			l.filtering = false
			l.applyFilter()
			return l, nil
		case "enter":
			l.filterInput.Blur()
			l.filtering = false
			return l, nil
		}
	}
	var cmd tea.Cmd
	l.filterInput, cmd = l.filterInput.Update(msg)
	l.applyFilter()
	return l, cmd
}

func (l MRList) View() string {
	var b strings.Builder
	if l.filtering {
		b.WriteString(l.filterInput.View() + "\n")
	} else {
		b.WriteString(fmt.Sprintf("%d merge request(s) (%d staged)\n", len(l.filtered), len(l.Selected)))
	}
	b.WriteString(l.table.View())
	b.WriteString("\n\n" + RenderHelp(
		[2]string{"/", "filter"},
		[2]string{"x", "stage"},
		[2]string{"a", "stage/unstage all"},
		[2]string{"enter", "jump to pipelines (staged, or highlighted)"},
		[2]string{"esc", "cancel"},
	))
	return lipgloss.NewStyle().Render(b.String())
}
