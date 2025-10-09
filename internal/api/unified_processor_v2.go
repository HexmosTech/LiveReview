package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/aiconnectors"
)

// Phase 7.1: Unified processor for provider-agnostic LLM processing
// Extracted from webhook_handler.go - provider-independent AI response generation

// UnifiedProcessorV2Impl implements the UnifiedProcessorV2 interface
type UnifiedProcessorV2Impl struct {
	server *Server // For accessing database operations and AI infrastructure
}

// NewUnifiedProcessorV2 creates a new unified processor instance
func NewUnifiedProcessorV2(server *Server) UnifiedProcessorV2 {
	return &UnifiedProcessorV2Impl{
		server: server,
	}
}

// CheckResponseWarrant determines if an event warrants an AI response (provider-agnostic)
// Extracted from checkAIResponseWarrant, checkUnifiedAIResponseWarrant
func (p *UnifiedProcessorV2Impl) CheckResponseWarrant(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, ResponseScenarioV2) {
	if event.Comment == nil {
		log.Printf("[DEBUG] No comment in event, skipping warrant check")
		return false, ResponseScenarioV2{}
	}

	log.Printf("[DEBUG] Checking AI response warrant for comment by %s", event.Comment.Author.Username)
	log.Printf("[DEBUG] Comment content: %s", event.Comment.Body)

	// Debug bot info
	if botInfo != nil {
		log.Printf("[DEBUG] Bot info available: Username=%s, UserID=%s, Name=%s", botInfo.Username, botInfo.UserID, botInfo.Name)
	} else {
		log.Printf("[DEBUG] No bot info available")
	}

	// Debug reply info
	if event.Comment.InReplyToID != nil {
		log.Printf("[DEBUG] Comment is a reply, InReplyToID=%s", *event.Comment.InReplyToID)
	} else {
		log.Printf("[DEBUG] Comment is not a reply (InReplyToID is nil)")
	}

	// Early anti-loop protection: Check for common bot usernames
	commonBotUsernames := []string{"livereviewbot", "LiveReviewBot", "ai-bot", "codebot", "reviewbot"}
	for _, botUsername := range commonBotUsernames {
		if strings.EqualFold(event.Comment.Author.Username, botUsername) {
			log.Printf("[DEBUG] Comment by bot user %s, skipping", event.Comment.Author.Username)
			return false, ResponseScenarioV2{}
		}
	}

	// PRIORITY 1: Check if replying to AI bot comment
	if event.Comment.InReplyToID != nil {
		// This indicates a reply to another comment
		// Check if the parent comment was by the bot
		if botInfo != nil && p.isReplyToBotComment(*event.Comment.InReplyToID, botInfo) {
			log.Printf("[DEBUG] Reply to AI bot comment detected")
			return true, ResponseScenarioV2{
				Type:       "bot_reply",
				Reason:     fmt.Sprintf("Reply to bot comment by %s", event.Comment.Author.Username),
				Confidence: 0.90,
				Metadata: map[string]interface{}{
					"content_type":  p.classifyContentTypeV2(event.Comment.Body),
					"response_type": p.determineResponseTypeV2(event.Comment.Body),
				},
			}
		}
	}

	// PRIORITY 2: Check for direct @mentions of the bot
	if botInfo != nil {
		isDirectMention := p.checkDirectBotMentionV2(event.Comment.Body, botInfo)
		if isDirectMention {
			log.Printf("[DEBUG] Direct bot mention detected in comment")
			return true, ResponseScenarioV2{
				Type:       "direct_mention",
				Reason:     fmt.Sprintf("Direct mention of bot by %s", event.Comment.Author.Username),
				Confidence: 0.95,
				Metadata: map[string]interface{}{
					"content_type":  p.classifyContentTypeV2(event.Comment.Body),
					"response_type": p.determineResponseTypeV2(event.Comment.Body),
				},
			}
		}
	}

	// PRIORITY 3: Check for contextual replies (GitLab discussion context)
	// If we have a discussion ID, this might be part of an ongoing conversation
	if event.Comment.DiscussionID != nil && *event.Comment.DiscussionID != "" {
		log.Printf("[DEBUG] Comment is part of discussion: %s", *event.Comment.DiscussionID)
		// For now, treat discussion participation as warranting response
		return true, ResponseScenarioV2{
			Type:       "discussion_reply",
			Reason:     fmt.Sprintf("Comment in discussion %s by %s", *event.Comment.DiscussionID, event.Comment.Author.Username),
			Confidence: 0.85,
			Metadata: map[string]interface{}{
				"content_type":  p.classifyContentTypeV2(event.Comment.Body),
				"response_type": p.determineResponseTypeV2(event.Comment.Body),
				"discussion_id": *event.Comment.DiscussionID,
			},
		}
	}

	// PRIORITY 4: Content analysis for implicit response triggers
	// Keep minimal to avoid false positives
	contentTriggers := []string{"help", "question", "explain", "how", "why", "what", "use", "not", "do not", "rule", "team"}
	commentLower := strings.ToLower(event.Comment.Body)

	for _, trigger := range contentTriggers {
		if strings.Contains(commentLower, trigger) {
			log.Printf("[DEBUG] Content trigger '%s' detected", trigger)
			return true, ResponseScenarioV2{
				Type:       "content_trigger",
				Reason:     fmt.Sprintf("Content trigger '%s' detected", trigger),
				Confidence: 0.70,
				Metadata: map[string]interface{}{
					"content_type":  p.classifyContentTypeV2(event.Comment.Body),
					"response_type": p.determineResponseTypeV2(event.Comment.Body),
					"trigger":       trigger,
				},
			}
		}
	}

	log.Printf("[DEBUG] No response warrant detected")
	return false, ResponseScenarioV2{}
} // ProcessCommentReply processes comment reply flow using original working logic
func (p *UnifiedProcessorV2Impl) ProcessCommentReply(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2, error) {
	if event.Comment == nil {
		return "", nil, fmt.Errorf("no comment in event for reply processing")
	}

	log.Printf("[INFO] Processing comment reply for %s provider using original contextual logic", event.Provider)

	// Use the original sophisticated contextual response logic
	response, learning := p.buildContextualResponseWithLearningV2(event, timeline, orgID)

	return response, learning, nil
}

// buildCommentReplyPromptWithLearning creates LLM prompt with learning instructions
func (p *UnifiedProcessorV2Impl) buildCommentReplyPromptWithLearning(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) string {
	prompt := &strings.Builder{}

	// Core context
	prompt.WriteString("You are LiveReviewBot, an AI code review assistant.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- User @%s commented: %s\n", event.Comment.Author.Username, event.Comment.Body))
	if event.Repository.Name != "" {
		prompt.WriteString(fmt.Sprintf("- Repository: %s\n", event.Repository.Name))
	}
	if event.MergeRequest != nil {
		prompt.WriteString(fmt.Sprintf("- MR/PR: %s\n", event.MergeRequest.Title))
	}

	// Timeline context if available
	if timeline != nil && len(timeline.Items) > 0 {
		prompt.WriteString("\nRECENT CONVERSATION:\n")
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

	prompt.WriteString("\nTASK:\n")
	prompt.WriteString("Provide a helpful, contextual response to the user's comment. Be specific, actionable, and professional.\n\n")

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

	prompt.WriteString("Only include learning block if there's genuinely something worth learning. Most responses won't have learnings.\n\n")
	prompt.WriteString("RESPONSE:\n")

	return prompt.String()
}

// buildContextualResponseWithLearningV2 creates response using LLM with learning instructions
func (p *UnifiedProcessorV2Impl) buildContextualResponseWithLearningV2(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2) {
	// Build LLM prompt with learning instructions
	prompt := p.buildCommentReplyPromptWithLearning(event, timeline)

	// Try to generate LLM response
	ctx := context.Background()
	llmResponse, learning, err := p.generateLLMResponseWithLearning(ctx, prompt, event, orgID)
	if err != nil {
		log.Printf("[ERROR] LLM generation failed: %v - cannot provide response", err)
		return fmt.Sprintf("I'm sorry, I'm unable to generate a response right now. Please try again later. (Error: %v)", err), nil
	}

	return llmResponse, learning
}

// ProcessFullReview processes full review flow when bot is assigned as reviewer
// Extracted from triggerReviewFor* functions (to be implemented in future phases)
func (p *UnifiedProcessorV2Impl) ProcessFullReview(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) ([]UnifiedReviewCommentV2, *LearningMetadataV2, error) {
	// TODO: Implement full review processing in Phase 7.2
	// This will extract review logic from the monolithic handler
	return nil, nil, fmt.Errorf("full review processing not yet implemented")
}

// Helper methods (extracted from webhook_handler.go)

// checkIfReplyingToBotCommentV2 checks if a comment is replying to a bot comment (ORIGINAL LOGIC ADAPTED)
func (p *UnifiedProcessorV2Impl) checkIfReplyingToBotCommentV2(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	// If this comment is not part of a discussion/thread, it can't be a reply
	if event.Comment.DiscussionID == nil || *event.Comment.DiscussionID == "" {
		log.Printf("[DEBUG] Comment has no discussion_id, not a thread reply")
		return false, nil
	}

	log.Printf("[DEBUG] Checking if comment is reply to bot in discussion: %s", *event.Comment.DiscussionID)

	// For GitLab provider, we need to check the discussion API
	if event.Provider == "gitlab" {
		return p.checkGitLabDiscussionForBotReply(event, botInfo)
	}

	// For other providers, implement similar logic
	log.Printf("[DEBUG] Provider %s not yet supported for reply checking", event.Provider)
	return false, nil
}

// checkGitLabDiscussionForBotReply checks GitLab discussion for bot replies (ORIGINAL LOGIC)
func (p *UnifiedProcessorV2Impl) checkGitLabDiscussionForBotReply(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, error) {
	// This would require API access to GitLab to check the discussion
	// For now, implement a simpler heuristic: if it's in a discussion, assume it might be a reply
	log.Printf("[DEBUG] GitLab discussion reply checking - would need API access")

	// TODO: Implement full GitLab API checking like the original
	// For now, return false to be conservative
	return false, nil
}

// isReplyToBotComment checks if a comment is replying to a bot comment (LEGACY METHOD)
func (p *UnifiedProcessorV2Impl) isReplyToBotComment(replyToID string, botInfo *UnifiedBotUserInfoV2) bool {
	// This is the old method signature - redirect to new method
	log.Printf("[DEBUG] Legacy isReplyToBotComment called with replyToID: %s", replyToID)
	return false // Conservative default
}

// checkDirectBotMentionV2 checks for direct @mentions of the bot (ORIGINAL WORKING LOGIC)
func (p *UnifiedProcessorV2Impl) checkDirectBotMentionV2(commentBody string, botInfo *UnifiedBotUserInfoV2) bool {
	if botInfo == nil {
		return false
	}

	commentLower := strings.ToLower(commentBody)
	log.Printf("[DEBUG] Checking for direct mentions in comment: '%s'", commentBody)

	// Check for exact bot username mention (ORIGINAL LOGIC)
	mentionPattern := "@" + strings.ToLower(botInfo.Username)
	log.Printf("[DEBUG] Looking for mention pattern: '%s' in comment", mentionPattern)
	if strings.Contains(commentLower, mentionPattern) {
		log.Printf("[DEBUG] Direct mention found: %s mentioned in comment", botInfo.Username)
		return true
	}

	// Check for common bot names as fallback (ORIGINAL LOGIC)
	commonBotNames := []string{"livereviewbot", "livereview", "ai-bot", "codebot", "reviewbot"}
	for _, botName := range commonBotNames {
		fallbackPattern := "@" + botName
		log.Printf("[DEBUG] Looking for fallback mention pattern: '%s' in comment", fallbackPattern)
		if strings.Contains(commentLower, fallbackPattern) {
			log.Printf("[DEBUG] Direct mention found (fallback): %s mentioned in comment", botName)
			return true
		}
	}

	log.Printf("[DEBUG] No direct mentions found")
	return false
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

	// Add context from event
	if event.Repository.Name != "" {
		learning.Metadata["repository"] = event.Repository.Name
	}
	if event.Comment != nil {
		learning.Metadata["author"] = event.Comment.Author.Username
		learning.Metadata["comment"] = event.Comment.Body
	}
	learning.Metadata["provider"] = event.Provider
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
