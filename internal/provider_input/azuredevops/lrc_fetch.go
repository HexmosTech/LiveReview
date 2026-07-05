package azuredevops

import (
	"context"
	"fmt"

	azuredevopsutils "github.com/livereview/internal/providers/azuredevops"
)

// GetRepoConfigFiles fetches the .lrc/ directory from an Azure DevOps
// repository at the given ref. Implements lrcfetch.Provider for the
// webhook/comment-reply path - resolves the connector's token/org URL, then
// delegates to the same fetch logic used by the one-shot review path.
//
// repoFullName is "{project}/{repo}" (event.Repository.FullName).
func (p *AzureDevOpsV2Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	token, orgURL, err := FindIntegrationTokenForAzureDevOpsRepo(p.db, repoFullName)
	if err != nil {
		return nil, false, fmt.Errorf("azure devops lrc: no token for %s: %w", repoFullName, err)
	}

	provider, err := azuredevopsutils.NewProvider(azuredevopsutils.Config{BaseURL: orgURL, Token: token.PatToken})
	if err != nil {
		return nil, false, fmt.Errorf("azure devops lrc: failed to construct provider: %w", err)
	}

	return provider.GetRepoConfigFiles(ctx, repoFullName, ref)
}
