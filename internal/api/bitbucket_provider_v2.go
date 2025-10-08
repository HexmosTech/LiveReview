package api

import (
	"fmt"
	"log"
	"strings"
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

// BitbucketV2BotUserInfo represents bot user information for Bitbucket
type BitbucketV2BotUserInfo struct {
	Provider string      `json:"provider"`
	User     UnifiedUser `json:"user"`
	APIToken string      `json:"-"` // Never marshal to JSON
	BaseURL  string      `json:"base_url"`
}

// BitbucketV2UserInfo represents user information from Bitbucket API
type BitbucketV2UserInfo struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
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
	// TODO: Implement conversion from BitbucketV2WebhookPayload to UnifiedWebhookEventV2
	log.Printf("[DEBUG] BitbucketV2Provider.ConvertCommentEvent called")
	return nil, fmt.Errorf("ConvertCommentEvent not yet implemented for Bitbucket V2 provider")
}

// ConvertReviewerEvent converts a Bitbucket reviewer event to unified format
func (p *BitbucketV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	// TODO: Implement conversion from BitbucketV2WebhookPayload to UnifiedWebhookEventV2
	log.Printf("[DEBUG] BitbucketV2Provider.ConvertReviewerEvent called")
	return nil, fmt.Errorf("ConvertReviewerEvent not yet implemented for Bitbucket V2 provider")
}

// FetchMergeRequestData fetches additional data for the merge request
func (p *BitbucketV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	// TODO: Implement fetching Bitbucket PR data
	log.Printf("[DEBUG] BitbucketV2Provider.FetchMergeRequestData called")
	return fmt.Errorf("FetchMergeRequestData not yet implemented for Bitbucket V2 provider")
}

// PostCommentReply posts a reply to a comment
func (p *BitbucketV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	// TODO: Implement posting Bitbucket comment reply
	log.Printf("[DEBUG] BitbucketV2Provider.PostCommentReply called")
	return fmt.Errorf("PostCommentReply not yet implemented for Bitbucket V2 provider")
}

// PostEmojiReaction posts an emoji reaction
func (p *BitbucketV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	// TODO: Implement posting Bitbucket emoji reaction
	log.Printf("[DEBUG] BitbucketV2Provider.PostEmojiReaction called with emoji: %s", emoji)
	return fmt.Errorf("PostEmojiReaction not yet implemented for Bitbucket V2 provider")
}

// PostFullReview posts a full review
func (p *BitbucketV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	// TODO: Implement posting Bitbucket full review
	log.Printf("[DEBUG] BitbucketV2Provider.PostFullReview called")
	return fmt.Errorf("PostFullReview not yet implemented for Bitbucket V2 provider")
}

// ProcessWebhookEvent processes a webhook event (main entry point)
func (p *BitbucketV2Provider) ProcessWebhookEvent(payload interface{}) error {
	// TODO: Implement main webhook processing logic
	log.Printf("[DEBUG] BitbucketV2Provider.ProcessWebhookEvent called")
	return fmt.Errorf("ProcessWebhookEvent not yet implemented for Bitbucket V2 provider")
}

// Helper methods that mirror the existing monolithic functions

// processBitbucketReviewerChangeV2 processes reviewer change events
func (p *BitbucketV2Provider) processBitbucketReviewerChangeV2(payload BitbucketV2WebhookPayload, eventKey string) *BitbucketV2ReviewerChangeInfo {
	// TODO: Port processBitbucketReviewerChange logic
	log.Printf("[DEBUG] processBitbucketReviewerChangeV2 called with eventKey: %s", eventKey)
	return nil
}

// convertBitbucketToUnifiedCommentV2 converts Bitbucket comment to unified format
func (p *BitbucketV2Provider) convertBitbucketToUnifiedCommentV2(payload BitbucketV2WebhookPayload) UnifiedCommentV2 {
	// TODO: Port convertBitbucketToUnifiedComment logic
	// This will be implemented when UnifiedCommentV2 type is available
	log.Printf("[DEBUG] convertBitbucketToUnifiedCommentV2 called")
	return UnifiedCommentV2{} // TODO: Replace with actual implementation
}

// fetchBitbucketPRCommitsV2 fetches PR commits from Bitbucket API
func (p *BitbucketV2Provider) fetchBitbucketPRCommitsV2(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketV2CommitInfo, error) {
	// TODO: Port fetchBitbucketPRCommits logic
	log.Printf("[DEBUG] fetchBitbucketPRCommitsV2 called for %s/%s PR %s", workspace, repository, prNumber)
	return nil, fmt.Errorf("fetchBitbucketPRCommitsV2 not yet implemented")
}

// fetchBitbucketPRCommentsV2 fetches PR comments from Bitbucket API
func (p *BitbucketV2Provider) fetchBitbucketPRCommentsV2(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketV2CommentInfo, error) {
	// TODO: Port fetchBitbucketPRComments logic
	log.Printf("[DEBUG] fetchBitbucketPRCommentsV2 called for %s/%s PR %s", workspace, repository, prNumber)
	return nil, fmt.Errorf("fetchBitbucketPRCommentsV2 not yet implemented")
}

// postBitbucketCommentReplyV2 posts a comment reply via Bitbucket API
func (p *BitbucketV2Provider) postBitbucketCommentReplyV2(comment UnifiedCommentV2, token *IntegrationToken, replyText string) error {
	// TODO: Port postBitbucketCommentReply logic
	// This will be implemented when UnifiedCommentV2 type is available
	log.Printf("[DEBUG] postBitbucketCommentReplyV2 called")
	return fmt.Errorf("postBitbucketCommentReplyV2 not yet implemented")
}
