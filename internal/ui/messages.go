package ui

import (
	"time"

	"github.com/joeca/gl-pipe/internal/api"
)

// reqID is a monotonic tag carried on every async response so Update can
// drop replies to requests that have since been superseded (invariant #2
// in the project plan: stale responses never clobber fresh state).
type reqID int64

// errMsg wraps a failed async operation for display in the status line.
type errMsg struct {
	reqID reqID
	err   error
}

// credentialsValidatedMsg reports the result of the wizard's live PAT check.
type credentialsValidatedMsg struct {
	reqID    reqID
	username string
	err      error
}

// configSavedMsg reports the result of persisting config to disk.
type configSavedMsg struct {
	err error
}

// projectsSyncedMsg carries a freshly-fetched project list for one instance.
type projectsSyncedMsg struct {
	reqID    reqID
	instance string
	projects []api.Project
	err      error
}

// groupsLoadedMsg carries the accessible group list for the discovery
// picker (<Space> g).
type groupsLoadedMsg struct {
	reqID  reqID
	groups []api.Group
	err    error
}

// myMRsLoadedMsg carries the result of a global "my MRs" fetch (<Space> m)
// — a single request, so it replaces rather than accumulates.
type myMRsLoadedMsg struct {
	reqID reqID
	mrs   []api.MergeRequest
	err   error
}

// projectMRsLoadedMsg carries one project's MRs from a batch fetch across
// staged projects (M on the explorer) — merged in, mirroring
// pipelinesLoadedMsg's accumulate-within-a-batch handling.
type projectMRsLoadedMsg struct {
	reqID     reqID
	projectID int
	mrs       []api.MergeRequest
	err       error
}

// refsLoadedMsg carries branches+tags for the ref picker.
type refsLoadedMsg struct {
	reqID     reqID
	projectID int
	refs      []api.Ref
	err       error
}

// refPickerLoadedMsg carries branches+tags for the ref picker inside the
// pipeline trigger modal — distinct from refsLoadedMsg (tags only, for the
// "lock to latest tag" flow on the explorer).
type refPickerLoadedMsg struct {
	reqID     reqID
	projectID int
	refs      []api.Ref
	err       error
}

// blobSearchResultsMsg carries blob search hits.
type blobSearchResultsMsg struct {
	reqID reqID
	hits  []api.BlobHit
	err   error
}

// pipelineTriggeredMsg reports the outcome of triggering one pipeline as
// part of a (possibly multi-repo) batch dispatch.
type pipelineTriggeredMsg struct {
	reqID     reqID
	projectID int
	pipeline  api.Pipeline
	err       error
}

// pipelinesLoadedMsg carries the pipeline list for one project.
type pipelinesLoadedMsg struct {
	reqID     reqID
	projectID int
	pipelines []api.Pipeline
	err       error
}

// pipelineDetailMsg enriches one pipeline row with author + duration.
type pipelineDetailMsg struct {
	reqID    reqID
	pipeline api.Pipeline
	err      error
}

// pipelineActionMsg reports the outcome of a retry/cancel on a pipeline.
type pipelineActionMsg struct {
	reqID     reqID
	projectID int
	pipeline  api.Pipeline
	err       error
}

// jobsLoadedMsg carries the job matrix for one pipeline.
type jobsLoadedMsg struct {
	reqID      reqID
	pipelineID int
	jobs       []api.Job
	err        error
}

// jobActionMsg reports the outcome of a retry/cancel on a single job.
type jobActionMsg struct {
	reqID      reqID
	projectID  int
	pipelineID int
	jobID      int
	err        error
}

// logStreamReadyMsg carries the channel a freshly-started log-streaming
// goroutine will publish chunks on. Update stores it and issues the first
// waitForLogChunkCmd; every subsequent chunk re-issues that same Cmd
// (invariant #3: log streaming is a channel + self-rescheduling Cmd).
type logStreamReadyMsg struct {
	reqID reqID
	jobID int
	ch    <-chan api.LogChunk
}

// logChunkMsg carries one incremental slice of job trace output.
type logChunkMsg struct {
	reqID reqID
	jobID int
	chunk api.LogChunk
}

// tickMsg drives the spinner and periodic background refresh.
type tickMsg time.Time
