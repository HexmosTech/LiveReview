package github

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/livereview/internal/prsync"
	"github.com/livereview/internal/webhookutils"
)

// ConvertPullRequestStateEvent handles GitHub's bare "pull_request" event
// (opened, edited, closed, reopened, synchronize, ready_for_review,
// converted_to_draft, ...). This event is already delivered today by the
// webhooks LiveReview installs (see internal/jobqueue's GitHub webhook install
// payload, which subscribes to "pull_request"), but until now nothing
// consumed it: WebhookOrchestratorV2.convertToUnifiedEvent only tries
// ConvertCommentEvent then ConvertReviewerEvent, neither of which recognizes a
// bare pull_request event, so it was rejected with an HTTP 400.
//
// matched=false is returned (with no error) for any event this method isn't
// meant to handle, including actions review_requested/review_request_removed
// - those must keep flowing through the existing ConvertReviewerEvent path
// unchanged, since this method is checked earlier in the dispatch order.
func (p *GitHubV2Provider) ConvertPullRequestStateEvent(headers map[string]string, body []byte) (*prsync.PullRequestStateEvent, bool, error) {
	eventType, _ := webhookutils.GetHeaderCaseInsensitive(headers, "X-GitHub-Event")
	if eventType != "pull_request" {
		return nil, false, nil
	}

	var payload GitHubV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, fmt.Errorf("failed to parse GitHub pull_request webhook: %w", err)
	}

	switch payload.Action {
	case "review_requested", "review_request_removed":
		// Handled by ConvertReviewerEvent instead - not a state-sync event.
		return nil, false, nil
	}

	state := prsync.NormalizeGitHubState(payload.PullRequest.State, payload.PullRequest.Merged)

	return &prsync.PullRequestStateEvent{
		RepositoryProviderID: fmt.Sprintf("%d", payload.Repository.ID),
		RepositoryFullName:   payload.Repository.FullName,
		RepositoryWebURL:     payload.Repository.HTMLURL,

		Number:            payload.PullRequest.Number,
		ProviderPRID:      fmt.Sprintf("%d", payload.PullRequest.ID),
		Title:             payload.PullRequest.Title,
		Description:       payload.PullRequest.Body,
		State:             state,
		AuthorID:          fmt.Sprintf("%d", payload.PullRequest.User.ID),
		AuthorUsername:    payload.PullRequest.User.Login,
		AuthorName:        payload.PullRequest.User.Name,
		AuthorAvatarURL:   payload.PullRequest.User.AvatarURL,
		SourceBranch:      payload.PullRequest.Head.Ref,
		TargetBranch:      payload.PullRequest.Base.Ref,
		WebURL:            payload.PullRequest.HTMLURL,
		ProviderCreatedAt: parseGitHubWebhookTime(payload.PullRequest.CreatedAt),
		ProviderUpdatedAt: parseGitHubWebhookTime(payload.PullRequest.UpdatedAt),
		Metadata: map[string]interface{}{
			"action": payload.Action,
			"draft":  payload.PullRequest.Draft,
			"merged": payload.PullRequest.Merged,
		},
	}, true, nil
}

func parseGitHubWebhookTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
