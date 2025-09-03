package prompts

// PromptTemplates contains all prompt template constants used across the application
type PromptTemplates struct{}

// Singleton instance
var Templates = &PromptTemplates{}

// System role definitions
const (
	// CodeReviewerRole defines the primary AI role for code review
	CodeReviewerRole = "You are an expert code reviewer"

	// SummaryWriterRole defines the AI role for generating summaries
	SummaryWriterRole = "You are an expert code reviewer. Given the following file-level summaries and line comments, synthesize a single, high-level summary"
)

// Core instruction templates
const (
	// CodeReviewInstructions provides the main instructions for code review
	CodeReviewInstructions = `Review the following code changes thoroughly and provide:
1. Specific actionable line comments highlighting issues, improvements, and best practices
2. File-level summaries ONLY for complex files that warrant explanation (not for every file)
3. DO NOT provide a general summary here - that will be synthesized separately`

	// ReviewGuidelines provides quality guidelines for reviews
	ReviewGuidelines = `IMPORTANT REVIEW GUIDELINES:
- Focus on finding bugs, security issues, and improvement opportunities
- Highlight unclear code and readability issues
- Keep comments concise and use active voice
- Avoid unnecessary praise or filler comments
- Avoid commenting on simplistic or obvious things (imports, blank space changes, etc.)
- File summaries should only be provided for complex changes that need explanation`

	// CommentRequirements specifies what each comment should include
	CommentRequirements = `For each line comment, include:
- File path
- Line number
- Severity (info, warning, critical)
- Clear suggestions for improvement

Focus on correctness, security, maintainability, and performance.`
)

// JSON structure templates
const (
	// JSONStructureExample provides the expected JSON output format
	JSONStructureExample = `Format your response as JSON with the following structure:
` + "```json" + `
{
  "fileSummary": "Optional: Brief summary of complex file changes (omit if file is simple)",
  "comments": [
    {
      "filePath": "path/to/file.ext",
      "lineNumber": 42,
      "content": "Description of the issue",
      "severity": "info|warning|critical",
      "suggestions": ["Specific improvement suggestion 1", "Specific improvement suggestion 2"],
      "isInternal": false
    }
  ]
}
` + "```"
)

// Comment classification guidelines
const (
	// CommentClassification provides rules for internal vs external comments
	CommentClassification = `COMMENT CLASSIFICATION:
- Set "isInternal": true for comments that are:
  * Obvious/trivial observations ("variable renamed", "method added")
  * Purely informational with no actionable insight
  * Low-value praise ("good practice", "nice naming")
  * Detailed technical analysis better suited for synthesis
- Set "isInternal": false for comments that are:
  * Security vulnerabilities or bugs
  * Performance issues
  * Maintainability concerns with clear suggestions
  * Important architectural decisions that need visibility
Only post comments that add real value to the developer!`
)

// Line number interpretation guidelines
const (
	// LineNumberInstructions provides critical guidance for line number handling
	LineNumberInstructions = `CRITICAL: LINE NUMBER REFERENCES!
- Each diff hunk is formatted as a table with columns: OLD | NEW | CONTENT
- The OLD column shows line numbers in the original file
- The NEW column shows line numbers in the modified file
- For added lines (+ prefix), use the NEW line number for comments
- For deleted lines (- prefix), use the OLD line number for comments
- For modified lines, comment on the NEW version (+ line) with NEW line number
- You can ONLY comment on lines with + or - prefixes (changed lines)
- Do NOT comment on context lines (space prefix) or lines outside the diff`
)

// Summary generation templates
const (
	// SummaryRequirements provides requirements for high-level summaries
	SummaryRequirements = `REQUIREMENTS:
1. Use markdown formatting with clear structure: # headings, ## subheadings, **bold**, bullet points
2. Focus on the big picture, impact, and intent - NOT individual file details
3. Make it scannable and easy to understand quickly
4. Start with a clear main title using # heading
5. Use bullet points for key changes and impacts
6. Keep it concise but informative`

	// SummaryStructure provides the expected markdown structure for summaries
	SummaryStructure = `Generate a well-formatted markdown summary following this structure:
# [Clear main title of what changed]

## Overview
Brief description of the change intent and scope.

## Key Changes
- **Area 1**: Description
- **Area 2**: Description

## Impact
- **Functionality**: How this affects functionality
- **Risk**: Any notable risks or considerations`
)

// Section headers
const (
	CodeChangesHeader = "# Code Changes"
	FilePrefix        = "## File: "
	NewFileMarker     = "(New file)"
	DeletedFileMarker = "(Deleted file)"
	RenamedFilePrefix = "(Renamed from: "
	RenamedFileSuffix = ")"
)
