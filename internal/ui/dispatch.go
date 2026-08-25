package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
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
		if m.presets.chosen != "" {
			if p, ok := m.cfg.Presets[m.presets.chosen]; ok {
				for k, v := range p.Variables {
					preset = append(preset, api.Variable{Key: k, Value: v, Type: api.VarTypeEnv})
				}
				sort.Slice(preset, func(i, j int) bool { return preset[i].Key < preset[j].Key })
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
		names := make([]string, 0, len(m.cfg.Presets))
		for name := range m.cfg.Presets {
			names = append(names, name)
		}
		sort.Strings(names)
		m.presets.Open(names)
		return m, nil

	case "s":
		names := sortedKeys(m.cfg.Instances)
		presetNames := make([]string, 0, len(m.cfg.Presets))
		for name := range m.cfg.Presets {
			presetNames = append(presetNames, name)
		}
		m.settings.Open(names, m.cfg.CurrentInstance, m.cfg.TTL(), presetNames)
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
func (m Model) startPipelinesBatch(n int, build func(id reqID) []tea.Cmd) (tea.Model, tea.Cmd) {
	if n == 0 {
		return m, nil
	}
	id := m.newReqID()
	m.genPipelines = id
	m.loading = true
	m.pipelinesPending = n
	m.pipelinesErrored = 0
	m.pipelinesTotal = n
	m.pipelineList.SetPipelines(nil)
	return m, tea.Batch(build(id)...)
}

// openPipelinesForSelected shows pipelines for every staged project
// together in one matrix (falling back to just the highlighted project if
// none are staged) — the same "staged, or highlighted" convention used by
// the trigger modal.
func (m Model) openPipelinesForSelected() (tea.Model, tea.Cmd) {
	projects := m.projectList.SelectedProjects()
	return m.startPipelinesBatch(len(projects), func(id reqID) []tea.Cmd {
		cmds := make([]tea.Cmd, len(projects))
		for i, proj := range projects {
			cmds[i] = pipelinesForProjectCmd(m.ctx, m.client, proj.ID, id)
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
	m.setStatus(fmt.Sprintf("searching %d project(s) for ref %q...", len(projects), ref))
	return m.startPipelinesBatch(len(projects), func(id reqID) []tea.Cmd {
		cmds := make([]tea.Cmd, len(projects))
		for i, proj := range projects {
			cmds[i] = pipelinesByRefCmd(m.ctx, m.client, proj.ID, ref, id)
		}
		return cmds
	})
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

// openDownstreamPipeline jumps from a trigger job to the pipeline it kicked
// off, replacing the job matrix with just that one pipeline — the same
// matrix everything else feeds, so sort/filter/Enter-into-jobs/bulk-retry
// all work on it exactly like any other pipeline.
func (m Model) openDownstreamPipeline(projectID, pipelineID int) (tea.Model, tea.Cmd) {
	return m.startPipelinesBatch(1, func(id reqID) []tea.Cmd {
		return []tea.Cmd{pipelineByIDCmd(m.ctx, m.client, projectID, pipelineID, id)}
	})
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
		if msg.Retry {
			cmds = append(cmds, retryJobCmd(m.ctx, m.client, j.ProjectID, j.PipelineID, j.ID, id))
		} else {
			cmds = append(cmds, cancelJobCmd(m.ctx, m.client, j.ProjectID, j.PipelineID, j.ID, id))
		}
	}
	m.loading = true
	return m, tea.Batch(cmds...)
}

// presetChosenMsg is emitted when the user picks a variable preset from the
// <Space> v picker; the next <Space> p trigger will prefill from it.
type presetChosenMsg struct {
	name string
}

// presetPicker is a minimal <Space> v modal: a plain list of preset names.
// It's small enough to keep inline here rather than as its own component.
type presetPicker struct {
	Active bool
	Names  []string
	cursor int
	chosen string
}

func (p *presetPicker) Open(names []string) {
	p.Active = true
	p.Names = names
	p.cursor = 0
}

func (p presetPicker) Update(msg tea.Msg) (presetPicker, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	switch km.String() {
	case "esc":
		p.Active = false
	case "j", "down":
		if p.cursor < len(p.Names)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		if p.cursor < len(p.Names) {
			name := p.Names[p.cursor]
			p.chosen = name
			p.Active = false
			return p, func() tea.Msg { return presetChosenMsg{name: name} }
		}
	}
	return p, nil
}

func (p presetPicker) View() string {
	var b strings.Builder
	b.WriteString("Select a variable preset:\n\n")
	for i, n := range p.Names {
		marker := "  "
		if i == p.cursor {
			marker = "> "
		}
		b.WriteString(marker + n + "\n")
	}
	if len(p.Names) == 0 {
		b.WriteString("(no presets configured in config.yaml)\n")
	}
	b.WriteString("\nj/k: move · enter: choose · esc: cancel")
	return b.String()
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
