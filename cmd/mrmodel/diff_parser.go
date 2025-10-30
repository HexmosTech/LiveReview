package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/cmd/mrmodel/lib"
)

// LocalParser is a self-contained parser for unified diffs.
type LocalParser struct{}

// NewLocalParser creates a new LocalParser.
func NewLocalParser() *LocalParser {
	return &LocalParser{}
}

// Parse parses a unified diff string into a slice of LocalCodeDiffs.
func (p *LocalParser) Parse(diffContent string) ([]lib.LocalCodeDiff, error) {
	var diffs []lib.LocalCodeDiff
	files := regexp.MustCompile(`(?m)^diff --git a/(.+) b/(.+)`).Split(diffContent, -1)
	if len(files) == 0 {
		return nil, nil
	}

	// The first element is usually empty, so skip it.
	fileChunks := regexp.MustCompile(`(?m)^diff --git a/`).Split(diffContent, -1)

	for _, fileContent := range fileChunks[1:] {
		if strings.TrimSpace(fileContent) == "" {
			continue
		}

		// Re-add the "diff --git a/" prefix that was removed by the split
		fullFileContent := "diff --git a/" + fileContent

		lines := strings.Split(fullFileContent, "\n")
		if len(lines) < 1 {
			continue
		}

		oldPath, newPath := parseDiffGitHeader(lines[0])
		if oldPath == "" && newPath == "" {
			// Fallback for cases where header parsing is tricky
			pathParts := strings.Fields(lines[0])
			if len(pathParts) >= 4 {
				oldPath = strings.TrimPrefix(pathParts[2], "a/")
				newPath = strings.TrimPrefix(pathParts[3], "b/")
			}
		}

		hunks, err := p.extractHunks(lines)
		if err != nil {
			return nil, fmt.Errorf("extracting hunks for %s: %w", newPath, err)
		}

		diffs = append(diffs, lib.LocalCodeDiff{
			OldPath: oldPath,
			NewPath: newPath,
			Hunks:   hunks,
		})
	}

	return diffs, nil
}

func parseDiffGitHeader(header string) (string, string) {
	// A simple parser for "diff --git a/old/path b/new/path"
	parts := strings.Fields(header)
	if len(parts) == 4 {
		return strings.TrimPrefix(parts[2], "a/"), strings.TrimPrefix(parts[3], "b/")
	}
	return "", ""
}

func (p *LocalParser) extractHunks(lines []string) ([]lib.LocalDiffHunk, error) {
	var hunks []lib.LocalDiffHunk
	hunkHeaderRegex := regexp.MustCompile(`^@@ -(\d+),(\d+) \+(\d+),(\d+) @@(.*)`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "@@") {
			continue
		}

		matches := hunkHeaderRegex.FindStringSubmatch(line)
		if len(matches) < 6 {
			continue
		}

		oldStart, _ := strconv.Atoi(matches[1])
		oldLines, _ := strconv.Atoi(matches[2])
		newStart, _ := strconv.Atoi(matches[3])
		newLines, _ := strconv.Atoi(matches[4])
		headerText := strings.TrimSpace(matches[5])

		hunk := lib.LocalDiffHunk{
			OldStartLine: oldStart,
			OldLineCount: oldLines,
			NewStartLine: newStart,
			NewLineCount: newLines,
			HeaderText:   headerText,
		}

		// Process lines within the hunk
		oldLineNo, newLineNo := oldStart, newStart
		hunkLinesAdded, hunkLinesDeleted := 0, 0

		// Start from the line after the hunk header
		i++
		for ; i < len(lines); i++ {
			hunkLine := lines[i]
			if strings.HasPrefix(hunkLine, "@@") {
				// We've reached the next hunk, so go back one step for the outer loop
				i--
				break
			}
			if strings.HasPrefix(hunkLine, "diff --git") {
				// Reached the start of a new file diff
				i--
				break
			}

			var dLine lib.LocalDiffLine
			switch {
			case strings.HasPrefix(hunkLine, "+"):
				dLine = lib.LocalDiffLine{Content: hunkLine[1:], LineType: "added", OldLineNo: 0, NewLineNo: newLineNo}
				newLineNo++
				hunkLinesAdded++
			case strings.HasPrefix(hunkLine, "-"):
				dLine = lib.LocalDiffLine{Content: hunkLine[1:], LineType: "deleted", OldLineNo: oldLineNo, NewLineNo: 0}
				oldLineNo++
				hunkLinesDeleted++
			case strings.HasPrefix(hunkLine, " "):
				dLine = lib.LocalDiffLine{Content: hunkLine[1:], LineType: "context", OldLineNo: oldLineNo, NewLineNo: newLineNo}
				oldLineNo++
				newLineNo++
			case hunkLine == `\ No newline at end of file`:
				// This is metadata, not a line of code. Skip for now.
				continue
			default:
				// Should be context line, but might not have a space if it's an empty line
				dLine = lib.LocalDiffLine{Content: hunkLine, LineType: "context", OldLineNo: oldLineNo, NewLineNo: newLineNo}
				oldLineNo++
				newLineNo++
			}
			hunk.Lines = append(hunk.Lines, dLine)
		}
		hunks = append(hunks, hunk)
	}

	return hunks, nil
}
