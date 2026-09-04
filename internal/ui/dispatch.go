package ui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui/components"
)

// handleKey implements the key dispatch precedence from the project plan:
// active modal → leader menu → focused pane → global. ctrl+c is handled by
// the caller before this is reached.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.view == viewWizard {
		var cmd tea.Cmd
		m.wizard, cmd = m.wizard.Update(msg)
		return m, cmd
	}

	switch {
	case m.variables.Active:
		var cmd tea.Cmd
		m.variables, cmd = m.variables.Update(msg)
		return m, cmd
	case m.settings.Active:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return m, cmd
	case m.presets.Active:
		var cmd tea.Cmd
		m.presets, cmd = m.presets.Update(msg)
		return m, cmd
	case m.groupPicker.Active:
		var cmd tea.Cmd
		m.groupPicker, cmd = m.groupPicker.Update(msg)
		return m, cmd
	case m.refSearch.Active:
		var cmd tea.Cmd
		m.refSearch, cmd = m.refSearch.Update(msg)
		return m, cmd
	case m.mrList.Active:
		var cmd tea.Cmd
		m.mrList, cmd = m.mrList.Update(msg)
		return m, cmd
	case m.logViewer.Active:
		var cmd tea.Cmd
		m.logViewer, cmd = m.logViewer.Update(msg)
		if !m.logViewer.Active && m.logCancel != nil {
			m.logCancel()
			m.logCancel = nil
		}
		return m, cmd
	case m.leaderMenu.Active:
		var cmd tea.Cmd
		m.leaderMenu, cmd = m.leaderMenu.Update(msg)
		return m, cmd
	}

	textFocused := (m.pane == paneExplorer && m.projectList.HasTextFocus()) ||
		(m.pane == panePipelines && m.pipelineList.HasTextFocus())
	if msg.String() == " " && !textFocused {
		m.leaderMenu.Open()
		return m, nil
	}

	if m.pane == paneExplorer {
		switch msg.String() {
		case "tab", "shift+tab":
			m.pane = panePipelines
			return m, nil
		case "T":
			if !textFocused {
				return m.toggleLockRef(refLockSemVer)
			}
		case "t":
			if !textFocused {
				return m.toggleLockRef(refLockLatestCreated)
			}
		case "enter":
			if !textFocused {
				return m.openPipelinesForSelected()
			}
		case "M":
			if !textFocused {
				return m.openMRsForSelected()
			}
		case "ctrl+r":
			if !textFocused {
				return m.openExplorerRefPicker()
			}
		}
		var cmd tea.Cmd
		m.projectList, cmd = m.projectList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "tab", "shift+tab":
		if !textFocused {
			m.pane = paneExplorer
			return m, nil
		}
	case "esc":
		if !textFocused && !m.pipelineList.InJobs() {
			if m.downstreamOrigin {
				m.pipelineList.ReturnToJobs()
				m.downstreamOrigin = false
				return m, nil
			}
			m.pane = paneExplorer
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.pipelineList, cmd = m.pipelineList.Update(msg)
	return m, cmd
}

// handleLeaderAction dispatches one <Space>-menu mnemonic.
func (m Model) handleLeaderAction(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "p":
		projects := m.projectList.SelectedProjects()
		if len(projects) == 0 {
			m.setStatus("no project selected — highlight or x-select one first")
			return m, nil
		}
		ref := projects[0].DefaultBranch
		if r, ok := m.projectList.LockedRef[projects[0].ID]; ok && r != "" {
			ref = r
		}
		if ref == "" {
			ref = "main"
		}
		var preset []api.Variable
		if p, ok := m.cfg.Presets[m.chosenPreset]; ok && m.chosenPreset != "" {
			preset = presetVars(p)
			if p.Ref != "" {
				ref = p.Ref
			}
		}
		m.variables.Open(projects, ref, preset)
		return m, nil

	case "f":
		if m.pane == paneExplorer {
			m.projectList.OpenBlobSearch()
		}
		return m, nil

	case "b":
		m.refSearch.Open()
		return m, nil

	case "g":
		id := m.newReqID()
		m.genGroups = id
		m.loading = true
		return m, loadGroupsCmd(m.ctx, m.client, id)

	case "m":
		id := m.newReqID()
		m.genMRs = id
		m.loading = true
		m.mrList.SetProjectNames(m.projectNames)
		return m, loadMyMRsCmd(m.ctx, m.client, id)

	case "v":
		m.presets.Open(m.presetEntries())
		return m, nil

	case "s":
		m.settings.Open(m.cfg)
		return m, nil

	case "o":
		url := m.focusedWebURL()
		if url == "" {
			m.setStatus("nothing to open here")
			return m, nil
		}
		if err := openBrowser(url); err != nil {
			m.setErr(err)
		}
		return m, nil

	case "r":
		return m, m.syncProjectsCmd()

	case "q":
		m.teardown()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) focusedWebURL() string {
	if m.logViewer.Active {
		return m.logJobURL
	}
	if m.pane == panePipelines {
		if m.pipelineList.InJobs() {
			if j, ok := m.pipelineList.HighlightedJob(); ok {
				return j.WebURL
			}
			return ""
		}
		if pl, ok := m.pipelineList.HighlightedPipeline(); ok {
			return pl.WebURL
		}
		return ""
	}
	if proj, ok := m.projectList.Highlighted(); ok {
		return proj.WebURL
	}
	return ""
}

// startPipelinesBatch is the shared plumbing behind every way of loading
// pipelines into the matrix — staged projects, a ref search across every
// known project, or an MR selection's pipelines: it clears the matrix,
// starts a new generation, tracks how many responses are outstanding (so
// the status line can report one clean summary on completion instead of
// flickering per-response — most projects miss a ref search, and showing
// each miss as its own status would bury the result), and fires n Cmds
// built by the caller (the reqID is only known once newReqID() runs here,
// so callers get it via the build callback rather than baking it in
// themselves).
func (m Model) startPipelinesBatch(n int, build func(id reqID) []tea.Cmd) (Model, tea.Cmd) {
	if n == 0 {
		return m, nil
	}
	id := m.newReqID()
	m.genPipelines = id
	m.loading = true
	m.pipelinesPending = n
	m.pipelinesErrored = 0
	m.pipelinesTotal = n
	// Any fresh, unrelated batch invalidates a pending "Esc returns to the
	// job matrix" — openDownstreamPipeline sets it back to true right
	// after this returns, for the one caller it actually applies to.
	m.downstreamOrigin = false
	m.pipelineList.SetPipelines(nil)
	return m, tea.Batch(build(id)...)
}

// openPipelinesForSelected shows pipelines for every staged project
// together in one matrix (falling back to just the highlighted project if
// none are staged) — the same "staged, or highlighted" convention used by
// the trigger modal.
func (m Model) openPipelinesForSelected() (tea.Model, tea.Cmd) {
	projects := m.projectList.SelectedProjects()
	cutoff := m.pipelineCreatedAfterCutoff()
	return m.startPipelinesBatch(len(projects), func(id reqID) []tea.Cmd {
		cmds := make([]tea.Cmd, len(projects))
		for i, proj := range projects {
			cmds[i] = pipelinesForProjectCmd(m.ctx, m.client, proj.ID, cutoff, id)
		}
		return cmds
	})
}

// searchPipelinesByRef searches every project currently in the cache for
// pipelines on an exact ref, with no staging required — for finding
// pipelines when you know the branch but not which repo it lives in (e.g.
// several MRs sharing a source branch).
func (m Model) searchPipelinesByRef(ref string) (tea.Model, tea.Cmd) {
	if m.cacheIdx == nil || len(m.cacheIdx.Projects) == 0 {
		m.setStatus("no synced projects to search — sync first (<space> r or <space> g)")
		return m, nil
	}
	projects := m.cacheIdx.Projects
	cutoff := m.pipelineCreatedAfterCutoff()
	m.setStatus(fmt.Sprintf("searching %d project(s) for ref %q...", len(projects), ref))
	return m.startPipelinesBatch(len(projects), func(id reqID) []tea.Cmd {
		cmds := make([]tea.Cmd, len(projects))
		for i, proj := range projects {
			cmds[i] = pipelinesByRefCmd(m.ctx, m.client, proj.ID, ref, cutoff, id)
		}
		return cmds
	})
}

// pipelineCreatedAfterCutoff returns the GitLab-side created_after cutoff
// derived from the configured pipeline age cap (config.yaml's
// pipelines.max_age_days), or the zero time.Time (no filter) if unset —
// the pre-existing default.
func (m Model) pipelineCreatedAfterCutoff() time.Time {
	maxAge := m.cfg.PipelineMaxAge()
	if maxAge <= 0 {
		return time.Time{}
	}
	return time.Now().Add(-maxAge)
}

// openPipelinesForMRs jumps straight from an MR selection to its
// pipelines, reusing the exact same matrix (and therefore sort/filter/
// bulk-retry) as every other way of getting pipelines into it.
func (m Model) openPipelinesForMRs(mrs []api.MergeRequest) (tea.Model, tea.Cmd) {
	return m.startPipelinesBatch(len(mrs), func(id reqID) []tea.Cmd {
		cmds := make([]tea.Cmd, len(mrs))
		for i, mr := range mrs {
			cmds[i] = mrPipelinesCmd(m.ctx, m.client, mr.ProjectID, mr.IID, id)
		}
		return cmds
	})
}

// openMRsForSelected fetches MRs for the staged (or highlighted-fallback)
// project(s) — M on the explorer — clearing and batching the same way
// openPipelinesForSelected does for pipelines.
func (m Model) openMRsForSelected() (tea.Model, tea.Cmd) {
	projects := m.projectList.SelectedProjects()
	if len(projects) == 0 {
		return m, nil
	}
	id := m.newReqID()
	m.genMRs = id
	m.loading = true
	m.mrsPending = len(projects)
	m.mrsErrored = 0
	m.mrsTotal = len(projects)
	m.mrList.SetProjectNames(m.projectNames)
	m.mrList.SetMRs(nil)

	cmds := make([]tea.Cmd, 0, len(projects))
	for _, proj := range projects {
		cmds = append(cmds, loadProjectMRsCmd(m.ctx, m.client, proj.ID, id))
	}
	return m, tea.Batch(cmds...)
}

// toggleLockRef is T/t on the explorer: lock the staged (or highlighted-
// fallback) project(s) to their latest tag under the given strategy, same
// "staged, or highlighted" convention as every other batch action.
//
// If *any* targeted project is currently locked, this unlocks every
// currently-locked project in the set instead of fetching — deliberately
// "any", not "all": with a large batch it's common for one or two projects
// to have no qualifying tag and so never end up locked, and requiring 100%
// coverage before the toggle could ever flip back to "unlock" meant a
// batch with even one such straggler could never be unlocked via T again.
func (m Model) toggleLockRef(strategy refLockStrategy) (tea.Model, tea.Cmd) {
	projects := m.projectList.SelectedProjects()
	if len(projects) == 0 {
		return m, nil
	}

	anyLocked := false
	for _, proj := range projects {
		if _, ok := m.projectList.LockedRef[proj.ID]; ok {
			anyLocked = true
			break
		}
	}
	if anyLocked {
		unlocked := 0
		for _, proj := range projects {
			if _, ok := m.projectList.LockedRef[proj.ID]; ok {
				m.projectList.ClearLockedRef(proj.ID)
				unlocked++
			}
		}
		m.setStatus(fmt.Sprintf("unlocked %d project(s)", unlocked))
		return m, nil
	}

	id := m.newReqID()
	m.genRefs = id
	m.refLockMode = strategy
	m.loading = true
	m.refsPending = len(projects)
	m.refsErrored = 0
	m.refsLocked = 0
	m.refsSkipped = 0
	m.refsTotal = len(projects)

	cmds := make([]tea.Cmd, 0, len(projects))
	for _, proj := range projects {
		cmds = append(cmds, loadRefsCmd(m.ctx, m.client, proj.ID, id))
	}
	return m, tea.Batch(cmds...)
}

// openExplorerRefPicker browses branches+tags for the highlighted project
// and lets the user pick one to lock just that project to. Deliberately
// always the highlighted project, never "staged, or highlighted" like
// T/t/M/Enter: the point is overriding one repo's ref without disturbing
// whatever's staged — e.g. stage a batch and lock it to a SemVer tag via
// T, then move the cursor to individual projects that need something else
// and ctrl+r each one in turn, leaving the rest of the staged batch alone.
func (m Model) openExplorerRefPicker() (tea.Model, tea.Cmd) {
	proj, ok := m.projectList.Highlighted()
	if !ok {
		m.setStatus("no project highlighted to browse refs for")
		return m, nil
	}
	m.projectList.PrepareRefOverride([]int{proj.ID})

	id := m.newReqID()
	m.genRefPicker = id
	m.refPickerFor = refPickerForExplorer
	return m, loadAllRefsCmd(m.ctx, m.client, proj.ID, id)
}

// openJobsForPipelines shows job matrices for one or more pipelines
// together, same "clear once, merge as each completes" pattern as
// openPipelinesForSelected.
func (m Model) openJobsForPipelines(pipelines []api.Pipeline) (tea.Model, tea.Cmd) {
	if len(pipelines) == 0 {
		return m, nil
	}
	id := m.newReqID()
	m.genJobs = id
	m.loading = true
	m.jobsPending = len(pipelines)
	m.jobsErrored = 0
	m.jobsTotal = len(pipelines)
	m.pipelineList.ClearJobs()

	cmds := make([]tea.Cmd, 0, len(pipelines))
	for _, pl := range pipelines {
		cmds = append(cmds, jobsForPipelineCmd(m.ctx, m.client, pl.ProjectID, pl.ID, id))
	}
	return m, tea.Batch(cmds...)
}

// startDigestBatch fetches the trace of every failed job currently in the
// job matrix and extracts each one's first error line, so a screen full of
// failures can be triaged from the matrix instead of one log viewer visit
// at a time. Deliberately on demand ('E'), never automatic: the 10s poll
// would otherwise fire a trace request per failed job on every tick.
func (m Model) startDigestBatch() (tea.Model, tea.Cmd) {
	// loadingProgress relies on at most one batch kind being in flight at a
	// time (the UI is modal). This is the first batch a user could fire
	// while another is still running, so it refuses rather than break that.
	if m.loading {
		m.setStatus("still loading — try the digest again once that finishes")
		return m, nil
	}
	failed := m.pipelineList.FailedJobs()
	if len(failed) == 0 {
		m.setStatus("no failed jobs in view to summarise")
		return m, nil
	}

	id := m.newReqID()
	m.genDigest = id
	m.loading = true
	m.digestPending = len(failed)
	m.digestErrored = 0
	m.digestTotal = len(failed)
	m.digestErrReason = ""

	cmds := make([]tea.Cmd, 0, len(failed))
	for _, j := range failed {
		cmds = append(cmds, jobDigestCmd(m.ctx, m.client, j.ProjectID, j.ID, id))
	}
	return m, tea.Batch(cmds...)
}

// digestSummary reports one line for a completed digest batch, in the same
// shape as every other batch's completion summary.
func (m Model) digestSummary() string {
	found := 0
	for _, j := range m.pipelineList.FailedJobs() {
		if d, ok := m.pipelineList.JobDigestFor(j.ID); ok && len(d.Lines) > 0 {
			found++
		}
	}
	summary := fmt.Sprintf("%d failed job(s) summarised, %d with an error line", m.digestTotal, found)
	if m.digestErrored > 0 {
		summary += fmt.Sprintf(", %d trace(s) failed to load", m.digestErrored)
		if m.digestErrReason != "" {
			summary += ": " + m.digestErrReason
		}
	}
	return summary
}

// openDownstreamPipeline jumps from a trigger job to the pipeline it kicked
// off, replacing the job matrix with just that one pipeline — the same
// matrix everything else feeds, so sort/filter/Enter-into-jobs/bulk-retry
// all work on it exactly like any other pipeline. Sets downstreamOrigin so
// Esc from here returns to the job matrix the jump came from instead of
// leaving the pipelines pane entirely (see the field's doc comment).
func (m Model) openDownstreamPipeline(projectID, pipelineID int) (tea.Model, tea.Cmd) {
	mm, cmd := m.startPipelinesBatch(1, func(id reqID) []tea.Cmd {
		return []tea.Cmd{pipelineByIDCmd(m.ctx, m.client, projectID, pipelineID, id)}
	})
	mm.downstreamOrigin = true
	return mm, cmd
}

// projectPathFromPipelineURL recovers a project's path_with_namespace from
// a pipeline's WebURL ("<base>/<namespace>/<project>/-/pipelines/<id>")
// without an extra API call — used to backfill the PROJECT column for a
// pipeline whose project isn't one of the user's synced projects (most
// visibly, a downstream pipeline a trigger job kicks off can live in an
// entirely different repo than any default_groups covers).
func projectPathFromPipelineURL(webURL string) (string, bool) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", false
	}
	const marker = "/-/pipelines/"
	i := strings.Index(u.Path, marker)
	if i < 0 {
		return "", false
	}
	path := strings.Trim(u.Path[:i], "/")
	if path == "" {
		return "", false
	}
	return path, true
}

// saveChosenGroups merges the discovery picker's selection into the active
// instance's default_groups (additive: existing groups are kept even if
// not re-selected this time), persists config, and triggers a resync.
func (m Model) saveChosenGroups(msg components.GroupsChosenMsg) (tea.Model, tea.Cmd) {
	if len(msg.FullPaths) == 0 {
		m.setStatus("no groups selected")
		return m, nil
	}
	inst, err := m.cfg.Active()
	if err != nil {
		m.setErr(err)
		return m, nil
	}
	existing := map[string]bool{}
	for _, p := range inst.DefaultGroups {
		existing[p] = true
	}
	added := 0
	for _, p := range msg.FullPaths {
		if !existing[p] {
			inst.DefaultGroups = append(inst.DefaultGroups, p)
			existing[p] = true
			added++
		}
	}
	m.cfg.Instances[m.cfg.CurrentInstance] = inst
	m.setStatus(fmt.Sprintf("added %d group(s), syncing...", added))
	return m, tea.Batch(saveConfigCmd(m.cfg, m.configPath), m.syncProjectsCmd())
}

func (m Model) dispatchBatch(msg components.DispatchMsg) (tea.Model, tea.Cmd) {
	if len(msg.Projects) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(msg.Projects))
	for _, proj := range msg.Projects {
		ref := msg.Ref
		if locked, ok := m.projectList.LockedRef[proj.ID]; ok && locked != "" {
			ref = locked
		}
		id := m.newReqID()
		cmds = append(cmds, createPipelineCmd(m.ctx, m.client, proj.ID, ref, msg.Vars, id))
	}
	m.loading = true
	return m, tea.Batch(cmds...)
}

func (m Model) openLogs(msg components.OpenLogsMsg) (tea.Model, tea.Cmd) {
	if m.logCancel != nil {
		m.logCancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.logCancel = cancel

	id := m.newReqID()
	m.genLogs = id
	m.logViewer.Open(msg.JobID)
	if j, ok := m.pipelineList.HighlightedJob(); ok {
		m.logJobURL = j.WebURL
	}
	return m, startLogStreamCmd(ctx, m.client, msg.ProjectID, msg.JobID, id)
}

func (m Model) bulkPipelineAction(msg components.BulkPipelineActionMsg) (tea.Model, tea.Cmd) {
	if len(msg.Targets) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(msg.Targets))
	for _, pl := range msg.Targets {
		id := m.newReqID()
		if msg.Retry {
			cmds = append(cmds, retryPipelineCmd(m.ctx, m.client, pl.ProjectID, pl.ID, id))
		} else {
			cmds = append(cmds, cancelPipelineCmd(m.ctx, m.client, pl.ProjectID, pl.ID, id))
		}
	}
	m.loading = true
	return m, tea.Batch(cmds...)
}

func (m Model) bulkJobAction(msg components.BulkJobActionMsg) (tea.Model, tea.Cmd) {
	if len(msg.Targets) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(msg.Targets))
	for _, j := range msg.Targets {
		id := m.newReqID()
		switch {
		case msg.Play:
			cmds = append(cmds, playJobCmd(m.ctx, m.client, j.ProjectID, j.PipelineID, j.ID, id))
		case msg.Retry:
			cmds = append(cmds, retryJobCmd(m.ctx, m.client, j.ProjectID, j.PipelineID, j.ID, id))
		default:
			cmds = append(cmds, cancelJobCmd(m.ctx, m.client, j.ProjectID, j.PipelineID, j.ID, id))
		}
	}
	m.loading = true
	return m, tea.Batch(cmds...)
}

// applyConfigChange persists an edit made in the settings screen and, when
// it touched the active connection (URL, token, or the instance set
// itself), rebuilds the API client and project cache so the change takes
// effect without a restart — the whole point of backlog 031.
func (m Model) applyConfigChange(msg components.ConfigChangedMsg) (tea.Model, tea.Cmd) {
	if msg.ReloadInstance {
		m.initInstance()
	}
	if msg.Reason != "" {
		m.setStatus(msg.Reason)
	}
	return m, saveConfigCmd(m.cfg, m.configPath)
}

// savePreset stores the trigger modal's current contents as a named,
// runnable preset — the projects it was about to fire at, the ref, and the
// variables as typed — so the same fan-out is one keystroke from <Space> v
// next time.
func (m Model) savePreset(msg components.SavePresetMsg) (tea.Model, tea.Cmd) {
	if msg.Name == "" {
		return m, nil
	}
	vars := make(map[string]string, len(msg.Vars))
	for _, v := range msg.Vars {
		if v.Key == "" {
			continue
		}
		vars[v.Key] = v.Value
	}
	if len(vars) == 0 {
		vars = nil
	}
	m.cfg.SetPreset(msg.Name, config.Preset{
		Ref:       msg.Ref,
		Projects:  msg.Projects,
		Variables: vars,
	})
	m.setStatus(fmt.Sprintf("saved preset %q (%d project(s), %d variable(s)) — <space> v to run it", msg.Name, len(msg.Projects), len(vars)))
	return m, saveConfigCmd(m.cfg, m.configPath)
}

// presetEntries flattens the configured presets into the picker's view
// model, in sorted name order.
func (m Model) presetEntries() []components.PresetEntry {
	names := m.cfg.PresetNames()
	out := make([]components.PresetEntry, 0, len(names))
	for _, name := range names {
		p := m.cfg.Presets[name]
		out = append(out, components.PresetEntry{
			Name:     name,
			Ref:      p.Ref,
			Projects: p.Projects,
			Vars:     presetVars(p),
		})
	}
	return out
}

// presetVars renders a preset's variable map as sorted api.Variables — the
// map is unordered, and both the trigger modal and a preset run need a
// stable order to display and dispatch.
func presetVars(p config.Preset) []api.Variable {
	vars := make([]api.Variable, 0, len(p.Variables))
	for k, v := range p.Variables {
		vars = append(vars, api.Variable{Key: k, Value: v, Type: api.VarTypeEnv})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	return vars
}

// presetTargets resolves a preset's path_with_namespace entries against the
// synced project cache, returning the projects it found (in the preset's own
// order) and the paths it couldn't. A miss is reported, not fatal: one repo
// renamed or moved out of default_groups must not block the rest of a
// preset that's otherwise fine.
func (m Model) presetTargets(p config.Preset) (found []api.Project, missing []string) {
	byPath := make(map[string]api.Project, len(m.cacheIdx.Projects))
	for _, proj := range m.cacheIdx.Projects {
		byPath[proj.PathWithNamespace] = proj
	}
	for _, path := range p.Projects {
		if proj, ok := byPath[path]; ok {
			found = append(found, proj)
			continue
		}
		missing = append(missing, path)
	}
	return found, missing
}

// presetRefFor picks the ref one project in a preset run is triggered on:
// the preset's own ref if it names one, otherwise that project's default
// branch. A preset is self-contained by design — the explorer's T/t/ctrl+r
// ref locks deliberately do *not* override it, unlike a <Space> p dispatch,
// where the lock is the whole point.
func presetRefFor(p config.Preset, proj api.Project) string {
	if p.Ref != "" {
		return p.Ref
	}
	if proj.DefaultBranch != "" {
		return proj.DefaultBranch
	}
	return "main"
}

// runPreset fires a runnable preset in one keystroke: its own projects, its
// own ref, its own variables, no trigger modal in between.
func (m Model) runPreset(name string) (tea.Model, tea.Cmd) {
	p, ok := m.cfg.Presets[name]
	if !ok {
		m.setErr(fmt.Errorf("preset %q is not configured", name))
		return m, nil
	}
	if !p.Runnable() {
		m.setErr(fmt.Errorf("preset %q names no projects — select it and use <space> p instead", name))
		return m, nil
	}

	found, missing := m.presetTargets(p)
	if len(found) == 0 {
		m.setErr(fmt.Errorf("preset %q: none of its %d project(s) are in the synced cache — <space> r to resync", name, len(p.Projects)))
		return m, nil
	}

	vars := presetVars(p)
	cmds := make([]tea.Cmd, 0, len(found))
	for _, proj := range found {
		id := m.newReqID()
		cmds = append(cmds, createPipelineCmd(m.ctx, m.client, proj.ID, presetRefFor(p, proj), vars, id))
	}

	status := fmt.Sprintf("running preset %q on %d project(s)", name, len(found))
	if len(missing) > 0 {
		status += fmt.Sprintf(" — skipped %d not in cache: %s", len(missing), strings.Join(missing, ", "))
	}
	m.setStatus(status)
	m.loading = true
	return m, tea.Batch(cmds...)
}

// refSearchSubmitMsg is emitted when the user submits a ref to search for.
type refSearchSubmitMsg struct {
	ref string
}

// refSearch is the <Space> b modal: search for pipelines by an exact ref
// name across every project currently in the cache, without needing to
// already know (or stage) which project it belongs to.
type refSearch struct {
	Active bool
	input  textinput.Model
}

func newRefSearch() refSearch {
	in := textinput.New()
	in.Prompt = "ref: "
	in.Placeholder = "exact branch or tag name..."
	in.Width = 40
	return refSearch{input: in}
}

func (r *refSearch) Open() {
	r.Active = true
	r.input.SetValue("")
	r.input.Focus()
}

func (r refSearch) Update(msg tea.Msg) (refSearch, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			r.Active = false
			r.input.Blur()
			return r, nil
		case "enter":
			ref := strings.TrimSpace(r.input.Value())
			r.Active = false
			r.input.Blur()
			if ref == "" {
				return r, nil
			}
			return r, func() tea.Msg { return refSearchSubmitMsg{ref: ref} }
		}
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	return r, cmd
}

func (r refSearch) View() string {
	return "Search pipelines by ref across every synced project (exact match):\n\n" +
		r.input.View() +
		"\n\nenter: search · esc: cancel"
}
