package azuredevops

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// threadIDPattern extracts the numeric thread id from a comment's `_links`
// hrefs, e.g. ".../pullRequests/12/threads/34" or ".../threads/34/comments/5".
// Azure DevOps does not expose threadId as a plain field on the comment resource.
var threadIDPattern = regexp.MustCompile(`/threads/(\d+)`)

// mentionTokenPattern matches Azure DevOps's raw @-mention token, e.g.
// "@<017b0bb4-cf70-6633-85a4-2298b5bae8d1>" - confirmed against a live
// captured payload. Azure DevOps never renders mentions as plain "@username"
// text in the stored content (the UI resolves the GUID to a display name
// client-side), unlike every other provider. Left verbatim in the LLM
// prompt, this opaque token visibly degrades response quality.
var mentionTokenPattern = regexp.MustCompile(`(?i)@<[0-9a-f-]+>`)

// stripMentionTokens removes Azure DevOps @<GUID> mention tokens from
// comment content, for use anywhere the text is shown to a human or an LLM
// (prompt building, display). Mention *detection* must use the raw content
// instead (stashed in Comment.Metadata["raw_content"]), since this strips
// the very token it looks for.
func stripMentionTokens(content string) string {
	return strings.TrimSpace(mentionTokenPattern.ReplaceAllString(content, ""))
}

// trailingIDPattern extracts a trailing path segment, used to recover the
// repository GUID and PR number from _links hrefs (neither is a plain field
// on the comment-event resource).
var trailingIDPattern = regexp.MustCompile(`/([^/]+)$`)

// ConvertAzureDevOpsPullRequestEvent converts a git.pullrequest.created or
// git.pullrequest.updated webhook payload to unified format.
func ConvertAzureDevOpsPullRequestEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload AzureWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps webhook envelope: %w", err)
	}

	var resource AzurePullRequestResource
	if err := json.Unmarshal(payload.Resource, &resource); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps pull request resource: %w", err)
	}

	eventType := "mr_updated"
	if payload.EventType == "git.pullrequest.created" {
		eventType = "mr_updated"
	}

	event := &UnifiedWebhookEventV2{
		EventType:    eventType,
		Provider:     "azuredevops",
		Timestamp:    payload.CreatedDate,
		MergeRequest: convertAzurePullRequestToUnified(&resource),
		Repository:   convertAzureRepositoryToUnified(&resource.Repository),
		Actor:        convertAzureIdentityToUnified(&resource.CreatedBy),
	}

	return event, nil
}

// ConvertAzureDevOpsCommentEvent converts a ms.vss-code.git-pullrequest-comment-event
// webhook payload to unified format.
//
// Unlike the created/updated PR events, this payload carries no
// project/repo/PR *names* at all - only a repository GUID and a PR-number
// link, both recovered here via _links hrefs. Repository.FullName is left
// empty; AzureDevOpsV2Provider.FetchMergeRequestData resolves the real names
// via one API call (using org_url/repo_id stashed in Metadata below) once it
// has looked up the connector's PAT.
func ConvertAzureDevOpsCommentEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload AzureWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps webhook envelope: %w", err)
	}

	var resource AzureCommentEventResource
	if err := json.Unmarshal(payload.Resource, &resource); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps comment resource: %w", err)
	}

	if strings.TrimSpace(resource.Content) == "" {
		return nil, fmt.Errorf("comment event ignored (empty content, likely a system/deleted comment)")
	}

	prNumber, ok := extractTrailingID(linksHref(resource.Links, "pullRequests"))
	if !ok {
		return nil, fmt.Errorf("could not determine pull request number from comment event links")
	}
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid pull request number %q: %w", prNumber, err)
	}

	repoID, ok := extractTrailingID(linksHref(resource.Links, "repository"))
	if !ok {
		return nil, fmt.Errorf("could not determine repository id from comment event links")
	}

	orgURL := ""
	if payload.ResourceContainers != nil {
		orgURL = strings.TrimRight(firstNonEmptyString(payload.ResourceContainers.Collection.BaseURL, payload.ResourceContainers.Account.BaseURL), "/")
	}

	comment := convertAzureCommentToUnified(&resource)

	event := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "azuredevops",
		Timestamp: resource.PublishedDate,
		Comment:   comment,
		MergeRequest: &UnifiedMergeRequestV2{
			ID:     strconv.Itoa(prNumberInt),
			Number: prNumberInt,
			Metadata: map[string]any{
				"repo_id": repoID,
				"org_url": orgURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID: repoID,
			Metadata: map[string]any{
				"repo_id": repoID,
				"org_url": orgURL,
			},
		},
		Actor: convertAzureIdentityToUnified(&resource.Author),
	}

	return event, nil
}

// linksHref extracts the href for a named link from an AzureCommentEventLinks,
// keyed by field for the 4 links we care about.
func linksHref(links *AzureCommentEventLinks, name string) string {
	if links == nil {
		return ""
	}
	switch name {
	case "pullRequests":
		if links.PullRequests != nil {
			return links.PullRequests.Href
		}
	case "repository":
		if links.Repository != nil {
			return links.Repository.Href
		}
	}
	return ""
}

// extractTrailingID returns the last path segment of an href, e.g.
// ".../git/pullRequests/1" -> "1" or ".../repositories/{guid}" -> "{guid}".
func extractTrailingID(href string) (string, bool) {
	if href == "" {
		return "", false
	}
	if m := trailingIDPattern.FindStringSubmatch(href); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func convertAzurePullRequestToUnified(pr *AzurePullRequestResource) *UnifiedMergeRequestV2 {
	if pr == nil {
		return nil
	}

	unified := &UnifiedMergeRequestV2{
		ID:           strconv.FormatInt(pr.PullRequestID, 10),
		Number:       int(pr.PullRequestID),
		Title:        pr.Title,
		Description:  pr.Description,
		State:        pr.Status,
		SourceBranch: strings.TrimPrefix(pr.SourceRefName, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(pr.TargetRefName, "refs/heads/"),
		WebURL:       pr.URL,
		CreatedAt:    pr.CreationDate,
		Author:       convertAzureIdentityToUnified(&pr.CreatedBy),
		Metadata: map[string]any{
			"head_sha":     pr.LastMergeSourceCommit.CommitID,
			"base_sha":     pr.LastMergeTargetCommit.CommitID,
			"project_name": pr.Repository.Project.Name,
			"repo_name":    pr.Repository.Name,
		},
	}

	return unified
}

func convertAzureRepositoryToUnified(repo *AzureRepository) UnifiedRepositoryV2 {
	if repo == nil {
		return UnifiedRepositoryV2{}
	}

	return UnifiedRepositoryV2{
		ID:       repo.ID,
		Name:     repo.Name,
		FullName: fmt.Sprintf("%s/%s", repo.Project.Name, repo.Name),
		WebURL:   repo.URL,
		CloneURL: repo.RemoteURL,
		Metadata: map[string]any{
			"project_id":   repo.Project.ID,
			"project_name": repo.Project.Name,
		},
	}
}

func convertAzureIdentityToUnified(id *AzureIdentity) UnifiedUserV2 {
	if id == nil {
		return UnifiedUserV2{}
	}

	return UnifiedUserV2{
		ID:        id.ID,
		Username:  id.UniqueName,
		Name:      id.DisplayName,
		AvatarURL: id.ImageURL,
		Metadata:  map[string]any{},
	}
}

func convertAzureCommentToUnified(comment *AzureCommentEventResource) *UnifiedCommentV2 {
	if comment == nil {
		return nil
	}

	unified := &UnifiedCommentV2{
		ID:        strconv.FormatInt(comment.ID, 10),
		Body:      stripMentionTokens(comment.Content),
		Author:    convertAzureIdentityToUnified(&comment.Author),
		CreatedAt: comment.PublishedDate,
		UpdatedAt: comment.LastUpdatedDate,
		Metadata: map[string]any{
			"comment_type": "thread_comment",
			// Raw, unstripped content - mention detection needs the @<GUID>
			// token stripMentionTokens just removed from Body.
			"raw_content": comment.Content,
		},
	}

	if comment.ParentCommentID > 0 {
		inReplyTo := strconv.FormatInt(comment.ParentCommentID, 10)
		unified.InReplyToID = &inReplyTo
	}

	if threadID, ok := extractThreadID(comment.Links); ok {
		unified.Metadata["thread_id"] = threadID
		threadIDStr := strconv.Itoa(threadID)
		unified.DiscussionID = &threadIDStr
	}

	return unified
}

// extractThreadID recovers the numeric thread id from a comment's hypermedia
// links, since Azure DevOps does not expose it as a plain field.
func extractThreadID(links *AzureCommentEventLinks) (int, bool) {
	if links == nil {
		return 0, false
	}
	for _, href := range []*AzureHref{links.Threads, links.Self} {
		if href == nil || href.Href == "" {
			continue
		}
		if m := threadIDPattern.FindStringSubmatch(href.Href); len(m) == 2 {
			if id, err := strconv.Atoi(m[1]); err == nil {
				return id, true
			}
		}
	}
	return 0, false
}
