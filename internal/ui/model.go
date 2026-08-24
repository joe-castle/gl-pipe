// Package ui implements gl-pipe's Bubbletea root model: view routing, key
// dispatch precedence, generation-counted async requests, and the Cmd
// factories that talk to internal/api. See internal/ui/components for the
// individual sub-models.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/cache"
	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui/components"
)

type appView int

const (
	viewWizard appView = iota
	viewMain
)

type pane int

const (
	paneExplorer pane = iota
	panePipelines
)

// Model is gl-pipe's root Bubbletea model.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg        *config.Config
	configPath string
	cacheDir   string
	client     *api.Client

	cacheIdx *cache.Index

	view appView
	pane pane

	wizard       components.Wizard
	projectList  components.ProjectList
	pipelineList components.PipelineList
	variables    components.Variables
	logViewer    components.LogViewer
	settings     components.Settings
	leaderMenu   components.LeaderMenu
	presets      presetPicker
	groupPicker  components.GroupPicker

	statusMsg string
	statusErr bool
	loading   bool

	width, height int

	nextReqID    reqID
	genProjects  reqID
	genPipelines reqID
	genJobs      reqID
	genLogs      reqID
	genGroups    reqID

	logCancel context.CancelFunc
	logJobURL string
	logChan   <-chan api.LogChunk

	projectNames map[int]string // id -> path_with_namespace, for matrix display + blob hit backfill
}

// New constructs the root model. If cfg is nil, gl-pipe starts in the
// first-run wizard; otherwise it starts in the main explorer against the
// config's current instance, using cacheDir for per-instance project caches.
func New(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, configPath, cacheDir string) Model {
	m := Model{
		ctx:          ctx,
		cancel:       cancel,
		cfg:          cfg,
		configPath:   configPath,
		cacheDir:     cacheDir,
		wizard:       components.NewWizard(),
		projectList:  components.NewProjectList(),
		pipelineList: components.NewPipelineList(),
		variables:    components.NewVariables(),
		logViewer:    components.NewLogViewer(),
		settings:     components.NewSettings(),
		leaderMenu:   components.NewLeaderMenu(toComponentActions(LeaderActions)),
		groupPicker:  components.NewGroupPicker(),
		projectNames: map[int]string{},
	}
	if cfg == nil {
		m.view = viewWizard
		return m
	}
	m.view = viewMain
	m.initInstance()
	return m
}

func toComponentActions(actions []LeaderAction) []components.LeaderAction {
	out := make([]components.LeaderAction, len(actions))
	for i, a := range actions {
		out[i] = components.LeaderAction{Key: a.Key, Label: a.Label}
	}
	return out
}

// initInstance (re)builds the API client and loads the on-disk cache for
// whichever instance is currently active in cfg. Called on startup and
// after switching instances.
func (m *Model) initInstance() {
	inst, err := m.cfg.Active()
	if err != nil {
		m.setErr(err)
		return
	}
	token, err := inst.ResolveToken()
	if err != nil {
		m.setErr(fmt.Errorf("resolving token for %q: %w", m.cfg.CurrentInstance, err))
		return
	}
	client, err := api.NewClient(inst.URL, token)
	if err != nil {
		m.setErr(err)
		return
	}
	m.client = client

	idx, err := cache.Load(m.instanceCachePath())
	if err != nil {
		idx = &cache.Index{}
	}
	m.cacheIdx = idx
	m.projectList.SetProjects(idx.Projects)
	m.projectList.DefaultGroup = firstOrEmpty(inst.DefaultGroups)
	m.rebuildProjectNames()
}

func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

func (m *Model) instanceCachePath() string {
	return filepath.Join(m.cacheDir, "cache-"+m.cfg.CurrentInstance+".json")
}

func (m *Model) rebuildProjectNames() {
	m.projectNames = make(map[int]string, len(m.cacheIdx.Projects))
	for _, p := range m.cacheIdx.Projects {
		m.projectNames[p.ID] = p.PathWithNamespace
	}
	m.pipelineList.SetProjectNames(m.projectNames)
}

func (m *Model) setErr(err error) {
	m.statusMsg = err.Error()
	m.statusErr = true
	m.loading = false
}

func (m *Model) setStatus(s string) {
	m.statusMsg = s
	m.statusErr = false
}

func (m *Model) newReqID() reqID {
	m.nextReqID++
	return m.nextReqID
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.view == viewWizard {
		return nil
	}
	if m.cacheIdx.Stale(m.cfg.TTL()) || len(m.cacheIdx.Projects) == 0 {
		return m.syncProjectsCmd()
	}
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.projectList.SetSize(msg.Width, msg.Height-4)
		m.pipelineList.SetSize(msg.Width, msg.Height-4)
		m.logViewer.SetSize(msg.Width, msg.Height-4)
		m.leaderMenu.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.teardown()
			return m, tea.Quit
		}
		return m.handleKey(msg)

	case components.WizardSubmitMsg:
		id := m.newReqID()
		m.wizard.SetValidating(true)
		return m, validateCredentialsCmd(m.ctx, msg.URL, msg.Token, id)

	case credentialsValidatedMsg:
		if msg.err != nil {
			m.wizard.SetError(msg.err)
			return m, nil
		}
		m.wizard.Username = msg.username
		m.wizard.SetValidating(false)
		m.cfg = &config.Config{
			CurrentInstance: "default",
			Instances: map[string]config.Instance{
				"default": {URL: m.wizard.URLInput.Value(), Token: m.wizard.TokenInput.Value()},
			},
			Cache: config.CacheConfig{TTLMinutes: 60},
		}
		return m, saveConfigCmd(m.cfg, m.configPath)

	case configSavedMsg:
		if msg.err != nil {
			m.wizard.SetError(msg.err)
			return m, nil
		}
		m.view = viewMain
		m.initInstance()
		return m, m.syncProjectsCmd()

	case projectsSyncedMsg:
		if msg.reqID != m.genProjects {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		inst, instErr := m.cfg.Active()
		if instErr == nil && len(inst.DefaultGroups) == 0 {
			// Deliberately not cached as a "fresh" sync (invariant: an
			// unconfigured instance must keep re-surfacing this status on
			// every launch/refresh, never silently settle into an empty
			// cache that looks the same as a real zero-project group).
			m.setStatus("no default_groups configured for " + msg.instance + " — add some in config.yaml, then <space> r to sync")
			return m, nil
		}
		m.cacheIdx = &cache.Index{Instance: msg.instance, SyncedAt: time.Now(), Projects: msg.projects}
		m.projectList.SetProjects(msg.projects)
		m.rebuildProjectNames()
		m.setStatus(fmt.Sprintf("synced %d projects from %d group(s)", len(msg.projects), len(inst.DefaultGroups)))
		return m, saveCacheCmd(m.cacheIdx, m.instanceCachePath())

	case blobSearchResultsMsg:
		if msg.reqID != m.genProjects {
			return m, nil
		}
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		hits := msg.hits
		for i := range hits {
			hits[i].ProjectPath = m.projectNames[hits[i].ProjectID]
		}
		m.projectList.BlobHits = hits
		m.setStatus(fmt.Sprintf("%d blob search hits", len(hits)))
		return m, nil

	case refsLoadedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		if ref, ok := api.LatestSemVerTag(msg.refs); ok {
			m.projectList.LockedRef[msg.projectID] = ref.Name
			m.setStatus("locked " + m.projectNames[msg.projectID] + " to " + ref.Name)
		} else {
			m.setStatus("no semver tags found for " + m.projectNames[msg.projectID])
		}
		return m, nil

	case groupsLoadedMsg:
		if msg.reqID != m.genGroups {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		inst, _ := m.cfg.Active()
		m.groupPicker.Open(msg.groups, inst.DefaultGroups)
		return m, nil

	case components.GroupsChosenMsg:
		return m.saveChosenGroups(msg)

	case components.DispatchMsg:
		return m.dispatchBatch(msg)

	case pipelineTriggeredMsg:
		m.loading = false
		if msg.err != nil {
			m.setErr(fmt.Errorf("triggering pipeline in %s: %w", m.projectNames[msg.projectID], msg.err))
			return m, nil
		}
		m.pipelineList.AddOrUpdate(msg.pipeline)
		m.pane = panePipelines
		m.setStatus("triggered pipeline in " + m.projectNames[msg.projectID])
		return m, nil

	case pipelinesLoadedMsg:
		if msg.reqID != m.genPipelines {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(fmt.Errorf("loading pipelines for %s: %w", m.projectNames[msg.projectID], msg.err))
			return m, nil
		}
		// Merge into whatever this generation's batch has already loaded
		// from other staged projects — the matrix itself was cleared once
		// when the batch started (see openPipelinesForSelected), so this
		// never mixes in results from an earlier, unrelated view.
		for _, pl := range msg.pipelines {
			m.pipelineList.AddOrUpdate(pl)
		}
		m.pane = panePipelines
		name := m.projectNames[msg.projectID]
		if len(msg.pipelines) == 0 {
			m.setStatus("no pipelines found for " + name)
		} else {
			m.setStatus(fmt.Sprintf("%d pipeline(s) for %s", len(msg.pipelines), name))
		}
		return m, nil

	case pipelineDetailMsg:
		if msg.err == nil {
			m.pipelineList.UpdatePipeline(msg.pipeline)
		}
		return m, nil

	case pipelineActionMsg:
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.pipelineList.UpdatePipeline(msg.pipeline)
		return m, nil

	case jobsLoadedMsg:
		if msg.reqID != m.genJobs {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		if pl, ok := m.pipelineList.FindPipeline(msg.pipelineID); ok {
			m.pipelineList.AddJobs(pl, msg.jobs)
		}
		return m, nil

	case jobActionMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		// Refresh just this one pipeline's jobs within the current
		// generation — AddJobs merges the refreshed statuses back in
		// without disturbing any other pipeline already shown.
		return m, jobsForPipelineCmd(m.ctx, m.client, msg.projectID, msg.pipelineID, m.genJobs)

	case components.OpenJobsMsg:
		return m.openJobsForPipelines(msg.Pipelines)

	case components.OpenLogsMsg:
		return m.openLogs(msg)

	case logStreamReadyMsg:
		if msg.reqID != m.genLogs {
			return m, nil
		}
		m.logChan = msg.ch
		return m, waitForLogChunkCmd(msg.ch, msg.reqID, msg.jobID)

	case logChunkMsg:
		if msg.reqID != m.genLogs {
			return m, nil
		}
		if msg.chunk.Err != nil {
			m.setErr(msg.chunk.Err)
			return m, nil
		}
		if msg.chunk.Content != "" {
			m.logViewer.Append(msg.chunk.Content)
		}
		if msg.chunk.Done {
			m.logViewer.MarkDone()
			return m, nil
		}
		return m, waitForLogChunkCmd(m.logChan, msg.reqID, msg.jobID)

	case components.BulkPipelineActionMsg:
		return m.bulkPipelineAction(msg)

	case components.BulkJobActionMsg:
		return m.bulkJobAction(msg)

	case components.SwitchInstanceMsg:
		if err := m.cfg.SetActive(msg.Name); err != nil {
			m.setErr(err)
			return m, nil
		}
		m.initInstance()
		m.setStatus("switched to instance " + msg.Name)
		return m, saveConfigCmd(m.cfg, m.configPath)

	case presetChosenMsg:
		m.presets.Active = false
		m.setStatus("preset " + msg.name + " selected — <space> p to trigger")
		return m, nil

	case components.LeaderActionMsg:
		return m.handleLeaderAction(msg.Key)

	case errMsg:
		if msg.err != nil {
			m.setErr(msg.err)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) teardown() {
	if m.logCancel != nil {
		m.logCancel()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// View implements tea.Model.
func (m Model) View() string {
	if m.view == viewWizard {
		return modalStyle.Render(m.wizard.View())
	}

	var body string
	switch {
	case m.logViewer.Active:
		body = m.logViewer.View()
	case m.variables.Active:
		body = modalStyle.Render(m.variables.View())
	case m.settings.Active:
		body = modalStyle.Render(m.settings.View())
	case m.presets.Active:
		body = modalStyle.Render(m.presets.View())
	case m.groupPicker.Active:
		body = modalStyle.Render(m.groupPicker.View())
	case m.pane == panePipelines:
		body = m.pipelineList.View()
	default:
		body = m.projectList.View()
	}

	title := titleStyle.Render(" gl-pipe ") + "  " + m.cfg.CurrentInstance
	status := statusBarStyle.Render(m.statusMsg)
	if m.statusErr {
		status = statusBarStyle.Render(errorStyle.Render(m.statusMsg))
	}
	if m.loading {
		status = statusBarStyle.Render("loading...")
	}

	out := lipgloss.JoinVertical(lipgloss.Left, title, body, status)
	if m.leaderMenu.Active {
		out = lipgloss.JoinVertical(lipgloss.Left, out, m.leaderMenu.View())
	}
	return out
}

func sortedKeys(m map[string]config.Instance) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
