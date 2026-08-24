package components

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// OpenJobsMsg asks the root model to load the job matrix for one or more
// pipelines (the staged, or highlighted-fallback, pipelines from the
// matrix — same convention as batch triggering).
type OpenJobsMsg struct {
	Pipelines []api.Pipeline
}

// OpenLogsMsg asks the root model to open the live log viewer for one job.
type OpenLogsMsg struct {
	ProjectID int
	JobID     int
}

// BulkPipelineActionMsg requests a retry or cancel across the selected (or
// highlighted) pipelines. Retry is true for retry, false for cancel.
type BulkPipelineActionMsg struct {
	Targets []api.Pipeline
	Retry   bool
}

// BulkJobActionMsg is the job-matrix equivalent of BulkPipelineActionMsg.
type BulkJobActionMsg struct {
	Targets []api.Job
	Retry   bool
}

type pipelineListMode int

const (
	modePipelines pipelineListMode = iota
	modeJobs
)

// pipelineSortField is one column the pipeline matrix can be sorted by,
// cycled with 's'.
type pipelineSortField int

const (
	sortByDate pipelineSortField = iota
	sortByProject
	sortByStatus
	sortByRef
	sortBySHA
	sortByAuthor
	sortByDuration
	sortFieldCount
)

func (f pipelineSortField) String() string {
	switch f {
	case sortByDate:
		return "Date"
	case sortByProject:
		return "Project"
	case sortByStatus:
		return "Status"
	case sortByRef:
		return "Ref"
	case sortBySHA:
		return "SHA"
	case sortByAuthor:
		return "Author"
	case sortByDuration:
		return "Duration"
	default:
		return ""
	}
}

// PipelineList renders the multi-project pipeline matrix and, drilled into
// one pipeline, its job matrix.
type PipelineList struct {
	mode pipelineListMode

	pipelines    []api.Pipeline // full set fetched into the matrix
	filtered     []api.Pipeline // pipelines after the text filter + sort; what's actually shown
	projectNames map[int]string
	pipeTable    table.Model
	Selected     map[int]bool // pipeline ID -> staged for bulk action (against the full set)
	sortField    pipelineSortField
	sortDesc     bool
	filtering    bool
	filterInput  textinput.Model

	jobs      []api.Job
	jobTable  table.Model
	Pipelines []api.Pipeline // the pipeline(s) whose jobs are currently shown
	SelectedJ map[int]bool   // job ID -> staged for bulk action

	width, height int
}

func NewPipelineList() PipelineList {
	pipeCols := []table.Column{
		{Title: "", Width: 3},
		{Title: "Project", Width: 24},
		{Title: "Status", Width: 12},
		{Title: "Ref", Width: 16},
		{Title: "SHA", Width: 10},
		{Title: "Author", Width: 12},
		{Title: "Duration", Width: 10},
		{Title: "Date", Width: 12},
	}
	pipeTable := table.New(table.WithColumns(pipeCols), table.WithFocused(true), table.WithHeight(15))

	jobCols := []table.Column{
		{Title: "", Width: 3},
		{Title: "Project", Width: 18},
		{Title: "Pipeline", Width: 9},
		{Title: "Stage", Width: 12},
		{Title: "Job", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Runner", Width: 12},
		{Title: "Retries", Width: 7},
		{Title: "Duration", Width: 9},
	}
	jobTable := table.New(table.WithColumns(jobCols), table.WithFocused(true), table.WithHeight(15))

	filter := textinput.New()
	filter.Placeholder = "filter by ref, status, project, author, or SHA..."
	filter.Prompt = "/ "

	return PipelineList{
		pipeTable:    pipeTable,
		jobTable:     jobTable,
		projectNames: map[int]string{},
		Selected:     map[int]bool{},
		SelectedJ:    map[int]bool{},
		sortField:    sortByDate,
		sortDesc:     true,
		filterInput:  filter,
	}
}

func (p *PipelineList) SetSize(w, h int) {
	p.width, p.height = w, h
	p.pipeTable.SetWidth(w)
	p.pipeTable.SetHeight(h - 3)
	p.jobTable.SetWidth(w)
	p.jobTable.SetHeight(h - 3)
}

// SetProjectNames supplies a project ID -> path lookup for the Project column.
func (p *PipelineList) SetProjectNames(names map[int]string) { p.projectNames = names }

// SetPipelines replaces the pipeline matrix contents. The text filter is
// reset — a fresh batch (e.g. re-pressing Enter for a different staged
// set) shouldn't leave stale filter text quietly hiding the new rows.
func (p *PipelineList) SetPipelines(pipelines []api.Pipeline) {
	p.pipelines = pipelines
	p.mode = modePipelines
	p.filtering = false
	p.filterInput.SetValue("")
	p.filterInput.Blur()
	p.syncPipeRows()
}

// UpdatePipeline patches one row in place (used when a detail/enrichment or
// retry/cancel response comes back for a pipeline already in the list).
func (p *PipelineList) UpdatePipeline(updated api.Pipeline) {
	for i, pl := range p.pipelines {
		if pl.ID == updated.ID {
			p.pipelines[i] = updated
			p.syncPipeRows()
			return
		}
	}
}

// AddOrUpdate inserts a new pipeline row, or patches it in place if a row
// for that pipeline ID already exists (e.g. one repo of a batch trigger
// completing after the matrix is already showing others).
func (p *PipelineList) AddOrUpdate(pl api.Pipeline) {
	for i, existing := range p.pipelines {
		if existing.ID == pl.ID {
			p.pipelines[i] = pl
			p.syncPipeRows()
			return
		}
	}
	p.pipelines = append(p.pipelines, pl)
	p.mode = modePipelines
	p.syncPipeRows()
}

// SortBy sets the sort field, always resetting to descending order (the
// default for a freshly-chosen column — most relevant first).
func (p *PipelineList) SortBy(field pipelineSortField) {
	p.sortField = field
	p.sortDesc = true
	p.syncPipeRows()
}

// CycleSort advances to the next sort field, per 's'.
func (p *PipelineList) CycleSort() {
	p.SortBy((p.sortField + 1) % sortFieldCount)
}

// ToggleSortDirection flips ascending/descending on the current sort
// field, per 'S', without changing which field is sorted.
func (p *PipelineList) ToggleSortDirection() {
	p.sortDesc = !p.sortDesc
	p.syncPipeRows()
}

// pipelineMatches reports whether pl matches query as a case-insensitive
// substring of its ref, status, project, author, or (short) SHA — covering
// both "find pipelines for this branch" and "show only failed" in one box.
func (p *PipelineList) pipelineMatches(pl api.Pipeline, query string) bool {
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		pl.Ref, string(pl.Status), p.projectNames[pl.ProjectID], pl.User, pl.SHA,
	}, " "))
	return strings.Contains(haystack, query)
}

func (p *PipelineList) filterPipelines() {
	query := strings.ToLower(p.filterInput.Value())
	if query == "" {
		p.filtered = append([]api.Pipeline(nil), p.pipelines...)
		return
	}
	filtered := make([]api.Pipeline, 0, len(p.pipelines))
	for _, pl := range p.pipelines {
		if p.pipelineMatches(pl, query) {
			filtered = append(filtered, pl)
		}
	}
	p.filtered = filtered
}

func (p *PipelineList) sortPipelines() {
	less := func(i, j int) bool {
		a, b := p.filtered[i], p.filtered[j]
		switch p.sortField {
		case sortByDate:
			return a.CreatedAt.Before(b.CreatedAt)
		case sortByProject:
			return p.projectNames[a.ProjectID] < p.projectNames[b.ProjectID]
		case sortByStatus:
			return a.Status < b.Status
		case sortByRef:
			return a.Ref < b.Ref
		case sortBySHA:
			return a.SHA < b.SHA
		case sortByAuthor:
			return a.User < b.User
		case sortByDuration:
			return a.Duration < b.Duration
		default:
			return false
		}
	}
	if p.sortDesc {
		sort.SliceStable(p.filtered, func(i, j int) bool { return less(j, i) })
	} else {
		sort.SliceStable(p.filtered, less)
	}
}

func (p *PipelineList) syncPipeRows() {
	p.filterPipelines()
	p.sortPipelines()
	rows := make([]table.Row, len(p.filtered))
	for i, pl := range p.filtered {
		check := " "
		if p.Selected[pl.ID] {
			check = "x"
		}
		sha := pl.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		rows[i] = table.Row{
			check,
			p.projectNames[pl.ProjectID],
			string(pl.Status),
			pl.Ref,
			sha,
			pl.User,
			formatDuration(pl.Duration),
			formatAge(pl.CreatedAt),
		}
	}
	setRows(&p.pipeTable, rows)
}

func formatDuration(d interface{ Seconds() float64 }) string {
	secs := int(d.Seconds())
	if secs <= 0 {
		return "-"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// formatAge renders a compact "time since" label (pipeline matrix Date
// column), since absolute timestamps don't fit the column width and
// relative age is what you actually want at a glance.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// HighlightedPipeline returns the pipeline under the cursor, if any.
func (p *PipelineList) HighlightedPipeline() (api.Pipeline, bool) {
	i := p.pipeTable.Cursor()
	if i < 0 || i >= len(p.filtered) {
		return api.Pipeline{}, false
	}
	return p.filtered[i], true
}

// SelectedPipelines returns staged pipelines, or the highlighted one if
// nothing is staged.
func (p *PipelineList) SelectedPipelines() []api.Pipeline {
	if len(p.Selected) == 0 {
		if pl, ok := p.HighlightedPipeline(); ok {
			return []api.Pipeline{pl}
		}
		return nil
	}
	out := make([]api.Pipeline, 0, len(p.Selected))
	for _, pl := range p.pipelines {
		if p.Selected[pl.ID] {
			out = append(out, pl)
		}
	}
	return out
}

// FindPipeline looks up a pipeline already known to the matrix by ID.
func (p *PipelineList) FindPipeline(id int) (api.Pipeline, bool) {
	for _, pl := range p.pipelines {
		if pl.ID == id {
			return pl, true
		}
	}
	return api.Pipeline{}, false
}

// ClearJobs resets the job matrix and enters job-matrix mode, ready for
// AddJobs calls to fill it in — mirrors how SetPipelines(nil) then
// AddOrUpdate is used to build up the multi-project pipeline matrix.
func (p *PipelineList) ClearJobs() {
	p.jobs = nil
	p.Pipelines = nil
	p.mode = modeJobs
	p.SelectedJ = map[int]bool{}
	p.syncJobRows()
}

// AddJobs merges one pipeline's jobs into the matrix (insert-or-update by
// job ID), and records that pipeline as part of the current view if it
// isn't already — this is how the job matrix can show several pipelines,
// from the same or different projects, at once.
func (p *PipelineList) AddJobs(pipeline api.Pipeline, jobs []api.Job) {
	known := false
	for _, existing := range p.Pipelines {
		if existing.ID == pipeline.ID {
			known = true
			break
		}
	}
	if !known {
		p.Pipelines = append(p.Pipelines, pipeline)
	}
	for _, j := range jobs {
		p.upsertJob(j)
	}
	p.mode = modeJobs
	p.syncJobRows()
}

func (p *PipelineList) upsertJob(j api.Job) {
	for i, existing := range p.jobs {
		if existing.ID == j.ID {
			p.jobs[i] = j
			return
		}
	}
	p.jobs = append(p.jobs, j)
}

func (p *PipelineList) syncJobRows() {
	byID := make(map[int]api.Pipeline, len(p.Pipelines))
	for _, pl := range p.Pipelines {
		byID[pl.ID] = pl
	}

	rows := make([]table.Row, len(p.jobs))
	for i, j := range p.jobs {
		check := " "
		if p.SelectedJ[j.ID] {
			check = "x"
		}
		retries := "-"
		if j.RetryCount > 0 {
			retries = fmt.Sprint(j.RetryCount)
		}
		pipelineLabel := "-"
		if pl, ok := byID[j.PipelineID]; ok {
			pipelineLabel = fmt.Sprintf("#%d", pl.IID)
		}
		rows[i] = table.Row{
			check,
			p.projectNames[j.ProjectID],
			pipelineLabel,
			j.Stage,
			j.Name,
			string(j.Status),
			j.RunnerTag,
			retries,
			formatDuration(j.Duration),
		}
	}
	setRows(&p.jobTable, rows)
}

// HighlightedJob returns the job under the cursor in job-matrix mode.
func (p *PipelineList) HighlightedJob() (api.Job, bool) {
	i := p.jobTable.Cursor()
	if i < 0 || i >= len(p.jobs) {
		return api.Job{}, false
	}
	return p.jobs[i], true
}

// SelectedJobs returns staged jobs, or the highlighted one if none staged.
func (p *PipelineList) SelectedJobs() []api.Job {
	if len(p.SelectedJ) == 0 {
		if j, ok := p.HighlightedJob(); ok {
			return []api.Job{j}
		}
		return nil
	}
	out := make([]api.Job, 0, len(p.SelectedJ))
	for _, j := range p.jobs {
		if p.SelectedJ[j.ID] {
			out = append(out, j)
		}
	}
	return out
}

// InJobs reports whether the job matrix (rather than the pipeline matrix)
// is currently showing.
func (p *PipelineList) InJobs() bool { return p.mode == modeJobs }

// Count returns how many pipelines are currently in the matrix (unfiltered).
func (p *PipelineList) Count() int { return len(p.pipelines) }

// HasTextFocus reports whether the pipeline-matrix filter input owns
// keystrokes, so the root model knows not to steal <Space> for the leader
// menu.
func (p *PipelineList) HasTextFocus() bool { return p.mode == modePipelines && p.filtering }

func (p PipelineList) updateFilter(msg tea.Msg) (PipelineList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			p.filterInput.SetValue("")
			p.filterInput.Blur()
			p.filtering = false
			p.syncPipeRows()
			return p, nil
		case "enter":
			p.filterInput.Blur()
			p.filtering = false
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.syncPipeRows()
	return p, cmd
}

func (p PipelineList) Update(msg tea.Msg) (PipelineList, tea.Cmd) {
	if p.mode == modePipelines && p.filtering {
		return p.updateFilter(msg)
	}

	km, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch p.mode {
		case modePipelines:
			switch km.String() {
			case "/":
				p.filtering = true
				p.filterInput.Focus()
				return p, nil
			case "x":
				if pl, ok := p.HighlightedPipeline(); ok {
					toggleSet(p.Selected, pl.ID)
					p.syncPipeRows()
				}
				return p, nil
			case "enter":
				targets := p.SelectedPipelines()
				if len(targets) == 0 {
					return p, nil
				}
				return p, func() tea.Msg { return OpenJobsMsg{Pipelines: targets} }
			case "R", "K":
				targets := p.SelectedPipelines()
				retry := km.String() == "R"
				return p, func() tea.Msg { return BulkPipelineActionMsg{Targets: targets, Retry: retry} }
			case "s":
				p.CycleSort()
				return p, nil
			case "S":
				p.ToggleSortDirection()
				return p, nil
			}
		case modeJobs:
			switch km.String() {
			case "esc":
				p.mode = modePipelines
				return p, nil
			case "x":
				if j, ok := p.HighlightedJob(); ok {
					toggleSet(p.SelectedJ, j.ID)
					p.syncJobRows()
				}
				return p, nil
			case "enter":
				if j, ok := p.HighlightedJob(); ok {
					return p, func() tea.Msg { return OpenLogsMsg{ProjectID: j.ProjectID, JobID: j.ID} }
				}
				return p, nil
			case "R", "K":
				targets := p.SelectedJobs()
				retry := km.String() == "R"
				return p, func() tea.Msg { return BulkJobActionMsg{Targets: targets, Retry: retry} }
			}
		}
	}

	var cmd tea.Cmd
	if p.mode == modePipelines {
		p.pipeTable, cmd = p.pipeTable.Update(msg)
	} else {
		p.jobTable, cmd = p.jobTable.Update(msg)
	}
	return p, cmd
}

func (p PipelineList) View() string {
	if p.mode == modeJobs {
		var header string
		switch len(p.Pipelines) {
		case 1:
			pl := p.Pipelines[0]
			header = fmt.Sprintf("Jobs for pipeline #%d (%s) — %d staged\n", pl.IID, pl.Ref, len(p.SelectedJ))
		default:
			header = fmt.Sprintf("Jobs for %d pipelines — %d staged\n", len(p.Pipelines), len(p.SelectedJ))
		}
		return lipgloss.NewStyle().Render(header + p.jobTable.View())
	}
	dir := "↓"
	if !p.sortDesc {
		dir = "↑"
	}

	count := fmt.Sprintf("%d pipelines", len(p.filtered))
	if p.filterInput.Value() != "" {
		count = fmt.Sprintf("%d/%d pipelines", len(p.filtered), len(p.pipelines))
	}
	header := fmt.Sprintf("%s (%d staged) — sorted by %s %s (s: cycle, S: reverse, /: filter)\n",
		count, len(p.Selected), p.sortField, dir)
	if p.filtering {
		header = p.filterInput.View() + "\n" + header
	}
	return lipgloss.NewStyle().Render(header + p.pipeTable.View())
}
