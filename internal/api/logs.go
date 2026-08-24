package api

import (
	"context"
	"fmt"
	"io"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

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
