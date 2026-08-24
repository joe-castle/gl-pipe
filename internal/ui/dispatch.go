package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

	textFocused := m.pane == paneExplorer && m.projectList.HasTextFocus()
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
				if proj, ok := m.projectList.Highlighted(); ok {
					id := m.newReqID()
					return m, loadRefsCmd(m.ctx, m.client, proj.ID, id)
				}
				return m, nil
			}
		case "enter":
			if !textFocused {
				return m.openPipelinesForSelected()
			}
		}
		var cmd tea.Cmd
		m.projectList, cmd = m.projectList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "tab", "shift+tab":
		m.pane = paneExplorer
		return m, nil
	case "esc":
		if !m.pipelineList.InJobs() {
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

	case "g":
		id := m.newReqID()
		m.genGroups = id
		m.loading = true
		return m, loadGroupsCmd(m.ctx, m.client, id)

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

// openPipelinesForSelected shows pipelines for every staged project
// together in one matrix (falling back to just the highlighted project if
// none are staged) — the same "staged, or highlighted" convention used by
// the trigger modal. The matrix is cleared up front so a fresh Enter never
// mixes rows left over from an earlier view; each project's pipelines are
// then merged in as its own request completes, so the matrix fills in
// incrementally rather than waiting on the slowest project.
func (m Model) openPipelinesForSelected() (tea.Model, tea.Cmd) {
	projects := m.projectList.SelectedProjects()
	if len(projects) == 0 {
		return m, nil
	}
	id := m.newReqID()
	m.genPipelines = id
	m.loading = true
	m.pipelineList.SetPipelines(nil)

	cmds := make([]tea.Cmd, 0, len(projects))
	for _, proj := range projects {
		cmds = append(cmds, pipelinesForProjectCmd(m.ctx, m.client, proj.ID, id))
	}
	return m, tea.Batch(cmds...)
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
			cmds = append(cmds, retryJobCmd(m.ctx, m.client, j.ProjectID, j.ID, id))
		} else {
			cmds = append(cmds, cancelJobCmd(m.ctx, m.client, j.ProjectID, j.ID, id))
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
