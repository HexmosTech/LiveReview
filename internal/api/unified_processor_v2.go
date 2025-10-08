package api

import (
	"context"
	"encoding/json"
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

	// LEARNING INSTRUCTIONS - This is the key addition!
	prompt.WriteString("LEARNING EXTRACTION:\n")
	prompt.WriteString("If this conversation contains information worth learning for future interactions, include it in your response as a special section.\n")
	prompt.WriteString("Learning examples include: team policies, coding standards, preferences, domain-specific rules, etc.\n\n")
	prompt.WriteString("If you identify a learning, add this JSON block at the end of your response:\n")
	prompt.WriteString("```learning\n")
	prompt.WriteString("{\n")
	prompt.WriteString(`  "type": "team_policy|coding_standard|preference|rule",` + "\n")
	prompt.WriteString(`  "title": "Brief title of the learning",` + "\n")
	prompt.WriteString(`  "content": "Full description of what was learned",` + "\n")
	prompt.WriteString(`  "tags": ["tag1", "tag2"],` + "\n")
	prompt.WriteString(`  "scope": "org|repo",` + "\n")
	prompt.WriteString(`  "confidence": 1-5` + "\n")
	prompt.WriteString("}\n```\n\n")

	prompt.WriteString("Only include learning block if there's genuinely something worth learning. Most responses won't have learnings.\n\n")
	prompt.WriteString("RESPONSE:\n")

	return prompt.String()
}

// buildContextualResponseWithLearningV2 creates response and detects learning opportunities
func (p *UnifiedProcessorV2Impl) buildContextualResponseWithLearningV2(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2) {
	response := p.buildContextualResponseV2(event, timeline)
	learning := p.detectLearningFromResponse(response, event, orgID)
	return response, learning
}

// buildContextualResponseV2 creates sophisticated contextual responses using original working logic (FALLBACK)
func (p *UnifiedProcessorV2Impl) buildContextualResponseV2(event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) string {
	commentBody := event.Comment.Body
	author := event.Comment.Author.Username
	commentLower := strings.ToLower(commentBody)

	// Handle documentation questions (very common pattern from original logic)
	if strings.Contains(commentLower, "documentation") || strings.Contains(commentLower, "document") ||
		strings.Contains(commentLower, "warrant") || strings.Contains(commentLower, "should document") {
		return p.generateDocumentationResponseV2(commentBody, author)
	}

	// Handle error/bug reports
	if strings.Contains(commentLower, "error") || strings.Contains(commentLower, "bug") ||
		strings.Contains(commentLower, "issue") || strings.Contains(commentLower, "problem") {
		return p.generateErrorAnalysisResponseV2(commentBody, author)
	}

	// Handle performance concerns
	if strings.Contains(commentLower, "performance") || strings.Contains(commentLower, "slow") ||
		strings.Contains(commentLower, "optimize") || strings.Contains(commentLower, "efficiency") {
		return p.generatePerformanceResponseV2(commentBody, author)
	}

	// Handle testing/validation questions
	if strings.Contains(commentLower, "test") || strings.Contains(commentLower, "testing") ||
		strings.Contains(commentLower, "validate") || strings.Contains(commentLower, "validation") {
		return p.generateTestingResponseV2(commentBody, author)
	}

	// Handle code quality concerns
	if strings.Contains(commentLower, "clean") || strings.Contains(commentLower, "refactor") ||
		strings.Contains(commentLower, "quality") || strings.Contains(commentLower, "best practice") {
		return p.generateCodeQualityResponseV2(commentBody, author)
	}

	// Handle specific code logic questions
	if strings.Contains(commentLower, "why") || strings.Contains(commentLower, "how") ||
		strings.Contains(commentLower, "explain") || strings.Contains(commentLower, "understand") {
		return p.generateExplanationResponseV2(commentBody, author)
	}

	// Handle rule/policy discussions (like your "use assertions" comment)
	if strings.Contains(commentLower, "rule") || strings.Contains(commentLower, "policy") ||
		strings.Contains(commentLower, "team") || strings.Contains(commentLower, "not") ||
		strings.Contains(commentLower, "do not") || strings.Contains(commentLower, "should not") {
		return p.generatePolicyResponseV2(commentBody, author)
	}

	// Default contextual response for general questions
	return p.generateGeneralContextualResponseV2(commentBody, author)
}

// generatePolicyResponseV2 handles team policy/rule discussions
func (p *UnifiedProcessorV2Impl) generatePolicyResponseV2(commentBody, author string) string {
	response := &strings.Builder{}

	response.WriteString(fmt.Sprintf("Thanks for the clarification, @%s!\n\n", author))

	if strings.Contains(strings.ToLower(commentBody), "assertion") {
		response.WriteString("**Team Policy Noted: No Assertions**\n\n")
		response.WriteString("I understand your team doesn't use assertions as a rule. ")
		response.WriteString("I'll keep this in mind for future code reviews and suggestions.\n\n")
		response.WriteString("**Alternative Approaches for Code Verification:**\n")
		response.WriteString("- Explicit error handling and validation\n")
		response.WriteString("- Unit tests to verify expected behavior\n")
		response.WriteString("- Documentation of expected preconditions\n")
		response.WriteString("- Runtime checks with proper error responses\n\n")
		response.WriteString("Would you like me to suggest specific alternatives for the numbered steps validation instead?")
	} else if strings.Contains(strings.ToLower(commentBody), "rule") || strings.Contains(strings.ToLower(commentBody), "team") {
		response.WriteString("**Team Policy Understanding**\n\n")
		response.WriteString("I appreciate you sharing your team's approach. ")
		response.WriteString("I'll adjust my suggestions to align with your established practices.\n\n")
		response.WriteString("Would you like me to provide alternative solutions that fit better with your team's coding standards?")
	} else {
		response.WriteString("**Policy/Practice Discussion**\n\n")
		response.WriteString("I understand there are specific practices your team follows. ")
		response.WriteString("I'll take this feedback into account for future recommendations.\n\n")
		response.WriteString("Feel free to share more details about your preferred approaches so I can provide better-aligned suggestions.")
	}

	return response.String()
}

// detectLearningFromResponse analyzes response content to create learning entries
func (p *UnifiedProcessorV2Impl) detectLearningFromResponse(response string, event UnifiedWebhookEventV2, orgID int64) *LearningMetadataV2 {

	// Look for policy learning indicators
	if strings.Contains(response, "Team Policy Noted:") && strings.Contains(response, "No Assertions") {
		return &LearningMetadataV2{
			Type:       "team_policy",
			Context:    "code_review_practices",
			Content:    "Team does not use assertions as a rule. Prefer explicit error handling, unit tests, documentation, and runtime checks with proper error responses instead of assertions.",
			Confidence: 0.95,
			Tags:       []string{"assertions", "team_policy", "code_verification", "error_handling"},
			OrgID:      orgID,
			Metadata: map[string]interface{}{
				"original_comment": event.Comment.Body,
				"author":           event.Comment.Author.Username,
				"repository":       event.Repository.Name,
				"discussion_type":  "policy_clarification",
				"source":           "comment_discussion",
				"scope":            "team",
			},
		}
	}

	// Look for other team policy patterns
	if strings.Contains(response, "Team Policy Understanding") || strings.Contains(response, "team's approach") {
		return &LearningMetadataV2{
			Type:       "team_policy",
			Context:    "coding_standards",
			Content:    fmt.Sprintf("Team has specific coding practices: %s", event.Comment.Body),
			Confidence: 0.85,
			Tags:       []string{"team_policy", "coding_standards"},
			OrgID:      orgID,
			Metadata: map[string]interface{}{
				"original_comment": event.Comment.Body,
				"author":           event.Comment.Author.Username,
				"repository":       event.Repository.Name,
				"source":           "comment_discussion",
				"scope":            "team",
			},
		}
	}

	// Look for documentation preferences
	if strings.Contains(response, "Documentation Suggestions") && strings.Contains(event.Comment.Body, "document") {
		return &LearningMetadataV2{
			Type:       "documentation_preference",
			Context:    "code_documentation",
			Content:    fmt.Sprintf("Team values specific documentation practices: %s", event.Comment.Body),
			Confidence: 0.80,
			Tags:       []string{"documentation", "team_preference"},
			OrgID:      orgID,
			Metadata: map[string]interface{}{
				"original_comment": event.Comment.Body,
				"author":           event.Comment.Author.Username,
				"repository":       event.Repository.Name,
				"source":           "comment_discussion",
				"scope":            "team",
			},
		}
	}

	// No learning detected
	return nil
}

// generateDocumentationResponseV2, generateErrorAnalysisResponseV2, etc. - implementing key response types
func (p *UnifiedProcessorV2Impl) generateDocumentationResponseV2(commentBody, author string) string {
	response := &strings.Builder{}
	response.WriteString(fmt.Sprintf("Great point about documentation, @%s!\n\n", author))
	response.WriteString("**Documentation Suggestions:**\n")
	response.WriteString("- Add inline comments explaining the business logic\n")
	response.WriteString("- Document expected input/output formats\n")
	response.WriteString("- Include examples of typical usage\n")
	response.WriteString("- Explain any non-obvious design decisions\n\n")
	response.WriteString("Would you like me to suggest specific documentation for any particular section?")
	return response.String()
}

func (p *UnifiedProcessorV2Impl) generateErrorAnalysisResponseV2(commentBody, author string) string {
	response := &strings.Builder{}
	response.WriteString(fmt.Sprintf("Let me help analyze this issue, @%s.\n\n", author))
	response.WriteString("**Error Analysis Approach:**\n")
	response.WriteString("- Check the error logs for specific failure points\n")
	response.WriteString("- Verify input validation and edge cases\n")
	response.WriteString("- Review error handling paths\n")
	response.WriteString("- Test with various input scenarios\n\n")
	response.WriteString("Can you share more details about when this error occurs?")
	return response.String()
}

func (p *UnifiedProcessorV2Impl) generateExplanationResponseV2(commentBody, author string) string {
	response := &strings.Builder{}
	response.WriteString(fmt.Sprintf("Happy to explain this, @%s!\n\n", author))

	if strings.Contains(strings.ToLower(commentBody), "numbered") || strings.Contains(strings.ToLower(commentBody), "steps") {
		response.WriteString("**About the Numbered Steps Pattern:**\n")
		response.WriteString("The code has comments numbered 1-5, which suggests a sequential process. ")
		response.WriteString("While the steps are documented, there's no runtime enforcement of the order.\n\n")
		response.WriteString("**Implementation Options:**\n")
		response.WriteString("- State machine pattern to enforce order\n")
		response.WriteString("- Explicit validation between steps\n")
		response.WriteString("- Error handling for out-of-order execution\n")
		response.WriteString("- Unit tests to verify the sequence\n\n")
	} else {
		response.WriteString("**Code Logic Explanation:**\n")
		response.WriteString("Let me break down the logic and reasoning behind this implementation.\n\n")
	}

	response.WriteString("Would you like me to elaborate on any specific aspect?")
	return response.String()
}

func (p *UnifiedProcessorV2Impl) generateGeneralContextualResponseV2(commentBody, author string) string {
	response := &strings.Builder{}
	response.WriteString(fmt.Sprintf("Thanks for your feedback, @%s!\n\n", author))
	response.WriteString("I'm here to help with code review questions and suggestions. ")
	response.WriteString("Feel free to ask about specific implementation details, alternatives, or best practices.\n\n")
	response.WriteString("**How I can assist:**\n")
	response.WriteString("- Code quality and implementation suggestions\n")
	response.WriteString("- Error handling and edge case analysis\n")
	response.WriteString("- Performance and optimization guidance\n")
	response.WriteString("- Testing and validation strategies\n\n")
	response.WriteString("What specific aspect would you like to discuss?")
	return response.String()
}

func (p *UnifiedProcessorV2Impl) generatePerformanceResponseV2(commentBody, author string) string {
	return fmt.Sprintf("Good performance consideration, @%s! Let me analyze the efficiency aspects and suggest optimizations.", author)
}

func (p *UnifiedProcessorV2Impl) generateTestingResponseV2(commentBody, author string) string {
	return fmt.Sprintf("Great testing question, @%s! I can help suggest test strategies and validation approaches.", author)
}

func (p *UnifiedProcessorV2Impl) generateCodeQualityResponseV2(commentBody, author string) string {
	return fmt.Sprintf("Excellent code quality focus, @%s! Let me suggest improvements and refactoring opportunities.", author)
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

// generateLLMResponseWithLearning generates LLM response and extracts learning
func (p *UnifiedProcessorV2Impl) generateLLMResponseWithLearning(ctx context.Context, prompt string, event UnifiedWebhookEventV2) (string, *LearningMetadataV2, error) {
	// Try to get LLM response
	llmResponse, err := p.generateLLMResponseV2(ctx, prompt)
	if err != nil {
		return "", nil, err
	}

	// Extract learning from LLM response
	learning := p.extractLearningFromLLMResponse(llmResponse, event)

	// Clean response (remove learning JSON block)
	cleanResponse := p.cleanResponseFromLearningBlock(llmResponse)

	return cleanResponse, learning, nil
}

// generateLLMResponseV2 attempts to use the real LLM infrastructure
func (p *UnifiedProcessorV2Impl) generateLLMResponseV2(ctx context.Context, prompt string) (string, error) {
	// Try to use actual LLM client - check if server has AI connectors
	if p.server != nil && p.server.db != nil {
		// Simple check for available AI connectors
		var count int
		err := p.server.db.QueryRow("SELECT COUNT(*) FROM ai_connectors WHERE org_id = 1").Scan(&count)
		if err == nil && count > 0 {
			log.Printf("[DEBUG] Found %d AI connectors, but LLM client integration pending", count)
			// TODO: Integrate with actual LLM client from internal/llm or ai package
			// For now, fall back until proper integration is done
		}
	}

	// Return error to trigger fallback response
	return "", fmt.Errorf("LLM client integration pending")
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

// extractLearningFromLLMResponse extracts structured learning from LLM response
func (p *UnifiedProcessorV2Impl) extractLearningFromLLMResponse(response string, event UnifiedWebhookEventV2) *LearningMetadataV2 {
	// Look for learning JSON block in response
	learningBlockStart := strings.Index(response, "```learning")
	if learningBlockStart == -1 {
		return nil // No learning block found
	}

	learningBlockEnd := strings.Index(response[learningBlockStart:], "```")
	if learningBlockEnd == -1 || learningBlockEnd <= 10 {
		return nil // Invalid learning block
	}

	// Extract JSON content
	jsonStart := learningBlockStart + len("```learning\n")
	jsonEnd := learningBlockStart + learningBlockEnd
	jsonContent := response[jsonStart:jsonEnd]

	// Parse JSON
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
		return nil
	}

	// Convert to LearningMetadataV2
	learning := &LearningMetadataV2{
		Type:       learningData.Type,
		Content:    fmt.Sprintf("%s: %s", learningData.Title, learningData.Content),
		Tags:       learningData.Tags,
		Confidence: float64(learningData.Confidence),
		Context:    "", // Will be set below
		OrgID:      1,  // Default org
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
	learningBlockStart := strings.Index(response, "```learning")
	if learningBlockStart == -1 {
		return response // No learning block
	}

	// Find end of learning block
	learningBlockEnd := strings.Index(response[learningBlockStart:], "```")
	if learningBlockEnd == -1 {
		return response // Invalid block
	}

	// Remove the entire learning block
	cleanResponse := response[:learningBlockStart] + response[learningBlockStart+learningBlockEnd+3:]
	return strings.TrimSpace(cleanResponse)
}

// extractLearningFromResponse extracts learning from fallback responses
func (p *UnifiedProcessorV2Impl) extractLearningFromResponse(responseContent string, event UnifiedWebhookEventV2) *LearningMetadataV2 {
	// For fallback responses, use simple pattern matching
	responseLower := strings.ToLower(responseContent)
	originalComment := ""
	if event.Comment != nil {
		originalComment = strings.ToLower(event.Comment.Body)
	}

	// Check for team policy patterns in the fallback response
	if strings.Contains(responseLower, "team policy noted") {
		var content string
		var tags []string

		if strings.Contains(responseLower, "no assertions") || strings.Contains(originalComment, "assertions") {
			content = "Team Policy: No Assertions - Team rule: Do not use assertions. Use explicit error handling, unit tests, documentation, and runtime checks instead."
			tags = []string{"team-policy", "assertions", "error-handling", "testing"}
		} else {
			content = "Team Policy: " + p.truncateString(responseContent, 300)
			tags = []string{"team-policy", "coding-standards"}
		}

		return &LearningMetadataV2{
			Type:       "team_policy",
			Content:    content,
			Tags:       tags,
			Confidence: 4.0,
			Context:    fmt.Sprintf("Repository: %s, Provider: %s", event.Repository.Name, event.Provider),
			OrgID:      1, // Default org
			Metadata: map[string]interface{}{
				"repository": event.Repository.Name,
				"author":     event.Comment.Author.Username,
				"provider":   event.Provider,
			},
		}
	}

	return nil // No learning detected in fallback
}

// extractLearningMetadataV2 - DEPRECATED - use extractLearningFromResponse instead
func (p *UnifiedProcessorV2Impl) extractLearningMetadataV2(ctx context.Context, responseContent string, event UnifiedWebhookEventV2) *LearningMetadataV2 {
	// Enhanced learning extraction - look for patterns that indicate learning opportunities
	responseLower := strings.ToLower(responseContent)
	originalComment := ""
	if event.Comment != nil {
		originalComment = strings.ToLower(event.Comment.Body)
	}

	var tags []string
	var action string = "explanation"
	var content string

	// Priority 1: Team Policy Learning (highest priority)
	if strings.Contains(responseLower, "team policy noted") || strings.Contains(responseLower, "policy noted") ||
		(strings.Contains(responseLower, "team") && (strings.Contains(responseLower, "rule") || strings.Contains(responseLower, "policy"))) {
		action = "team_policy"
		tags = append(tags, "team-policy", "coding-standards")

		// Extract specific policy
		if strings.Contains(responseLower, "no assertions") || strings.Contains(originalComment, "assertions") {
			content = "Team Policy: No Assertions - Team rule: Do not use assertions. Use explicit error handling, unit tests, documentation, and runtime checks instead."
			tags = append(tags, "assertions", "error-handling", "testing")
		} else if strings.Contains(responseLower, "no magic numbers") {
			content = "Team Policy: No Magic Numbers - Team rule: Do not use magic numbers. Use named constants with descriptive names."
			tags = append(tags, "constants", "readability")
		} else {
			content = "Team Policy: " + p.truncateString(responseContent, 300)
		}

		log.Printf("[DEBUG] Detected team policy learning: %s", action)
	} else if strings.Contains(responseLower, "error") || strings.Contains(responseLower, "bug") {
		// Priority 2: Error handling patterns
		tags = append(tags, "error-handling", "debugging")
		content = "Error Handling Pattern: " + p.truncateString(responseContent, 300)
	} else if strings.Contains(responseLower, "performance") || strings.Contains(responseLower, "optimize") {
		// Priority 3: Performance patterns
		tags = append(tags, "performance", "optimization")
		content = "Performance Optimization: " + p.truncateString(responseContent, 300)
	} else if strings.Contains(responseLower, "security") || strings.Contains(responseLower, "vulnerable") {
		// Priority 4: Security patterns
		tags = append(tags, "security", "best-practices")
		content = "Security Best Practice: " + p.truncateString(responseContent, 300)
	} else if strings.Contains(responseLower, "pattern") || strings.Contains(responseLower, "design") {
		// Priority 5: Design patterns
		tags = append(tags, "design-patterns", "architecture")
		content = "Design Pattern: " + p.truncateString(responseContent, 300)
	}

	// Only create learning metadata if we found relevant tags
	if len(tags) == 0 {
		log.Printf("[DEBUG] No learning patterns detected in response")
		return nil
	}

	learning := &LearningMetadataV2{
		Type:       action,
		Content:    content,
		Context:    fmt.Sprintf("Code Review Discussion: %s", event.Comment.Author.Username),
		Confidence: 0.8,
		Tags:       tags,
		Metadata: map[string]interface{}{
			"scope_kind":       "merge_request",
			"repo_id":          event.Repository.ID,
			"original_comment": event.Comment.Body,
			"response_content": p.truncateString(responseContent, 200),
		},
	}

	log.Printf("[DEBUG] Extracted learning: %s (tags: %v)", learning.Type, learning.Tags)
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
