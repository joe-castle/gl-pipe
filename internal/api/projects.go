package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/mod/semver"
)

// ListGroups returns every group/namespace the authenticated user is a
// member of, for the group discovery picker (<Space> g).
func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	var out []Group
	opts := &gitlab.ListGroupsOptions{ListOptions: gitlab.ListOptions{PerPage: 100}}
	for {
		groups, resp, err := c.gl.Groups.ListGroups(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("listing groups: %w", err)
		}
		for _, g := range groups {
			out = append(out, Group{ID: int(g.ID), Name: g.Name, FullPath: g.FullPath})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = int64(resp.NextPage)
	}
	return out, nil
}

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
		var createdAt time.Time
		if b.Commit != nil {
			sha = b.Commit.ID
			createdAt = timeOrZero(b.Commit.CreatedAt)
		}
		refs = append(refs, Ref{Name: b.Name, IsTag: false, CommitSHA: sha, CreatedAt: createdAt})
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
		refs = append(refs, Ref{Name: t.Name, IsTag: true, CommitSHA: sha, CreatedAt: timeOrZero(t.CreatedAt)})
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

// LatestCreatedTag returns the most recently created tag among refs,
// regardless of whether its name parses as SemVer — an alternative to
// LatestSemVerTag for repos that don't tag with version numbers (date
// stamps, build numbers, etc.). The second return value is false if no
// tag has a known creation date.
func LatestCreatedTag(refs []Ref) (Ref, bool) {
	var best Ref
	found := false

	for _, r := range refs {
		if !r.IsTag || r.CreatedAt.IsZero() {
			continue
		}
		if !found || r.CreatedAt.After(best.CreatedAt) {
			best = r
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
