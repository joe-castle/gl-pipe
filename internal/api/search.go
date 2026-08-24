package api

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// BlobSearchInGroup runs a GitLab blob (code) search scoped to a group/namespace,
// per the spec's "filter all repos in group X containing Y" workflow.
func (c *Client) BlobSearchInGroup(ctx context.Context, groupPath, query string) ([]BlobHit, error) {
	var out []BlobHit
	opts := &gitlab.SearchOptions{ListOptions: gitlab.ListOptions{PerPage: 100}}
	for {
		blobs, resp, err := c.gl.Search.BlobsByGroup(groupPath, query, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("blob search in group %q for %q: %w", groupPath, query, err)
		}
		for _, b := range blobs {
			out = append(out, BlobHit{
				ProjectID: int(b.ProjectID),
				Path:      b.Path,
				Ref:       b.Ref,
				StartLine: int(b.Startline),
				Snippet:   b.Data,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = int64(resp.NextPage)
	}
	return out, nil
}
