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
- Escalate severity to critical when the plausible impact includes data corruption, data deletion, lost updates, wrong permissions, wrong billing/accounting state, or wrong user-visible information
- Treat race conditions, missing async cleanup, and stale-state bugs as critical when they can silently overwrite data, drop data, or show incorrect state
- Info comments should be rare; if a point is mostly explanatory, speculative, obvious, or better captured in the file summary, omit it or mark it internal instead
- For trivial refactors that preserve behavior, default to zero external comments
- Omit external comments for parameter renames, constant extraction, placeholder constants, doc-comment/style nits, and readability-only suggestions unless they create a real bug or concrete maintenance risk
- Technical file summaries must explain the why/intent/architecture for substantive changes and should call out data model or interface impacts`

	// CommentRequirements specifies what each comment should include
	CommentRequirements = `For each line comment, include:
- File path
- Line number
- Severity (info, warning, critical)
	- Confidence (High, Medium, Low)
	- Type (Bug, Risk, Optimization, Code Smell, Best Practice, Technical Debt)
	- Category
	- Subcategory
- Clear suggestions for improvement

Severity rules:
- critical = plausible destructive or silently incorrect impact, including data corruption, data deletion, lost updates, auth/permission mistakes, wrong billing state, or wrong information shown to users
- warning = concrete issue with meaningful risk, but the likely impact is contained and not destructive or silently incorrect
- info = rare; use only for non-obvious, line-specific, actionable guidance that is not already better captured in the file summary
- Do not use info for readability-only suggestions on small local refactors

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
			"confidence": "High|Medium|Low",
			"type": "Bug|Risk|Optimization|Code Smell|Best Practice|Technical Debt",
			"category": "one of the 10 categories from TAXONOMY CLASSIFICATION RULES",
			"subcategory": "one of the allowed subcategories for that category from TAXONOMY CLASSIFICATION RULES",
			"suggestions": ["Specific improvement suggestion 1", "Specific improvement suggestion 2"],
			"isInternal": false
		}
	]
}
` + "```"
)

// Taxonomy classification rules
const (
	// TaxonomyClassificationRules enforces a fixed, closed category/subcategory taxonomy.
	// The model MUST classify every comment using exactly one category and exactly one
	// subcategory taken verbatim from the list under that category — no other values allowed.
	TaxonomyClassificationRules = `TAXONOMY CLASSIFICATION RULES:
- "category" MUST be exactly one of the 10 top-level keys in the taxonomy below, verbatim
- "subcategory" MUST be exactly one of the values listed under the chosen category, verbatim
- Never invent a new category or subcategory, never use a subcategory from a different category's list, and never put a subcategory's name in "category"
- Pick the single best-fitting category, then the single best-fitting subcategory from that category's list. If a finding spans multiple concerns, choose the most dominant one
- "UI/UX" and "Accessibility" are valid under both "Maintainability" and "Developer Experience". Use "Maintainability" for issues with the UI code itself (structure, duplication, hardcoded styling); use "Developer Experience" for issues affecting the end user's experience of the UI (usability, consistency, responsiveness, a11y)

TAXONOMY (category -> allowed subcategories):
` + "```json" + `
{
  "Security": ["Authentication", "Authorization", "Secrets Management", "Input Validation", "Injection Vulnerabilities", "Cryptography", "Dependency Vulnerabilities", "Data Exposure", "Session Management", "Security Logging & Auditing"],
  "Reliability": ["Error Handling", "Fault Tolerance", "Retry Logic", "Timeout Management", "Resilience Patterns", "Availability Risks", "Data Integrity", "Race Conditions", "Resource Cleanup", "Failure Recovery"],
  "Correctness": ["Logic Errors", "Edge Cases", "Data Validation", "State Management", "Concurrency Bugs", "Business Rule Violations", "Numerical Accuracy", "Null Handling", "Type Safety", "API Contract Violations"],
  "Performance": ["Database Efficiency", "Algorithmic Complexity", "Memory Usage", "CPU Utilization", "Network Efficiency", "Caching", "Concurrency", "Resource Contention", "Rendering Performance", "Startup Performance"],
  "Cost": ["Cloud Resource Waste", "Infrastructure Overprovisioning", "Storage Optimization", "Database Cost Optimization", "Excessive API Usage", "Third-Party Service Costs", "Redundant Computation", "LLM Token Consumption", "Caching Opportunities", "Data Transfer Costs"],
  "Scalability": ["Horizontal Scaling", "Vertical Scaling", "Distributed Systems", "Load Balancing", "Capacity Planning", "Bottleneck Risks", "Concurrency Limits", "Service Growth Constraints", "Database Scaling", "Queue Backpressure"],
  "Maintainability": ["Code Complexity", "Readability", "Documentation", "Code Duplication", "Dead Code", "Naming Quality", "Testability", "Technical Debt", "Refactoring Opportunities", "Configuration Management", "UI/UX", "Accessibility"],
  "Architecture": ["Separation of Concerns", "Modularity", "Coupling", "Cohesion", "Layering Violations", "Dependency Management", "Service Boundaries", "Domain Modeling", "API Design", "Extensibility"],
  "Developer Experience": ["Testing", "CI/CD", "Build System", "Local Development", "Debuggability", "Observability", "Deployment Process", "Automation", "Developer Tooling", "Documentation Quality", "UI/UX", "Accessibility"],
  "Compliance & Governance": ["Privacy", "Regulatory Compliance", "Auditability", "Data Retention", "Data Residency", "Licensing", "Policy Enforcement", "Access Controls", "Change Management", "Governance Standards"]
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
- Missing effect cleanup, cancellation, or lifecycle guards for async work that can outlive the current request/component
- Stale-state bugs that can show wrong information, overwrite newer state, or drop user actions
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
	* Explanatory context, speculation, or summary material that does not need a line-level comment
	* Parameter renames, constant extraction, placeholder constants, doc-comment nits, style nits, or readability-only suggestions without a real bug or concrete maintenance risk
- Set "isInternal": false for comments that are:
  * Security vulnerabilities or bugs
  * Performance issues
  * Maintainability concerns with clear suggestions
  * Important architectural decisions that need visibility
	* Rare info comments only when they are non-obvious, actionable now, line-specific, and not better suited to the file summary
If you are deciding between an external info comment and omission, omit it.
If a diff is a trivial refactor with no behavioral change, prefer zero external comments.
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
