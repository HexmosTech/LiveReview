package core_processor

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// ContextBuilderV2 defines the contract for building unified context
// components that are provider agnostic.
type ContextBuilderV2 interface {
	BuildTimeline(mr UnifiedMergeRequestV2, provider string) (*UnifiedTimelineV2, error)
	ExtractCommentContext(comment UnifiedCommentV2, timeline UnifiedTimelineV2) (*CommentContextV2, error)
	FindTargetComment(timeline UnifiedTimelineV2, commentID string) (*UnifiedCommentV2, error)
	BuildPrompt(context CommentContextV2, scenario ResponseScenarioV2) (string, error)
}

// UnifiedContextBuilderV2 implements the ContextBuilderV2 interface.
type UnifiedContextBuilderV2 struct{}

// NewUnifiedContextBuilderV2 creates a new unified context builder instance.
func NewUnifiedContextBuilderV2() ContextBuilderV2 {
	return &UnifiedContextBuilderV2{}
}

// BuildTimeline builds a unified timeline from MR data (provider-agnostic).
func (cb *UnifiedContextBuilderV2) BuildTimeline(mr UnifiedMergeRequestV2, provider string) (*UnifiedTimelineV2, error) {
	log.Printf("[DEBUG] Building unified timeline for %s MR %s", provider, mr.ID)

	timeline := &UnifiedTimelineV2{
		Items: []UnifiedTimelineItemV2{},
	}

	log.Printf("[DEBUG] Created empty timeline structure for population by %s provider", provider)
	return timeline, nil
}

// BuildTimelineFromData builds a unified timeline from provided commits and comments.
func (cb *UnifiedContextBuilderV2) BuildTimelineFromData(commits []UnifiedCommitV2, comments []UnifiedCommentV2) *UnifiedTimelineV2 {
	var items []UnifiedTimelineItemV2

	for _, commit := range commits {
		createdAt := cb.parseTimeBestEffortV2(commit.Timestamp)
		commitCopy := commit
		items = append(items, UnifiedTimelineItemV2{
			Type:      "commit",
			Timestamp: createdAt.Format(time.RFC3339),
			Commit:    &commitCopy,
		})
	}

	for _, comment := range comments {
		if comment.System {
			continue
		}
		createdAt := cb.parseTimeBestEffortV2(comment.CreatedAt)
		commentCopy := comment
		items = append(items, UnifiedTimelineItemV2{
			Type:      "comment",
			Timestamp: createdAt.Format(time.RFC3339),
			Comment:   &commentCopy,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		timeI := cb.parseTimeBestEffortV2(items[i].Timestamp)
		timeJ := cb.parseTimeBestEffortV2(items[j].Timestamp)
		return timeI.Before(timeJ)
	})

	return &UnifiedTimelineV2{
		Items: items,
	}
}

// ExtractCommentContext extracts context around a target comment.
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

	if len(beforeCommits) > 8 {
		beforeCommits = beforeCommits[len(beforeCommits)-8:]
	}

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
			Items: beforeTimeline,
		},
		RelatedComments: relatedComments,
		Metadata: map[string]interface{}{
			"after_timeline":      afterTimeline,
			"target_comment_time": targetTime.Format(time.RFC3339),
		},
	}

	return context, nil
}

// FindTargetComment locates a target comment in the timeline.
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

// BuildPrompt builds an enhanced prompt using unified context data.
func (cb *UnifiedContextBuilderV2) BuildPrompt(context CommentContextV2, scenario ResponseScenarioV2) (string, error) {
	var prompt strings.Builder

	prompt.WriteString("You are an AI code review assistant analyzing a development discussion.\n\n")

	if beforeCommits, ok := context.MRContext.Metadata["before_commits"].([]string); ok && len(beforeCommits) > 0 {
		prompt.WriteString("**Recent Commits:**\n")
		for _, commit := range beforeCommits {
			prompt.WriteString(fmt.Sprintf("- %s\n", commit))
		}
		prompt.WriteString("\n")
	}

	if beforeComments, ok := context.MRContext.Metadata["before_comments"].([]string); ok && len(beforeComments) > 0 {
		prompt.WriteString("**Thread Context:**\n")
		for _, comment := range beforeComments {
			prompt.WriteString(fmt.Sprintf("%s\n", comment))
		}
		prompt.WriteString("\n")
	}

	if context.CodeContext != "" {
		prompt.WriteString("**Code Context:**\n")
		prompt.WriteString(context.CodeContext)
		prompt.WriteString("\n\n")
	}

	if afterCommits, ok := context.MRContext.Metadata["after_commits"].([]string); ok && len(afterCommits) > 0 {
		prompt.WriteString("**Subsequent Changes:**\n")
		for _, commit := range afterCommits {
			prompt.WriteString(fmt.Sprintf("- %s\n", commit))
		}
		prompt.WriteString("\n")
	}

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

// ExtractCodeContext extracts code-specific context for positioned comments.
func (cb *UnifiedContextBuilderV2) ExtractCodeContext(comment UnifiedCommentV2, provider string) (string, error) {
	if comment.Position == nil {
		return "", nil
	}

	var context strings.Builder

	context.WriteString("**Code Location:**\n")
	context.WriteString(fmt.Sprintf("- File: %s\n", comment.Position.FilePath))

	if comment.Position.LineNumber > 0 {
		context.WriteString(fmt.Sprintf("- Line: %d\n", comment.Position.LineNumber))
	}

	if comment.Position.LineType != "" {
		context.WriteString(fmt.Sprintf("- Type: %s\n", comment.Position.LineType))
	}

	if metadata := comment.Metadata; metadata != nil {
		if diffHunk, ok := metadata["diff_hunk"].(string); ok && diffHunk != "" {
			context.WriteString("\n**Diff Context:**\n```diff\n")
			context.WriteString(diffHunk)
			context.WriteString("\n```\n")
		}

		if fileContent, ok := metadata["file_content"].(string); ok && fileContent != "" {
			context.WriteString("\n**File Content:**\n```\n")
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

func (cb *UnifiedContextBuilderV2) parseTimeBestEffortV2(timestamp string) time.Time {
	if timestamp == "" {
		return time.Time{}
	}

	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05 UTC",
		"2006-01-02 15:04:05 -0700",
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

func (cb *UnifiedContextBuilderV2) shortSHAV2(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

func (cb *UnifiedContextBuilderV2) truncateStringV2(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

func (cb *UnifiedContextBuilderV2) firstNonEmptyV2(strings ...string) string {
	for _, s := range strings {
		if s != "" {
			return s
		}
	}
	return ""
}

func (cb *UnifiedContextBuilderV2) minV2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AnalyzeCommentThread analyzes the context of a comment thread for better understanding.
func (cb *UnifiedContextBuilderV2) AnalyzeCommentThread(comment UnifiedCommentV2, relatedComments []UnifiedCommentV2) map[string]interface{} {
	analysis := make(map[string]interface{})

	analysis["thread_length"] = len(relatedComments) + 1
	analysis["is_continuation"] = len(relatedComments) > 0

	participants := make(map[string]bool)
	participants[comment.Author.Username] = true

	for _, related := range relatedComments {
		participants[related.Author.Username] = true
	}

	analysis["participant_count"] = len(participants)
	analysis["is_multi_participant"] = len(participants) > 2

	hasQuestions := strings.Contains(strings.ToLower(comment.Body), "?")
	for _, related := range relatedComments {
		if strings.Contains(strings.ToLower(related.Body), "?") {
			hasQuestions = true
			break
		}
	}
	analysis["has_questions"] = hasQuestions

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

// BuildEnhancedContext provides additional context analysis for advanced prompts.
func (cb *UnifiedContextBuilderV2) BuildEnhancedContext(comment UnifiedCommentV2, timeline UnifiedTimelineV2) (*CommentContextV2, error) {
	context, err := cb.ExtractCommentContext(comment, timeline)
	if err != nil {
		return nil, err
	}

	analysis := cb.AnalyzeCommentThread(comment, context.RelatedComments)
	context.Metadata["analysis"] = analysis

	return context, nil
}
