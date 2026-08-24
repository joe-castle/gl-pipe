package api

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Client wraps the GitLab SDK client and exposes only the operations
// gl-pipe needs, returning domain types instead of gitlab.* structs.
type Client struct {
	gl      *gitlab.Client
	baseURL string
}

// NewClient constructs a Client for the given instance URL and PAT.
func NewClient(baseURL, token string) (*Client, error) {
	gl, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("constructing gitlab client: %w", err)
	}
	return &Client{gl: gl, baseURL: baseURL}, nil
}

// Validate confirms the instance URL and token are usable by calling
// GET /user, per the spec's first-run credential check.
func (c *Client) Validate(ctx context.Context) (username string, err error) {
	user, _, err := c.gl.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("validating credentials: %w", err)
	}
	return user.Username, nil
}
