package azuredevops

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/livereview/internal/prsync"
)

// ConvertPullRequestStateEvent handles git.pullrequest.created/updated so new PRs get the review links.
func (p *AzureDevOpsV2Provider) ConvertPullRequestStateEvent(headers map[string]string, body []byte) (*prsync.PullRequestStateEvent, bool, error) {
	var payload AzureWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, nil
	}
	action := map[string]string{"git.pullrequest.created": "opened", "git.pullrequest.updated": "updated"}[payload.EventType]
	if action == "" {
		return nil, false, nil
	}

	var pr AzurePullRequestResource
	if err := json.Unmarshal(payload.Resource, &pr); err != nil {
		return nil, true, fmt.Errorf("failed to parse Azure DevOps pull request resource: %w", err)
	}
	repoWebURL := azureRepoWebURL(pr.Repository.RemoteURL)

	return &prsync.PullRequestStateEvent{
		RepositoryProviderID: pr.Repository.ID,
		RepositoryFullName:   pr.Repository.Project.Name + "/" + pr.Repository.Name,
		RepositoryWebURL:     repoWebURL,

		Number:            int(pr.PullRequestID),
		ProviderPRID:      fmt.Sprintf("%d", pr.PullRequestID),
		Title:             pr.Title,
		Description:       pr.Description,
		State:             normalizeAzureState(pr.Status),
		AuthorID:          pr.CreatedBy.ID,
		AuthorUsername:    pr.CreatedBy.UniqueName,
		AuthorName:        pr.CreatedBy.DisplayName,
		AuthorAvatarURL:   pr.CreatedBy.ImageURL,
		SourceBranch:      strings.TrimPrefix(pr.SourceRefName, "refs/heads/"),
		TargetBranch:      strings.TrimPrefix(pr.TargetRefName, "refs/heads/"),
		WebURL:            fmt.Sprintf("%s/pullrequest/%d", repoWebURL, pr.PullRequestID),
		ProviderCreatedAt: parseAzureTime(pr.CreationDate),
		ProviderUpdatedAt: parseAzureTime(payload.CreatedDate),
		Metadata:          map[string]interface{}{"action": action},
	}, true, nil
}

// azureRepoWebURL drops the "user@" part Azure puts in remoteUrl, giving the repo's browser URL.
func azureRepoWebURL(remoteURL string) string {
	u, err := neturl.Parse(remoteURL)
	if err != nil {
		return remoteURL
	}
	u.User = nil
	return strings.TrimSuffix(u.String(), "/")
}

func normalizeAzureState(status string) string {
	switch status {
	case "completed":
		return "merged"
	case "abandoned":
		return "closed"
	default:
		return "open"
	}
}

func parseAzureTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
