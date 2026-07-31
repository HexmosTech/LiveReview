package gitlab

import (
	"context"
	"strconv"
	"time"

	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/prsync"
)

func gitlabListStateParam(state providers.PullRequestListState) string {
	switch state {
	case providers.PRListStateOpen:
		return "opened"
	case providers.PRListStateClosed:
		return "closed"
	default:
		return "all"
	}
}

// ListPullRequests fetches one page of merge requests for a project
// (GET /projects/:id/merge_requests), following GitLab's X-Next-Page response
// header for pagination via the existing GitLabHTTPClient.ListMergeRequests.
// cursor is a page number (empty/absent means page 1).
func ListPullRequests(ctx context.Context, baseURL, pat, projectID string, state providers.PullRequestListState, cursor string) (*providers.PullRequestPage, error) {
	page := 1
	if cursor != "" {
		if p, err := strconv.Atoi(cursor); err == nil {
			page = p
		}
	}

	client := NewHTTPClient(baseURL, pat)
	mrs, nextPage, err := client.ListMergeRequests(projectID, gitlabListStateParam(state), page, 100)
	if err != nil {
		return nil, err
	}

	result := &providers.PullRequestPage{NextCursor: nextPage}
	for _, mr := range mrs {
		createdAt, _ := time.Parse(time.RFC3339, mr.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, mr.UpdatedAt)
		result.PullRequests = append(result.PullRequests, prsync.PullRequestSummary{
			ProviderPRID:      strconv.Itoa(mr.ID),
			Number:            mr.IID,
			Title:             mr.Title,
			Description:       mr.Description,
			State:             prsync.NormalizeGitLabState(mr.State),
			AuthorID:          strconv.Itoa(mr.Author.ID),
			AuthorUsername:    mr.Author.Username,
			AuthorName:        mr.Author.Name,
			AuthorAvatarURL:   mr.Author.AvatarURL,
			SourceBranch:      mr.SourceBranch,
			TargetBranch:      mr.TargetBranch,
			WebURL:            mr.WebURL,
			ProviderCreatedAt: createdAt,
			ProviderUpdatedAt: updatedAt,
			Metadata:          map[string]interface{}{},
		})
	}

	return result, nil
}
