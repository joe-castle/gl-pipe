package api

import (
	"context"
	"fmt"
	"io"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// JobTrace fetches a job's complete trace in one shot. StreamJobTrace is
// the wrong tool for a caller that just wants the text once — it's a
// polling producer that keeps running until the job finishes — so the
// failure digest, which reads many jobs' traces and keeps only a line from
// each, uses this instead.
func (c *Client) JobTrace(ctx context.Context, projectID, jobID int) (string, error) {
	reader, _, err := c.gl.Jobs.GetTraceFile(projectID, int64(jobID), gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("fetching trace for job %d in project %d: %w", jobID, projectID, err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading trace for job %d in project %d: %w", jobID, projectID, err)
	}
	return string(data), nil
}

// StreamJobTrace polls a job's trace endpoint (GitLab has no incremental
// trace API — each poll returns the full trace so far) and pushes only the
// newly-appended suffix on chunks, computing the diff client-side. It runs
// until the job reaches a terminal status or ctx is canceled, then closes
// chunks. This is meant to be launched in its own goroutine by a tea.Cmd;
// callers receive chunks via a self-rescheduling Cmd, never touching UI
// state directly from here.
func (c *Client) StreamJobTrace(ctx context.Context, projectID, jobID int, chunks chan<- LogChunk, pollInterval time.Duration) {
	defer close(chunks)
	offset := 0

	for {
		reader, _, err := c.gl.Jobs.GetTraceFile(projectID, int64(jobID), gitlab.WithContext(ctx))
		if err != nil {
			select {
			case chunks <- LogChunk{JobID: jobID, Err: fmt.Errorf("fetching trace for job %d: %w", jobID, err)}:
			case <-ctx.Done():
			}
			return
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			select {
			case chunks <- LogChunk{JobID: jobID, Err: fmt.Errorf("reading trace for job %d: %w", jobID, err)}:
			case <-ctx.Done():
			}
			return
		}

		if len(data) > offset {
			chunk := LogChunk{JobID: jobID, Offset: offset, Content: string(data[offset:])}
			offset = len(data)
			select {
			case chunks <- chunk:
			case <-ctx.Done():
				return
			}
		}

		job, _, err := c.gl.Jobs.GetJob(projectID, int64(jobID), gitlab.WithContext(ctx))
		if err == nil && isTerminalStatus(PipelineStatus(job.Status)) {
			select {
			case chunks <- LogChunk{JobID: jobID, Offset: offset, Done: true}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

func isTerminalStatus(s PipelineStatus) bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusCanceled, StatusSkipped:
		return true
	default:
		return false
	}
}
