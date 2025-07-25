package github

import "github.com/livereview/internal/providers"

func New(config GitHubConfig) (providers.Provider, error) {
	p := NewGitHubProvider(config.Token)
	p.Configure(map[string]interface{}{"pat_token": config.Token})
	return p, nil
}

type GitHubConfig struct {
	Token string
}
