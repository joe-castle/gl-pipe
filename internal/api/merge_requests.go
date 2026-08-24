package api

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// ListMyMergeRequests returns every open merge request assigned to or
// authored by the authenticated user, across every project on the
// instance — deduplicated, since an MR can appear in both scopes. This is
// what makes "find pipelines for my MRs" possible without knowing (or
// staging) which project they live in first.
func (c *Client) ListMyMergeRequests(ctx context.Context) ([]MergeRequest, error) {
	seen := map[int]bool{}
	var out []MergeRequest
	for _, scope := range []string{"assigned_to_me", "created_by_me"} {
		opts := &gitlab.ListMergeRequestsOptions{
			ListOptions: gitlab.ListOptions{PerPage: 100},
			Scope:       gitlab.Ptr(scope),
			State:       gitlab.Ptr("opened"),
			OrderBy:     gitlab.Ptr("updated_at"),
			Sort:        gitlab.Ptr("desc"),
		}
		for {
			mrs, resp, err := c.gl.MergeRequests.ListMergeRequests(opts, gitlab.WithContext(ctx))
			if err != nil {
				return nil, fmt.Errorf("listing merge requests (%s): %w", scope, err)
			}
			for _, mr := range mrs {
				if id := int(mr.ID); !seen[id] {
					seen[id] = true
					out = append(out, toMergeRequest(mr))
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opts.Page = int64(resp.NextPage)
		}
	}
	return out, nil
}

// ListProjectMergeRequests returns every open merge request in a project.
func (c *Client) ListProjectMergeRequests(ctx context.Context, projectID int) ([]MergeRequest, error) {
	var out []MergeRequest
	opts := &gitlab.ListProjectMergeRequestsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
		State:       gitlab.Ptr("opened"),
		OrderBy:     gitlab.Ptr("updated_at"),
		Sort:        gitlab.Ptr("desc"),
	}
	for {
		mrs, resp, err := c.gl.MergeRequests.ListProjectMergeRequests(projectID, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("listing merge requests for project %d: %w", projectID, err)
		}
		for _, mr := range mrs {
			out = append(out, toMergeRequest(mr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = int64(resp.NextPage)
	}
	return out, nil
}

func toMergeRequest(mr *gitlab.BasicMergeRequest) MergeRequest {
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return MergeRequest{
		ID:           int(mr.ID),
		IID:          int(mr.IID),
		ProjectID:    int(mr.ProjectID),
		Title:        mr.Title,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Author:       author,
		Draft:        mr.Draft,
		WebURL:       mr.WebURL,
		UpdatedAt:    timeOrZero(mr.UpdatedAt),
	}
}

// ListMergeRequestPipelines returns the pipelines associated with a merge
// request (its latest pipeline per commit, most recent first) — this is
// the bridge from "here's an MR" to the existing pipeline matrix.
func (c *Client) ListMergeRequestPipelines(ctx context.Context, projectID, mrIID int) ([]Pipeline, error) {
	infos, _, err := c.gl.MergeRequests.ListMergeRequestPipelines(projectID, int64(mrIID), gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing pipelines for merge request !%d in project %d: %w", mrIID, projectID, err)
	}
	return toPipelines(infos), nil
}
