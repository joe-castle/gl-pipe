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

// OpenDownstreamPipelineMsg asks the root model to load a trigger job's
// downstream pipeline into the pipeline matrix — Enter on a bridge job
// (which has no log to stream) instead of OpenLogsMsg. PipelineID is 0 if
// the downstream pipeline hasn't been created yet.
type OpenDownstreamPipelineMsg struct {
	ProjectID  int
	PipelineID int
}

// BulkPipelineActionMsg requests a retry or cancel across the selected (or
// highlighted) pipelines. Retry is true for retry, false for cancel.
type BulkPipelineActionMsg struct {
	Targets []api.Pipeline
	Retry   bool
}

// BulkJobActionMsg is the job-matrix equivalent of BulkPipelineActionMsg.
// Retry and Play are mutually exclusive; both false means cancel.
type BulkJobActionMsg struct {
	Targets []api.Job
	Retry   bool
	Play    bool
}

// RefreshRequestMsg asks the root model to immediately re-fetch whatever's
// currently shown (pipeline or job matrix), independent of the periodic
// poll — the manual 'r' key inside either matrix.
type RefreshRequestMsg struct{}

// FailureDigestRequestMsg asks the root model to fetch the trace of every
// failed job currently in the matrix and extract each one's first error
// line — the 'E' key in job mode. Deliberately on demand rather than
// automatic: the 10s auto-poll would otherwise fire a trace request per
// failed job on every tick.
type FailureDigestRequestMsg struct{}

// JobDigest is one failed job's extracted failure summary. The three states
// are distinct and the panel renders each differently: no map entry at all
// means the digest hasn't been run for this job; an entry with empty Lines
// and empty Err means the trace was read but nothing matched; Err means the
// fetch itself failed.
type JobDigest struct {
	Lines []string
	Err   string
}

// digestPanelHeight is how many rows the job table gives up to the digest
// panel while one exists (a header line, up to 3 digest lines, and the
// blank line separating it from the table).
const digestPanelHeight = 5

// minJobTableHeight is the floor the digest panel may not shrink the job
// matrix past, so a short terminal degrades instead of showing no rows.
const minJobTableHeight = 3

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

	jobs        []api.Job
	jobFiltered []api.Job // jobs after the text filter; what's actually shown
	jobTable    table.Model
	Pipelines   []api.Pipeline    // the pipeline(s) whose jobs are currently shown
	SelectedJ   map[int]bool      // job ID -> staged for bulk action
	jobDigest   map[int]JobDigest // job ID -> failure summary, once 'E' has run

	width, height int
}

func NewPipelineList() PipelineList {
	pipeCols := []table.Column{
		{Title: "", Width: 3},
		{Title: "PROJECT", Width: 24},
		{Title: "STATUS", Width: 12},
		{Title: "REF", Width: 16},
		{Title: "SHA", Width: 10},
		{Title: "AUTHOR", Width: 12},
		{Title: "DURATION", Width: 10},
		{Title: "DATE", Width: 12},
	}
	pipeTable := table.New(table.WithColumns(pipeCols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))

	jobCols := []table.Column{
		{Title: "", Width: 3},
		{Title: "PROJECT", Width: 18},
		{Title: "PIPELINE", Width: 9},
		{Title: "STAGE", Width: 12},
		{Title: "JOB", Width: 20},
		{Title: "STATUS", Width: 12},
		{Title: "RUNNER", Width: 12},
		{Title: "RETRIES", Width: 7},
		{Title: "DURATION", Width: 9},
	}
	jobTable := table.New(table.WithColumns(jobCols), table.WithFocused(true), table.WithHeight(15), table.WithStyles(TableStyles()))

	filter := textinput.New()
	filter.Placeholder = "filter..." // pipelines: ref/status/project/author/SHA — jobs: name/stage/status/project/runner
	filter.Prompt = "/ "

	return PipelineList{
		pipeTable:    pipeTable,
		jobTable:     jobTable,
		projectNames: map[int]string{},
		Selected:     map[int]bool{},
		SelectedJ:    map[int]bool{},
		jobDigest:    map[int]JobDigest{},
		sortField:    sortByDate,
		sortDesc:     true,
		filterInput:  filter,
	}
}

func (p *PipelineList) SetSize(w, h int) {
	p.width, p.height = w, h

	const checkW, statusW, refW, shaW, authorW, durationW, dateW = 3, 12, 16, 10, 12, 10, 12
	p.pipeTable.SetColumns([]table.Column{
		{Title: "", Width: checkW},
		{Title: "PROJECT", Width: flexColumnWidth(w, []int{checkW, statusW, refW, shaW, authorW, durationW, dateW}, 18)},
		{Title: "STATUS", Width: statusW},
		{Title: "REF", Width: refW},
		{Title: "SHA", Width: shaW},
		{Title: "AUTHOR", Width: authorW},
		{Title: "DURATION", Width: durationW},
		{Title: "DATE", Width: dateW},
	})
	p.pipeTable.SetWidth(w)
	p.pipeTable.SetHeight(h - 3)

	const pipelineW, stageW, jobStatusW, runnerW, retriesW, jobDurationW = 9, 12, 12, 12, 7, 9
	projectW, jobW := splitFlexWidth(w, []int{checkW, pipelineW, stageW, jobStatusW, runnerW, retriesW, jobDurationW}, 0.4, 14, 16)
	p.jobTable.SetColumns([]table.Column{
		{Title: "", Width: checkW},
		{Title: "PROJECT", Width: projectW},
		{Title: "PIPELINE", Width: pipelineW},
		{Title: "STAGE", Width: stageW},
		{Title: "JOB", Width: jobW},
		{Title: "STATUS", Width: jobStatusW},
		{Title: "RUNNER", Width: runnerW},
		{Title: "RETRIES", Width: retriesW},
		{Title: "DURATION", Width: jobDurationW},
	})
	p.jobTable.SetWidth(w)
	p.applyJobTableHeight()
}

// applyJobTableHeight sizes the job table, giving up rows to the failure
// digest panel only while there's actually a digest to show — the matrix
// must not be permanently shorter for a feature you aren't using. Called
// from SetSize and from every place the digest map changes.
func (p *PipelineList) applyJobTableHeight() {
	h := p.height - 3
	if len(p.jobDigest) > 0 {
		h -= digestPanelHeight
	}
	// On a short terminal the panel would otherwise consume the whole table
	// (bubbles/table happily accepts a height that leaves no rows at all).
	// Degrade to a cramped matrix rather than an empty one.
	if h < minJobTableHeight {
		h = minJobTableHeight
	}
	p.jobTable.SetHeight(h)
}

// SetProjectNames supplies a project ID -> path lookup for the Project column.
func (p *PipelineList) SetProjectNames(names map[int]string) { p.projectNames = names }

// SetPipelines replaces the pipeline matrix contents. The text filter and
// staging are both reset — a fresh batch (e.g. re-pressing Enter for a
// different staged set, or jumping to a downstream pipeline from a bridge
// job) shouldn't leave stale filter text quietly hiding the new rows, nor
// stale Selected IDs that don't overlap with the new batch: since
// SelectedPipelines() only falls back to the highlighted row when
// Selected is empty, a leftover staged ID from an unrelated earlier batch
// silently made every batch action (Enter included) act on nothing.
func (p *PipelineList) SetPipelines(pipelines []api.Pipeline) {
	p.pipelines = pipelines
	p.mode = modePipelines
	p.filtering = false
	p.filterInput.SetValue("")
	p.filterInput.Blur()
	p.Selected = map[int]bool{}
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
			StatusIcon(pl.Status),
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

// AllPipelines returns every pipeline currently in the pipeline-mode
// matrix, unfiltered — what a poll or manual refresh re-fetches.
func (p *PipelineList) AllPipelines() []api.Pipeline { return p.pipelines }

// NeedsPoll reports whether the currently active view (pipeline or job
// matrix) has anything non-terminal left, i.e. whether periodic polling is
// still worth doing. Once everything shown has settled (success, failed,
// canceled, skipped, or manual), polling stops making API calls on its
// own — a manual 'r' still works regardless.
func (p *PipelineList) NeedsPoll() bool {
	if p.mode == modeJobs {
		for _, j := range p.jobs {
			if !j.Status.Terminal() {
				return true
			}
		}
		return false
	}
	for _, pl := range p.pipelines {
		if !pl.Status.Terminal() {
			return true
		}
	}
	return false
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
// AddOrUpdate is used to build up the multi-project pipeline matrix. The
// text filter is reset too, for the same reason SetPipelines resets it: a
// fresh job batch shouldn't inherit stale filter text from a previous job
// view (or from browsing pipelines) quietly hiding the new rows.
func (p *PipelineList) ClearJobs() {
	p.jobs = nil
	p.Pipelines = nil
	p.mode = modeJobs
	p.SelectedJ = map[int]bool{}
	p.filtering = false
	p.filterInput.SetValue("")
	p.filterInput.Blur()
	// A fresh job view is a different set of jobs entirely, so last view's
	// failure summaries would be stale (and would keep the table shortened).
	// AddJobs deliberately does *not* do this — see SetJobDigest.
	p.jobDigest = map[int]JobDigest{}
	p.applyJobTableHeight()
	p.syncJobRows()
}

// FailedJobs returns every failed job currently loaded, ignoring the text
// filter: the digest acts on everything in the matrix, not just what's on
// screen, so scrolling to a row a filter had hidden still shows a summary.
//
// Bridges are excluded. A bridge is not a job — GitLab exposes no trace
// endpoint for one, so fetching it is a guaranteed 404 that surfaces to the
// user as "trace(s) failed to load" for something that never had a log in
// the first place. Enter on a bridge already opens the downstream pipeline
// for the same reason; the failure worth reading is in that pipeline's own
// jobs.
func (p *PipelineList) FailedJobs() []api.Job {
	out := make([]api.Job, 0, len(p.jobs))
	for _, j := range p.jobs {
		if j.Status == api.StatusFailed && !j.IsBridge {
			out = append(out, j)
		}
	}
	return out
}

// SetJobDigest records one job's failure summary. Digests deliberately
// outlive AddJobs — the 10s auto-poll re-issues AddJobs for every shown
// pipeline, and clearing there would make the panel flicker away seconds
// after it appeared. Only ClearJobs (a genuinely new job view) drops them.
func (p *PipelineList) SetJobDigest(jobID int, d JobDigest) {
	if p.jobDigest == nil {
		p.jobDigest = map[int]JobDigest{}
	}
	p.jobDigest[jobID] = d
	p.applyJobTableHeight()
}

// JobDigestFor returns a job's failure summary, if the digest has been run
// for it. ok distinguishes "not fetched" from "fetched, nothing matched".
func (p *PipelineList) JobDigestFor(jobID int) (JobDigest, bool) {
	d, ok := p.jobDigest[jobID]
	return d, ok
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

// jobMatches reports whether j matches query as a case-insensitive
// substring of its name, stage, status, project, or runner — mirrors
// pipelineMatches for the job matrix's own '/' filter.
func (p *PipelineList) jobMatches(j api.Job, query string) bool {
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		j.Name, j.Stage, string(j.Status), p.projectNames[j.ProjectID], j.RunnerTag,
	}, " "))
	return strings.Contains(haystack, query)
}

func (p *PipelineList) filterJobsList() {
	query := strings.ToLower(p.filterInput.Value())
	if query == "" {
		p.jobFiltered = append([]api.Job(nil), p.jobs...)
		return
	}
	filtered := make([]api.Job, 0, len(p.jobs))
	for _, j := range p.jobs {
		if p.jobMatches(j, query) {
			filtered = append(filtered, j)
		}
	}
	p.jobFiltered = filtered
}

func (p *PipelineList) syncJobRows() {
	p.filterJobsList()

	byID := make(map[int]api.Pipeline, len(p.Pipelines))
	for _, pl := range p.Pipelines {
		byID[pl.ID] = pl
	}

	rows := make([]table.Row, len(p.jobFiltered))
	for i, j := range p.jobFiltered {
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
			StatusIcon(j.Status),
			jobRunnerCell(j),
			retries,
			formatDuration(j.Duration),
		}
	}
	setRows(&p.jobTable, rows)
}

// jobRunnerCell renders the RUNNER column: the runner tag for a regular
// job, or — for a pipeline trigger job (`trigger:`, e.g. a deploy step
// that kicks off a downstream deployment pipeline) — where it points, so
// the downstream pipeline is visible at a glance instead of not showing
// up at all. GitLab reports trigger jobs with no downstream_pipeline yet
// (still pending) distinctly from one that's already running.
func jobRunnerCell(j api.Job) string {
	if !j.IsBridge {
		return j.RunnerTag
	}
	if j.DownstreamPipelineID == 0 {
		// Manual bridge not yet played shows its own status; a played bridge
		// whose downstream pipeline hasn't been created yet shows as pending.
		if j.Status == api.StatusManual {
			return "→ (manual — press p)"
		}
		return "→ (pending)"
	}
	return fmt.Sprintf("→ #%d %s", j.DownstreamPipelineIID, StatusIcon(j.DownstreamStatus))
}

// HighlightedJob returns the job under the cursor in job-matrix mode.
func (p *PipelineList) HighlightedJob() (api.Job, bool) {
	i := p.jobTable.Cursor()
	if i < 0 || i >= len(p.jobFiltered) {
		return api.Job{}, false
	}
	return p.jobFiltered[i], true
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

// ReturnToJobs switches back to the job matrix without re-fetching —
// jobs/Pipelines are left exactly as they were, only the mode changed
// (see SetPipelines, and Model.downstreamOrigin's doc comment for why
// this is safe to call: it's only reached along a path that started from
// a job matrix in the first place, so there's always something valid to
// return to).
func (p *PipelineList) ReturnToJobs() { p.mode = modeJobs }

// Count returns how many pipelines are currently in the matrix (unfiltered).
func (p *PipelineList) Count() int { return len(p.pipelines) }

// HasTextFocus reports whether the filter input (pipeline or job matrix —
// both share it, only one mode is ever active at a time) owns keystrokes,
// so the root model knows not to steal <Space> for the leader menu.
func (p *PipelineList) HasTextFocus() bool { return p.filtering }

// syncRowsForMode re-runs the filter+row build for whichever matrix is
// currently showing — used by updateFilter, which is shared between both.
func (p *PipelineList) syncRowsForMode() {
	if p.mode == modeJobs {
		p.syncJobRows()
	} else {
		p.syncPipeRows()
	}
}

func (p PipelineList) updateFilter(msg tea.Msg) (PipelineList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			p.filterInput.SetValue("")
			p.filterInput.Blur()
			p.filtering = false
			p.syncRowsForMode()
			return p, nil
		case "enter":
			p.filterInput.Blur()
			p.filtering = false
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.filterInput, cmd = p.filterInput.Update(msg)
	p.syncRowsForMode()
	return p, cmd
}

func (p PipelineList) Update(msg tea.Msg) (PipelineList, tea.Cmd) {
	if p.filtering {
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
			case "a":
				ids := make([]int, len(p.filtered))
				for i, pl := range p.filtered {
					ids[i] = pl.ID
				}
				toggleSetAll(p.Selected, ids)
				p.syncPipeRows()
				return p, nil
			case "r":
				return p, func() tea.Msg { return RefreshRequestMsg{} }
			}
		case modeJobs:
			switch km.String() {
			case "/":
				p.filtering = true
				p.filterInput.Focus()
				return p, nil
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
					if j.IsBridge {
						return p, func() tea.Msg {
							return OpenDownstreamPipelineMsg{ProjectID: j.DownstreamProjectID, PipelineID: j.DownstreamPipelineID}
						}
					}
					return p, func() tea.Msg { return OpenLogsMsg{ProjectID: j.ProjectID, JobID: j.ID} }
				}
				return p, nil
			case "R", "K":
				targets := p.SelectedJobs()
				retry := km.String() == "R"
				return p, func() tea.Msg { return BulkJobActionMsg{Targets: targets, Retry: retry} }
			case "p":
				targets := p.SelectedJobs()
				return p, func() tea.Msg { return BulkJobActionMsg{Targets: targets, Play: true} }
			case "E":
				return p, func() tea.Msg { return FailureDigestRequestMsg{} }
			case "a":
				ids := make([]int, len(p.jobFiltered))
				for i, j := range p.jobFiltered {
					ids[i] = j.ID
				}
				toggleSetAll(p.SelectedJ, ids)
				p.syncJobRows()
				return p, nil
			case "r":
				return p, func() tea.Msg { return RefreshRequestMsg{} }
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

// digestPanel renders the highlighted job's failure summary under the job
// table — the same "expand the highlighted row underneath the list" idiom
// as presetDetail. It lives outside the table on purpose: a bubbles/table
// cell can carry no ANSI (TableStyles' doc comment) and has nowhere near
// the width for a real error line.
func (p PipelineList) digestPanel() string {
	if len(p.jobDigest) == 0 {
		return ""
	}
	j, ok := p.HighlightedJob()
	if !ok {
		return ""
	}
	d, fetched := p.jobDigest[j.ID]
	if !fetched {
		// Silence here is how this feature looks broken: the status line
		// says the digest ran, and then every row that wasn't part of it
		// shows nothing at all. Say so instead.
		return "\n" + helpDescStyle.Render("▸ no summary for this job — E summarises failed jobs")
	}

	var b strings.Builder
	b.WriteString("▸ " + j.Stage + " · " + j.Name + "\n")
	switch {
	case d.Err != "":
		b.WriteString("  couldn't read the trace: " + d.Err)
	case len(d.Lines) == 0:
		b.WriteString("  no error line found — enter to read the whole log")
	default:
		b.WriteString("  " + strings.Join(d.Lines, "\n  "))
	}
	return "\n" + helpDescStyle.Render(b.String())
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
		if p.filterInput.Value() != "" {
			header = fmt.Sprintf("%d/%d jobs shown — %d staged\n", len(p.jobFiltered), len(p.jobs), len(p.SelectedJ))
		}
		if p.filtering {
			header = p.filterInput.View() + "\n" + header
		}
		help := "\n" + RenderHelp(
			[2]string{"x", "stage"},
			[2]string{"a", "stage/unstage all"},
			[2]string{"enter", "view logs / downstream pipeline"},
			[2]string{"p", "play manual job(s)"},
			[2]string{"E", "why did it fail?"},
			[2]string{"R", "bulk retry"},
			[2]string{"K", "bulk cancel"},
			[2]string{"/", "filter"},
			[2]string{"r", "refresh now"},
			[2]string{"esc", "back"},
		)
		return lipgloss.NewStyle().Render(header + p.jobTable.View() + p.digestPanel() + help)
	}

	dir := "↓"
	if !p.sortDesc {
		dir = "↑"
	}
	count := fmt.Sprintf("%d pipelines", len(p.filtered))
	if p.filterInput.Value() != "" {
		count = fmt.Sprintf("%d/%d pipelines", len(p.filtered), len(p.pipelines))
	}
	header := fmt.Sprintf("%s (%d staged) — sorted by %s %s\n", count, len(p.Selected), p.sortField, dir)
	if p.filtering {
		header = p.filterInput.View() + "\n" + header
	}
	help := "\n" + RenderHelp(
		[2]string{"x", "stage"},
		[2]string{"a", "stage/unstage all"},
		[2]string{"enter", "view jobs"},
		[2]string{"R", "bulk retry"},
		[2]string{"K", "bulk cancel"},
		[2]string{"s/S", "sort/reverse"},
		[2]string{"/", "filter"},
		[2]string{"r", "refresh now"},
	)
	return lipgloss.NewStyle().Render(header + p.pipeTable.View() + help)
}
