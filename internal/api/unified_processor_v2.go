package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/livereview/internal/aiconnectors"
	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/livereview/internal/learnings"
	bitbucketmentions "github.com/livereview/internal/providers/bitbucket"
	githubmentions "github.com/livereview/internal/providers/github"
	gitlabmentions "github.com/livereview/internal/providers/gitlab"
)

// Phase 7.1: Unified processor for provider-agnostic LLM processing
// Extracted from webhook_handler.go - provider-independent AI response generation

// UnifiedProcessorV2Impl implements the UnifiedProcessorV2 interface
type UnifiedProcessorV2Impl struct {
	server *Server // For accessing database operations and AI infrastructure
}

type bitbucketParentComment struct {
	User struct {
		Username  string `json:"username"`
		AccountID string `json:"account_id"`
		UUID      string `json:"uuid"`
	} `json:"user"`
}

// NewUnifiedProcessorV2 creates a new unified processor instance
func NewUnifiedProcessorV2(server *Server) UnifiedProcessorV2 {
	return &UnifiedProcessorV2Impl{
		server: server,
	}
}

func normalizeIdentifier(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "{}")
	return strings.ToLower(trimmed)
}

// CheckResponseWarrant determines if an event warrants an AI response (provider-agnostic)
// Extracted from checkAIResponseWarrant, checkUnifiedAIResponseWarrant
func (p *UnifiedProcessorV2Impl) CheckResponseWarrant(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, ResponseScenarioV2) {
	hardFailure := func(reason, missing string) (bool, ResponseScenarioV2) {
		metadata := map[string]interface{}{
			"provider": event.Provider,
		}
		if missing != "" {
			metadata["missing"] = missing
		}
		log.Printf("[ERROR] Response warrant precondition failed: %s", reason)
		return false, ResponseScenarioV2{
			Type:       "hard_failure",
			Reason:     reason,
			Confidence: 0.0,
			Metadata:   metadata,
		}
	}

	if event.Comment == nil {
		return hardFailure("comment payload absent in webhook event", "event.comment")
	}

	commentBody := strings.TrimSpace(event.Comment.Body)
	if commentBody == "" {
		return hardFailure("comment body empty; cannot evaluate warrant", "event.comment.body")
	}

	if botInfo == nil {
		return hardFailure("bot user info unavailable for warrant evaluation", "bot_info")
	}
	if strings.TrimSpace(botInfo.Username) == "" && strings.TrimSpace(botInfo.UserID) == "" {
		return hardFailure("bot user info missing identifiers", "bot_info.identifiers")
	}

	log.Printf("[DEBUG] Checking AI response warrant for comment by %s", event.Comment.Author.Username)
	log.Printf("[DEBUG] Comment content: %s", commentBody)
	log.Printf("[DEBUG] Bot info available: Username=%s, UserID=%s, Name=%s, Metadata=%v", botInfo.Username, botInfo.UserID, botInfo.Name, botInfo.Metadata)

	if event.Comment.InReplyToID != nil {
		log.Printf("[DEBUG] Comment is a reply, InReplyToID=%s", *event.Comment.InReplyToID)
	} else {
		log.Printf("[DEBUG] Comment is not a reply (InReplyToID is nil)")
	}

	if event.Comment.DiscussionID != nil && *event.Comment.DiscussionID != "" {
		log.Printf("[DEBUG] Comment discussion ID: %s", *event.Comment.DiscussionID)
	}

	contentType := p.classifyContentTypeV2(commentBody)
	responseType := p.determineResponseTypeV2(commentBody)

	makeMetadata := func() map[string]interface{} {
		return map[string]interface{}{
			"content_type":  contentType,
			"response_type": responseType,
		}
	}

	if p.isCommentAuthoredByBot(event, botInfo) {
		log.Printf("[DEBUG] Comment authored by registered bot user, skipping")
		metadata := makeMetadata()
		metadata["reason"] = "authored_by_bot"
		return false, ResponseScenarioV2{
			Type:       "no_response",
			Reason:     "comment authored by bot",
			Confidence: 0.0,
			Metadata:   metadata,
		}
	}

	// Replies take precedence to prevent missed follow-ups.
	isReply := event.Comment.InReplyToID != nil && *event.Comment.InReplyToID != ""
	if isReply {
		replyToBot, err := p.isReplyToBotComment(event, botInfo)
		if err != nil {
			log.Printf("[WARN] Failed to verify reply parent for provider %s: %v", event.Provider, err)
		}
		if replyToBot {
			metadata := makeMetadata()
			if event.Comment.InReplyToID != nil {
				metadata["in_reply_to"] = *event.Comment.InReplyToID
			}
			log.Printf("[DEBUG] Reply to AI bot comment detected")
			return true, ResponseScenarioV2{
				Type:       "bot_reply",
				Reason:     fmt.Sprintf("Reply to bot comment by %s", event.Comment.Author.Username),
				Confidence: 0.90,
				Metadata:   metadata,
			}
		}
	}

	isDirectMention := p.checkDirectBotMentionV2(event, botInfo)
	if isDirectMention {
		log.Printf("[DEBUG] Direct bot mention detected in comment")
		metadata := makeMetadata()
		return true, ResponseScenarioV2{
			Type:       "direct_mention",
			Reason:     fmt.Sprintf("Direct mention of bot by %s", event.Comment.Author.Username),
			Confidence: 0.95,
			Metadata:   metadata,
		}
	}

	if !isReply {
		log.Printf("[DEBUG] Top-level comment without direct mention, skipping per warrant policy")
		metadata := makeMetadata()
		metadata["reason"] = "top_level_not_addressed_to_bot"
		return false, ResponseScenarioV2{
			Type:       "no_response",
			Reason:     "comment not directed at bot",
			Confidence: 0.0,
			Metadata:   metadata,
		}
	}

	log.Printf("[DEBUG] Reply present but parent not bot and no direct mention; skipping")
	metadata := makeMetadata()
	metadata["reason"] = "reply_not_directed_to_bot"
	return false, ResponseScenarioV2{
		Type:       "no_response",
		Reason:     "comment not directed at bot",
		Confidence: 0.0,
		Metadata:   metadata,
	}
}

func (p *UnifiedProcessorV2Impl) isCommentAuthoredByBot(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) bool {
	if botInfo == nil || event.Comment == nil {
		return false
	}

	botIdentifiers := map[string]string{}
	if botInfo.UserID != "" {
		botIdentifiers[normalizeIdentifier(botInfo.UserID)] = botInfo.UserID
	}
	if botInfo.Metadata != nil {
		if accountID, ok := botInfo.Metadata["account_id"].(string); ok && accountID != "" {
			botIdentifiers[normalizeIdentifier(accountID)] = accountID
		}
		if uuid, ok := botInfo.Metadata["uuid"].(string); ok && uuid != "" {
			botIdentifiers[normalizeIdentifier(uuid)] = uuid
		}
	}

	authorIdentifiers := map[string]string{}
	if event.Comment.Author.ID != "" {
		authorIdentifiers[normalizeIdentifier(event.Comment.Author.ID)] = event.Comment.Author.ID
	}
	if event.Comment.Author.Metadata != nil {
		if accountID, ok := event.Comment.Author.Metadata["account_id"].(string); ok && accountID != "" {
			authorIdentifiers[normalizeIdentifier(accountID)] = accountID
		}
		if uuid, ok := event.Comment.Author.Metadata["uuid"].(string); ok && uuid != "" {
			authorIdentifiers[normalizeIdentifier(uuid)] = uuid
		}
	}
	if event.Comment.Metadata != nil {
		if accountID, ok := event.Comment.Metadata["account_id"].(string); ok && accountID != "" {
			authorIdentifiers[normalizeIdentifier(accountID)] = accountID
		}
		if uuid, ok := event.Comment.Metadata["user_uuid"].(string); ok && uuid != "" {
			authorIdentifiers[normalizeIdentifier(uuid)] = uuid
		}
	}

	for normalizedAuthor, rawAuthor := range authorIdentifiers {
		if normalizedAuthor == "" {
			continue
		}
		if rawBot, found := botIdentifiers[normalizedAuthor]; found {
			log.Printf("[DEBUG] Author matches bot identifier (%s vs %s)", rawAuthor, rawBot)
			return true
		}
	}

	if botInfo.Username != "" && strings.EqualFold(event.Comment.Author.Username, botInfo.Username) {
		log.Printf("[DEBUG] Author username %s matches bot username", event.Comment.Author.Username)
		return true
	}

	return false
}

// ProcessCommentReply processes comment reply flow using original working logic
func (p *UnifiedProcessorV2Impl) ProcessCommentReply(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2, error) {
	if event.Comment == nil {
		return "", nil, fmt.Errorf("no comment in event for reply processing")
	}

	log.Printf("[INFO] Processing comment reply for %s provider using original contextual logic", event.Provider)

	// Use the original sophisticated contextual response logic
	response, learning := p.buildContextualResponseWithLearningV2(ctx, event, timeline, orgID)

	return response, learning, nil
}

// buildCommentReplyPromptWithLearning creates LLM prompt with learning instructions
func (p *UnifiedProcessorV2Impl) buildCommentReplyPromptWithLearning(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) string {
	prompt := &strings.Builder{}

	// Core context
	prompt.WriteString("You are LiveReviewBot, an AI code review assistant.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Repository: %s\n", event.Repository.Name))
	if event.MergeRequest != nil {
		prompt.WriteString(fmt.Sprintf("- MR/PR title: %s\n", event.MergeRequest.Title))
	}

	prompt.WriteString("\nCURRENT COMMENT (reply only to this message unless explicitly asked otherwise):\n")
	prompt.WriteString(fmt.Sprintf("@%s wrote: %s\n", event.Comment.Author.Username, event.Comment.Body))
	if event.Repository.Name != "" {
		prompt.WriteString(fmt.Sprintf("Repository path: %s\n", event.Repository.FullName))
	}

	// Timeline context if available
	if timeline != nil && len(timeline.Items) > 0 {
		prompt.WriteString("\nRECENT CONVERSATION ACROSS THREAD (for context only, do not respond to prior messages unless they are referenced in the current comment):\n")
		for _, item := range timeline.Items {
			if item.Comment != nil {
				content := item.Comment.Body
				if len(content) > 100 {
					content = content[:100]
				}
				prompt.WriteString(fmt.Sprintf("- %s: %s\n", item.Comment.Author.Username, content))
			}
		}
	}

	if event.Comment != nil {
		builder := coreprocessor.UnifiedContextBuilderV2{}
		if codeContext, err := builder.ExtractCodeContext(*event.Comment, event.Provider); err == nil && codeContext != "" {
			prompt.WriteString("\nCODE CONTEXT:\n")
			prompt.WriteString(codeContext)
			prompt.WriteString("\n")
		}
	}

	prompt.WriteString("\nTASK:\n")
	prompt.WriteString("Answer the CURRENT COMMENT directly. Keep the reply focused on the exact question or concern that was raised. Reference surrounding code or prior discussion only when it improves the specific answer.\n")
	prompt.WriteString("- Do not summarise unrelated feedback or earlier conversations unless the user explicitly asked for it.\n")
	prompt.WriteString("- If the user asks \"what is this about?\" or similar, explain the referenced code fragment plainly and briefly.\n")
	prompt.WriteString("- Stay concise, professional, and actionable.\n\n")

	// LEARNING INSTRUCTIONS - Enhanced with specific examples and triggers
	prompt.WriteString("LEARNING EXTRACTION:\n")
	prompt.WriteString("IMPORTANT: Look for team policies, coding standards, and preferences that should be remembered for future interactions.\n\n")

	prompt.WriteString("EXTRACT LEARNING when you see phrases like:\n")
	prompt.WriteString("- \"our team prefers...\", \"we generally...\", \"in our team...\"\n")
	prompt.WriteString("- \"we don't use...\", \"we always...\", \"our standard is...\"\n")
	prompt.WriteString("- \"team policy\", \"coding standard\", \"house rule\"\n")
	prompt.WriteString("- User correcting you about team practices\n")
	prompt.WriteString("- Specific technology choices: \"we use X instead of Y\"\n\n")

	prompt.WriteString("LEARNING EXAMPLES:\n")
	prompt.WriteString("✓ \"our team prefers assertion-based error management\" → Extract team policy about assertions\n")
	prompt.WriteString("✓ \"we don't use magic numbers, always use constants\" → Extract coding standard\n")
	prompt.WriteString("✓ \"in our codebase, we use TypeScript instead of JavaScript\" → Extract technology preference\n")
	prompt.WriteString("✗ \"this code has a bug\" → No learning (just reporting an issue)\n")
	prompt.WriteString("✗ \"can you explain this function?\" → No learning (just asking for help)\n\n")

	prompt.WriteString("If you identify a learning, add this JSON block at the end of your response:\n")
	prompt.WriteString("```learning\n")
	prompt.WriteString("{\n")
	prompt.WriteString(`  "type": "team_policy|coding_standard|preference|rule",` + "\n")
	prompt.WriteString(`  "title": "Brief descriptive title of what you learned",` + "\n")
	prompt.WriteString(`  "content": "Full description of the team's practice, preference, or rule",` + "\n")
	prompt.WriteString(`  "tags": ["relevant", "keywords", "for_searching"],` + "\n")
	prompt.WriteString(`  "scope": "org|repo",` + "\n")
	prompt.WriteString(`  "confidence": 1-5` + "\n")
	prompt.WriteString("}\n```\n\n")

	prompt.WriteString("Only include learning block if there's genuinely something worth learning. Most responses won't have learnings. Never repeat a previously acknowledged learning unless this comment introduces new guidance.\n\n")
	prompt.WriteString("RESPONSE:\n")

	return prompt.String()
}

// buildContextualResponseWithLearningV2 creates response using LLM with learning instructions
func (p *UnifiedProcessorV2Impl) buildContextualResponseWithLearningV2(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2) {
	prompt := p.buildCommentReplyPromptWithLearning(event, timeline)

	var relevantLearnings []*learnings.Learning
	if orgID != 0 {
		repoID := event.Repository.FullName
		if repoID == "" {
			repoID = event.Repository.Name
		}

		title := ""
		description := ""
		if event.MergeRequest != nil {
			title = event.MergeRequest.Title
			description = event.MergeRequest.Description
		}

		fetched, err := p.fetchRelevantLearnings(ctx, orgID, repoID, nil, title, description)
		if err != nil {
			log.Printf("[WARN] Failed to fetch relevant learnings: %v", err)
		} else {
			relevantLearnings = fetched
		}
	}

	prompt = p.appendLearningsToPrompt(prompt, relevantLearnings)

	// write the prompt into a file for debugging
	err := os.WriteFile("debug_prompt.txt", []byte(prompt), 0644)
	if err != nil {
		log.Printf("[WARN] Failed to write debug prompt to file: %v", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	llmResponse, learning, err := p.generateLLMResponseWithLearning(ctx, prompt, event, orgID)
	if err != nil {
		log.Printf("[ERROR] LLM generation failed: %v - cannot provide response", err)
		return fmt.Sprintf("I'm sorry, I'm unable to generate a response right now. Please try again later. (Error: %v)", err), nil
	}

	return llmResponse, learning
}

func (p *UnifiedProcessorV2Impl) fetchRelevantLearnings(ctx context.Context, orgID int64, repoID string, changedFiles []string, title, description string) ([]*learnings.Learning, error) {
	if p.server == nil || p.server.learningsService == nil || orgID == 0 {
		return nil, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	return p.server.learningsService.ListActiveByOrg(ctx, orgID)
}

func (p *UnifiedProcessorV2Impl) appendLearningsToPrompt(prompt string, items []*learnings.Learning) string {
	if len(items) == 0 {
		return prompt
	}

	section := p.formatLearningsSection(items)
	if section == "" {
		return prompt
	}

	if !strings.HasSuffix(prompt, "\n") {
		prompt += "\n"
	}

	return prompt + section
}

func (p *UnifiedProcessorV2Impl) formatLearningsSection(items []*learnings.Learning) string {
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("=== Org learnings ===\n")
	b.WriteString("Incorporate the following established org guidance when drafting your reply:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", item.ShortID, item.Title))
		if body := truncateLearningBodyV2(item.Body, 260); body != "" {
			b.WriteString("  ")
			b.WriteString(body)
			b.WriteString("\n")
		}
		if len(item.Tags) > 0 {
			b.WriteString("  Tags: ")
			b.WriteString(strings.Join(item.Tags, ", "))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func truncateLearningBodyV2(body string, limit int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	if limit > 0 && len(body) > limit {
		body = body[:limit]
		if idx := strings.LastIndex(body, " "); idx > limit-40 {
			body = body[:idx]
		}
		body = strings.TrimSpace(body) + "..."
	}

	return body
}

// ProcessFullReview processes full review flow when bot is assigned as reviewer
// Extracted from triggerReviewFor* functions (to be implemented in future phases)
func (p *UnifiedProcessorV2Impl) ProcessFullReview(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) ([]UnifiedReviewCommentV2, *LearningMetadataV2, error) {
	// TODO: Implement full review processing in Phase 7.2
	// This will extract review logic from the monolithic handler
	return nil, nil, fmt.Errorf("full review processing not yet implemented")
}

// Helper methods (extracted from webhook_handler.go)

func (p *UnifiedProcessorV2Impl) isReplyToBotComment(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	if event.Comment == nil || botInfo == nil {
		return false, nil
	}

	if event.Comment.InReplyToID == nil || *event.Comment.InReplyToID == "" {
		return false, nil
	}

	replyToID := *event.Comment.InReplyToID
	switch event.Provider {
	case "github":
		return p.checkGitHubParentCommentAuthor(event, replyToID, botInfo)
	case "bitbucket":
		return p.checkBitbucketParentCommentAuthor(event, replyToID, botInfo)
	case "gitlab":
		return p.checkGitLabDiscussionForBotReply(event, botInfo)
	default:
		return false, fmt.Errorf("reply detection not implemented for provider %s", event.Provider)
	}
}

func (p *UnifiedProcessorV2Impl) checkGitHubParentCommentAuthor(event UnifiedWebhookEventV2, parentID string, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	if p.server == nil || p.server.githubProviderV2 == nil {
		return false, fmt.Errorf("github provider unavailable for parent lookup")
	}

	repoFullName := event.Repository.FullName
	if repoFullName == "" {
		return false, fmt.Errorf("missing repository full name for GitHub reply detection")
	}

	token, err := p.server.githubProviderV2.FindIntegrationTokenForRepo(repoFullName)
	if err != nil || token == nil {
		return false, fmt.Errorf("failed to find GitHub token: %w", err)
	}

	var apiURL string
	if event.Comment.Position != nil && event.Comment.Position.FilePath != "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/comments/%s", repoFullName, parentID)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/comments/%s", repoFullName, parentID)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+token.PatToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("GitHub API error getting parent comment (status %d): %s", resp.StatusCode, string(body))
	}

	var parent struct {
		User struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parent); err != nil {
		return false, err
	}

	if parent.User.Login == "" {
		return false, nil
	}

	if botInfo.Username != "" && strings.EqualFold(parent.User.Login, botInfo.Username) {
		return true, nil
	}

	if botInfo.UserID != "" && fmt.Sprintf("%d", parent.User.ID) == botInfo.UserID {
		return true, nil
	}

	return false, nil
}

func (p *UnifiedProcessorV2Impl) checkBitbucketParentCommentAuthor(event UnifiedWebhookEventV2, parentID string, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	if p.server == nil || p.server.bitbucketProviderV2 == nil {
		return false, fmt.Errorf("bitbucket provider unavailable for parent lookup")
	}

	if event.Comment.Metadata == nil {
		return false, fmt.Errorf("missing Bitbucket metadata for reply detection")
	}

	workspace, _ := event.Comment.Metadata["workspace"].(string)
	repository, _ := event.Comment.Metadata["repository"].(string)

	prNumber := ""
	switch value := event.Comment.Metadata["pr_number"].(type) {
	case string:
		prNumber = value
	case int:
		prNumber = fmt.Sprintf("%d", value)
	case float64:
		prNumber = fmt.Sprintf("%d", int(value))
	}
	if prNumber == "" && event.MergeRequest != nil {
		prNumber = event.MergeRequest.ID
	}

	if workspace == "" || repository == "" || prNumber == "" {
		return false, fmt.Errorf("insufficient Bitbucket metadata for reply detection")
	}

	token, err := p.server.bitbucketProviderV2.FindIntegrationTokenForRepo(event.Repository.FullName)
	if err != nil || token == nil {
		return false, fmt.Errorf("failed to find Bitbucket token: %w", err)
	}

	email := ""
	if token.Metadata != nil {
		if raw, ok := token.Metadata["email"]; ok {
			switch v := raw.(type) {
			case string:
				email = v
			case []byte:
				email = string(v)
			}
		}
	}
	if email == "" {
		return false, fmt.Errorf("bitbucket email missing in token metadata")
	}

	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments/%s", workspace, repository, prNumber, parentID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, err
	}

	req.SetBasicAuth(email, token.PatToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("Bitbucket API error getting parent comment (status %d): %s", resp.StatusCode, string(body))
	}

	var parent bitbucketParentComment
	if err := json.NewDecoder(resp.Body).Decode(&parent); err != nil {
		return false, err
	}

	if botInfo.Username != "" && strings.EqualFold(parent.User.Username, botInfo.Username) {
		return true, nil
	}

	if botInfo.Metadata != nil {
		if accountID, ok := botInfo.Metadata["account_id"].(string); ok && accountID != "" {
			if normalizeIdentifier(parent.User.AccountID) == normalizeIdentifier(accountID) {
				return true, nil
			}
		}
		if uuid, ok := botInfo.Metadata["uuid"].(string); ok && uuid != "" {
			if normalizeIdentifier(parent.User.UUID) == normalizeIdentifier(uuid) {
				return true, nil
			}
		}
	}

	if botInfo.UserID != "" {
		if normalizeIdentifier(parent.User.AccountID) == normalizeIdentifier(botInfo.UserID) {
			return true, nil
		}
	}

	return false, nil
}

func (p *UnifiedProcessorV2Impl) checkGitLabDiscussionForBotReply(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	if event.Comment == nil || event.Comment.DiscussionID == nil || *event.Comment.DiscussionID == "" {
		return false, nil
	}

	log.Printf("[DEBUG] GitLab discussion reply checking not yet implemented for discussion %s", *event.Comment.DiscussionID)
	return false, nil
}

// checkDirectBotMentionV2 checks for direct @mentions of the bot using provider-aware helpers.
func (p *UnifiedProcessorV2Impl) checkDirectBotMentionV2(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) bool {
	if botInfo == nil || event.Comment == nil {
		return false
	}

	body := event.Comment.Body
	switch strings.ToLower(event.Provider) {
	case "github":
		return githubmentions.DetectDirectMention(body, botInfo)
	case "gitlab":
		return gitlabmentions.DetectDirectMention(body, botInfo)
	case "bitbucket":
		return bitbucketmentions.DetectDirectMention(body, botInfo)
	default:
		return fallbackUsernameMention(body, botInfo)
	}
}

func fallbackUsernameMention(commentBody string, botInfo *UnifiedBotUserInfoV2) bool {
	if botInfo == nil {
		return false
	}

	username := strings.TrimSpace(botInfo.Username)
	if username == "" {
		return false
	}

	mentionPattern := "@" + strings.ToLower(username)
	return strings.Contains(strings.ToLower(commentBody), mentionPattern)
}

// classifyContentTypeV2 classifies the type of content in a comment
// Extracted from classifyContentType
func (p *UnifiedProcessorV2Impl) classifyContentTypeV2(commentBody string) string {
	commentLower := strings.ToLower(commentBody)

	if strings.Contains(commentLower, "documentation") || strings.Contains(commentLower, "document") {
		return "documentation"
	}
	if strings.Contains(commentLower, "error") || strings.Contains(commentLower, "bug") {
		return "error_report"
	}
	if strings.Contains(commentLower, "performance") || strings.Contains(commentLower, "slow") {
		return "performance"
	}
	if strings.Contains(commentLower, "security") || strings.Contains(commentLower, "vulnerable") {
		return "security"
	}
	if strings.Contains(commentLower, "?") || strings.Contains(commentLower, "question") {
		return "question"
	}
	if strings.Contains(commentLower, "help") || strings.Contains(commentLower, "explain") {
		return "help_request"
	}

	return "general"
}

// determineResponseTypeV2 determines the appropriate response type
// Extracted from determineResponseType
func (p *UnifiedProcessorV2Impl) determineResponseTypeV2(commentBody string) string {
	commentLower := strings.ToLower(commentBody)

	if strings.Contains(commentLower, "brief") || strings.Contains(commentLower, "quick") {
		return "brief_acknowledgment"
	}
	if strings.Contains(commentLower, "explain") || strings.Contains(commentLower, "detail") {
		return "detailed_response"
	}
	if strings.Contains(commentLower, "how") || strings.Contains(commentLower, "implement") {
		return "implementation_guidance"
	}

	return "contextual_analysis"
}

// buildUnifiedPromptV2 builds a provider-agnostic prompt for AI processing
// Unified version of buildGeminiPromptEnhanced
func (p *UnifiedProcessorV2Impl) buildUnifiedPromptV2(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) string {
	var prompt strings.Builder

	// Context header
	prompt.WriteString("You are an AI code review assistant. ")
	prompt.WriteString(fmt.Sprintf("Analyzing a %s comment from %s.\n\n", event.Provider, event.Comment.Author.Username))

	// Repository and MR context
	if event.MergeRequest != nil {
		prompt.WriteString(fmt.Sprintf("**Repository:** %s\n", event.Repository.FullName))
		prompt.WriteString(fmt.Sprintf("**Merge Request:** %s\n", event.MergeRequest.Title))
		prompt.WriteString(fmt.Sprintf("**Description:** %s\n\n", event.MergeRequest.Description))
	}

	// Comment content
	prompt.WriteString(fmt.Sprintf("**Comment by @%s:**\n", event.Comment.Author.Username))
	prompt.WriteString(fmt.Sprintf("%s\n\n", event.Comment.Body))

	// Timeline context if available
	if timeline != nil && len(timeline.Items) > 0 {
		prompt.WriteString("**Recent Activity:**\n")

		// Show last few timeline items for context
		start := 0
		if len(timeline.Items) > 5 {
			start = len(timeline.Items) - 5
		}

		for i := start; i < len(timeline.Items); i++ {
			item := timeline.Items[i]
			switch item.Type {
			case "commit":
				if item.Commit != nil {
					prompt.WriteString(fmt.Sprintf("- Commit: %s\n", item.Commit.Message))
				}
			case "comment":
				if item.Comment != nil {
					prompt.WriteString(fmt.Sprintf("- Comment by @%s: %s\n",
						item.Comment.Author.Username,
						p.truncateString(item.Comment.Body, 100)))
				}
			}
		}
		prompt.WriteString("\n")
	}

	// Code position context if available
	if event.Comment.Position != nil {
		prompt.WriteString("**Code Context:**\n")
		prompt.WriteString(fmt.Sprintf("File: %s\n", event.Comment.Position.FilePath))
		if event.Comment.Position.LineNumber > 0 {
			prompt.WriteString(fmt.Sprintf("Line: %d\n", event.Comment.Position.LineNumber))
		}
		prompt.WriteString("\n")
	}

	// Response guidance
	prompt.WriteString("Please provide a helpful, technical response that:\n")
	prompt.WriteString("1. Addresses the specific question or concern\n")
	prompt.WriteString("2. Provides actionable guidance when appropriate\n")
	prompt.WriteString("3. Maintains a professional, collaborative tone\n")
	prompt.WriteString("4. Focuses on code quality and best practices\n\n")

	return prompt.String()
}

// generateLLMResponseWithLearning generates LLM response and extracts learning
func (p *UnifiedProcessorV2Impl) generateLLMResponseWithLearning(ctx context.Context, prompt string, event UnifiedWebhookEventV2, orgID int64) (string, *LearningMetadataV2, error) {
	// Try to get LLM response
	llmResponse, err := p.generateLLMResponseV2(ctx, prompt)
	if err != nil {
		return "", nil, err
	}

	// Extract learning from LLM response
	learning := p.extractLearningFromLLMResponse(llmResponse, event, orgID)

	// Clean response by removing learning block
	cleanResponse := p.cleanResponseFromLearningBlock(llmResponse)

	return cleanResponse, learning, nil
}

// generateLLMResponseV2 uses the actual AI connectors infrastructure
func (p *UnifiedProcessorV2Impl) generateLLMResponseV2(ctx context.Context, prompt string) (string, error) {
	if p.server == nil || p.server.db == nil {
		return "", fmt.Errorf("server or database not available")
	}

	// Get available AI connectors
	storage := aiconnectors.NewStorage(p.server.db)
	connectors, err := storage.GetAllConnectors(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get AI connectors: %w", err)
	}

	if len(connectors) == 0 {
		return "", fmt.Errorf("no AI connectors configured")
	}

	// Use the first available connector (could be enhanced with priority logic)
	connectorRecord := connectors[0]

	// Create connector options
	options := connectorRecord.GetConnectorOptions()

	// Create connector client
	client, err := aiconnectors.NewConnector(ctx, options)
	if err != nil {
		return "", fmt.Errorf("failed to create AI connector: %w", err)
	}

	// Generate response
	response, err := client.Call(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("AI connector call failed: %w", err)
	}

	log.Printf("[DEBUG] Generated LLM response using %s connector", connectorRecord.ProviderName)
	return response, nil
}

// extractLearningFromLLMResponse extracts structured learning from LLM response
func (p *UnifiedProcessorV2Impl) extractLearningFromLLMResponse(response string, event UnifiedWebhookEventV2, orgID int64) *LearningMetadataV2 {
	// Look for learning JSON block in response (handle multiple possible formats)
	learningBlockStart := strings.Index(response, "```learning")
	if learningBlockStart == -1 {
		// Try alternative format
		learningBlockStart = strings.Index(response, "```json")
		if learningBlockStart == -1 {
			return nil // No learning block found
		}
	}

	// Find the end of the code block
	searchStart := learningBlockStart + 10 // Skip past ```learning or ```json
	learningBlockEnd := strings.Index(response[searchStart:], "```")
	if learningBlockEnd == -1 {
		log.Printf("[WARN] Learning block not properly closed")
		return nil // Invalid learning block
	}

	// Extract JSON content - handle various newline patterns
	jsonStartOffset := strings.Index(response[learningBlockStart:], "\n")
	if jsonStartOffset == -1 {
		jsonStartOffset = strings.Index(response[learningBlockStart:], "{")
		if jsonStartOffset == -1 {
			log.Printf("[WARN] No JSON content found in learning block")
			return nil
		}
		jsonStartOffset = learningBlockStart + jsonStartOffset
	} else {
		jsonStartOffset = learningBlockStart + jsonStartOffset + 1
	}

	jsonEnd := searchStart + learningBlockEnd
	jsonContent := strings.TrimSpace(response[jsonStartOffset:jsonEnd])

	log.Printf("[DEBUG] Extracted JSON content: %s", jsonContent)

	// Parse JSON with flexible struct
	var learningData struct {
		Type       string   `json:"type"`
		Title      string   `json:"title"`
		Content    string   `json:"content"`
		Tags       []string `json:"tags"`
		Scope      string   `json:"scope"`
		Confidence int      `json:"confidence"`
	}

	err := json.Unmarshal([]byte(jsonContent), &learningData)
	if err != nil {
		log.Printf("[WARN] Failed to parse learning JSON: %v", err)
		log.Printf("[DEBUG] Raw JSON content: %q", jsonContent)
		return nil
	}

	// Validate required fields
	if learningData.Type == "" || learningData.Title == "" || learningData.Content == "" {
		log.Printf("[WARN] Learning JSON missing required fields: type=%s, title=%s, content=%s",
			learningData.Type, learningData.Title, learningData.Content)
		return nil
	}

	// Convert to LearningMetadataV2
	learning := &LearningMetadataV2{
		Type:       learningData.Type,
		Content:    fmt.Sprintf("%s: %s", learningData.Title, learningData.Content),
		Tags:       learningData.Tags,
		Confidence: float64(learningData.Confidence),
		Context:    "", // Will be set below
		OrgID:      orgID,
		Metadata:   map[string]interface{}{},
	}

	// Create comprehensive source context from webhook event
	sourceContext := map[string]interface{}{
		"provider":   event.Provider,
		"repository": event.Repository.Name,
	}

	// Add repository details
	if event.Repository.WebURL != "" {
		sourceContext["repository_url"] = event.Repository.WebURL
	}
	if event.Repository.FullName != "" {
		sourceContext["repository_full_name"] = event.Repository.FullName
	}

	// Collect source URLs
	var sourceURLs []string

	// Add MR-specific context
	if event.MergeRequest != nil {
		sourceContext["mr_number"] = event.MergeRequest.Number
		sourceContext["mr_title"] = event.MergeRequest.Title
		sourceContext["mr_id"] = event.MergeRequest.ID
		sourceContext["source_branch"] = event.MergeRequest.SourceBranch
		sourceContext["target_branch"] = event.MergeRequest.TargetBranch
		if event.MergeRequest.Author.Username != "" {
			sourceContext["mr_author"] = event.MergeRequest.Author.Username
		}
		if event.MergeRequest.WebURL != "" {
			sourceURLs = append(sourceURLs, event.MergeRequest.WebURL)
		}
	}

	// Add comment-specific context
	if event.Comment != nil {
		sourceContext["comment_id"] = event.Comment.ID
		sourceContext["comment_author"] = event.Comment.Author.Username
		if event.Comment.DiscussionID != nil {
			sourceContext["discussion_id"] = *event.Comment.DiscussionID
		}
		if event.Comment.WebURL != "" {
			sourceURLs = append(sourceURLs, event.Comment.WebURL)
		}

		// Add file position details if available
		if event.Comment.Position != nil {
			sourceContext["file_path"] = event.Comment.Position.FilePath
			sourceContext["line_number"] = event.Comment.Position.LineNumber
			sourceContext["line_type"] = event.Comment.Position.LineType
			if event.Comment.Position.StartLine != nil {
				sourceContext["line_start"] = *event.Comment.Position.StartLine
			}
			if event.Comment.Position.EndLine != nil {
				sourceContext["line_end"] = *event.Comment.Position.EndLine
			}
		}
	}

	// Store in learning metadata
	learning.Metadata["source_context"] = sourceContext
	learning.Metadata["source_urls"] = sourceURLs
	learning.Context = fmt.Sprintf("Repository: %s, Provider: %s", event.Repository.Name, event.Provider)

	log.Printf("[DEBUG] Extracted learning: type=%s, title=%s", learningData.Type, learningData.Title)
	return learning
}

// cleanResponseFromLearningBlock removes learning JSON block from response
func (p *UnifiedProcessorV2Impl) cleanResponseFromLearningBlock(response string) string {
	// Look for learning block (handle multiple formats)
	learningBlockStart := strings.Index(response, "```learning")
	if learningBlockStart == -1 {
		// Try alternative format where learning might be in ```json block
		// But only remove if it contains learning-like content
		jsonStart := strings.Index(response, "```json")
		if jsonStart != -1 {
			// Check if it contains learning content
			jsonEnd := strings.Index(response[jsonStart+7:], "```")
			if jsonEnd != -1 {
				jsonContent := response[jsonStart+7 : jsonStart+7+jsonEnd]
				if strings.Contains(jsonContent, "\"type\"") &&
					(strings.Contains(jsonContent, "team_policy") ||
						strings.Contains(jsonContent, "coding_standard") ||
						strings.Contains(jsonContent, "preference")) {
					learningBlockStart = jsonStart
				}
			}
		}
	}

	if learningBlockStart == -1 {
		return response // No learning block found
	}

	// Find end of learning block - search from a bit after the start
	searchStart := learningBlockStart + 10
	learningBlockEnd := strings.Index(response[searchStart:], "```")
	if learningBlockEnd == -1 {
		return response // Invalid block
	}

	// Remove the entire learning block
	endPos := searchStart + learningBlockEnd + 3
	cleanResponse := response[:learningBlockStart] + response[endPos:]
	return strings.TrimSpace(cleanResponse)
}

// Utility helper methods

// truncateString truncates a string to maxLength with ellipsis
func (p *UnifiedProcessorV2Impl) truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}
