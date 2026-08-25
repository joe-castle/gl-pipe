// Package api wraps the GitLab client-go SDK behind domain types so the UI
// layer never depends on gitlab.* structs directly.
package api

import "time"

// Project is a GitLab project (repository).
type Project struct {
	ID                int
	Name              string
	PathWithNamespace string
	WebURL            string
	DefaultBranch     string
}

// Group is a GitLab group/namespace, used by the group discovery picker to
// populate an instance's default_groups.
type Group struct {
	ID       int
	Name     string
	FullPath string
}

// Ref is a branch or tag.
type Ref struct {
	Name      string
	IsTag     bool
	CommitSHA string
	CreatedAt time.Time // tags: the tag's own creation date; branches: the tip commit's date
}

// PipelineStatus mirrors GitLab's pipeline/job status strings.
type PipelineStatus string

const (
	StatusCreated  PipelineStatus = "created"
	StatusWaiting  PipelineStatus = "waiting_for_resource"
	StatusPending  PipelineStatus = "pending"
	StatusRunning  PipelineStatus = "running"
	StatusSuccess  PipelineStatus = "success"
	StatusFailed   PipelineStatus = "failed"
	StatusCanceled PipelineStatus = "canceled"
	StatusSkipped  PipelineStatus = "skipped"
	StatusManual   PipelineStatus = "manual"
)

// Terminal reports whether a pipeline/job status is done changing on its
// own. Manual counts as terminal even though it's not "finished" — a
// manual job sits still until a person presses play again, so polling it
// would never see it move without that.
func (s PipelineStatus) Terminal() bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusCanceled, StatusSkipped, StatusManual:
		return true
	default:
		return false
	}
}

// Pipeline is a single CI/CD pipeline run on a project.
type Pipeline struct {
	ID        int
	ProjectID int
	IID       int
	Status    PipelineStatus
	Ref       string
	SHA       string
	WebURL    string
	User      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Duration  time.Duration
}

// Job is a single job within a pipeline.
type Job struct {
	ID         int
	ProjectID  int
	PipelineID int
	Name       string
	Stage      string
	Status     PipelineStatus
	RunnerTag  string
	RetryCount int
	WebURL     string
	Duration   time.Duration
}

// VariableType matches GitLab's pipeline variable types.
type VariableType string

const (
	VarTypeEnv  VariableType = "env_var"
	VarTypeFile VariableType = "file"
)

// Variable is a pipeline trigger variable.
type Variable struct {
	Key       string
	Value     string
	Type      VariableType
	Masked    bool
	Protected bool
}

// MergeRequest is a GitLab merge request, used to jump straight to its
// pipelines without needing to already know which project it's in.
type MergeRequest struct {
	ID           int
	IID          int
	ProjectID    int
	Title        string
	SourceBranch string
	TargetBranch string
	Author       string
	Draft        bool
	WebURL       string
	UpdatedAt    time.Time
}

// BlobHit is one match from a GitLab blob (code) search.
type BlobHit struct {
	ProjectID   int
	ProjectPath string
	Path        string
	Ref         string
	StartLine   int
	Snippet     string
}

// LogChunk is an incremental slice of a job's trace output.
type LogChunk struct {
	JobID   int
	Offset  int
	Content string
	Done    bool
	Err     error
}
