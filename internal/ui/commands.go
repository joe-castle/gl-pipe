package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/cache"
	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui/components"
)

// pollInterval is how often the pipeline/job matrix refreshes itself while
// it has anything non-terminal showing.
const pollInterval = 10 * time.Second

// Every Cmd body below is wrapped in safeCmd (see crashlog.go): these run
// on bubbletea's own goroutines, where an unrecovered panic kills the whole
// session and prints its trace into the alt screen, where it can't be read.

// pollTickCmd schedules the next tickMsg. Model.pollTick reissues this
// every time it fires, for the life of the program (invariant #3's
// self-rescheduling pattern) — the tick itself is cheap; Model.pollTick
// decides whether it's actually worth fetching anything.
func pollTickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func validateCredentialsCmd(ctx context.Context, url, token string, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		client, err := api.NewClient(url, token)
		if err != nil {
			return credentialsValidatedMsg{reqID: id, err: err}
		}
		username, err := client.Validate(ctx)
		return credentialsValidatedMsg{reqID: id, username: username, err: err}
	})
}

func saveConfigCmd(cfg *config.Config, path string) tea.Cmd {
	return safeCmd(func() tea.Msg {
		return configSavedMsg{err: cfg.Save(path)}
	})
}

func saveCacheCmd(idx *cache.Index, path string) tea.Cmd {
	return safeCmd(func() tea.Msg {
		_ = idx.Save(path)
		return nil
	})
}

// syncProjectsCmd re-fetches the project index for the active instance from
// every configured default group, deduplicating by project ID.
func (m *Model) syncProjectsCmd() tea.Cmd {
	id := m.newReqID()
	m.genProjects = id
	m.loading = true
	instance := m.cfg.CurrentInstance
	inst, _ := m.cfg.Active()
	groups := inst.DefaultGroups
	client := m.client
	ctx := m.ctx
	return safeCmd(func() tea.Msg {
		seen := map[int]bool{}
		var all []api.Project
		for _, g := range groups {
			projects, err := client.ListGroupProjects(ctx, g)
			if err != nil {
				return projectsSyncedMsg{reqID: id, instance: instance, err: err}
			}
			for _, p := range projects {
				if !seen[p.ID] {
					seen[p.ID] = true
					all = append(all, p)
				}
			}
		}
		return projectsSyncedMsg{reqID: id, instance: instance, projects: all}
	})
}

func blobSearchCmd(ctx context.Context, client *api.Client, group, query string, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		hits, err := client.BlobSearchInGroup(ctx, group, query)
		return blobSearchResultsMsg{reqID: id, hits: hits, err: err}
	})
}

func loadGroupsCmd(ctx context.Context, client *api.Client, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		groups, err := client.ListGroups(ctx)
		return groupsLoadedMsg{reqID: id, groups: groups, err: err}
	})
}

func loadMyMRsCmd(ctx context.Context, client *api.Client, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		mrs, err := client.ListMyMergeRequests(ctx)
		return myMRsLoadedMsg{reqID: id, mrs: mrs, err: err}
	})
}

func loadProjectMRsCmd(ctx context.Context, client *api.Client, projectID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		mrs, err := client.ListProjectMergeRequests(ctx, projectID)
		return projectMRsLoadedMsg{reqID: id, projectID: projectID, mrs: mrs, err: err}
	})
}

func mrPipelinesCmd(ctx context.Context, client *api.Client, projectID, mrIID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		pipelines, err := client.ListMergeRequestPipelines(ctx, projectID, mrIID)
		return pipelinesLoadedMsg{reqID: id, projectID: projectID, pipelines: pipelines, err: err}
	})
}

func loadRefsCmd(ctx context.Context, client *api.Client, projectID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		tags, err := client.ListTags(ctx, projectID)
		return refsLoadedMsg{reqID: id, projectID: projectID, refs: tags, err: err}
	})
}

// loadAllRefsCmd fetches both branches and tags for the ref picker inside
// the trigger modal — unlike loadRefsCmd (tags only, for the "lock to
// latest tag" flow), triggering a pipeline can target either kind of ref.
func loadAllRefsCmd(ctx context.Context, client *api.Client, projectID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		branches, err := client.ListBranches(ctx, projectID)
		if err != nil {
			return refPickerLoadedMsg{reqID: id, projectID: projectID, err: err}
		}
		tags, err := client.ListTags(ctx, projectID)
		if err != nil {
			return refPickerLoadedMsg{reqID: id, projectID: projectID, err: err}
		}
		refs := make([]api.Ref, 0, len(branches)+len(tags))
		refs = append(refs, branches...)
		refs = append(refs, tags...)
		return refPickerLoadedMsg{reqID: id, projectID: projectID, refs: refs}
	})
}

func createPipelineCmd(ctx context.Context, client *api.Client, projectID int, ref string, vars []api.Variable, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		p, err := client.CreatePipeline(ctx, projectID, ref, vars)
		return pipelineTriggeredMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	})
}

func pipelinesForProjectCmd(ctx context.Context, client *api.Client, projectID int, createdAfter time.Time, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		pipelines, err := client.ListPipelines(ctx, projectID, createdAfter)
		return pipelinesLoadedMsg{reqID: id, projectID: projectID, pipelines: pipelines, err: err}
	})
}

// pipelinesByRefCmd fetches one project's pipelines matching a specific
// ref. It reports through the same pipelinesLoadedMsg as
// pipelinesForProjectCmd, so a ref search across every known project reuses
// the exact same accumulate-into-the-matrix handling as viewing staged
// projects' pipelines.
func pipelinesByRefCmd(ctx context.Context, client *api.Client, projectID int, ref string, createdAfter time.Time, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		pipelines, err := client.ListPipelinesByRef(ctx, projectID, ref, createdAfter)
		return pipelinesLoadedMsg{reqID: id, projectID: projectID, pipelines: pipelines, err: err}
	})
}

// pipelineByIDCmd fetches one specific pipeline by ID and reports it
// through the same pipelinesLoadedMsg as every other way of loading
// pipelines into the matrix — used to jump from a trigger job to its
// downstream pipeline (a fixed pipeline ID, not "every pipeline for a
// project" like pipelinesForProjectCmd).
func pipelineByIDCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		p, err := client.GetPipeline(ctx, projectID, pipelineID)
		return pipelinesLoadedMsg{reqID: id, projectID: projectID, pipelines: []api.Pipeline{p}, err: err}
	})
}

func pipelineDetailCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		p, err := client.GetPipeline(ctx, projectID, pipelineID)
		return pipelineDetailMsg{reqID: id, pipeline: p, err: err}
	})
}

func retryPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		p, err := client.RetryPipeline(ctx, projectID, pipelineID)
		return pipelineActionMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	})
}

func cancelPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		p, err := client.CancelPipeline(ctx, projectID, pipelineID)
		return pipelineActionMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	})
}

func jobsForPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		jobs, err := client.ListJobs(ctx, projectID, pipelineID)
		return jobsLoadedMsg{reqID: id, pipelineID: pipelineID, jobs: jobs, err: err}
	})
}

// digestLineCount is how many lines of a failed job's trace the digest
// keeps — the last few meaningful ones, which is where a build tool prints
// its diagnosis and where the shell prints its complaint.
const digestLineCount = 3

// jobDigestCmd reads one failed job's whole trace and keeps only its first
// error line (plus a little context) — the trace itself is discarded here
// rather than held in the model, since the digest is a summary and the log
// viewer already exists for reading the real thing.
func jobDigestCmd(ctx context.Context, client *api.Client, projectID, jobID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		trace, err := client.JobTrace(ctx, projectID, jobID)
		if err != nil {
			return jobDigestMsg{reqID: id, jobID: jobID, err: err}
		}
		return jobDigestMsg{
			reqID:  id,
			jobID:  jobID,
			lines:  components.FailureSummary(trace, digestLineCount),
			reason: components.TraceFailureReason(trace),
		}
	})
}

func retryJobCmd(ctx context.Context, client *api.Client, projectID, pipelineID, jobID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		err := client.RetryJob(ctx, projectID, jobID)
		return jobActionMsg{reqID: id, projectID: projectID, pipelineID: pipelineID, jobID: jobID, err: err}
	})
}

func cancelJobCmd(ctx context.Context, client *api.Client, projectID, pipelineID, jobID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		err := client.CancelJob(ctx, projectID, jobID)
		return jobActionMsg{reqID: id, projectID: projectID, pipelineID: pipelineID, jobID: jobID, err: err}
	})
}

func playJobCmd(ctx context.Context, client *api.Client, projectID, pipelineID, jobID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		err := client.PlayJob(ctx, projectID, jobID)
		return jobActionMsg{reqID: id, projectID: projectID, pipelineID: pipelineID, jobID: jobID, err: err}
	})
}

// startLogStreamCmd launches the trace-polling goroutine and hands its
// channel back via logStreamReadyMsg; Update then drives it with
// waitForLogChunkCmd (invariant #3). The producer outlives the Cmd, so it
// carries its own panic containment (safeGo) rather than safeCmd's.
func startLogStreamCmd(ctx context.Context, client *api.Client, projectID, jobID int, id reqID) tea.Cmd {
	return safeCmd(func() tea.Msg {
		ch := make(chan api.LogChunk, 16)
		safeGo(func() { client.StreamJobTrace(ctx, projectID, jobID, ch, 2*time.Second) })
		return logStreamReadyMsg{reqID: id, jobID: jobID, ch: ch}
	})
}

func waitForLogChunkCmd(ch <-chan api.LogChunk, id reqID, jobID int) tea.Cmd {
	return safeCmd(func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return logChunkMsg{reqID: id, jobID: jobID, chunk: api.LogChunk{Done: true}}
		}
		return logChunkMsg{reqID: id, jobID: jobID, chunk: chunk}
	})
}
