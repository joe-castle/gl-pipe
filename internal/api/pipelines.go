package api

import (
	"context"
	"fmt"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// ListPipelines returns the most recent pipelines for a project, newest
// first, optionally excluding anything created before createdAfter (the
// zero time.Time means no cap — every pipeline the page-size limit below
// allows). Entries do not include the triggering user or duration; call
// GetPipeline to enrich a specific row once it's visible.
func (c *Client) ListPipelines(ctx context.Context, projectID int, createdAfter time.Time) ([]Pipeline, error) {
	opts := &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 50},
		OrderBy:     gitlab.Ptr("updated_at"),
		Sort:        gitlab.Ptr("desc"),
	}
	if !createdAfter.IsZero() {
		opts.CreatedAfter = gitlab.Ptr(createdAfter)
	}
	return c.listPipelines(ctx, projectID, opts)
}

// ListPipelinesByRef returns pipelines for a project matching an exact ref
// (branch or tag) name — GitLab's ref filter is an exact match, not a
// substring search. Used to search for pipelines across many projects by
// ref without first knowing which project they belong to. createdAfter is
// the same optional age cap as ListPipelines.
func (c *Client) ListPipelinesByRef(ctx context.Context, projectID int, ref string, createdAfter time.Time) ([]Pipeline, error) {
	opts := &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 20},
		Ref:         gitlab.Ptr(ref),
		OrderBy:     gitlab.Ptr("updated_at"),
		Sort:        gitlab.Ptr("desc"),
	}
	if !createdAfter.IsZero() {
		opts.CreatedAfter = gitlab.Ptr(createdAfter)
	}
	return c.listPipelines(ctx, projectID, opts)
}

func (c *Client) listPipelines(ctx context.Context, projectID int, opts *gitlab.ListProjectPipelinesOptions) ([]Pipeline, error) {
	infos, _, err := c.gl.Pipelines.ListProjectPipelines(projectID, opts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing pipelines for project %d: %w", projectID, err)
	}
	return toPipelines(infos), nil
}

func toPipelines(infos []*gitlab.PipelineInfo) []Pipeline {
	out := make([]Pipeline, 0, len(infos))
	for _, p := range infos {
		out = append(out, Pipeline{
			ID:        int(p.ID),
			ProjectID: int(p.ProjectID),
			IID:       int(p.IID),
			Status:    PipelineStatus(p.Status),
			Ref:       p.Ref,
			SHA:       p.SHA,
			WebURL:    p.WebURL,
			CreatedAt: timeOrZero(p.CreatedAt),
			UpdatedAt: timeOrZero(p.UpdatedAt),
		})
	}
	return out
}

// GetPipeline fetches full detail for one pipeline, including the triggering
// user and duration.
func (c *Client) GetPipeline(ctx context.Context, projectID, pipelineID int) (Pipeline, error) {
	p, _, err := c.gl.Pipelines.GetPipeline(projectID, int64(pipelineID), gitlab.WithContext(ctx))
	if err != nil {
		return Pipeline{}, fmt.Errorf("getting pipeline %d in project %d: %w", pipelineID, projectID, err)
	}
	return toPipeline(p), nil
}

func toPipeline(p *gitlab.Pipeline) Pipeline {
	user := ""
	if p.User != nil {
		user = p.User.Username
	}
	return Pipeline{
		ID:        int(p.ID),
		ProjectID: int(p.ProjectID),
		IID:       int(p.IID),
		Status:    PipelineStatus(p.Status),
		Ref:       p.Ref,
		SHA:       p.SHA,
		WebURL:    p.WebURL,
		User:      user,
		CreatedAt: timeOrZero(p.CreatedAt),
		UpdatedAt: timeOrZero(p.UpdatedAt),
		Duration:  time.Duration(p.Duration) * time.Second,
	}
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// CreatePipeline triggers a new pipeline on ref with the given variables.
func (c *Client) CreatePipeline(ctx context.Context, projectID int, ref string, vars []Variable) (Pipeline, error) {
	opts := &gitlab.CreatePipelineOptions{Ref: gitlab.Ptr(ref)}
	if len(vars) > 0 {
		pv := make([]*gitlab.PipelineVariableOptions, 0, len(vars))
		for _, v := range vars {
			vt := gitlab.EnvVariableType
			if v.Type == VarTypeFile {
				vt = gitlab.FileVariableType
			}
			pv = append(pv, &gitlab.PipelineVariableOptions{
				Key:          gitlab.Ptr(v.Key),
				Value:        gitlab.Ptr(v.Value),
				VariableType: gitlab.Ptr(vt),
			})
		}
		opts.Variables = &pv
	}

	p, _, err := c.gl.Pipelines.CreatePipeline(projectID, opts, gitlab.WithContext(ctx))
	if err != nil {
		return Pipeline{}, fmt.Errorf("creating pipeline in project %d on ref %q: %w", projectID, ref, err)
	}
	return toPipeline(p), nil
}

// RetryPipeline retries all failed jobs in a pipeline.
func (c *Client) RetryPipeline(ctx context.Context, projectID, pipelineID int) (Pipeline, error) {
	p, _, err := c.gl.Pipelines.RetryPipelineBuild(projectID, int64(pipelineID), gitlab.WithContext(ctx))
	if err != nil {
		return Pipeline{}, fmt.Errorf("retrying pipeline %d in project %d: %w", pipelineID, projectID, err)
	}
	return toPipeline(p), nil
}

// CancelPipeline cancels all running jobs in a pipeline.
func (c *Client) CancelPipeline(ctx context.Context, projectID, pipelineID int) (Pipeline, error) {
	p, _, err := c.gl.Pipelines.CancelPipelineBuild(projectID, int64(pipelineID), gitlab.WithContext(ctx))
	if err != nil {
		return Pipeline{}, fmt.Errorf("canceling pipeline %d in project %d: %w", pipelineID, projectID, err)
	}
	return toPipeline(p), nil
}

// ListJobs returns every job for a pipeline, with a client-computed retry
// count per (stage, name) group since the API does not report it directly.
// This includes pipeline trigger jobs (the `trigger:` keyword — e.g. a
// deploy job that kicks off a downstream deployment pipeline): GitLab
// reports those through a separate "bridges" endpoint rather than the
// regular jobs list, so they'd otherwise silently never appear here.
func (c *Client) ListJobs(ctx context.Context, projectID, pipelineID int) ([]Job, error) {
	jobs, _, err := c.gl.Jobs.ListPipelineJobs(projectID, int64(pipelineID), &gitlab.ListJobsOptions{
		ListOptions:    gitlab.ListOptions{PerPage: 100},
		IncludeRetried: gitlab.Ptr(true),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing jobs for pipeline %d in project %d: %w", pipelineID, projectID, err)
	}

	bridges, _, err := c.gl.Jobs.ListPipelineBridges(projectID, int64(pipelineID), &gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing trigger jobs for pipeline %d in project %d: %w", pipelineID, projectID, err)
	}

	out := make([]Job, 0, len(jobs)+len(bridges))
	counted := map[string]int{}
	for _, j := range jobs {
		key := j.Stage + "/" + j.Name
		retry := counted[key]
		counted[key]++
		out = append(out, Job{
			ID:         int(j.ID),
			ProjectID:  projectID,
			PipelineID: pipelineID,
			Name:       j.Name,
			Stage:      j.Stage,
			Status:     PipelineStatus(j.Status),
			RunnerTag:  j.Runner.Description,
			RetryCount: retry,
			WebURL:     j.WebURL,
			Duration:   time.Duration(j.Duration * float64(time.Second)),
		})
	}
	for _, b := range bridges {
		key := b.Stage + "/" + b.Name
		retry := counted[key]
		counted[key]++
		job := Job{
			ID:         int(b.ID),
			ProjectID:  projectID,
			PipelineID: pipelineID,
			Name:       b.Name,
			Stage:      b.Stage,
			Status:     PipelineStatus(b.Status),
			RetryCount: retry,
			WebURL:     b.WebURL,
			Duration:   time.Duration(b.Duration * float64(time.Second)),
			IsBridge:   true,
		}
		if b.DownstreamPipeline != nil {
			job.DownstreamProjectID = int(b.DownstreamPipeline.ProjectID)
			job.DownstreamPipelineID = int(b.DownstreamPipeline.ID)
			job.DownstreamPipelineIID = int(b.DownstreamPipeline.IID)
			job.DownstreamStatus = PipelineStatus(b.DownstreamPipeline.Status)
		}
		out = append(out, job)
	}
	return out, nil
}

// RetryJob retries a single job.
func (c *Client) RetryJob(ctx context.Context, projectID, jobID int) error {
	_, _, err := c.gl.Jobs.RetryJob(projectID, int64(jobID), gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("retrying job %d in project %d: %w", jobID, projectID, err)
	}
	return nil
}

// CancelJob cancels a single running job.
func (c *Client) CancelJob(ctx context.Context, projectID, jobID int) error {
	_, _, err := c.gl.Jobs.CancelJob(projectID, int64(jobID), gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("canceling job %d in project %d: %w", jobID, projectID, err)
	}
	return nil
}
