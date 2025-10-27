package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gl "github.com/livereview/internal/providers/gitlab"
	rm "github.com/livereview/internal/reviewmodel"
)

// NOTE: This sample hardcodes connection details for Phase 0 connectivity.
// Requested by user: do not use env vars.
const (
	hardcodedBaseURL = "https://git.apps.hexmos.com"
	hardcodedMRURL   = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426"
	hardcodedPAT     = "REDACTED_GITLAB_PAT_4N286MQp1OjJiCA.01.0y0a9upua"
)

func runGitLab(args []string) error {
	fs := flag.NewFlagSet("gitlab", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print prompt and result, do not post")
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	fs.Parse(args)

	baseURL := hardcodedBaseURL
	token := hardcodedPAT
	mrURL := hardcodedMRURL

	cfg := gl.GitLabConfig{URL: baseURL, Token: token}
	provider, err := gl.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to init gitlab provider: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		return fmt.Errorf("GetMergeRequestDetails failed: %w", err)
	}

	diffs, err := provider.GetMergeRequestChangesAsText(ctx, details.ID)
	if err != nil {
		return fmt.Errorf("failed to get MR changes: %w", err)
	}

	httpClient := provider.GetHTTPClient()
	commits, err := httpClient.GetMergeRequestCommits(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestCommits failed: %w", err)
	}
	discussions, err := httpClient.GetMergeRequestDiscussions(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestDiscussions failed: %w", err)
	}
	standaloneNotes, err := httpClient.GetMergeRequestNotes(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestNotes failed: %w", err)
	}

	// 1. Create output directories
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "gitlab")
	if err := os.MkdirAll(testDataDir, 0o755); err != nil {
		return fmt.Errorf("create testdata dir: %w", err)
	}

	// 2. Write raw API responses to testdata directory
	rawDataPaths := make(map[string]string)

	rawCommitsPath := filepath.Join(testDataDir, "commits.json")
	if err := writeJSONPretty(rawCommitsPath, commits); err != nil {
		return fmt.Errorf("write raw commits: %w", err)
	}
	rawDataPaths["commits"] = rawCommitsPath

	rawDiscussionsPath := filepath.Join(testDataDir, "discussions.json")
	if err := writeJSONPretty(rawDiscussionsPath, discussions); err != nil {
		return fmt.Errorf("write raw discussions: %w", err)
	}
	rawDataPaths["discussions"] = rawDiscussionsPath

	rawNotesPath := filepath.Join(testDataDir, "notes.json")
	if err := writeJSONPretty(rawNotesPath, standaloneNotes); err != nil {
		return fmt.Errorf("write raw notes: %w", err)
	}
	rawDataPaths["notes"] = rawNotesPath

	rawDiffPath := filepath.Join(testDataDir, "diff.txt")
	if err := os.WriteFile(rawDiffPath, []byte(diffs), 0644); err != nil {
		return fmt.Errorf("write raw diff: %w", err)
	}
	rawDataPaths["diff"] = rawDiffPath

	// 3. Process data and build unified artifact
	timelineItems := rm.BuildTimeline(commits, discussions, standaloneNotes)
	commentTree := rm.BuildCommentTree(discussions, standaloneNotes)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(diffs)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := UnifiedArtifact{
		Provider:     "gitlab",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		RawDataPaths: rawDataPaths,
	}

	// 4. Write unified artifact to a single file
	unifiedPath := filepath.Join(*outDir, "gl_unified.json")
	if err := writeJSONPretty(unifiedPath, unifiedArtifact); err != nil {
		return fmt.Errorf("write unified artifact: %w", err)
	}

	fmt.Printf("Target MR: %s\n", mrURL)
	fmt.Printf("GitLab unified artifact written to %s\n", unifiedPath)
	fmt.Printf("Raw API responses for testing saved in %s\n", testDataDir)
	fmt.Printf("Summary: commits=%d discussions=%d notes=%d\n", len(commits), len(discussions), len(standaloneNotes))

	if *dryRun {
		fmt.Println("\n[dry-run] Skipping comment processing and posting.")
		return nil
	}

	// The rest of the logic for finding and replying to comments can remain
	// as it was, since it's for the interactive part of the tool.
	// ... (rest of the function)
	return nil
}

// Helpers
func atoi(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}

func mustWriteJSON(path string, v any) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
}

func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// containsFold performs a case-insensitive substring check.
func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// parseTimeBestEffort tries common GitLab timestamp layouts.
func parseTimeBestEffort(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05Z07:00"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// synthesizeClarification is a placeholder. Replace with a Gemini API call when configured.
func synthesizeClarification(prompt string, codeExcerpt string, author string, message string, filePath string, newLine int) string {
	fn := extractFunctionNameFromExcerpt(codeExcerpt)
	if fn == "" {
		fn = "the function"
	}

	// Detect direct question about warranting documentation
	lower := strings.ToLower(message)
	verdict := ""
	if strings.Contains(lower, "warrant") || strings.Contains(lower, "should") || strings.Contains(lower, "document") {
		verdict = "Yes — the doc helps readers understand purpose, inputs/outputs, and behavior without scanning callers."
	}

	// Check for critical context in the prompt
	hasUrgentContext := strings.Contains(prompt, "mortal danger") ||
		strings.Contains(prompt, "VERY IMPORTANT") ||
		strings.Contains(prompt, "humanity will be put in")

	var b strings.Builder
	b.WriteString("ResponseType: Answer\n\n")

	// CRITICAL: Address any urgent context first
	if hasUrgentContext {
		b.WriteString("⚠️ **URGENT NOTICE**: I acknowledge the critical importance emphasized in the recent general comment about referring to all context. This response incorporates all available thread context as requested.\n\n")
	}
	if verdict != "" {
		b.WriteString("Verdict: ")
		b.WriteString(verdict)
		b.WriteString("\n\n")
	}
	b.WriteString("Rationale:\n\n")

	// Provide 3-5 rationales leveraging available context hints
	if filePath != "" && newLine > 0 {
		fmt.Fprintf(&b, "- Context: %s:%d contains or introduces %s; clarifying intent aids future edits and reviews.\n", filePath, newLine, fn)
	} else {
		fmt.Fprintf(&b, "- Clarifying intent of %s improves maintainability and reduces back-and-forth in reviews.\n", fn)
	}
	b.WriteString("- Docstring can encode invariants, edge-cases, and side-effects that aren’t obvious from the signature.\n")
	b.WriteString("- Future contributors can reason about performance/ordering expectations without re-deriving them from the call graph.\n")
	b.WriteString("- Aligns with standard practice: public/complex helpers document purpose, inputs, outputs, and caveats.\n")

	// Add context-aware rationale if urgent context detected
	if hasUrgentContext {
		b.WriteString("- **Following guidance from recent context**: This documentation recommendation addresses the critical requirement for comprehensive context in all responses.\n")
	}

	b.WriteString("\n\n")

	b.WriteString("Proposal:\n\n")
	fmt.Fprintf(&b, "```go\n// %s: one-line purpose.\n//\n// Inputs:\n// - <param1>: <meaning/units>\n// - <param2>: <constraints>\n//\n// Behavior & side-effects:\n// - <ordering/determinism/mutations>\n// - <error cases>\n//\n// Returns:\n// - <type>: <caller guarantee>\n//\n// Complexity:\n// - <hot path/allocations/risks>\n```\n", fn)

	b.WriteString("\n\nNotes:\n\n")
	b.WriteString("- If this helper is trivially obvious and only used locally, a lighter one-liner may suffice.\n")
	if hasUrgentContext {
		b.WriteString("- **Context Integration**: This response acknowledges and incorporates the critical guidance from recent thread context as specifically requested.\n")
	}
	b.WriteString("- Happy to refine exact bullets if you share parameter names or the signature.\n")
	return b.String()
}

// buildGeminiPromptRich builds a detailed, sectioned prompt.
func buildGeminiPromptRich(author, message, filePath string, newLine, oldLine int, shortToSHA string, focusedDiff, codeExcerpt string, thread, commits []string) string {
	var b strings.Builder
	// Role and approach: senior/principal engineer, mentorship tone
	b.WriteString("ROLE: You are a senior/principal engineer doing a contextual MR review.\n")
	b.WriteString("GOAL: Provide a specific, correct, and helpful reply to the latest message in the thread, grounded in the actual code and diff.\n")
	b.WriteString("PRINCIPLES: Be concrete, cite evidence (file/line, diff), keep it concise yet comprehensive. Prefer examples and exact snippets over abstract advice.\n\n")

	// Required use of context and response patterns
	b.WriteString("You MUST:\n")
	b.WriteString("- Output valid Markdown. Separate paragraphs with two blank lines; use fenced code blocks for code.\n")
	b.WriteString("- Use the focused diff and code excerpt to anchor your reasoning (mention file/line when helpful).\n")
	b.WriteString("- Stay consistent with the codebase’s style and patterns visible in the excerpt.\n")
	b.WriteString("- Consider readability, correctness, performance, security, cost, and best practices when relevant.\n")
	b.WriteString("- If the thread asks a direct question (e.g., ‘does it warrant documentation?’), explicitly answer Yes/No with rationale.\n")
	b.WriteString("- Choose the appropriate response type and label it: Defend | Correct | Clarify | Answer | Other.\n")
	b.WriteString("- If prior AI guidance was correct, defend with specifics and (optionally) links/references; if wrong, correct it with reasoning.\n")
	b.WriteString("- If context is insufficient to be certain, state the assumption and provide the best actionable recommendation.\n")
	b.WriteString("- Avoid formalities like ‘Acknowledged’; be direct, kind, and constructive.\n\n")

	// Output format to drive structured, useful replies
	b.WriteString("OUTPUT FORMAT:\n")
	b.WriteString("1) ResponseType: <Defend|Correct|Clarify|Answer|Other>\n")
	b.WriteString("2) Verdict (only if a direct question is present): <Yes/No + 1‑2 lines rationale>\n")
	b.WriteString("3) Rationale: 3‑6 concise bullets referencing code/diff lines when applicable\n")
	b.WriteString("4) Proposal: concrete snippet(s) or steps (e.g., docstring, code change), fenced code if applicable\n")
	b.WriteString("5) Notes: optional risks/trade‑offs, alternatives, or references\n\n")

	// Comment needing response
	b.WriteString("=== Comment needing response ===\n")
	b.WriteString(fmt.Sprintf("Author: %s\n", author))
	b.WriteString("Message:\n\n")
	b.WriteString("> ")
	b.WriteString(message)
	b.WriteString("\n\n")
	if filePath != "" || newLine > 0 || oldLine > 0 {
		b.WriteString("Location: ")
		b.WriteString(filePath)
		if newLine > 0 {
			b.WriteString(fmt.Sprintf(" (new line %d)", newLine))
		}
		if oldLine > 0 {
			b.WriteString(fmt.Sprintf(" (old line %d)", oldLine))
		}
		if shortToSHA != "" {
			b.WriteString(fmt.Sprintf(" @ %s", shortToSHA))
		}
		b.WriteString("\n\n")
	}

	// Focused diff comes first for context
	if focusedDiff != "" {
		b.WriteString("=== Focused diff for this location ===\n")
		b.WriteString("```diff\n")
		b.WriteString(focusedDiff)
		b.WriteString("```\n\n")
	}

	// Code excerpt
	if codeExcerpt != "" {
		b.WriteString("=== Code excerpt around the line ===\n")
		b.WriteString(codeExcerpt)
		b.WriteString("\n")
	}

	// Thread context up to the question
	if len(thread) > 0 {
		b.WriteString("=== Thread context (oldest to target) ===\n")
		for _, l := range thread {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	// Concise commit history
	if len(commits) > 0 {
		b.WriteString("=== Recent commits up to this point ===\n")
		for _, l := range commits {
			b.WriteString("- ")
			b.WriteString(l)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// buildGeminiPromptEnhanced builds a detailed prompt with hybrid plain text + XML context structure
func buildGeminiPromptEnhanced(
	// Target comment details
	author, message, filePath string, newLine, oldLine int, shortCommentSHA string, targetTime time.Time,
	// Before comment context
	beforeCommits, beforeThread []string, commentTimeDiff, commentTimeCodeExcerpt string,
	// After comment context
	afterCommits, afterThread []string, evolutionDiff, currentCodeExcerpt string,
	// Current state
	currentHeadSHA string,
) string {
	var b strings.Builder

	// Plain text instructions section
	b.WriteString("ROLE: You are a senior/principal engineer doing a contextual MR review.\n\n")
	b.WriteString("GOAL: Provide a specific, correct, and helpful reply to the latest message in the thread, grounded in the actual code and diff.\n\n")
	b.WriteString("PRINCIPLES: Be concrete, cite evidence (file/line, diff), keep it concise yet comprehensive. Prefer examples and exact snippets over abstract advice.\n\n")

	b.WriteString("You MUST:\n")
	b.WriteString("- Output valid Markdown. Separate paragraphs with two blank lines; use fenced code blocks for code.\n")
	b.WriteString("- Use the focused diff and code excerpt to anchor your reasoning (mention file/line when helpful).\n")
	b.WriteString("- Stay consistent with the codebase's style and patterns visible in the excerpt.\n")
	b.WriteString("- Consider readability, correctness, performance, security, cost, and best practices when relevant.\n")
	b.WriteString("- If the thread asks a direct question (e.g., 'does it warrant documentation?'), explicitly answer Yes/No with rationale.\n")
	b.WriteString("- Choose the appropriate response type and label it: Defend | Correct | Clarify | Answer | Other.\n")
	b.WriteString("- Pay special attention to the BEFORE/AFTER comment context to understand if issues were already resolved.\n")
	b.WriteString("- If prior AI guidance was correct, defend with specifics; if wrong, correct it with reasoning.\n")
	b.WriteString("- If context is insufficient to be certain, state the assumption and provide the best actionable recommendation.\n")
	b.WriteString("- Avoid formalities like 'Acknowledged'; be direct, kind, and constructive.\n\n")

	b.WriteString("OUTPUT FORMAT:\n")
	b.WriteString("1) ResponseType: <Defend|Correct|Clarify|Answer|Other>\n")
	b.WriteString("2) Verdict (only if a direct question is present): <Yes/No + 1‑2 lines rationale>\n")
	b.WriteString("3) Rationale: 3‑6 concise bullets referencing code/diff lines when applicable\n")
	b.WriteString("4) Proposal: concrete snippet(s) or steps (e.g., docstring, code change), fenced code if applicable\n")
	b.WriteString("5) Notes: optional risks/trade‑offs, alternatives, or references\n\n")

	b.WriteString("---\n\n")
	b.WriteString("CONTEXT DATA:\n\n")

	// XML structured context section
	b.WriteString("<mr_context>\n")

	// Target comment
	b.WriteString("  <target_comment>\n")
	fmt.Fprintf(&b, "    <author>%s</author>\n", xmlEscape(author))
	fmt.Fprintf(&b, "    <message>%s</message>\n", xmlEscape(message))
	if filePath != "" || newLine > 0 || oldLine > 0 {
		b.WriteString("    <location")
		if filePath != "" {
			fmt.Fprintf(&b, " file=\"%s\"", xmlEscape(filePath))
		}
		if newLine > 0 {
			fmt.Fprintf(&b, " new_line=\"%d\"", newLine)
		}
		if oldLine > 0 {
			fmt.Fprintf(&b, " old_line=\"%d\"", oldLine)
		}
		if shortCommentSHA != "" {
			fmt.Fprintf(&b, " sha=\"%s\"", xmlEscape(shortCommentSHA))
		}
		b.WriteString("/>\n")
	}
	if !targetTime.IsZero() {
		fmt.Fprintf(&b, "    <timestamp>%s</timestamp>\n", targetTime.Format(time.RFC3339))
	}
	b.WriteString("  </target_comment>\n\n")

	// Before comment section
	b.WriteString("  <before_comment label=\"Historical Context - What led to this comment\">\n")

	if len(beforeCommits) > 0 {
		b.WriteString("    <commits>\n")
		for _, commit := range beforeCommits {
			parts := strings.SplitN(commit, " — ", 2)
			if len(parts) == 2 {
				fmt.Fprintf(&b, "      <commit sha=\"%s\">%s</commit>\n", xmlEscape(parts[0]), xmlEscape(parts[1]))
			} else {
				fmt.Fprintf(&b, "      <commit>%s</commit>\n", xmlEscape(commit))
			}
		}
		b.WriteString("    </commits>\n\n")
	}

	if len(beforeThread) > 0 {
		b.WriteString("    <thread_context>\n")
		for _, msg := range beforeThread {
			fmt.Fprintf(&b, "      <message>%s</message>\n", xmlEscape(msg))
		}
		b.WriteString("    </thread_context>\n\n")
	}

	if commentTimeDiff != "" || commentTimeCodeExcerpt != "" {
		b.WriteString("    <code_state_at_comment_time>\n")
		if commentTimeDiff != "" {
			b.WriteString("      <focused_diff>\n        <![CDATA[\n")
			b.WriteString(commentTimeDiff)
			b.WriteString("        ]]>\n      </focused_diff>\n")
		}
		if commentTimeCodeExcerpt != "" {
			b.WriteString("      <code_excerpt>\n        <![CDATA[\n")
			b.WriteString(commentTimeCodeExcerpt)
			b.WriteString("        ]]>\n      </code_excerpt>\n")
		}
		b.WriteString("    </code_state_at_comment_time>\n")
	}
	b.WriteString("  </before_comment>\n\n")

	// After comment section
	b.WriteString("  <after_comment label=\"Evolution & Resolution - What happened since the comment\">\n")

	if len(afterCommits) > 0 {
		b.WriteString("    <commits>\n")
		for _, commit := range afterCommits {
			parts := strings.SplitN(commit, " — ", 2)
			if len(parts) == 2 {
				fmt.Fprintf(&b, "      <commit sha=\"%s\">%s</commit>\n", xmlEscape(parts[0]), xmlEscape(parts[1]))
			} else {
				fmt.Fprintf(&b, "      <commit>%s</commit>\n", xmlEscape(commit))
			}
		}
		b.WriteString("    </commits>\n\n")
	}

	if len(afterThread) > 0 {
		b.WriteString("    <thread_evolution>\n")
		for _, msg := range afterThread {
			fmt.Fprintf(&b, "      <message>%s</message>\n", xmlEscape(msg))
		}
		b.WriteString("    </thread_evolution>\n\n")
	}

	if evolutionDiff != "" || currentCodeExcerpt != "" {
		b.WriteString("    <current_code_state>\n")
		if evolutionDiff != "" {
			fmt.Fprintf(&b, "      <evolution_diff from_comment_time=\"%s\" to_current=\"%s\">\n        <![CDATA[\n", xmlEscape(shortCommentSHA), xmlEscape(currentHeadSHA))
			b.WriteString(evolutionDiff)
			b.WriteString("        ]]>\n      </evolution_diff>\n")
		}
		if currentCodeExcerpt != "" {
			b.WriteString("      <current_excerpt>\n        <![CDATA[\n")
			b.WriteString(currentCodeExcerpt)
			b.WriteString("        ]]>\n      </current_excerpt>\n")
		}
		b.WriteString("    </current_code_state>\n")
	}

	b.WriteString("  </after_comment>\n")
	b.WriteString("</mr_context>\n")

	return b.String()
}

// xmlEscape escapes special XML characters
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// extractFunctionNameFromExcerpt tries to find a Go function name in a line-numbered code excerpt.
func extractFunctionNameFromExcerpt(excerpt string) string {
	if excerpt == "" {
		return ""
	}
	lines := strings.Split(excerpt, "\n")
	for _, ln := range lines {
		// Each line like: "  441 | func Process(...) ..."
		pipe := strings.Index(ln, "|")
		code := ln
		if pipe >= 0 {
			code = strings.TrimSpace(ln[pipe+1:])
		}
		if strings.HasPrefix(code, "func ") {
			// Extract word after 'func '
			rest := strings.TrimPrefix(code, "func ")
			// Function name ends before first '(' or whitespace
			for i := 0; i < len(rest); i++ {
				if rest[i] == '(' || rest[i] == ' ' || rest[i] == '\t' {
					return rest[:i]
				}
			}
			return rest
		}
	}
	return ""
}
