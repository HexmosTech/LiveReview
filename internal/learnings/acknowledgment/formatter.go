package acknowledgment

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	coreprocessor "github.com/livereview/internal/core_processor"
)

// Format returns a markdown acknowledgment block for the provided learning metadata.
// When the learning includes a persisted short ID the output surfaces detailed metadata so
// reviewers can inspect exactly what was stored.
func Format(learning *coreprocessor.LearningMetadataV2) string {
	if learning == nil {
		return ""
	}

	meta := learning.Metadata
	shortID := metaText(meta, "short_id", "")
	title := metaText(meta, "title", "")

	if shortID == "" {
		if title != "" {
			return fmt.Sprintf("💡 *Learning captured: %s*", title)
		}
		return "💡 *Learning opportunity noted for future reference.*"
	}

	if title == "" {
		title = fmt.Sprintf("LR-%s", shortID)
	}

	scopeLabel := formatScope(metaText(meta, "scope_kind", "org"))
	repository := resolveRepository(meta)
	source := resolveSource(meta)
	summary := shortenText(learning.Content, 200)
	tags := formatTags(learning.Tags)
	confidence := confidenceText(learning.Confidence)

	detailLines := []string{fmt.Sprintf("ID: LR-%s", shortID), fmt.Sprintf("Scope: %s", scopeLabel)}

	if confidence != "" {
		detailLines = append(detailLines, fmt.Sprintf("Confidence: %s", confidence))
	}
	if tags != "" {
		detailLines = append(detailLines, fmt.Sprintf("Tags: %s", tags))
	}
	if repository != "" {
		detailLines = append(detailLines, fmt.Sprintf("Repository: %s", repository))
	}
	if source != "" {
		detailLines = append(detailLines, fmt.Sprintf("Source: %s", source))
	}
	if summary != "" {
		detailLines = append(detailLines, fmt.Sprintf("Summary: %s", summary))
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("💡 *Learning captured: [%s](#%s)*", title, shortID))
	builder.WriteString("\n\n```markdown\n")
	for _, line := range detailLines {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteString("```")

	return strings.TrimSpace(builder.String())
}

// metaText returns a trimmed string value from the metadata map or the provided fallback.
func metaText(metadata map[string]interface{}, key, fallback string) string {
	if metadata == nil {
		return fallback
	}

	value, ok := metadata[key]
	if !ok || value == nil {
		return fallback
	}

	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case fmt.Stringer:
		raw = v.String()
	default:
		raw = fmt.Sprintf("%v", v)
	}

	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback
	}

	return text
}

// formatScope normalizes scope metadata to a human readable label and defaults to Organization.
func formatScope(scope string) string {
	trimmed := strings.TrimSpace(strings.ToLower(scope))
	if trimmed == "" {
		return "Organization"
	}

	switch trimmed {
	case "org", "organization":
		return "Organization"
	case "repo", "repository":
		return "Repository"
	case "merge_request", "merge-request":
		return "Merge Request"
	case "project":
		return "Project"
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return "Organization"
	}

	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}

	return strings.Join(parts, " ")
}

// formatTags returns a printable representation of the tags slice.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	return strings.Join(tags, ", ")
}

// shortenText trims whitespace, applies a rune limit, and returns an empty string when no content.
func shortenText(value string, limit int) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}

	if limit <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}

	if limit <= 3 {
		return "..."
	}

	trimmed := strings.TrimSpace(string(runes[:limit-3]))
	if trimmed == "" {
		return ""
	}

	return trimmed + "..."
}

func resolveRepository(metadata map[string]interface{}) string {
	if repository := metaText(metadata, "repository", ""); repository != "" {
		return repository
	}

	if ctx, ok := metadata["source_context"].(map[string]interface{}); ok {
		if repo := metaText(ctx, "repository_full_name", ""); repo != "" {
			return repo
		}
		if repo := metaText(ctx, "repository", ""); repo != "" {
			return repo
		}
	}

	if ctx, ok := metadata["context"].(map[string]interface{}); ok {
		if repo := metaText(ctx, "repository", ""); repo != "" {
			return repo
		}
	}

	return ""
}

func resolveSource(metadata map[string]interface{}) string {
	if source := metaText(metadata, "extraction_method", ""); source != "" {
		return source
	}
	if source := metaText(metadata, "learning_source", ""); source != "" {
		return source
	}
	return ""
}

func confidenceText(confidence float64) string {
	if confidence <= 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(confidence, 'f', 2, 64), "0"), ".")
}
