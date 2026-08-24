package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// OpenJobsMsg asks the root model to load the job matrix for one pipeline.
type OpenJobsMsg struct {
	ProjectID  int
	PipelineID int
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

// PipelineList renders the multi-project pipeline matrix and, drilled into
// one pipeline, its job matrix.
type PipelineList struct {
	mode pipelineListMode

	pipelines    []api.Pipeline
	projectNames map[int]string
	pipeTable    table.Model
	Selected     map[int]bool // pipeline ID -> staged for bulk action

	jobs      []api.Job
	jobTable  table.Model
	Pipeline  api.Pipeline
	SelectedJ map[int]bool // job ID -> staged for bulk action

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
	}
	pipeTable := table.New(table.WithColumns(pipeCols), table.WithFocused(true), table.WithHeight(15))

	jobCols := []table.Column{
		{Title: "", Width: 3},
		{Title: "Stage", Width: 14},
		{Title: "Job", Width: 24},
		{Title: "Status", Width: 12},
		{Title: "Runner", Width: 14},
		{Title: "Retries", Width: 8},
		{Title: "Duration", Width: 10},
	}
	jobTable := table.New(table.WithColumns(jobCols), table.WithFocused(true), table.WithHeight(15))

	return PipelineList{
		pipeTable:    pipeTable,
		jobTable:     jobTable,
		projectNames: map[int]string{},
		Selected:     map[int]bool{},
		SelectedJ:    map[int]bool{},
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

// SetPipelines replaces the pipeline matrix contents.
func (p *PipelineList) SetPipelines(pipelines []api.Pipeline) {
	p.pipelines = pipelines
	p.mode = modePipelines
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

func (p *PipelineList) syncPipeRows() {
	rows := make([]table.Row, len(p.pipelines))
	for i, pl := range p.pipelines {
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

// HighlightedPipeline returns the pipeline under the cursor, if any.
func (p *PipelineList) HighlightedPipeline() (api.Pipeline, bool) {
	i := p.pipeTable.Cursor()
	if i < 0 || i >= len(p.pipelines) {
		return api.Pipeline{}, false
	}
	return p.pipelines[i], true
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

// SetJobs enters job-matrix mode for the given pipeline.
func (p *PipelineList) SetJobs(pipeline api.Pipeline, jobs []api.Job) {
	p.Pipeline = pipeline
	p.jobs = jobs
	p.mode = modeJobs
	p.SelectedJ = map[int]bool{}
	p.syncJobRows()
}

func (p *PipelineList) syncJobRows() {
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
		rows[i] = table.Row{check, j.Stage, j.Name, string(j.Status), j.RunnerTag, retries, formatDuration(j.Duration)}
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

// Count returns how many pipelines are currently in the matrix.
func (p *PipelineList) Count() int { return len(p.pipelines) }

func (p PipelineList) Update(msg tea.Msg) (PipelineList, tea.Cmd) {
	km, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch p.mode {
		case modePipelines:
			switch km.String() {
			case "x":
				if pl, ok := p.HighlightedPipeline(); ok {
					p.Selected[pl.ID] = !p.Selected[pl.ID]
					p.syncPipeRows()
				}
				return p, nil
			case "enter":
				if pl, ok := p.HighlightedPipeline(); ok {
					return p, func() tea.Msg {
						return OpenJobsMsg{ProjectID: pl.ProjectID, PipelineID: pl.ID}
					}
				}
				return p, nil
			case "R", "K":
				targets := p.SelectedPipelines()
				retry := km.String() == "R"
				return p, func() tea.Msg { return BulkPipelineActionMsg{Targets: targets, Retry: retry} }
			}
		case modeJobs:
			switch km.String() {
			case "esc":
				p.mode = modePipelines
				return p, nil
			case "x":
				if j, ok := p.HighlightedJob(); ok {
					p.SelectedJ[j.ID] = !p.SelectedJ[j.ID]
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
		header := fmt.Sprintf("Jobs for pipeline #%d (%s) — %d staged\n", p.Pipeline.IID, p.Pipeline.Ref, len(p.SelectedJ))
		return lipgloss.NewStyle().Render(header + p.jobTable.View())
	}
	header := fmt.Sprintf("%d pipelines (%d staged)\n", len(p.pipelines), len(p.Selected))
	return lipgloss.NewStyle().Render(header + p.pipeTable.View())
}
