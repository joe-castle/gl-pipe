package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
	"github.com/joeca/gl-pipe/internal/cache"
	"github.com/joeca/gl-pipe/internal/config"
)

func validateCredentialsCmd(ctx context.Context, url, token string, id reqID) tea.Cmd {
	return func() tea.Msg {
		client, err := api.NewClient(url, token)
		if err != nil {
			return credentialsValidatedMsg{reqID: id, err: err}
		}
		username, err := client.Validate(ctx)
		return credentialsValidatedMsg{reqID: id, username: username, err: err}
	}
}

func saveConfigCmd(cfg *config.Config, path string) tea.Cmd {
	return func() tea.Msg {
		return configSavedMsg{err: cfg.Save(path)}
	}
}

func saveCacheCmd(idx *cache.Index, path string) tea.Cmd {
	return func() tea.Msg {
		_ = idx.Save(path)
		return nil
	}
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
	return func() tea.Msg {
		if len(groups) == 0 {
			return projectsSyncedMsg{reqID: id, instance: instance, err: nil}
		}
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
	}
}

func blobSearchCmd(ctx context.Context, client *api.Client, group, query string, id reqID) tea.Cmd {
	return func() tea.Msg {
		hits, err := client.BlobSearchInGroup(ctx, group, query)
		return blobSearchResultsMsg{reqID: id, hits: hits, err: err}
	}
}

func loadRefsCmd(ctx context.Context, client *api.Client, projectID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		tags, err := client.ListTags(ctx, projectID)
		return refsLoadedMsg{reqID: id, projectID: projectID, refs: tags, err: err}
	}
}

func createPipelineCmd(ctx context.Context, client *api.Client, projectID int, ref string, vars []api.Variable, id reqID) tea.Cmd {
	return func() tea.Msg {
		p, err := client.CreatePipeline(ctx, projectID, ref, vars)
		return pipelineTriggeredMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	}
}

func pipelinesForProjectCmd(ctx context.Context, client *api.Client, projectID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		pipelines, err := client.ListPipelines(ctx, projectID)
		return pipelinesLoadedMsg{reqID: id, projectID: projectID, pipelines: pipelines, err: err}
	}
}

func pipelineDetailCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		p, err := client.GetPipeline(ctx, projectID, pipelineID)
		return pipelineDetailMsg{reqID: id, pipeline: p, err: err}
	}
}

func retryPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		p, err := client.RetryPipeline(ctx, projectID, pipelineID)
		return pipelineActionMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	}
}

func cancelPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		p, err := client.CancelPipeline(ctx, projectID, pipelineID)
		return pipelineActionMsg{reqID: id, projectID: projectID, pipeline: p, err: err}
	}
}

func jobsForPipelineCmd(ctx context.Context, client *api.Client, projectID, pipelineID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		jobs, err := client.ListJobs(ctx, projectID, pipelineID)
		return jobsLoadedMsg{reqID: id, pipelineID: pipelineID, jobs: jobs, err: err}
	}
}

func retryJobCmd(ctx context.Context, client *api.Client, projectID, jobID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		err := client.RetryJob(ctx, projectID, jobID)
		return jobActionMsg{reqID: id, projectID: projectID, jobID: jobID, err: err}
	}
}

func cancelJobCmd(ctx context.Context, client *api.Client, projectID, jobID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		err := client.CancelJob(ctx, projectID, jobID)
		return jobActionMsg{reqID: id, projectID: projectID, jobID: jobID, err: err}
	}
}

// startLogStreamCmd launches the trace-polling goroutine and hands its
// channel back via logStreamReadyMsg; Update then drives it with
// waitForLogChunkCmd (invariant #3).
func startLogStreamCmd(ctx context.Context, client *api.Client, projectID, jobID int, id reqID) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan api.LogChunk, 16)
		go client.StreamJobTrace(ctx, projectID, jobID, ch, 2*time.Second)
		return logStreamReadyMsg{reqID: id, jobID: jobID, ch: ch}
	}
}

func waitForLogChunkCmd(ch <-chan api.LogChunk, id reqID, jobID int) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return logChunkMsg{reqID: id, jobID: jobID, chunk: api.LogChunk{Done: true}}
		}
		return logChunkMsg{reqID: id, jobID: jobID, chunk: chunk}
	}
}
