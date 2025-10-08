package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
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

	// PRIORITY 3: Content analysis for implicit response triggers
	// Keep minimal to avoid false positives
	contentTriggers := []string{"help", "question", "explain", "how", "why", "what"}
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
} // ProcessCommentReply processes comment reply flow (provider-agnostic)
// Extracted from buildContextualAIResponse and related functions
func (p *UnifiedProcessorV2Impl) ProcessCommentReply(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) (string, *LearningMetadataV2, error) {
	if event.Comment == nil {
		return "", nil, fmt.Errorf("no comment in event for reply processing")
	}

	log.Printf("[INFO] Processing comment reply for %s provider", event.Provider)

	// Build enhanced prompt for AI processing
	prompt := p.buildUnifiedPromptV2(event, timeline)

	// Generate AI response using the server's infrastructure
	aiResponse, err := p.generateAIResponseFromPromptV2(prompt, event.Comment.Author.Username)
	if err != nil {
		log.Printf("[ERROR] Failed to generate AI response: %v", err)
		// Use structured fallback
		aiResponse = p.generateStructuredFallbackResponseV2(prompt, event.Comment.Author.Username)
	}

	// Extract learning metadata if present
	learning := p.extractLearningMetadataV2(ctx, aiResponse, event)

	return aiResponse, learning, nil
}

// ProcessFullReview processes full review flow when bot is assigned as reviewer
// Extracted from triggerReviewFor* functions (to be implemented in future phases)
func (p *UnifiedProcessorV2Impl) ProcessFullReview(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) ([]UnifiedReviewCommentV2, *LearningMetadataV2, error) {
	// TODO: Implement full review processing in Phase 7.2
	// This will extract review logic from the monolithic handler
	return nil, nil, fmt.Errorf("full review processing not yet implemented")
}

// Helper methods (extracted from webhook_handler.go)

// isReplyToBotComment checks if a comment is replying to a bot comment
func (p *UnifiedProcessorV2Impl) isReplyToBotComment(replyToID string, botInfo *UnifiedBotUserInfoV2) bool {
	// This would need to check against the actual comment being replied to
	// For now, return false as a safe default
	// TODO: Implement proper parent comment checking
	return false
}

// checkDirectBotMentionV2 checks for direct @mentions of the bot
func (p *UnifiedProcessorV2Impl) checkDirectBotMentionV2(commentBody string, botInfo *UnifiedBotUserInfoV2) bool {
	if botInfo == nil {
		return false
	}

	// Check for @username mentions
	mentionPatterns := []string{
		"@" + botInfo.Username,
		"@" + strings.ToLower(botInfo.Username),
	}

	for _, pattern := range mentionPatterns {
		if strings.Contains(strings.ToLower(commentBody), strings.ToLower(pattern)) {
			return true
		}
	}

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

// generateAIResponseFromPromptV2 generates AI response using server infrastructure
// Extracted from generateAIResponseFromPrompt
func (p *UnifiedProcessorV2Impl) generateAIResponseFromPromptV2(prompt, username string) (string, error) {
	log.Printf("[DEBUG] Generating AI response using LLM infrastructure")
	log.Printf("[DEBUG] Prompt length: %d characters", len(prompt))

	// Use context with reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to use the server's LLM infrastructure
	aiResponse, err := p.generateLLMResponseV2(ctx, prompt)
	if err != nil {
		log.Printf("[WARN] LLM generation failed, using structured fallback: %v", err)
		return p.generateStructuredFallbackResponseV2(prompt, username), nil
	}

	// Clean up response
	cleanResponse := strings.TrimSpace(aiResponse)
	if cleanResponse == "" {
		log.Printf("[WARN] Empty AI response, using fallback")
		return p.generateStructuredFallbackResponseV2(prompt, username), nil
	}

	log.Printf("[DEBUG] Successfully generated AI response: %d characters", len(cleanResponse))
	return cleanResponse, nil
}

// generateLLMResponseV2 attempts to use the real LLM infrastructure
// Extracted from generateLLMResponse
func (p *UnifiedProcessorV2Impl) generateLLMResponseV2(ctx context.Context, prompt string) (string, error) {
	// TODO: Integrate with the actual LLM client from internal/llm
	// For now, return error to trigger fallback
	// This will be implemented when LLM client is available
	return "", fmt.Errorf("LLM client not yet integrated with unified processor")
}

// generateStructuredFallbackResponseV2 provides structured response when LLM unavailable
// Extracted from generateStructuredFallbackResponse
func (p *UnifiedProcessorV2Impl) generateStructuredFallbackResponseV2(prompt, username string) string {
	response := &strings.Builder{}
	promptLower := strings.ToLower(prompt)

	response.WriteString(fmt.Sprintf("Thanks for your question, @%s! ", username))

	// Pattern matching for common developer questions
	if strings.Contains(promptLower, "error handling") {
		response.WriteString("Regarding error handling:\n\n")
		response.WriteString("**Key Considerations:**\n")
		response.WriteString("- Always check and handle potential errors explicitly\n")
		response.WriteString("- Consider using wrapped errors for better context\n")
		response.WriteString("- Ensure error paths are tested\n")
		response.WriteString("- Document expected error conditions\n\n")
		response.WriteString("Would you like me to elaborate on any specific aspect?")
	} else if strings.Contains(promptLower, "performance") {
		response.WriteString("Regarding performance considerations:\n\n")
		response.WriteString("**Optimization Areas:**\n")
		response.WriteString("- Profile before optimizing to identify bottlenecks\n")
		response.WriteString("- Consider algorithmic complexity improvements\n")
		response.WriteString("- Look for unnecessary allocations or operations\n")
		response.WriteString("- Implement caching where appropriate\n\n")
		response.WriteString("Happy to discuss specific performance patterns!")
	} else if strings.Contains(promptLower, "security") {
		response.WriteString("Regarding security considerations:\n\n")
		response.WriteString("**Security Best Practices:**\n")
		response.WriteString("- Validate and sanitize all input data\n")
		response.WriteString("- Use secure communication (HTTPS, encrypted storage)\n")
		response.WriteString("- Implement proper authentication and authorization\n")
		response.WriteString("- Follow principle of least privilege\n\n")
		response.WriteString("Let me know if you'd like to discuss specific security aspects!")
	} else {
		response.WriteString("I'm here to help with your code review question.\n\n")
		response.WriteString("**How I can assist:**\n")
		response.WriteString("- Code quality and best practices\n")
		response.WriteString("- Implementation suggestions\n")
		response.WriteString("- Error handling patterns\n")
		response.WriteString("- Performance optimization tips\n\n")
		response.WriteString("Feel free to ask specific questions about the code changes!")
	}

	return response.String()
}

// extractLearningMetadataV2 extracts learning metadata from AI responses
// Extracted from augmentResponseWithLearningMetadata
func (p *UnifiedProcessorV2Impl) extractLearningMetadataV2(ctx context.Context, responseContent string, event UnifiedWebhookEventV2) *LearningMetadataV2 {
	// Simple learning extraction - look for patterns that indicate learning opportunities
	responseLower := strings.ToLower(responseContent)

	var tags []string
	var action string = "explanation"

	// Detect learning categories
	if strings.Contains(responseLower, "error") || strings.Contains(responseLower, "bug") {
		tags = append(tags, "error-handling", "debugging")
	}
	if strings.Contains(responseLower, "performance") || strings.Contains(responseLower, "optimize") {
		tags = append(tags, "performance", "optimization")
	}
	if strings.Contains(responseLower, "security") || strings.Contains(responseLower, "vulnerable") {
		tags = append(tags, "security", "best-practices")
	}
	if strings.Contains(responseLower, "pattern") || strings.Contains(responseLower, "design") {
		tags = append(tags, "design-patterns", "architecture")
	}

	// Only create learning metadata if we found relevant tags
	if len(tags) == 0 {
		return nil
	}

	learning := &LearningMetadataV2{
		Type:       action,
		Content:    p.truncateString(responseContent, 500),
		Context:    fmt.Sprintf("Code Review Discussion: %s", event.Comment.Author.Username),
		Confidence: 0.8,
		Tags:       tags,
		Metadata: map[string]interface{}{
			"scope_kind": "merge_request",
			"repo_id":    event.Repository.ID,
		},
	}

	return learning
}

// Utility helper methods

// truncateString truncates a string to maxLength with ellipsis
func (p *UnifiedProcessorV2Impl) truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}
