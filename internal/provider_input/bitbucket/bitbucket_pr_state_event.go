package bitbucket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/prsync"
	"github.com/livereview/internal/webhookutils"
)

// bitbucketStateActions maps the PR lifecycle event keys this handles to the action name the sync worker expects.
// "pullrequest:updated" is left out on purpose: it drives the reviewer-assignment flow.
var bitbucketStateActions = map[string]string{
	"pullrequest:created":   "opened",
	"pullrequest:fulfilled": "merged",
	"pullrequest:rejected":  "declined",
}

// ConvertPullRequestStateEvent handles Bitbucket PR created/merged/declined events so new PRs get the review links.
func (p *BitbucketV2Provider) ConvertPullRequestStateEvent(headers map[string]string, body []byte) (*prsync.PullRequestStateEvent, bool, error) {
	eventKey, _ := webhookutils.GetHeaderCaseInsensitive(headers, "X-Event-Key")
	action, ok := bitbucketStateActions[eventKey]
	if !ok {
		return nil, false, nil
	}

	var payload BitbucketV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, fmt.Errorf("failed to parse Bitbucket %s webhook: %w", eventKey, err)
	}
	pr, repo := payload.PullRequest, payload.Repository

	username := pr.Author.Username
	if username == "" {
		username = pr.Author.DisplayName
	}
	return &prsync.PullRequestStateEvent{
		RepositoryProviderID: repo.UUID,
		RepositoryFullName:   repo.FullName,
		RepositoryWebURL:     repo.Links.HTML.Href,

		Number:            pr.ID,
		ProviderPRID:      fmt.Sprintf("%d", pr.ID),
		Title:             pr.Title,
		Description:       pr.Description,
		State:             normalizeBitbucketState(pr.State),
		AuthorID:          pr.Author.AccountID,
		AuthorUsername:    username,
		AuthorName:        pr.Author.DisplayName,
		SourceBranch:      pr.Source.Name,
		TargetBranch:      pr.Destination.Name,
		WebURL:            pr.Links.HTML.Href,
		ProviderCreatedAt: parseBitbucketTime(pr.CreatedOn),
		ProviderUpdatedAt: parseBitbucketTime(pr.UpdatedOn),
		Metadata:          map[string]interface{}{"action": action},
	}, true, nil
}

func normalizeBitbucketState(state string) string {
	switch strings.ToUpper(state) {
	case "MERGED":
		return "merged"
	case "DECLINED", "SUPERSEDED":
		return "closed"
	default:
		return "open"
	}
}

func parseBitbucketTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
