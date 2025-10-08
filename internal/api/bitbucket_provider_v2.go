package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Bitbucket V2 Types - All Bitbucket-specific types with V2 naming to avoid conflicts

// BitbucketV2WebhookPayload represents a Bitbucket webhook payload
type BitbucketV2WebhookPayload struct {
	EventKey    string                 `json:"eventKey"`
	Date        string                 `json:"date"`
	Actor       BitbucketV2User        `json:"actor"`
	Repository  BitbucketV2Repository  `json:"repository"`
	Changes     BitbucketV2Changes     `json:"changes,omitempty"`
	PullRequest BitbucketV2PullRequest `json:"pullrequest,omitempty"`
	Comment     BitbucketV2Comment     `json:"comment,omitempty"` // For comment events
}

// BitbucketV2Comment represents a Bitbucket comment
type BitbucketV2Comment struct {
	ID        int                       `json:"id"`
	Content   BitbucketV2CommentContent `json:"content"`
	User      BitbucketV2User           `json:"user"`
	CreatedOn string                    `json:"created_on"`
	UpdatedOn string                    `json:"updated_on"`
	Parent    *BitbucketV2CommentRef    `json:"parent,omitempty"`
	Inline    *BitbucketV2InlineInfo    `json:"inline,omitempty"`
	Links     BitbucketV2CommentLinks   `json:"links"`
	Type      string                    `json:"type"`
}

// BitbucketV2CommentContent represents the content of a Bitbucket comment
type BitbucketV2CommentContent struct {
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
	Type   string `json:"type"`
}

// BitbucketV2CommentRef represents a reference to another comment
type BitbucketV2CommentRef struct {
	ID    int                     `json:"id"`
	Links BitbucketV2CommentLinks `json:"links"`
}

// BitbucketV2InlineInfo represents inline comment positioning
type BitbucketV2InlineInfo struct {
	Path string `json:"path"`
	From int    `json:"from,omitempty"`
	To   int    `json:"to,omitempty"`
}

// BitbucketV2CommentLinks represents links for a comment
type BitbucketV2CommentLinks struct {
	Self struct {
		Href string `json:"href"`
	} `json:"self"`
	HTML struct {
		Href string `json:"href"`
	} `json:"html"`
}

// BitbucketV2User represents a Bitbucket user
type BitbucketV2User struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
}

// BitbucketV2Repository represents a Bitbucket repository
type BitbucketV2Repository struct {
	UUID     string             `json:"uuid"`
	Name     string             `json:"name"`
	FullName string             `json:"full_name"`
	Links    BitbucketV2Links   `json:"links"`
	Project  BitbucketV2Project `json:"project,omitempty"`
	Owner    BitbucketV2User    `json:"owner"`
	Type     string             `json:"type"`
}

// BitbucketV2Project represents a Bitbucket project
type BitbucketV2Project struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Key  string `json:"key"`
	Type string `json:"type"`
}

// BitbucketV2Links represents Bitbucket links
type BitbucketV2Links struct {
	HTML struct {
		Href string `json:"href"`
	} `json:"html"`
}

// BitbucketV2PullRequest represents a Bitbucket pull request
type BitbucketV2PullRequest struct {
	ID           int                      `json:"id"`
	Title        string                   `json:"title"`
	Description  string                   `json:"description,omitempty"`
	State        string                   `json:"state"`
	Source       BitbucketV2Branch        `json:"source"`
	Destination  BitbucketV2Branch        `json:"destination"`
	Author       BitbucketV2User          `json:"author"`
	Reviewers    []BitbucketV2Reviewer    `json:"reviewers"`
	Participants []BitbucketV2Participant `json:"participants"`
	Links        BitbucketV2Links         `json:"links"`
	CreatedOn    string                   `json:"created_on"`
	UpdatedOn    string                   `json:"updated_on"`
}

// BitbucketV2Branch represents a Bitbucket branch
type BitbucketV2Branch struct {
	Name   string                `json:"name"`
	Commit BitbucketV2BranchInfo `json:"commit"`
}

// BitbucketV2BranchInfo represents branch commit information
type BitbucketV2BranchInfo struct {
	Hash string `json:"hash"`
}

// BitbucketV2Reviewer represents a Bitbucket reviewer
type BitbucketV2Reviewer struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
	Approved    bool   `json:"approved"`
}

// BitbucketV2Participant represents a Bitbucket participant
type BitbucketV2Participant struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
	Role        string `json:"role"`
	Approved    bool   `json:"approved"`
}

// BitbucketV2Changes represents changes in the webhook payload
type BitbucketV2Changes struct {
	Reviewers *BitbucketV2ReviewerChanges `json:"reviewers,omitempty"`
}

// BitbucketV2ReviewerChanges represents reviewer changes
type BitbucketV2ReviewerChanges struct {
	Added   []BitbucketV2ReviewerChangeInfo `json:"added,omitempty"`
	Removed []BitbucketV2ReviewerChangeInfo `json:"removed,omitempty"`
}

// BitbucketV2ReviewerChangeInfo represents detailed reviewer change information
type BitbucketV2ReviewerChangeInfo struct {
	UUID                string                 `json:"uuid"`
	Username            string                 `json:"username"`
	DisplayName         string                 `json:"display_name"`
	AccountID           string                 `json:"account_id"`
	Type                string                 `json:"type"`
	PullRequest         BitbucketV2PullRequest `json:"pullrequest"`
	Repository          BitbucketV2Repository  `json:"repository"`
	IntegrationTokenPtr *IntegrationToken      `json:"-"` // Internal use only
}

// BitbucketV2CommitInfo represents commit information from Bitbucket API
type BitbucketV2CommitInfo struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Date    string `json:"date"`
	Author  struct {
		User BitbucketV2User `json:"user"`
	} `json:"author"`
}

// BitbucketV2CommentInfo represents comment information from Bitbucket API
type BitbucketV2CommentInfo struct {
	ID        int                       `json:"id"`
	Content   BitbucketV2CommentContent `json:"content"`
	User      BitbucketV2User           `json:"user"`
	CreatedOn string                    `json:"created_on"`
	UpdatedOn string                    `json:"updated_on"`
}

// BitbucketV2Provider implements the WebhookProviderV2 interface for Bitbucket
type BitbucketV2Provider struct {
	server *Server
}

// NewBitbucketV2Provider creates a new Bitbucket V2 provider
func NewBitbucketV2Provider(server *Server) *BitbucketV2Provider {
	return &BitbucketV2Provider{
		server: server,
	}
}

// ProviderName returns the name of this provider
func (p *BitbucketV2Provider) ProviderName() string {
	return "bitbucket"
}

// CanHandleWebhook determines if this provider can handle the webhook
func (p *BitbucketV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	// Check for Bitbucket-specific headers
	if eventKey, exists := headers["X-Event-Key"]; exists {
		// Common Bitbucket event keys
		bitbucketEvents := []string{
			"pullrequest:comment_created", "pullrequest:comment_updated", "pullrequest:comment_deleted",
			"pullrequest:approved", "pullrequest:unapproved", "pullrequest:rejected",
			"pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected",
			"repo:push", "repo:commit_comment_created",
		}

		for _, event := range bitbucketEvents {
			if strings.Contains(eventKey, event) {
				log.Printf("[DEBUG] Bitbucket provider can handle event: %s", eventKey)
				return true
			}
		}
	}

	// Check User-Agent for Bitbucket
	if userAgent, exists := headers["User-Agent"]; exists {
		if strings.Contains(strings.ToLower(userAgent), "bitbucket") {
			log.Printf("[DEBUG] Bitbucket provider detected via User-Agent: %s", userAgent)
			return true
		}
	}

	// Check for Bitbucket-specific request headers
	bitbucketHeaders := []string{"X-Request-UUID", "X-Hook-UUID"}
	for _, header := range bitbucketHeaders {
		if _, exists := headers[header]; exists {
			log.Printf("[DEBUG] Bitbucket provider detected via header: %s", header)
			return true
		}
	}

	return false
}

// ConvertCommentEvent converts a Bitbucket comment event to unified format
func (p *BitbucketV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	log.Printf("[DEBUG] BitbucketV2Provider.ConvertCommentEvent called")

	// Parse the Bitbucket webhook payload
	var payload BitbucketV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bitbucket webhook payload: %w", err)
	}

	// Convert to unified format
	unifiedComment := p.convertBitbucketToUnifiedCommentV2(payload)

	// Create unified webhook event
	event := &UnifiedWebhookEventV2{
		Provider:   "bitbucket",
		EventType:  "comment_created",
		Comment:    &unifiedComment,
		Repository: p.convertRepositoryV2(payload.Repository),
		Actor:      p.convertUserV2(payload.Actor),
		Timestamp:  payload.Date,
	}

	log.Printf("[DEBUG] Converted Bitbucket comment event: ID=%s, Author=%s", event.Comment.ID, event.Comment.Author.Username)
	return event, nil
}

// ConvertReviewerEvent converts a Bitbucket reviewer event to unified format
func (p *BitbucketV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	log.Printf("[DEBUG] BitbucketV2Provider.ConvertReviewerEvent called")

	// Parse the Bitbucket webhook payload
	var payload BitbucketV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bitbucket webhook payload: %w", err)
	}

	// Create unified webhook event for reviewer changes
	event := &UnifiedWebhookEventV2{
		Provider:     "bitbucket",
		EventType:    "reviewer_assigned",
		MergeRequest: p.convertPullRequestV2(payload.PullRequest),
		Repository:   p.convertRepositoryV2(payload.Repository),
		Actor:        p.convertUserV2(payload.Actor),
		Timestamp:    payload.Date,
	}

	// Add reviewer change information if available
	if payload.Changes.Reviewers != nil {
		event.ReviewerChange = p.convertReviewerChangeV2(*payload.Changes.Reviewers)
	}

	log.Printf("[DEBUG] Converted Bitbucket reviewer event: PR=%d", payload.PullRequest.ID)
	return event, nil
}

// FetchMergeRequestData fetches additional data for the merge request
func (p *BitbucketV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	log.Printf("[DEBUG] BitbucketV2Provider.FetchMergeRequestData called")

	// Extract information from event
	var workspace, repository, prNumber string

	if event.Comment != nil {
		workspace, _ = event.Comment.Metadata["workspace"].(string)
		repository, _ = event.Comment.Metadata["repository"].(string)
		if prNum, ok := event.Comment.Metadata["pr_number"].(int); ok {
			prNumber = fmt.Sprintf("%d", prNum)
		}
	} else if event.MergeRequest != nil {
		// Extract from repository full name if available
		parts := strings.Split(event.Repository.FullName, "/")
		if len(parts) == 2 {
			workspace = parts[0]
			repository = parts[1]
		}
		prNumber = event.MergeRequest.ID
	}

	if workspace == "" || repository == "" || prNumber == "" {
		return fmt.Errorf("insufficient data to fetch PR information")
	}

	// Find integration token
	token, err := p.findIntegrationTokenForBitbucketRepoV2(fmt.Sprintf("%s/%s", workspace, repository))
	if err != nil {
		return fmt.Errorf("failed to find integration token: %w", err)
	}

	// Fetch PR timeline data (commits and comments)
	commits, err := p.fetchBitbucketPRCommitsV2(workspace, repository, prNumber, token)
	if err != nil {
		log.Printf("[WARN] Failed to fetch Bitbucket PR commits: %v", err)
	}

	comments, err := p.fetchBitbucketPRCommentsV2(workspace, repository, prNumber, token)
	if err != nil {
		log.Printf("[WARN] Failed to fetch Bitbucket PR comments: %v", err)
	}

	// Store the fetched data in metadata for later use
	if event.Comment != nil {
		if event.Comment.Metadata == nil {
			event.Comment.Metadata = make(map[string]interface{})
		}
		event.Comment.Metadata["commits"] = commits
		event.Comment.Metadata["pr_comments"] = comments
	}

	log.Printf("[DEBUG] Fetched %d commits and %d comments for Bitbucket PR %s", len(commits), len(comments), prNumber)
	return nil
}

// PostCommentReply posts a reply to a comment
func (p *BitbucketV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	log.Printf("[DEBUG] BitbucketV2Provider.PostCommentReply called")

	if event.Comment == nil {
		return fmt.Errorf("comment event is nil")
	}

	// Extract metadata for API call
	workspace, _ := event.Comment.Metadata["workspace"].(string)
	repository, _ := event.Comment.Metadata["repository"].(string)
	prNumber, _ := event.Comment.Metadata["pr_number"].(int)

	if workspace == "" || repository == "" || prNumber == 0 {
		return fmt.Errorf("missing required metadata: workspace=%s, repository=%s, pr_number=%d", workspace, repository, prNumber)
	}

	// Find integration token
	token, err := p.findIntegrationTokenForBitbucketRepoV2(fmt.Sprintf("%s/%s", workspace, repository))
	if err != nil {
		return fmt.Errorf("failed to find integration token: %w", err)
	}

	return p.postBitbucketCommentReplyV2(workspace, repository, fmt.Sprintf("%d", prNumber), event.Comment.InReplyToID, content, token)
}

// PostEmojiReaction posts an emoji reaction
func (p *BitbucketV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	log.Printf("[DEBUG] BitbucketV2Provider.PostEmojiReaction called with emoji: %s", emoji)
	// Bitbucket doesn't have direct emoji reactions like GitHub, so we'll post a short comment
	return p.PostCommentReply(event, emoji)
}

// PostFullReview posts a full review
func (p *BitbucketV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	log.Printf("[DEBUG] BitbucketV2Provider.PostFullReview called")
	// For Bitbucket, we'll post the review as a top-level comment
	// TODO: In future, this could create multiple inline comments if event contains detailed review data
	return p.PostCommentReply(event, overallComment)
}

// Helper methods for V2 conversions

// convertUserV2 converts Bitbucket user to unified format
func (p *BitbucketV2Provider) convertUserV2(user BitbucketV2User) UnifiedUserV2 {
	return UnifiedUserV2{
		ID:       user.AccountID,
		Username: user.Username,
		Name:     user.DisplayName,
		WebURL:   "", // Not available in basic payload
		Metadata: map[string]interface{}{
			"uuid": user.UUID,
			"type": user.Type,
		},
	}
}

// convertRepositoryV2 converts Bitbucket repository to unified format
func (p *BitbucketV2Provider) convertRepositoryV2(repo BitbucketV2Repository) UnifiedRepositoryV2 {
	return UnifiedRepositoryV2{
		ID:       repo.UUID,
		Name:     repo.Name,
		FullName: repo.FullName,
		WebURL:   repo.Links.HTML.Href,
		Owner:    p.convertUserV2(repo.Owner),
		Metadata: map[string]interface{}{
			"type": repo.Type,
		},
	}
}

// convertPullRequestV2 converts Bitbucket PR to unified format
func (p *BitbucketV2Provider) convertPullRequestV2(pr BitbucketV2PullRequest) *UnifiedMergeRequestV2 {
	reviewers := make([]UnifiedUserV2, len(pr.Reviewers))
	for i, reviewer := range pr.Reviewers {
		reviewers[i] = UnifiedUserV2{
			ID:       reviewer.AccountID,
			Username: reviewer.Username,
			Name:     reviewer.DisplayName,
			Metadata: map[string]interface{}{
				"approved": reviewer.Approved,
			},
		}
	}

	return &UnifiedMergeRequestV2{
		ID:           fmt.Sprintf("%d", pr.ID),
		Number:       pr.ID,
		Title:        pr.Title,
		Description:  pr.Description,
		State:        pr.State,
		Author:       p.convertUserV2(pr.Author),
		SourceBranch: pr.Source.Name,
		TargetBranch: pr.Destination.Name,
		WebURL:       pr.Links.HTML.Href,
		CreatedAt:    pr.CreatedOn,
		UpdatedAt:    pr.UpdatedOn,
		Reviewers:    reviewers,
		Metadata: map[string]interface{}{
			"source_commit": pr.Source.Commit.Hash,
			"dest_commit":   pr.Destination.Commit.Hash,
		},
	}
}

// convertReviewerChangeV2 converts Bitbucket reviewer changes to unified format
func (p *BitbucketV2Provider) convertReviewerChangeV2(changes BitbucketV2ReviewerChanges) *UnifiedReviewerChangeV2 {
	added := make([]UnifiedUserV2, len(changes.Added))
	for i, reviewer := range changes.Added {
		added[i] = UnifiedUserV2{
			ID:       reviewer.AccountID,
			Username: reviewer.Username,
			Name:     reviewer.DisplayName,
		}
	}

	removed := make([]UnifiedUserV2, len(changes.Removed))
	for i, reviewer := range changes.Removed {
		removed[i] = UnifiedUserV2{
			ID:       reviewer.AccountID,
			Username: reviewer.Username,
			Name:     reviewer.DisplayName,
		}
	}

	return &UnifiedReviewerChangeV2{
		Action:            "modified",
		CurrentReviewers:  added,
		PreviousReviewers: removed,
		ChangedBy:         UnifiedUserV2{}, // Will be filled by caller
	}
}

// convertBitbucketToUnifiedCommentV2 converts Bitbucket comment to unified format
func (p *BitbucketV2Provider) convertBitbucketToUnifiedCommentV2(payload BitbucketV2WebhookPayload) UnifiedCommentV2 {
	log.Printf("[DEBUG] convertBitbucketToUnifiedCommentV2 called")

	// Determine if this is an inline comment
	var position *UnifiedPositionV2
	if payload.Comment.Inline != nil {
		position = &UnifiedPositionV2{
			FilePath:   payload.Comment.Inline.Path,
			LineNumber: payload.Comment.Inline.To,
			LineType:   "new", // Default for Bitbucket inline comments
		}
	}

	// Extract parent comment ID if this is a reply
	var inReplyToID *string
	if payload.Comment.Parent != nil {
		parentIDStr := fmt.Sprintf("%d", payload.Comment.Parent.ID)
		inReplyToID = &parentIDStr
	}

	return UnifiedCommentV2{
		ID:          fmt.Sprintf("%d", payload.Comment.ID),
		Body:        payload.Comment.Content.Raw,
		Author:      p.convertUserV2(payload.Comment.User),
		CreatedAt:   payload.Comment.CreatedOn,
		UpdatedAt:   payload.Comment.UpdatedOn,
		WebURL:      payload.Comment.Links.HTML.Href,
		InReplyToID: inReplyToID,
		Position:    position,
		Metadata: map[string]interface{}{
			"provider":     "bitbucket",
			"workspace":    payload.Repository.Owner.Username,
			"repository":   payload.Repository.Name,
			"pr_number":    payload.PullRequest.ID,
			"comment_type": payload.Comment.Type,
			"pr_title":     payload.PullRequest.Title,
			"pr_url":       payload.PullRequest.Links.HTML.Href,
		},
	}
}

// findIntegrationTokenForBitbucketRepoV2 finds the integration token for a Bitbucket repository
func (p *BitbucketV2Provider) findIntegrationTokenForBitbucketRepoV2(repoFullName string) (*IntegrationToken, error) {
	log.Printf("[DEBUG] Looking for integration token for Bitbucket repo: %s", repoFullName)

	// Use the server's existing method for finding integration tokens
	token, err := p.server.findIntegrationTokenForBitbucketRepo(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to find integration token for repo %s: %w", repoFullName, err)
	}

	return token, nil
}

// postBitbucketCommentReplyV2 posts a comment reply via Bitbucket API
func (p *BitbucketV2Provider) postBitbucketCommentReplyV2(workspace, repository, prNumber string, inReplyToID *string, replyText string, token *IntegrationToken) error {
	log.Printf("[DEBUG] postBitbucketCommentReplyV2 called for %s/%s PR %s", workspace, repository, prNumber)

	// Bitbucket API endpoint for posting comments
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments",
		workspace, repository, prNumber)

	// Prepare payload
	payload := map[string]interface{}{
		"content": map[string]interface{}{
			"raw": replyText,
		},
	}

	// If this is a reply, include parent reference
	if inReplyToID != nil && *inReplyToID != "" {
		if parentID, err := strconv.Atoi(*inReplyToID); err == nil {
			payload["parent"] = map[string]interface{}{
				"id": parentID,
			}
		} else {
			log.Printf("[WARN] Bitbucket reply: invalid parent InReplyToID '%s' (not an int): %v; posting as top-level comment",
				*inReplyToID, err)
		}
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	log.Printf("[DEBUG] Bitbucket API call: %s", apiURL)
	log.Printf("[DEBUG] Bitbucket API payload: %s", string(jsonData))

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Bitbucket API: use Basic auth with email + API token
	var bbEmail string
	if token.Metadata != nil {
		if e, ok := token.Metadata["email"].(string); ok {
			bbEmail = e
		}
	}
	if bbEmail == "" {
		return fmt.Errorf("bitbucket email missing in integration token metadata; cannot authenticate")
	}

	log.Printf("[DEBUG] Bitbucket auth details - Using Basic (email), email: '%s', PatToken length: %d",
		bbEmail, len(token.PatToken))

	req.SetBasicAuth(bbEmail, token.PatToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[INFO] Successfully posted Bitbucket comment reply")
	return nil
}

// fetchBitbucketPRCommitsV2 fetches PR commits from Bitbucket API
func (p *BitbucketV2Provider) fetchBitbucketPRCommitsV2(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketV2CommitInfo, error) {
	log.Printf("[DEBUG] fetchBitbucketPRCommitsV2 called for %s/%s PR %s", workspace, repository, prNumber)

	// Use the server's existing method
	commits, err := p.server.fetchBitbucketPRCommits(workspace, repository, prNumber, token)
	if err != nil {
		return nil, err
	}

	// Convert to V2 format
	v2Commits := make([]BitbucketV2CommitInfo, len(commits))
	for i, commit := range commits {
		v2Commits[i] = BitbucketV2CommitInfo{
			Hash:    commit.Hash,
			Message: commit.Message,
			Date:    "", // Not available in simplified BitbucketCommitInfo
			Author: struct {
				User BitbucketV2User `json:"user"`
			}{
				User: BitbucketV2User{
					Username:    "unknown", // Not available in simplified structure
					DisplayName: "unknown",
					AccountID:   "unknown",
					UUID:        "unknown",
					Type:        "user",
				},
			},
		}
	}

	return v2Commits, nil
}

// fetchBitbucketPRCommentsV2 fetches PR comments from Bitbucket API
func (p *BitbucketV2Provider) fetchBitbucketPRCommentsV2(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketV2CommentInfo, error) {
	log.Printf("[DEBUG] fetchBitbucketPRCommentsV2 called for %s/%s PR %s", workspace, repository, prNumber)

	// Use the server's existing method
	comments, err := p.server.fetchBitbucketPRComments(workspace, repository, prNumber, token)
	if err != nil {
		return nil, err
	}

	// Convert to V2 format
	v2Comments := make([]BitbucketV2CommentInfo, len(comments))
	for i, comment := range comments {
		v2Comments[i] = BitbucketV2CommentInfo{
			ID: 0, // Not available in simplified BitbucketCommentInfo
			Content: BitbucketV2CommentContent{
				Raw:    comment.Content, // Content is a string in simplified structure
				Markup: "",
				HTML:   "",
				Type:   "text/plain",
			},
			User: BitbucketV2User{
				Username:    comment.Author, // Author is a string in simplified structure
				DisplayName: comment.Author,
				AccountID:   "unknown",
				UUID:        "unknown",
				Type:        "user",
			},
			CreatedOn: "", // Not available in simplified structure
			UpdatedOn: "",
		}
	}

	return v2Comments, nil
}
