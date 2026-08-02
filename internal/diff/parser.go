package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/pkg/models"
)

// Parser parses git diff output into structured data
type Parser struct{}

// NewParser creates a new diff parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a git diff string into a slice of CodeDiff
func (p *Parser) Parse(diffText string) ([]*models.CodeDiff, error) {
	if diffText == "" {
		return nil, nil
	}

	// Split the diff by file
	fileDiffs := p.splitDiffByFile(diffText)

	result := make([]*models.CodeDiff, 0, len(fileDiffs))

	for _, fileDiff := range fileDiffs {
		codeDiff, err := p.parseFileDiff(fileDiff)
		if err != nil {
			return nil, err
		}

		if codeDiff != nil {
			result = append(result, codeDiff)
		}
	}

	return result, nil
}

// splitDiffByFile splits a unified diff into separate file diffs
func (p *Parser) splitDiffByFile(diffText string) []string {
	// This is a simplified implementation
	// TODO: Implement proper diff splitting
	diffFiles := strings.Split(diffText, "diff --git ")

	result := make([]string, 0, len(diffFiles))
	for i, file := range diffFiles {
		if i == 0 && !strings.HasPrefix(file, "diff --git ") {
			continue
		}

		if i > 0 {
			file = "diff --git " + file
		}

		result = append(result, file)
	}

	return result
}

// parseFileDiff parses a single file diff
func (p *Parser) parseFileDiff(diffText string) (*models.CodeDiff, error) {
	// Extract file path, file type, etc.
	// TODO: Implement proper diff parsing

	// This is a simplified implementation
	filePath, err := p.extractFilePath(diffText)
	if err != nil {
		return nil, err
	}

	hunks, err := p.extractHunks(diffText)
	if err != nil {
		return nil, err
	}

	fileType := p.determineFileType(filePath)

	return &models.CodeDiff{
		FilePath: filePath,
		Hunks:    hunks,
		FileType: fileType,
	}, nil
}

// extractFilePath extracts the file path from a diff
func (p *Parser) extractFilePath(diffText string) (string, error) {
	// Example: diff --git a/file.go b/file.go
	re := regexp.MustCompile(`diff --git a/(.*) b/(.*)`)
	matches := re.FindStringSubmatch(diffText)

	if len(matches) < 3 {
		return "", fmt.Errorf("could not extract file path from diff")
	}

	return matches[2], nil
}

// extractHunks extracts diff hunks from a file diff
func (p *Parser) extractHunks(diffText string) ([]models.DiffHunk, error) {
	// Example: @@ -1,3 +1,4 @@
	re := regexp.MustCompile(`@@ -(\d+),(\d+) \+(\d+),(\d+) @@`)

	hunkMatches := re.FindAllStringSubmatchIndex(diffText, -1)
	if len(hunkMatches) == 0 {
		return nil, nil
	}

	hunks := make([]models.DiffHunk, 0, len(hunkMatches))

	for i, match := range hunkMatches {
		// Extract hunk header information
		oldStart, _ := strconv.Atoi(diffText[match[2]:match[3]])
		oldCount, _ := strconv.Atoi(diffText[match[4]:match[5]])
		newStart, _ := strconv.Atoi(diffText[match[6]:match[7]])
		newCount, _ := strconv.Atoi(diffText[match[8]:match[9]])

		// Extract hunk content
		var content string
		if i < len(hunkMatches)-1 {
			content = diffText[match[1]:hunkMatches[i+1][0]]
		} else {
			content = diffText[match[1]:]
		}

		// Skip the hunk header line
		contentLines := strings.SplitN(content, "\n", 2)
		if len(contentLines) > 1 {
			content = contentLines[1]
		}

		hunks = append(hunks, models.DiffHunk{
			OldStartLine: oldStart,
			OldLineCount: oldCount,
			NewStartLine: newStart,
			NewLineCount: newCount,
			Content:      content,
		})
	}

	return hunks, nil
}

// determineFileType determines the type of file based on its path
func (p *Parser) determineFileType(filePath string) string {
	parts := strings.Split(filePath, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}
