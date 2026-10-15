package gitea

import (
	"encoding/json"
	"fmt"

	"github.com/livereview/internal/prsync"
	"github.com/livereview/internal/webhookutils"
)

// ConvertPullRequestStateEvent handles Gitea's "pull_request" event so new PRs get the review links.
// Review-request actions are left to the existing reviewer flow.
func (p *GiteaV2Provider) ConvertPullRequestStateEvent(headers map[string]string, body []byte) (*prsync.PullRequestStateEvent, bool, error) {
	eventType, _ := webhookutils.GetHeaderCaseInsensitive(headers, "X-Gitea-Event")
	if eventType != "pull_request" {
		return nil, false, nil
	}

	var payload GiteaV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, fmt.Errorf("failed to parse Gitea pull_request webhook: %w", err)
	}
	switch payload.Action {
	case "review_requested", "review_request_removed":
		return nil, false, nil
	}
	pr, repo := payload.PullRequest, payload.Repository
	if pr == nil || repo == nil {
		return nil, true, fmt.Errorf("gitea pull_request webhook is missing pull_request or repository")
	}

	event := &prsync.PullRequestStateEvent{
		RepositoryProviderID: fmt.Sprintf("%d", repo.ID),
		RepositoryFullName:   repo.FullName,
		RepositoryWebURL:     repo.HTMLURL,

		Number:            int(pr.Number),
		ProviderPRID:      fmt.Sprintf("%d", pr.ID),
		Title:             pr.Title,
		Description:       pr.Body,
		State:             prsync.NormalizeGitHubState(pr.State, pr.Merged),
		WebURL:            pr.HTMLURL,
		ProviderCreatedAt: pr.CreatedAt,
		ProviderUpdatedAt: pr.UpdatedAt,
		Metadata:          map[string]interface{}{"action": payload.Action, "merged": pr.Merged},
	}
	if pr.User != nil {
		event.AuthorID = fmt.Sprintf("%d", pr.User.ID)
		event.AuthorUsername = pr.User.Login
		event.AuthorName = pr.User.FullName
		event.AuthorAvatarURL = pr.User.AvatarURL
	}
	if pr.Head != nil {
		event.SourceBranch = pr.Head.Ref
	}
	if pr.Base != nil {
		event.TargetBranch = pr.Base.Ref
	}
	return event, true, nil
}
