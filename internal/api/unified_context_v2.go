package api

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// Phase 7.2: Unified context builder for provider-agnostic context processing
// Extracted from webhook_handler.go and provider-specific files

// UnifiedContextBuilderV2 implements the ContextBuilderV2 interface
type UnifiedContextBuilderV2 struct {
	server *Server // For accessing database operations and helper functions
}

// NewUnifiedContextBuilderV2 creates a new unified context builder instance
func NewUnifiedContextBuilderV2(server *Server) ContextBuilderV2 {
	return &UnifiedContextBuilderV2{
		server: server,
	}
}

// BuildTimeline builds a unified timeline from MR data (provider-agnostic)
// Extracted from buildTimeline, buildTimelineV2, buildGitHubTimeline, buildBitbucketTimeline
func (cb *UnifiedContextBuilderV2) BuildTimeline(mr UnifiedMergeRequestV2, provider string) (*UnifiedTimelineV2, error) {
	log.Printf("[DEBUG] Building unified timeline for %s MR %s", provider, mr.ID)

	// The actual implementation depends on fetching data from providers
	// For now, we'll create a basic timeline structure that can be populated by providers
	timeline := &UnifiedTimelineV2{
		Items: []UnifiedTimelineItemV2{},
	}

	// This method should be called by providers after they fetch their specific data
	// The providers will populate the timeline and then call this to unify the format

	log.Printf("[DEBUG] Created empty timeline structure for population by %s provider", provider)
	return timeline, nil
}

// BuildTimelineFromData builds a unified timeline from provided commits and comments
// This is a helper method that providers can use to build timeline after fetching their data
func (cb *UnifiedContextBuilderV2) BuildTimelineFromData(commits []UnifiedCommitV2, comments []UnifiedCommentV2) *UnifiedTimelineV2 {
	var items []UnifiedTimelineItemV2

	// Add commits to timeline
	for _, commit := range commits {
		createdAt := cb.parseTimeBestEffortV2(commit.Timestamp)
		items = append(items, UnifiedTimelineItemV2{
			Type:      "commit",
			Timestamp: createdAt.Format(time.RFC3339),
			Commit:    &commit,
		})
	}

	// Add comments to timeline
	for _, comment := range comments {
		// Skip system comments
		if comment.System {
			continue
		}
		createdAt := cb.parseTimeBestEffortV2(comment.CreatedAt)
		items = append(items, UnifiedTimelineItemV2{
			Type:      "comment",
			Timestamp: createdAt.Format(time.RFC3339),
			Comment:   &comment,
		})
	}

	// Sort timeline by creation time
	sort.Slice(items, func(i, j int) bool {
		timeI := cb.parseTimeBestEffortV2(items[i].Timestamp)
		timeJ := cb.parseTimeBestEffortV2(items[j].Timestamp)
		return timeI.Before(timeJ)
	})

	return &UnifiedTimelineV2{
		Items: items,
	}
}

// ExtractCommentContext extracts context around a target comment
// Extracted from extractCommentContext, extractCommentContextV2
func (cb *UnifiedContextBuilderV2) ExtractCommentContext(comment UnifiedCommentV2, timeline UnifiedTimelineV2) (*CommentContextV2, error) {
	log.Printf("[DEBUG] Extracting context for comment %s", comment.ID)

	targetTime := cb.parseTimeBestEffortV2(comment.CreatedAt)

	beforeCommits := []string{}
	beforeComments := []string{}
	afterCommits := []string{}
	afterComments := []string{}

	beforeTimeline := []UnifiedTimelineItemV2{}
	afterTimeline := []UnifiedTimelineItemV2{}
	relatedComments := []UnifiedCommentV2{}

	// Process timeline items to separate before/after context
	for _, item := range timeline.Items {
		itemTime := cb.parseTimeBestEffortV2(item.Timestamp)

		if item.Type == "commit" && item.Commit != nil {
			commitLine := fmt.Sprintf("%s — %s",
				cb.shortSHAV2(item.Commit.SHA),
				item.Commit.Message)

			if targetTime.IsZero() || !itemTime.After(targetTime) {
				beforeCommits = append(beforeCommits, commitLine)
				beforeTimeline = append(beforeTimeline, item)
			} else {
				afterCommits = append(afterCommits, commitLine)
				afterTimeline = append(afterTimeline, item)
			}
		} else if item.Type == "comment" && item.Comment != nil {
			commentLine := fmt.Sprintf("[%s] %s: %s",
				item.Timestamp,
				item.Comment.Author.Username,
				cb.truncateStringV2(item.Comment.Body, 100))

			if item.Comment.ID == comment.ID {
				// This is the target comment itself
				beforeComments = append(beforeComments, commentLine)
			} else if targetTime.IsZero() || !itemTime.After(targetTime) {
				beforeComments = append(beforeComments, commentLine)
				beforeTimeline = append(beforeTimeline, item)
				relatedComments = append(relatedComments, *item.Comment)
			} else {
				afterComments = append(afterComments, commentLine)
				afterTimeline = append(afterTimeline, item)
				relatedComments = append(relatedComments, *item.Comment)
			}
		}
	}

	// Limit before commits to last 8 entries (like original implementation)
	if len(beforeCommits) > 8 {
		beforeCommits = beforeCommits[len(beforeCommits)-8:]
	}

	// Create the context structure using the actual V2 types
	context := &CommentContextV2{
		MRContext: UnifiedMRContextV2{
			Metadata: map[string]interface{}{
				"before_commits":  beforeCommits,
				"before_comments": beforeComments,
				"after_commits":   afterCommits,
				"after_comments":  afterComments,
			},
		},
		Timeline: UnifiedTimelineV2{
			Items: beforeTimeline, // Primary timeline with before context
		},
		RelatedComments: relatedComments,
		Metadata: map[string]interface{}{
			"after_timeline":      afterTimeline,
			"target_comment_time": targetTime.Format(time.RFC3339),
		},
	}

	return context, nil
} // FindTargetComment locates a target comment in the timeline
// Extracted from findTargetComment, findTargetCommentV2
func (cb *UnifiedContextBuilderV2) FindTargetComment(timeline UnifiedTimelineV2, commentID string) (*UnifiedCommentV2, error) {
	log.Printf("[DEBUG] Finding target comment %s in timeline", commentID)

	for _, item := range timeline.Items {
		if item.Type == "comment" && item.Comment != nil {
			if item.Comment.ID == commentID {
				log.Printf("[DEBUG] Found target comment %s", commentID)
				return item.Comment, nil
			}
		}
	}

	return nil, fmt.Errorf("target comment %s not found in timeline", commentID)
}

// BuildPrompt builds an enhanced prompt using unified context data
// Extracted from buildGeminiPromptEnhanced, buildGitHubEnhancedPrompt, buildBitbucketEnhancedPrompt
func (cb *UnifiedContextBuilderV2) BuildPrompt(context CommentContextV2, scenario ResponseScenarioV2) (string, error) {
	var prompt strings.Builder

	// Context header
	prompt.WriteString("You are an AI code review assistant analyzing a development discussion.\n\n")

	// MR Context from metadata
	if beforeCommits, ok := context.MRContext.Metadata["before_commits"].([]string); ok && len(beforeCommits) > 0 {
		prompt.WriteString("**Recent Commits:**\n")
		for _, commit := range beforeCommits {
			prompt.WriteString(fmt.Sprintf("- %s\n", commit))
		}
		prompt.WriteString("\n")
	}

	// Discussion Context from metadata
	if beforeComments, ok := context.MRContext.Metadata["before_comments"].([]string); ok && len(beforeComments) > 0 {
		prompt.WriteString("**Thread Context:**\n")
		for _, comment := range beforeComments {
			prompt.WriteString(fmt.Sprintf("%s\n", comment))
		}
		prompt.WriteString("\n")
	}

	// Code Context if available
	if context.CodeContext != "" {
		prompt.WriteString("**Code Context:**\n")
		prompt.WriteString(context.CodeContext)
		prompt.WriteString("\n\n")
	}

	// Future commits/comments for additional context
	if afterCommits, ok := context.MRContext.Metadata["after_commits"].([]string); ok && len(afterCommits) > 0 {
		prompt.WriteString("**Subsequent Changes:**\n")
		for _, commit := range afterCommits {
			prompt.WriteString(fmt.Sprintf("- %s\n", commit))
		}
		prompt.WriteString("\n")
	}

	// Response guidance based on scenario
	prompt.WriteString("**Response Guidelines:**\n")
	switch scenario.Type {
	case "bot_reply":
		prompt.WriteString("- This is a follow-up to a previous bot response\n")
		prompt.WriteString("- Address any clarifications or additional questions\n")
	case "direct_mention":
		prompt.WriteString("- User directly mentioned the bot, expects a response\n")
		prompt.WriteString("- Provide helpful, actionable guidance\n")
	case "content_trigger":
		prompt.WriteString("- Comment contains help/question keywords\n")
		prompt.WriteString("- Assess if response adds value to the discussion\n")
	default:
		prompt.WriteString("- Provide contextual, technical guidance\n")
	}

	prompt.WriteString("- Maintain professional, collaborative tone\n")
	prompt.WriteString("- Focus on code quality and best practices\n")
	prompt.WriteString("- Be concise but thorough\n\n")

	return prompt.String(), nil
}

// ExtractCodeContext extracts code-specific context for positioned comments
// This is a provider-agnostic wrapper that delegates to provider-specific implementations
func (cb *UnifiedContextBuilderV2) ExtractCodeContext(comment UnifiedCommentV2, provider string) (string, error) {
	if comment.Position == nil {
		return "", nil // No code position, no code context
	}

	var context strings.Builder

	// Basic code context information
	context.WriteString("**Code Location:**\n")
	context.WriteString(fmt.Sprintf("- File: %s\n", comment.Position.FilePath))

	if comment.Position.LineNumber > 0 {
		context.WriteString(fmt.Sprintf("- Line: %d\n", comment.Position.LineNumber))
	}

	if comment.Position.LineType != "" {
		context.WriteString(fmt.Sprintf("- Type: %s\n", comment.Position.LineType))
	}

	// Additional context from metadata if available
	if metadata := comment.Metadata; metadata != nil {
		if diffHunk, ok := metadata["diff_hunk"].(string); ok && diffHunk != "" {
			context.WriteString("\n**Diff Context:**\n```diff\n")
			context.WriteString(diffHunk)
			context.WriteString("\n```\n")
		}

		if fileContent, ok := metadata["file_content"].(string); ok && fileContent != "" {
			context.WriteString("\n**File Content:**\n```\n")
			// Limit file content to reasonable size
			if len(fileContent) > 1000 {
				context.WriteString(fileContent[:1000])
				context.WriteString("\n... (content truncated)\n")
			} else {
				context.WriteString(fileContent)
			}
			context.WriteString("\n```\n")
		}
	}

	return context.String(), nil
}

// Helper methods (extracted from various files)

// parseTimeBestEffortV2 parses timestamp with best effort approach
// Extracted from parseTimeBestEffort, parseTimeBestEffortV2
func (cb *UnifiedContextBuilderV2) parseTimeBestEffortV2(timestamp string) time.Time {
	if timestamp == "" {
		return time.Time{}
	}

	// Try different timestamp formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t
		}
	}

	log.Printf("[WARN] Failed to parse timestamp %s, using current time", timestamp)
	return time.Now()
}

// shortSHAV2 returns a short version of a commit SHA
// Extracted from shortSHA, shortSHAV2
func (cb *UnifiedContextBuilderV2) shortSHAV2(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

// truncateStringV2 truncates a string to maxLength with ellipsis
func (cb *UnifiedContextBuilderV2) truncateStringV2(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

// firstNonEmptyV2 returns the first non-empty string from the provided strings
// Extracted from firstNonEmpty, firstNonEmptyV2
func (cb *UnifiedContextBuilderV2) firstNonEmptyV2(strings ...string) string {
	for _, s := range strings {
		if s != "" {
			return s
		}
	}
	return ""
}

// minV2 returns the minimum of two integers
func (cb *UnifiedContextBuilderV2) minV2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Advanced context analysis methods

// AnalyzeCommentThread analyzes the context of a comment thread for better understanding
func (cb *UnifiedContextBuilderV2) AnalyzeCommentThread(comment UnifiedCommentV2, relatedComments []UnifiedCommentV2) map[string]interface{} {
	analysis := make(map[string]interface{})

	// Thread statistics
	analysis["thread_length"] = len(relatedComments) + 1 // +1 for the target comment
	analysis["is_continuation"] = len(relatedComments) > 0

	// Participant analysis
	participants := make(map[string]bool)
	participants[comment.Author.Username] = true

	for _, related := range relatedComments {
		participants[related.Author.Username] = true
	}

	analysis["participant_count"] = len(participants)
	analysis["is_multi_participant"] = len(participants) > 2

	// Content analysis
	hasQuestions := strings.Contains(strings.ToLower(comment.Body), "?")
	for _, related := range relatedComments {
		if strings.Contains(strings.ToLower(related.Body), "?") {
			hasQuestions = true
			break
		}
	}
	analysis["has_questions"] = hasQuestions

	// Urgency indicators
	urgentKeywords := []string{"urgent", "asap", "immediately", "critical", "blocking"}
	hasUrgency := false

	commentLower := strings.ToLower(comment.Body)
	for _, keyword := range urgentKeywords {
		if strings.Contains(commentLower, keyword) {
			hasUrgency = true
			break
		}
	}
	analysis["has_urgency"] = hasUrgency

	return analysis
}

// BuildEnhancedContext builds enhanced context with timeline analysis
func (cb *UnifiedContextBuilderV2) BuildEnhancedContext(comment UnifiedCommentV2, timeline UnifiedTimelineV2) (*CommentContextV2, error) {
	// First get basic context
	basicContext, err := cb.ExtractCommentContext(comment, timeline)
	if err != nil {
		return nil, err
	}

	// Enhance with code context
	codeContext, err := cb.ExtractCodeContext(comment, "unified")
	if err != nil {
		log.Printf("[WARN] Failed to extract code context: %v", err)
		codeContext = ""
	}
	basicContext.CodeContext = codeContext

	// Add thread analysis
	threadAnalysis := cb.AnalyzeCommentThread(comment, basicContext.RelatedComments)
	if basicContext.MRContext.Metadata == nil {
		basicContext.MRContext.Metadata = make(map[string]interface{})
	}
	basicContext.MRContext.Metadata["thread_analysis"] = threadAnalysis

	return basicContext, nil
}
