package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/livereview/internal/logging"
)

// ProcessorResult contains the result of LLM response processing
type ProcessorResult struct {
	ParsedData   interface{}     `json:"parsed_data"`
	RepairStats  JsonRepairStats `json:"repair_stats"`
	OriginalJSON string          `json:"-"` // Don't marshal raw JSON
	RepairedJSON string          `json:"-"` // Don't marshal repaired JSON
	Success      bool            `json:"success"`
	Error        string          `json:"error,omitempty"`
}

// ProcessLLMResponse processes a raw LLM response, attempting repair if needed
func ProcessLLMResponse(raw string, target interface{}) (ProcessorResult, error) {
	logger := logging.GetCurrentLogger()

	result := ProcessorResult{
		OriginalJSON: raw,
		Success:      false,
	}

	// Log the original response for debugging
	if logger != nil {
		logger.Log("Processing LLM response (%d bytes)", len(raw))
	}

	// Extract JSON from response (handle cases where LLM adds explanatory text)
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		result.Error = "no JSON found in LLM response"
		if logger != nil {
			logger.Log("No JSON found in LLM response: %s", truncateForLog(raw, 200))
		}
		return result, fmt.Errorf("no JSON found in response")
	}

	// Attempt to repair JSON if needed
	repairedJSON, repairStats, err := RepairJSON(jsonStr)
	result.RepairStats = repairStats
	result.RepairedJSON = repairedJSON

	// Log repair statistics if repair was attempted
	if repairStats.WasRepaired {
		if logger != nil {
			logger.Log("JSON repair applied: %d strategies, %d errors fixed, %d comments lost, repair time: %v",
				len(repairStats.RepairStrategies), repairStats.ErrorsFixed, repairStats.CommentsLost, repairStats.RepairTime)
			logger.Log("Repair strategies used: %s", strings.Join(repairStats.RepairStrategies, ", "))
		}
	}

	if err != nil {
		result.Error = fmt.Sprintf("JSON repair failed: %v", err)
		if logger != nil {
			logger.Log("JSON repair failed: %v", err)
			logger.Log("Original JSON: %s", truncateForLog(jsonStr, 500))
			logger.Log("Repaired JSON: %s", truncateForLog(repairedJSON, 500))
		}
		return result, err
	}

	// Parse the repaired JSON
	if err := json.Unmarshal([]byte(repairedJSON), target); err != nil {
		result.Error = fmt.Sprintf("JSON parsing failed after repair: %v", err)
		if logger != nil {
			logger.Log("JSON parsing failed after repair: %v", err)
			logger.Log("Final JSON: %s", truncateForLog(repairedJSON, 500))
		}
		return result, err
	}

	result.ParsedData = target
	result.Success = true

	// Log successful processing
	if logger != nil {
		if repairStats.WasRepaired {
			logger.Log("LLM response successfully processed with repair (%d -> %d bytes)",
				repairStats.OriginalBytes, repairStats.RepairedBytes)
		} else {
			logger.Log("LLM response successfully processed without repair (%d bytes)", len(raw))
		}
	}

	return result, nil
}

// extractJSON extracts JSON content from mixed text/JSON responses
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)

	// If it starts with { or [, assume it's pure JSON
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		return raw
	}

	// Look for JSON blocks marked with ```json or ```
	if strings.Contains(raw, "```") {
		// Extract from code blocks
		lines := strings.Split(raw, "\n")
		var jsonLines []string
		inCodeBlock := false

		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				jsonLines = append(jsonLines, line)
			}
		}

		if len(jsonLines) > 0 {
			return strings.Join(jsonLines, "\n")
		}
	}

	// Look for the first { and try to find matching }
	startIdx := strings.Index(raw, "{")
	if startIdx == -1 {
		// Try array format
		startIdx = strings.Index(raw, "[")
		if startIdx == -1 {
			return ""
		}
	}

	// Find the matching closing brace/bracket
	openChar := raw[startIdx]
	closeChar := '}'
	if openChar == '[' {
		closeChar = ']'
	}

	count := 0
	for i := startIdx; i < len(raw); i++ {
		if raw[i] == byte(openChar) {
			count++
		} else if raw[i] == byte(closeChar) {
			count--
			if count == 0 {
				return raw[startIdx : i+1]
			}
		}
	}

	// If we couldn't find a complete JSON structure, return from start to end
	return raw[startIdx:]
}

// truncateForLog truncates text for logging purposes
func truncateForLog(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// LogRepairStats logs detailed repair statistics
func LogRepairStats(stats JsonRepairStats) {
	logger := logging.GetCurrentLogger()
	if logger == nil {
		return
	}

	if !stats.WasRepaired {
		logger.Log("JSON was valid, no repair needed (%d bytes)", stats.OriginalBytes)
		return
	}

	logger.Log("=== JSON Repair Statistics ===")
	logger.Log("Original size: %d bytes", stats.OriginalBytes)
	logger.Log("Repaired size: %d bytes", stats.RepairedBytes)
	logger.Log("Repair time: %v", stats.RepairTime)
	logger.Log("Errors fixed: %d", stats.ErrorsFixed)
	logger.Log("Comments lost: %d", stats.CommentsLost)
	logger.Log("Fields recovered: %d", stats.FieldsRecovered)
	logger.Log("Strategies used: %s", strings.Join(stats.RepairStrategies, ", "))
	logger.Log("==============================")
}
