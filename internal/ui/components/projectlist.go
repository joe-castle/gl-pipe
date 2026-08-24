package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/cache"
)

// BlobSearchRequestMsg is emitted when the user submits a blob search query.
// Group is derived from a "group: query" prefix, falling back to the
// component's DefaultGroup when omitted.
type BlobSearchRequestMsg struct {
	Group string
	Query string
}

// projectListMode tracks which of the project list's overlays, if any, has
// input focus.
type projectListMode int

const (
	modeBrowse projectListMode = iota
	modeFilter
	modeBlobSearch
)

// ProjectList is the fuzzy project explorer: multi-select, `/`-filter, and
// a blob-search overlay.
type ProjectList struct {
	table       table.Model
	filterInput textinput.Model
	blobInput   textinput.Model
	mode        projectListMode

	all      []api.Project
	filtered []api.Project

	Selected     map[int]bool
	LockedRef    map[int]string
	DefaultGroup string

	BlobHits   []api.BlobHit
	blobActive bool

	width, height int
}

func NewProjectList() ProjectList {
	cols := []table.Column{
		{Title: "", Width: 3},
		{Title: "Project", Width: 40},
		{Title: "Ref", Width: 20},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	filter := textinput.New()
	filter.Placeholder = "fuzzy filter..."
	filter.Prompt = "/ "

	blob := textinput.New()
	blob.Placeholder = "group: search term (e.g. backend: @SpringBootApplication)"
	blob.Prompt = "blob search: "
	blob.Width = 60

	return ProjectList{
		table:       t,
		filterInput: filter,
		blobInput:   blob,
		Selected:    map[int]bool{},
		LockedRef:   map[int]string{},
	}
}

// SetProjects replaces the full project set (e.g. after a cache sync) and
// re-applies the current filter.
func (p *ProjectList) SetProjects(projects []api.Project) {
	p.all = projects
	p.applyFilter()
}

func (p *ProjectList) applyFilter() {
	idx := cache.Index{Projects: p.all}
	p.filtered = idx.Filter(p.filterInput.Value())
	p.syncRows()
}

func (p *ProjectList) syncRows() {
	rows := make([]table.Row, len(p.filtered))
	for i, proj := range p.filtered {
		check := " "
		if p.Selected[proj.ID] {
			check = "x"
		}
		ref := "(default)"
		if r, ok := p.LockedRef[proj.ID]; ok {
			ref = r
		}
		rows[i] = table.Row{check, proj.PathWithNamespace, ref}
	}
	setRows(&p.table, rows)
}

// Highlighted returns the project currently under the cursor, if any.
func (p *ProjectList) Highlighted() (api.Project, bool) {
	i := p.table.Cursor()
	if i < 0 || i >= len(p.filtered) {
		return api.Project{}, false
	}
	return p.filtered[i], true
}

// SelectedProjects returns every staged (x-toggled) project, or just the
// highlighted one if nothing is staged, matching the spec's "1 to N repos"
// batch semantics.
func (p *ProjectList) SelectedProjects() []api.Project {
	if len(p.Selected) == 0 {
		if proj, ok := p.Highlighted(); ok {
			return []api.Project{proj}
		}
		return nil
	}
	out := make([]api.Project, 0, len(p.Selected))
	for _, proj := range p.all {
		if p.Selected[proj.ID] {
			out = append(out, proj)
		}
	}
	return out
}

func (p *ProjectList) SetSize(w, h int) {
	p.width, p.height = w, h
	p.table.SetWidth(w)
	p.table.SetHeight(h - 4)
}

// HasTextFocus reports whether a text input currently owns keystrokes, so
// the root model knows not to steal <Space> for the leader menu.
func (p *ProjectList) HasTextFocus() bool {
	return p.mode == modeFilter || p.mode == modeBlobSearch
}

// OpenBlobSearch activates the blob-search overlay.
func (p *ProjectList) OpenBlobSearch() {
	p.mode = modeBlobSearch
	p.blobActive = true
	p.blobInput.Focus()
}

func (p ProjectList) Update(msg tea.Msg) (ProjectList, tea.Cmd) {
	switch p.mode {
	case modeBlobSearch:
		return p.updateBlobSearch(msg)
	case modeFilter:
		return p.updateFilter(msg)
	default:
		return p.updateBrowse(msg)
	}
}

func (p ProjectList) updateBlobSearch(msg tea.Msg) (ProjectList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			p.mode = modeBrowse
			p.blobActive = false
			p.blobInput.Blur()
			return p, nil
		case "enter":
			text := p.blobInput.Value()
			group, query := p.DefaultGroup, text
			if idx := strings.Index(text, ":"); idx >= 0 {
				group = strings.TrimSpace(text[:idx])
				query = strings.TrimSpace(text[idx+1:])
			}
			if query == "" {
				return p, nil
			}
			return p, func() tea.Msg { return BlobSearchRequestMsg{Group: group, Query: query} }
		}
	}
	var cmd tea.Cmd
	p.blobInput, cmd = p.blobInput.Update(msg)
	return p, cmd
}

func (p ProjectList) updateFilter(msg tea.Msg) (ProjectList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			p.filterInput.SetValue("")
			p.filterInput.Blur()
			p.mode = modeBrowse
			p.applyFilter()
			return p, nil
		case "enter":
			p.filterInput.Blur()
			p.mode = modeBrowse
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.applyFilter()
	return p, cmd
}

func (p ProjectList) updateBrowse(msg tea.Msg) (ProjectList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "/":
			p.mode = modeFilter
			p.filterInput.Focus()
			return p, nil
		case "x":
			if proj, ok := p.Highlighted(); ok {
				toggleSet(p.Selected, proj.ID)
				p.syncRows()
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	return p, cmd
}

func (p ProjectList) View() string {
	var b strings.Builder
	if p.mode == modeFilter {
		b.WriteString(p.filterInput.View() + "\n")
	} else {
		b.WriteString(fmt.Sprintf("%d projects (%d selected)\n", len(p.filtered), len(p.Selected)))
	}
	b.WriteString(p.table.View())
	if p.mode == modeBlobSearch {
		b.WriteString("\n\n" + p.blobInput.View())
		if len(p.BlobHits) > 0 {
			b.WriteString("\n")
			for i, h := range p.BlobHits {
				if i >= 10 {
					b.WriteString(fmt.Sprintf("... and %d more\n", len(p.BlobHits)-10))
					break
				}
				b.WriteString(fmt.Sprintf("  %s:%d %s\n", h.Path, h.StartLine, h.ProjectPath))
			}
		}
	}
	return lipgloss.NewStyle().Render(b.String())
}
