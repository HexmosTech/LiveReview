package providers

import (
	"strings"
)

// Use shared.go for Provider, MergeRequestDetails, DiffRefs

// Detect if a URL is a GitHub PR
func IsGitHubPRURL(url string) bool {
	return strings.HasPrefix(url, "https://github.com/") && strings.Contains(url, "/pull/")
}

// ProviderFactory implementation (partial)
type StandardProviderFactory struct{}

// Implementation now in factories.go

func (f *StandardProviderFactory) SupportsProvider(providerType string) bool {
	return providerType == "gitlab" || providerType == "github"
}
