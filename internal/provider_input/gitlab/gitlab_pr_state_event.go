package gitlab

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/livereview/internal/prsync"
)

// ConvertPullRequestStateEvent handles GitLab's "merge_request" object_kind
// webhook (already delivered today by the webhooks LiveReview installs - see
// internal/jobqueue's GitLab webhook install payload, which subscribes to
// "merge_requests"). Until now nothing meaningfully consumed it:
// WebhookOrchestratorV2.convertToUnifiedEvent tries ConvertCommentEvent first,
// which unconditionally unmarshals any payload into a note-shaped struct and
// always succeeds (even for a merge_request payload, just with an empty
// comment body), so the event silently became a no-op via
// handleCommentReplyFlow's empty-body skip - ConvertReviewerEvent was never
// reached in practice.
//
// matched=false is returned (with no error) for any object_kind other than
// "merge_request", so comment ("note") events are untouched.
func (p *GitLabV2Provider) ConvertPullRequestStateEvent(headers map[string]string, body []byte) (*prsync.PullRequestStateEvent, bool, error) {
	var probe struct {
		ObjectKind string `json:"object_kind"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, false, nil
	}
	if probe.ObjectKind != "merge_request" {
		return nil, false, nil
	}

	var payload GitLabV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, fmt.Errorf("failed to parse GitLab merge_request webhook: %w", err)
	}

	attrs := payload.ObjectAttributes
	state := prsync.NormalizeGitLabState(attrs.State)

	return &prsync.PullRequestStateEvent{
		RepositoryProviderID: fmt.Sprintf("%d", payload.Project.ID),
		RepositoryFullName:   payload.Project.PathWithNamespace,
		RepositoryWebURL:     payload.Project.WebURL,

		Number:       attrs.IID,
		ProviderPRID: fmt.Sprintf("%d", attrs.ID),
		Title:        attrs.Title,
		Description:  attrs.Description,
		State:        state,
		// GitLab's merge_request object_attributes only carries a numeric
		// author_id, not a full author profile - AuthorUsername/Name/AvatarURL
		// are left blank here and filled in shortly after by the periodic
		// reconciliation poll, which calls the GitLab API's full MR list.
		AuthorID:          fmt.Sprintf("%d", attrs.AuthorID),
		SourceBranch:      attrs.SourceBranch,
		TargetBranch:      attrs.TargetBranch,
		WebURL:            attrs.URL,
		ProviderCreatedAt: parseGitLabWebhookTime(attrs.CreatedAt),
		ProviderUpdatedAt: parseGitLabWebhookTime(attrs.UpdatedAt),
		Metadata: map[string]interface{}{
			"action":       attrs.Action,
			"merge_status": attrs.MergeStatus,
		},
	}, true, nil
}

func parseGitLabWebhookTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// GitLab sends merge_request timestamps as "2026-01-02 15:04:05 UTC".
	if t, err := time.Parse("2006-01-02 15:04:05 MST", s); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
