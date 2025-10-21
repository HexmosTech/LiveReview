package review

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/livereview/internal/learnings/acknowledgment"
)

// appendLearningAcknowledgment removes any embedded learning block from the content and appends a
// formatted acknowledgment so user-facing comments stay readable while still surfacing the learning.
func appendLearningAcknowledgment(content string) string {
	cleaned, learning := extractLearningMetadata(content)
	if learning == nil {
		return strings.TrimSpace(cleaned)
	}

	ack := acknowledgment.Format(learning)
	if ack == "" {
		return strings.TrimSpace(cleaned)
	}

	trimmed := strings.TrimSpace(cleaned)
	if trimmed == "" {
		return ack
	}

	return trimmed + "\n\n" + ack
}

// extractLearningMetadata strips the learning block from the comment content and returns the cleaned
// response along with reconstructed learning metadata when the block parses successfully.
func extractLearningMetadata(content string) (string, *coreprocessor.LearningMetadataV2) {
	blockStart, markerLength := findLearningBlock(content)
	if blockStart == -1 {
		return content, nil
	}

	afterMarker := content[blockStart+markerLength:]
	newlineIdx := strings.Index(afterMarker, "\n")
	if newlineIdx == -1 {
		return content, nil
	}

	jsonStart := blockStart + markerLength + newlineIdx + 1
	remaining := content[jsonStart:]
	endIdx := strings.Index(remaining, "```")
	if endIdx == -1 {
		return content, nil
	}

	jsonPayload := strings.TrimSpace(remaining[:endIdx])
	cleaned := strings.TrimSpace(content[:blockStart] + remaining[endIdx+3:])

	learning := parseLearningPayload(jsonPayload)
	return cleaned, learning
}

// findLearningBlock locates the first supported fenced code block containing learning metadata.
func findLearningBlock(content string) (start int, markerLength int) {
	markers := []string{"```learning", "```json"}
	for _, marker := range markers {
		idx := strings.Index(content, marker)
		if idx != -1 {
			return idx, len(marker)
		}
	}
	return -1, 0
}

// parseLearningPayload converts the JSON payload inside the learning block into LearningMetadataV2.
func parseLearningPayload(raw string) *coreprocessor.LearningMetadataV2 {
	if raw == "" {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}

	typeValue := stringValue(payload["type"])
	contentValue := stringValue(payload["content"])
	if contentValue == "" {
		contentValue = stringValue(payload["title"])
	}

	learning := &coreprocessor.LearningMetadataV2{
		Type:       typeValue,
		Content:    contentValue,
		Tags:       stringSlice(payload["tags"]),
		Confidence: floatValue(payload["confidence"]),
		Metadata: map[string]interface{}{
			"title":             stringValue(payload["title"]),
			"scope_kind":        stringValue(payload["scope"]),
			"extraction_method": "embedded_learning_block",
			"repository":        stringValue(payload["repository"]),
			"original_comment":  stringValue(payload["original_comment"]),
			"learning_source":   stringValue(payload["source"]),
			"short_id":          stringValue(payload["short_id"]),
		},
	}

	return learning
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), "."))
	case int, int32, int64:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	default:
		return ""
	}
}

func floatValue(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		if num, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return num
		}
	}
	return 0
}

func stringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringValue(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	case nil:
		return nil
	default:
		if s := stringValue(v); s != "" {
			return []string{s}
		}
	}
	return nil
}
