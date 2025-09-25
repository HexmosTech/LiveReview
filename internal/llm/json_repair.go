package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kaptinlin/jsonrepair"
)

// JsonRepairStats tracks statistics about JSON repair operations
type JsonRepairStats struct {
	OriginalBytes    int           `json:"original_bytes"`
	RepairedBytes    int           `json:"repaired_bytes"`
	CommentsLost     int           `json:"comments_lost"`
	FieldsRecovered  int           `json:"fields_recovered"`
	ErrorsFixed      int           `json:"errors_fixed"`
	RepairTime       time.Duration `json:"repair_time"`
	RepairStrategies []string      `json:"repair_strategies"`
	WasRepaired      bool          `json:"was_repaired"`
}

// RepairJSON attempts to repair malformed JSON using multiple strategies in order:
// 1. Remove trailing commas
// 2. Fix unescaped quotes in strings
// 3. Complete incomplete objects/arrays
// 4. Remove JavaScript-style comments
// 5. Add missing quotes around keys
// 6. Convert single quotes to double quotes
// 7. Use jsonrepair library as sophisticated fallback
func RepairJSON(raw string) (repaired string, stats JsonRepairStats, err error) {
	startTime := time.Now()
	stats.OriginalBytes = len(raw)

	// First, try to parse as-is
	var testObj interface{}
	if json.Unmarshal([]byte(raw), &testObj) == nil {
		// JSON is already valid
		stats.RepairedBytes = len(raw)
		stats.RepairTime = time.Since(startTime)
		stats.WasRepaired = false
		return raw, stats, nil
	}

	// JSON needs repair - try multiple strategies
	stats.WasRepaired = true
	repaired = raw

	// Strategy 1: Remove trailing commas
	if strings.Contains(repaired, ",}") || strings.Contains(repaired, ",]") {
		repaired = removeTrailingCommas(repaired)
		stats.RepairStrategies = append(stats.RepairStrategies, "trailing_commas")
		stats.ErrorsFixed++
	}

	// Strategy 2: Fix unescaped quotes in strings
	if hasUnescapedQuotes(repaired) {
		original := repaired
		repaired = fixUnescapedQuotes(repaired)
		if repaired != original {
			stats.RepairStrategies = append(stats.RepairStrategies, "unescaped_quotes")
			stats.ErrorsFixed++
		}
	}

	// Strategy 3: Fix incomplete objects/arrays
	if needsCompletion(repaired) {
		original := repaired
		repaired = completeJSON(repaired)
		if repaired != original {
			stats.RepairStrategies = append(stats.RepairStrategies, "completion")
			stats.ErrorsFixed++
		}
	}

	// Strategy 4: Remove JavaScript-style comments
	if containsComments(repaired) {
		original := repaired
		repaired, stats.CommentsLost = removeComments(repaired)
		if repaired != original {
			stats.RepairStrategies = append(stats.RepairStrategies, "comments_removed")
			stats.ErrorsFixed++
		}
	}

	// Strategy 5: Fix missing quotes around keys
	if hasMissingKeyQuotes(repaired) {
		original := repaired
		repaired = addKeyQuotes(repaired)
		if repaired != original {
			stats.RepairStrategies = append(stats.RepairStrategies, "key_quotes")
			stats.ErrorsFixed++
			stats.FieldsRecovered++
		}
	}

	// Strategy 6: Fix single quotes to double quotes
	if hasSingleQuotes(repaired) {
		original := repaired
		repaired = fixSingleQuotes(repaired)
		if repaired != original {
			stats.RepairStrategies = append(stats.RepairStrategies, "single_quotes")
			stats.ErrorsFixed++
		}
	}

	// Strategy 7: Use jsonrepair library as sophisticated fallback
	if json.Unmarshal([]byte(repaired), &testObj) != nil {
		original := repaired
		libraryRepaired, libraryErr := jsonrepair.JSONRepair(repaired)
		if libraryErr == nil && libraryRepaired != original {
			repaired = libraryRepaired
			stats.RepairStrategies = append(stats.RepairStrategies, "jsonrepair_library")
			stats.ErrorsFixed++
			// Test if library repair was successful
			if json.Unmarshal([]byte(repaired), &testObj) == nil {
				// Success with library repair!
				stats.RepairedBytes = len(repaired)
				stats.RepairTime = time.Since(startTime)
				return repaired, stats, nil
			}
		}
	}

	// Final validation
	if json.Unmarshal([]byte(repaired), &testObj) != nil {
		// Still invalid - return error but include partial repair info
		stats.RepairedBytes = len(repaired)
		stats.RepairTime = time.Since(startTime)
		return repaired, stats, fmt.Errorf("JSON repair failed after %d strategies", len(stats.RepairStrategies))
	}

	stats.RepairedBytes = len(repaired)
	stats.RepairTime = time.Since(startTime)

	return repaired, stats, nil
}

// removeTrailingCommas removes trailing commas before } and ]
func removeTrailingCommas(json string) string {
	// Remove trailing comma before closing brace
	re1 := regexp.MustCompile(`,\s*}`)
	json = re1.ReplaceAllString(json, "}")

	// Remove trailing comma before closing bracket
	re2 := regexp.MustCompile(`,\s*]`)
	json = re2.ReplaceAllString(json, "]")

	return json
}

// hasUnescapedQuotes checks if there are unescaped quotes in string values
func hasUnescapedQuotes(json string) bool {
	// Look for patterns like "value with "quotes" inside"
	re := regexp.MustCompile(`"[^"]*"[^"]*"[^"]*"`)
	return re.MatchString(json)
}

// fixUnescapedQuotes escapes unescaped quotes in string values
func fixUnescapedQuotes(json string) string {
	// This is a simplified implementation - in practice, you'd need more sophisticated parsing
	// For now, we'll handle the most common case of quotes in comment text
	re := regexp.MustCompile(`("comment":\s*")([^"]*)"([^"]*)"([^"]*)("[\s,}])`)
	return re.ReplaceAllString(json, `$1$2\"$3\"$4$5`)
}

// needsCompletion checks if JSON objects or arrays are incomplete
func needsCompletion(json string) bool {
	json = strings.TrimSpace(json)

	// Check if we have unmatched braces or brackets
	openBraces := strings.Count(json, "{") - strings.Count(json, "}")
	openBrackets := strings.Count(json, "[") - strings.Count(json, "]")

	return openBraces > 0 || openBrackets > 0
}

// completeJSON adds missing closing braces/brackets in the correct order
func completeJSON(json string) string {
	json = strings.TrimSpace(json)

	// We need to close structures in the correct order (last opened, first closed)
	// Parse through the string to determine the correct closing sequence
	var stack []rune

	for _, char := range json {
		switch char {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '}' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == ']' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Add the remaining closing characters in reverse order (LIFO)
	for i := len(stack) - 1; i >= 0; i-- {
		json += string(stack[i])
	}

	return json
} // containsComments checks for JavaScript-style comments
func containsComments(json string) bool {
	return strings.Contains(json, "//") || strings.Contains(json, "/*")
}

// removeComments removes JavaScript-style comments and counts them
func removeComments(json string) (string, int) {
	commentsRemoved := 0

	// Remove single-line comments
	lines := strings.Split(json, "\n")
	var cleanLines []string
	for _, line := range lines {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
			commentsRemoved++
		}
		cleanLines = append(cleanLines, line)
	}
	json = strings.Join(cleanLines, "\n")

	// Remove multi-line comments
	re := regexp.MustCompile(`/\*.*?\*/`)
	matches := re.FindAllString(json, -1)
	commentsRemoved += len(matches)
	json = re.ReplaceAllString(json, "")

	return json, commentsRemoved
}

// hasMissingKeyQuotes checks for unquoted object keys
func hasMissingKeyQuotes(json string) bool {
	// Look for patterns like { key: "value" } or , key: "value" instead of { "key": "value" }
	re := regexp.MustCompile(`[{,]\s*[a-zA-Z_][a-zA-Z0-9_]*\s*:`)
	return re.MatchString(json)
}

// addKeyQuotes adds quotes around unquoted object keys
func addKeyQuotes(json string) string {
	// Add quotes around unquoted keys (handle both { key: and , key:)
	re := regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)(\s*:)`)
	return re.ReplaceAllString(json, `$1"$2"$3`)
}

// hasSingleQuotes checks for single-quoted strings
func hasSingleQuotes(json string) bool {
	// Look for single quotes around values
	re := regexp.MustCompile(`'[^']*'`)
	return re.MatchString(json)
}

// fixSingleQuotes converts single quotes to double quotes
func fixSingleQuotes(json string) string {
	// Replace single quotes with double quotes, being careful about apostrophes
	// This is a simplified implementation
	re := regexp.MustCompile(`'([^']*)'`)
	return re.ReplaceAllString(json, `"$1"`)
}
