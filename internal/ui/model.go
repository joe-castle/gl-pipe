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

	"github.com/charmbracelet/bubbles/progress"
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

// refLockStrategy picks how T/t decide a project's "latest" tag.
type refLockStrategy int

const (
	refLockSemVer        refLockStrategy = iota // T: highest parsed SemVer
	refLockLatestCreated                        // t: most recently created tag, any name
)

func (s refLockStrategy) pick(refs []api.Ref) (api.Ref, bool) {
	if s == refLockLatestCreated {
		return api.LatestCreatedTag(refs)
	}
	return api.LatestSemVerTag(refs)
}

func (s refLockStrategy) label() string {
	if s == refLockLatestCreated {
		return "latest created tag"
	}
	return "latest SemVer tag"
}

// refPickerSource says which ctrl+r flow an in-flight ref-picker fetch
// belongs to — the trigger modal and the explorer both open the same
// underlying overlay pattern but for different components.
type refPickerSource int

const (
	refPickerForTrigger  refPickerSource = iota // trigger modal: pick the ref to dispatch with
	refPickerForExplorer                        // explorer: override a project's locked ref
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
	presets      components.PresetList
	groupPicker  components.GroupPicker
	refSearch    refSearch
	mrList       components.MRList

	// pipelinesPending/pipelinesErrored track a batch of concurrent
	// pipelinesLoadedMsg responses (staged-projects view, a ref search
	// across every known project, or an MR selection's pipelines) so the
	// status line reports one clean summary on completion instead of
	// flickering per-response — with dozens of projects most responses are
	// empty misses, and showing each as its own status would bury (or
	// overwrite) the real result. pipelinesTotal is the batch's original
	// size — Pending alone only tells you what's left, not how far along
	// the batch is — used to drive the loading progress bar (loadingProgress).
	pipelinesPending int
	pipelinesErrored int
	pipelinesTotal   int

	// jobsPending/jobsErrored/jobsTotal is the same tracking for a job-
	// matrix batch (Enter on one or more staged pipelines, or the "view
	// downstream pipeline" jump).
	jobsPending int
	jobsErrored int
	jobsTotal   int

	// digestPending/digestErrored/digestTotal is the same tracking for a
	// failure-digest batch (E in the job matrix: one trace fetch per failed
	// job currently loaded).
	digestPending   int
	digestErrored   int
	digestTotal     int
	digestErrReason string // first fetch error of the batch, for the summary line

	// mrsPending/mrsErrored is the same tracking for a project-scoped MR
	// fetch batch (M on the explorer, across staged projects). "My MRs"
	// (<Space> m) is a single global request and doesn't need this.
	mrsPending int
	mrsErrored int
	mrsTotal   int

	// refsPending/refsErrored/refsLocked/refsSkipped track a "lock to
	// latest tag" batch (T/t on the explorer, across staged/highlighted
	// projects), same completion-summary pattern as the pipeline and MR
	// batches. refsSkipped is a project with no tag qualifying under the
	// active refLockMode — distinct from refsErrored (an actual API
	// failure).
	refsPending int
	refsErrored int
	refsLocked  int
	refsSkipped int
	refsTotal   int
	refLockMode refLockStrategy // which strategy the in-flight refsPending batch is using

	// downstreamOrigin is true right after landing a downstream pipeline
	// via a bridge job (openDownstreamPipeline) — PipelineList.Pipelines/
	// jobs are left untouched by that jump (only the *mode* changes, via
	// SetPipelines), so the job matrix the jump came from is still sitting
	// there in memory to go back to. It lets Esc from the resulting
	// (single-row) pipeline view return to that job matrix instead of
	// leaving the pipelines pane for the explorer, same as every other
	// top-level pipeline view's Esc would. Any other action that starts a
	// fresh, unrelated pipeline batch (startPipelinesBatch) resets it,
	// since "Esc goes back to a job view" would be wrong once the
	// underlying job data no longer corresponds to what's showing.
	downstreamOrigin bool

	// progressBar renders the loading status line's progress bar
	// (loadingProgress picks which *Pending/*Total pair, if any, is
	// currently in flight) — a static ViewAs(fraction) render, not the
	// animated SetPercent path, so it needs no Update()/tea.Cmd wiring of
	// its own.
	progressBar progress.Model

	// refPickerFor disambiguates who an in-flight genRefPicker fetch is
	// for: the trigger modal's "pick the ref to dispatch with" (ctrl+r
	// there) and the explorer's "override this project's locked ref"
	// (ctrl+r here) both use loadAllRefsCmd/refPickerLoadedMsg, but the
	// result has to open a different overlay depending on which asked.
	refPickerFor refPickerSource

	// chosenPreset is the preset most recently selected (not run) from the
	// <Space> v picker: the next trigger modal prefills its variables and
	// ref. Runnable presets bypass this entirely — they dispatch directly.
	chosenPreset string

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
	genMRs       reqID
	genBlob      reqID
	genRefs      reqID
	genRefPicker reqID
	genDigest    reqID

	logCancel context.CancelFunc
	logJobURL string
	logChan   <-chan api.LogChunk

	projectNames map[int]string // id -> path_with_namespace, for matrix display + blob hit backfill
}

// New constructs the root model. If cfg is nil, gl-pipe starts in the
// first-run wizard; otherwise it starts in the main explorer against the
// config's current instance, using cacheDir for per-instance project caches.
func New(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, configPath, cacheDir string) Model {
	// Panic reports live alongside config.yaml and the project caches: the
	// alt screen swallows anything bubbletea prints on the way down, so a
	// crash is only diagnosable if it's also on disk (see crashlog.go).
	SetCrashLog(filepath.Join(cacheDir, "crash.log"))

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
		presets:      components.NewPresetList(),
		groupPicker:  components.NewGroupPicker(),
		refSearch:    newRefSearch(),
		mrList:       components.NewMRList(),
		projectNames: map[int]string{},
		progressBar:  progress.New(progress.WithSolidFill("51"), progress.WithoutPercentage(), progress.WithWidth(24)),
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

// loadingProgress reports how far along the currently in-flight batch (if
// any) is, for the status line's progress bar. At most one of the four
// batch kinds is ever actually in flight at a time — the UI is modal, so
// concurrent batches can't be triggered — so checking Pending > 0 in turn
// is enough to find the active one without a separate "which kind"
// field. ok is false when nothing batch-shaped is loading (a single-item
// fetch like a project sync or a trigger dispatch), in which case the
// status line falls back to a plain "loading...".
func (m Model) loadingProgress() (done, total int, ok bool) {
	switch {
	case m.pipelinesPending > 0 && m.pipelinesTotal > 0:
		return m.pipelinesTotal - m.pipelinesPending, m.pipelinesTotal, true
	case m.jobsPending > 0 && m.jobsTotal > 0:
		return m.jobsTotal - m.jobsPending, m.jobsTotal, true
	case m.digestPending > 0 && m.digestTotal > 0:
		return m.digestTotal - m.digestPending, m.digestTotal, true
	case m.mrsPending > 0 && m.mrsTotal > 0:
		return m.mrsTotal - m.mrsPending, m.mrsTotal, true
	case m.refsPending > 0 && m.refsTotal > 0:
		return m.refsTotal - m.refsPending, m.refsTotal, true
	default:
		return 0, 0, false
	}
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
	cmds := []tea.Cmd{pollTickCmd()}
	if m.cacheIdx.Stale(m.cfg.TTL()) || len(m.cacheIdx.Projects) == 0 {
		cmds = append(cmds, m.syncProjectsCmd())
	}
	return tea.Batch(cmds...)
}

// pollTick fires every pollInterval for the life of the program. It's a
// no-op — beyond rescheduling itself — unless the pipeline/job matrix is
// on screen and has anything non-terminal left to refresh; a mid-batch
// fetch (m.loading) is skipped too, so a poll tick never piles requests on
// top of one already in flight.
func (m Model) pollTick() (tea.Model, tea.Cmd) {
	next := pollTickCmd()
	if !m.shouldPoll() {
		return m, next
	}
	cmd := m.refreshActiveMatrix()
	if cmd == nil {
		return m, next
	}
	return m, tea.Batch(cmd, next)
}

// shouldPoll gates the periodic tick: only while the pipeline/job matrix is
// on screen, nothing else is already loading, and something shown hasn't
// settled yet.
func (m Model) shouldPoll() bool {
	return m.pane == panePipelines && !m.loading && m.pipelineList.NeedsPoll()
}

// refreshActiveMatrix re-fetches whatever's currently shown in the
// pipeline/job matrix — the job matrix by pipeline (jobsForPipelineCmd,
// checked against the live genJobs), the pipeline matrix by individual
// pipeline (pipelineDetailCmd, which patches in place and is harmless if
// stale). Shared by the periodic poll and the manual 'r' refresh key.
func (m Model) refreshActiveMatrix() tea.Cmd {
	// The digest is a view of the job matrix's own data, so it refreshes the
	// same way — by pipeline, not by pipeline detail.
	if m.pipelineList.InJobs() || m.pipelineList.InDigest() {
		pipelines := m.pipelineList.Pipelines
		if len(pipelines) == 0 {
			return nil
		}
		cmds := make([]tea.Cmd, len(pipelines))
		for i, pl := range pipelines {
			cmds[i] = jobsForPipelineCmd(m.ctx, m.client, pl.ProjectID, pl.ID, m.genJobs)
		}
		return tea.Batch(cmds...)
	}
	pipelines := m.pipelineList.AllPipelines()
	if len(pipelines) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(pipelines))
	for i, pl := range pipelines {
		cmds[i] = pipelineDetailCmd(m.ctx, m.client, pl.ProjectID, pl.ID, m.genPipelines)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// header(1) + breadcrumb(1) + status(1) = 3 lines outside the body
		// always; the explorer/pipeline panes additionally sit inside
		// contentBorderStyle's frame (2 rows, 2 cols) — see View().
		m.projectList.SetSize(msg.Width-2, msg.Height-5)
		m.pipelineList.SetSize(msg.Width-2, msg.Height-5)
		m.logViewer.SetSize(msg.Width, msg.Height-3)
		m.mrList.SetSize(msg.Width, msg.Height-4)
		m.leaderMenu.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.teardown()
			return m, tea.Quit
		}
		// Intercepted ahead of every modal and pane so a diagnostic dump is
		// always reachable, whatever has focus.
		if msg.String() == "ctrl+d" {
			return m.dumpDebugState()
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
		// Only the wizard's very first save graduates into the main view
		// and kicks off the initial sync. Every later save — an instance
		// switch, the group picker, the settings editor — is a background
		// write against an already-running app, and re-running
		// initInstance/syncProjects for each of those would resync the
		// whole project list every time a setting is nudged.
		if m.view == viewWizard {
			if msg.err != nil {
				m.wizard.SetError(msg.err)
				return m, nil
			}
			m.view = viewMain
			m.initInstance()
			return m, m.syncProjectsCmd()
		}
		if msg.err != nil {
			m.setErr(fmt.Errorf("saving config: %w", msg.err))
		}
		return m, nil

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

	case components.BlobSearchRequestMsg:
		id := m.newReqID()
		m.genBlob = id
		m.loading = true
		m.setStatus(fmt.Sprintf("searching %s for %q...", msg.Group, msg.Query))
		return m, blobSearchCmd(m.ctx, m.client, msg.Group, msg.Query, id)

	case blobSearchResultsMsg:
		if msg.reqID != m.genBlob {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		hits := msg.hits
		for i := range hits {
			hits[i].ProjectPath = m.projectNames[hits[i].ProjectID]
		}
		m.projectList.BlobHits = hits
		m.setStatus(fmt.Sprintf("%d blob search hit(s)", len(hits)))
		return m, nil

	case refsLoadedMsg:
		if msg.reqID != m.genRefs {
			return m, nil
		}
		if msg.err != nil {
			m.refsErrored++
		} else if ref, ok := m.refLockMode.pick(msg.refs); ok {
			m.projectList.SetLockedRef(msg.projectID, ref.Name)
			m.refsLocked++
		} else {
			m.refsSkipped++
		}
		if m.refsPending > 0 {
			m.refsPending--
		}
		if m.refsPending == 0 {
			m.loading = false
			summary := fmt.Sprintf("locked %d project(s) to %s", m.refsLocked, m.refLockMode.label())
			if m.refsSkipped > 0 {
				summary += fmt.Sprintf(", %d with no qualifying tags", m.refsSkipped)
			}
			if m.refsErrored > 0 {
				summary += fmt.Sprintf(", %d failed", m.refsErrored)
			}
			m.setStatus(summary)
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

	case myMRsLoadedMsg:
		if msg.reqID != m.genMRs {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.mrList.SetProjectNames(m.projectNames)
		m.mrList.SetMRs(msg.mrs)
		m.setStatus(fmt.Sprintf("%d merge request(s)", len(msg.mrs)))
		return m, nil

	case projectMRsLoadedMsg:
		if msg.reqID != m.genMRs {
			return m, nil
		}
		if msg.err != nil {
			m.mrsErrored++
		} else {
			m.mrList.AddMRs(msg.mrs)
		}
		if m.mrsPending > 0 {
			m.mrsPending--
		}
		if m.mrsPending == 0 {
			m.loading = false
			summary := fmt.Sprintf("%d merge request(s) found", m.mrList.Count())
			if m.mrsErrored > 0 {
				summary += fmt.Sprintf(" (%d project(s) failed to load)", m.mrsErrored)
			}
			m.setStatus(summary)
		}
		return m, nil

	case components.MRsChosenMsg:
		return m.openPipelinesForMRs(msg.MRs)

	case refSearchSubmitMsg:
		return m.searchPipelinesByRef(msg.ref)

	case components.RefPickerRequestMsg:
		if msg.ProjectID == 0 {
			m.setStatus("no project staged to browse refs for")
			return m, nil
		}
		id := m.newReqID()
		m.genRefPicker = id
		m.refPickerFor = refPickerForTrigger
		return m, loadAllRefsCmd(m.ctx, m.client, msg.ProjectID, id)

	case refPickerLoadedMsg:
		if msg.reqID != m.genRefPicker {
			return m, nil
		}
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		if m.refPickerFor == refPickerForExplorer {
			m.projectList.OpenRefOverridePicker(msg.refs)
		} else {
			m.variables.OpenRefPicker(msg.refs)
		}
		return m, nil

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
		// Merge into whatever this generation's batch has already loaded
		// from other projects — the matrix itself was cleared once when
		// the batch started (see startPipelinesBatch), so this never mixes
		// in results from an earlier, unrelated view.
		if msg.err != nil {
			m.pipelinesErrored++
		} else {
			for _, pl := range msg.pipelines {
				// A pipeline's project isn't necessarily one the user has
				// synced (default_groups) — most visibly, a downstream
				// pipeline a trigger job kicks off can live in an entirely
				// different repo. Without an entry here the PROJECT column
				// silently rendered blank; recover it from the pipeline's
				// own WebURL rather than an extra API round trip.
				if _, known := m.projectNames[pl.ProjectID]; !known {
					if path, ok := projectPathFromPipelineURL(pl.WebURL); ok {
						m.projectNames[pl.ProjectID] = path
					}
				}
				m.pipelineList.AddOrUpdate(pl)
			}
			m.pane = panePipelines
		}
		if m.pipelinesPending > 0 {
			m.pipelinesPending--
		}
		// Report one summary on completion rather than per-response: a ref
		// search can span dozens of projects, and most responses are empty
		// misses — showing each as its own status would bury the result or
		// leave the status line ending on an uninteresting miss.
		if m.pipelinesPending == 0 {
			m.loading = false
			summary := fmt.Sprintf("%d pipeline(s) found", m.pipelineList.Count())
			if m.pipelinesErrored > 0 {
				summary += fmt.Sprintf(" (%d project(s) failed to load)", m.pipelinesErrored)
			}
			m.setStatus(summary)
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
		if msg.err != nil {
			m.jobsErrored++
		} else if pl, ok := m.pipelineList.FindPipeline(msg.pipelineID); ok {
			m.pipelineList.AddJobs(pl, msg.jobs)
		}
		if m.jobsPending > 0 {
			m.jobsPending--
		}
		if m.jobsPending == 0 {
			m.loading = false
			if m.jobsErrored > 0 {
				m.setStatus(fmt.Sprintf("jobs loaded (%d pipeline(s) failed to load)", m.jobsErrored))
			}
		}
		return m, nil

	case components.FailureDigestRequestMsg:
		return m.startDigestBatch()

	case jobDigestMsg:
		if msg.reqID != m.genDigest {
			return m, nil
		}
		// A failed fetch is recorded too, not dropped: the panel says why,
		// which reads very differently from a job that looks like the digest
		// was never run for it.
		d := components.JobDigest{Lines: msg.lines}
		if msg.err != nil {
			m.digestErrored++
			d = components.JobDigest{Err: msg.err.Error()}
			// Keep one reason for the completion summary: a batch that
			// reports "2 trace(s) failed to load" and nothing else leaves
			// the user with no way to find out why without hunting for the
			// right row.
			if m.digestErrReason == "" {
				m.digestErrReason = msg.err.Error()
			}
		}
		m.pipelineList.SetJobDigest(msg.jobID, d)
		if m.digestPending > 0 {
			m.digestPending--
		}
		if m.digestPending == 0 {
			m.loading = false
			m.setStatus(m.digestSummary())
			// Land the user in the digest rather than back on the job
			// matrix: they asked "why did these fail?", so show the answers.
			m.pipelineList.OpenDigestView()
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

	case tickMsg:
		return m.pollTick()

	case components.RefreshRequestMsg:
		cmd := m.refreshActiveMatrix()
		if cmd == nil {
			return m, nil
		}
		m.setStatus("refreshing...")
		return m, cmd

	case components.OpenJobsMsg:
		return m.openJobsForPipelines(msg.Pipelines)

	case components.OpenDownstreamPipelineMsg:
		if msg.PipelineID == 0 {
			m.setStatus("downstream pipeline hasn't started yet")
			return m, nil
		}
		return m.openDownstreamPipeline(msg.ProjectID, msg.PipelineID)

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

	case components.PresetChosenMsg:
		m.presets.Active = false
		m.chosenPreset = msg.Name
		m.setStatus("preset " + msg.Name + " selected — <space> p to trigger")
		return m, nil

	case components.RunPresetMsg:
		m.presets.Active = false
		return m.runPreset(msg.Name)

	case components.ConfigChangedMsg:
		return m.applyConfigChange(msg)

	case components.SavePresetMsg:
		return m.savePreset(msg)

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
	framed := true
	switch {
	case m.logViewer.Active:
		body = m.logViewer.View()
		framed = false
	case m.variables.Active:
		body = modalStyle.Render(m.variables.View())
		framed = false
	case m.settings.Active:
		body = modalStyle.Render(m.settings.View())
		framed = false
	case m.presets.Active:
		body = modalStyle.Render(m.presets.View())
		framed = false
	case m.groupPicker.Active:
		body = modalStyle.Render(m.groupPicker.View())
		framed = false
	case m.refSearch.Active:
		body = modalStyle.Render(m.refSearch.View())
		framed = false
	case m.mrList.Active:
		body = modalStyle.Render(m.mrList.View())
		framed = false
	case m.pane == panePipelines:
		body = m.pipelineList.View()
	default:
		body = m.projectList.View()
	}
	if framed {
		body = contentBorderStyle.Render(body)
	}

	header := titleStyle.Render(" gl-pipe ") + "  " + contextStyle.Render(m.cfg.CurrentInstance)
	crumb := breadcrumbStyle.Render(m.breadcrumb())

	status := statusBarStyle.Render(m.statusMsg)
	if m.statusErr {
		status = statusBarStyle.Render(errorStyle.Render(m.statusMsg))
	}
	if m.loading {
		if done, total, ok := m.loadingProgress(); ok && total > 1 {
			bar := m.progressBar.ViewAs(float64(done) / float64(total))
			status = statusBarStyle.Render(fmt.Sprintf("loading %d/%d ", done, total) + bar)
		} else {
			status = statusBarStyle.Render("loading...")
		}
	}

	out := lipgloss.JoinVertical(lipgloss.Left, header, crumb, body, status)
	if m.leaderMenu.Active {
		out = lipgloss.JoinVertical(lipgloss.Left, out, m.leaderMenu.View())
	}
	return out
}

// breadcrumb names the current view for the header's second line, k9s-style.
func (m Model) breadcrumb() string {
	switch {
	case m.logViewer.Active:
		return "LOGS"
	case m.variables.Active:
		return "TRIGGER PIPELINE"
	case m.settings.Active:
		return "SETTINGS"
	case m.presets.Active:
		return "PRESETS"
	case m.groupPicker.Active:
		return "DISCOVER GROUPS"
	case m.refSearch.Active:
		return "SEARCH BY REF"
	case m.mrList.Active:
		return "MERGE REQUESTS"
	case m.pane == panePipelines:
		if m.pipelineList.InDigest() {
			return "FAILURE DIGEST"
		}
		if m.pipelineList.InJobs() {
			return "JOBS"
		}
		return "PIPELINES"
	default:
		return "EXPLORER"
	}
}

func sortedKeys(m map[string]config.Instance) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
