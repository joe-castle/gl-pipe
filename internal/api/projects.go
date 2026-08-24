package api

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/mod/semver"
)

// ListGroupProjects returns every project under the given group/namespace path,
// paging through the full result set.
func (c *Client) ListGroupProjects(ctx context.Context, groupPath string) ([]Project, error) {
	var out []Project
	opts := &gitlab.ListGroupProjectsOptions{
		ListOptions:      gitlab.ListOptions{PerPage: 100},
		IncludeSubGroups: gitlab.Ptr(true),
		Archived:         gitlab.Ptr(false),
	}
	for {
		projects, resp, err := c.gl.Groups.ListGroupProjects(groupPath, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("listing projects for group %q: %w", groupPath, err)
		}
		for _, p := range projects {
			out = append(out, toProject(p))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = int64(resp.NextPage)
	}
	return out, nil
}

func toProject(p *gitlab.Project) Project {
	return Project{
		ID:                int(p.ID),
		Name:              p.Name,
		PathWithNamespace: p.PathWithNamespace,
		WebURL:            p.WebURL,
		DefaultBranch:     p.DefaultBranch,
	}
}

// ListBranches returns every branch for a project.
func (c *Client) ListBranches(ctx context.Context, projectID int) ([]Ref, error) {
	branches, _, err := c.gl.Branches.ListBranches(projectID, &gitlab.ListBranchesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing branches for project %d: %w", projectID, err)
	}
	refs := make([]Ref, 0, len(branches))
	for _, b := range branches {
		sha := ""
		if b.Commit != nil {
			sha = b.Commit.ID
		}
		refs = append(refs, Ref{Name: b.Name, IsTag: false, CommitSHA: sha})
	}
	return refs, nil
}

// ListTags returns every tag for a project.
func (c *Client) ListTags(ctx context.Context, projectID int) ([]Ref, error) {
	tags, _, err := c.gl.Tags.ListTags(projectID, &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing tags for project %d: %w", projectID, err)
	}
	refs := make([]Ref, 0, len(tags))
	for _, t := range tags {
		sha := ""
		if t.Commit != nil {
			sha = t.Commit.ID
		}
		refs = append(refs, Ref{Name: t.Name, IsTag: true, CommitSHA: sha})
	}
	return refs, nil
}

// LatestSemVerTag returns the tag with the highest semantic version among
// refs, per the spec's "T" (lock to latest SemVer tag) shortcut. Tags that
// don't parse as valid SemVer (with or without a leading "v") are ignored.
// The second return value is false if no ref qualifies.
func LatestSemVerTag(refs []Ref) (Ref, bool) {
	var best Ref
	var bestVersion string
	found := false

	for _, r := range refs {
		if !r.IsTag {
			continue
		}
		v := normalizeSemVer(r.Name)
		if v == "" || !semver.IsValid(v) {
			continue
		}
		if !found || semver.Compare(v, bestVersion) > 0 {
			best = r
			bestVersion = v
			found = true
		}
	}
	return best, found
}

func normalizeSemVer(tag string) string {
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}
