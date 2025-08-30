package gemini

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/pkg/models"
)

// LineTypeDetector is a utility function that properly identifies line types in diff hunks
// It specifically looks for the format used in the formatted hunks which is more complex
// than simple +/- prefixes
func DetectLineType(lineNumber int, hunk models.DiffHunk) (bool, error) {
	// Split the hunk content into lines
	lines := strings.Split(strings.TrimSpace(hunk.Content), "\n")

	// First find and skip any hunk headers (@@ lines)
	lineIdx := 0
	for lineIdx < len(lines) {
		if strings.HasPrefix(lines[lineIdx], "@@") {
			lineIdx++
			break
		}
		lineIdx++
	}

	// Skip any header lines like "OLD | NEW | CONTENT"
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], " ") &&
		!strings.HasPrefix(lines[lineIdx], "+") && !strings.HasPrefix(lines[lineIdx], "-") {
		lineIdx++
	}

	// Process each line to find the target line number
	for lineIdx < len(lines) {
		line := lines[lineIdx]
		if line == "" {
			lineIdx++
			continue
		}

		// Advanced line format detection (compatible with the formatted hunk logs)
		// This handles formats like:
		// 159|160|  	}
		// 160|    |-	defer client.Close()
		// 161|161|
		//
		// Or the standard format:
		// +	TopK           *float64 `json:"top_k,omitempty"`
		// -	TopK           *int     `json:"top_k,omitempty"`
		//  	ShouldContinue *bool    `json:"should_continue,omitempty"`

		// Check for the complex formatted hunk format first
		r := regexp.MustCompile(`(\d+)\|(\s*)\|(-?)`)
		match := r.FindStringSubmatch(line)
		if match != nil && len(match) >= 3 {
			// This is a formatted hunk line
			oldLine, _ := strconv.Atoi(match[1])
			if oldLine == lineNumber && match[2] == "    " && match[3] == "-" {
				// This is a deleted line with the matching line number
				fmt.Printf("ADVANCED LINE DETECTION: Line %d is a DELETED line\n", lineNumber)
				return true, nil
			}
			lineIdx++
			continue
		}

		// Check for the standard unified diff format
		if strings.HasPrefix(line, "-") {
			// This is a deleted line in standard format
			oldLineNumber := hunk.OldStartLine + lineIdx
			if oldLineNumber == lineNumber {
				fmt.Printf("STANDARD LINE DETECTION: Line %d is a DELETED line\n", lineNumber)
				return true, nil
			}
		}

		lineIdx++
	}

	// If we didn't find a match as a deleted line, it's either added or context
	fmt.Printf("LINE DETECTION: Line %d is NOT a deleted line\n", lineNumber)
	return false, nil
}
