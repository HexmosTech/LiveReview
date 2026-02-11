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
	SummaryWriterRole = "You are an expert code reviewer. Given the following file-level technical summaries, synthesize a single, coherent markdown summary"
)

// Core instruction templates
const (
	// CodeReviewInstructions provides the main instructions for code review
	CodeReviewInstructions = `Review the following code changes thoroughly and provide:
1. Specific actionable line comments highlighting issues, improvements, and best practices
2. A structured technical summary entry for every file with meaningful changes (intent, architecture, data flows, edge cases)
3. Skip trivial/no-op files entirely and DO NOT provide a global summary here — that will be synthesized separately`

	// ReviewGuidelines provides quality guidelines for reviews
	ReviewGuidelines = `IMPORTANT REVIEW GUIDELINES:
- Focus on finding bugs, security issues, and improvement opportunities
- Highlight unclear code and readability issues
- Use direct, active voice and short sentences (max 15-20 words)
- Be specific and concise - avoid wordiness, passive voice, and meandering explanations
- Avoid unnecessary praise or filler comments
- Avoid commenting on simplistic or obvious things (imports, blank space changes, etc.)
- Technical file summaries must explain the why/intent/architecture for substantive changes and should call out data model or interface impacts`

	// CommentRequirements specifies what each comment should include
	CommentRequirements = `For each line comment, include:
- File path
- Line number
- Severity (info, warning, critical)
- Clear suggestions for improvement

Focus on correctness, security, maintainability, performance, cloud cost risks, and code quality.`
)

// JSON structure templates
const (
	// JSONStructureExample provides the expected JSON output format
	JSONStructureExample = `Format your response as JSON with the following structure:
` + "```json" + `
{
	"fileSummaries": [
		{
			"filePath": "path/to/file.ext",
			"summary": "Technical intent and implementation details for this file",
			"keyChanges": [
				"Primary architectural or data-flow implication",
				"Any noteworthy edge cases or follow-up tasks"
			]
		}
	],
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

// Issue detection checklist
const (
	// IssueDetectionChecklist enumerates the categories and specific issues the reviewer must watch for
	IssueDetectionChecklist = `ISSUE DETECTION FOCUS AREAS:
You must actively scan for the following categories of issues. This list is non-exhaustive — flag any other problems you notice.

**Security & Secrets**
- Hardcoded secrets, API keys, passwords, or tokens
- SQL injection, cross-site scripting (XSS), and other injection risks
- Missing input validation or sanitization
- Insecure cryptographic usage or weak auth checks

**Correctness & Bugs**
- Off-by-one errors
- Null/nil pointer dereferences
- Infinite loops or unbounded recursion
- Unhandled promise rejections or missing error handling
- Type mismatches and unsafe type coercion
- Race conditions, deadlocks, and concurrency bugs
- Unreachable or dead code

**Performance & Resources**
- Memory leaks and unclosed resources
- Unoptimized database queries (N+1, missing indexes, full table scans)
- Potential cloud cost explosions (unbounded S3 puts, SQS fan-out, runaway Lambda invocations, etc.)
- Suboptimal algorithms or data structures where a better choice is straightforward

**Code Quality & Maintainability**
- Duplicated code blocks that should be extracted
- Overly complex or deeply nested functions
- Magic numbers and hardcoded file paths
- Unused variables, imports, or parameters
- Unclear or misleading variable/function names
- Missing or misleading code comments and documentation
- Functions doing too much — suggest smaller, reusable units
- Excessive nesting that harms readability

**Style & Conventions**
- Naming convention violations
- Inconsistent indentation or formatting
- Line length violations
- Inconsistent logging patterns
- Deprecated API or library usage
- Version compatibility issues

**Reliability & Ops**
- Missing or inadequate error messages
- Inconsistent or missing error propagation
- Config file inconsistencies
- Internationalization (i18n) issues where relevant
- Incomplete or missing unit test coverage for changed code`
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
2. Focus strictly on technical artifacts, architectural shifts, data flows, and new interfaces
3. Do NOT restate inline review comments, opinions, or meta feedback
4. Make it scannable and easy to understand quickly
5. Use DIRECT, CONCISE language - short sentences (max 15-20 words), active voice, no fluff
6. Title must be specific and action-oriented - NEVER start with "Enhanced", "Improved", or generic verbs
7. Title should describe WHAT changed, not that something was enhanced (e.g., "Async Logging for CLI Reviews" not "Enhanced CLI Review Logging")
8. Use bullet points that highlight concrete technical impacts and follow-up risks`

	// SummaryStructure provides the expected markdown structure for summaries
	SummaryStructure = `Generate a well-formatted markdown summary following this structure:
# [Use IMPERATIVE VERB + specific object - command form, active voice]

Title MUST use imperative verbs (command form): Add, Fix, Refactor, Update, Remove, Standardize, Implement, Extract, etc.

Examples of GOOD titles (imperative, active):
- "Add Async Logging for CLI Reviews"
- "Extract Author Info from JWT Context"
- "Fix Provider Null Check for CLI Workflows"
- "Standardize Prompt Template for AI Output"
- "Refactor Stage Completion Event Emission"

Examples of BAD titles (passive, noun phrases):
- "Prompt Template Standardization for AI Output Generation" ❌
- "Async Logging Infrastructure for CLI Reviews" ❌
- "JWT-Based Author Attribution in Review Records" ❌
- "Provider Null Safety for CLI Diff Workflows" ❌

## Overview
Brief technical description (2-3 short sentences max). Use active voice and direct language.

## Technical Highlights
- **Component / File**: Concrete technical takeaway or architectural shift (1 sentence)
- **Component / File**: Concrete technical takeaway or architectural shift (1 sentence)

## Impact
- **Functionality**: What capability changed or was added (1 sentence)
- **Risk**: Notable risks, migration considerations, or debt (1 sentence)`
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
