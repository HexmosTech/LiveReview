package api

import (
	providergitlab "github.com/livereview/internal/provider_input/gitlab"
)

// GitLabV2Provider is the relocated provider implementation.
type GitLabV2Provider = providergitlab.GitLabV2Provider

// NewGitLabV2Provider wires the new GitLab provider into the legacy server setup.
func NewGitLabV2Provider(server *Server) *GitLabV2Provider {
	return providergitlab.NewGitLabV2Provider(server.db)
}
