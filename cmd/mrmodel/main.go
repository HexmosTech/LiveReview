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
	hardcodedPAT     = "REDACTED_GITLAB_PAT_1cm86MQp1OjJiCA.01.0y0woxo1s"
)

func main() {
	// Flags: --dry-run prints prompt and synthesized output, skips posting
	dryRun := flag.Bool("dry-run", false, "print prompt and result, do not post")
	flag.Parse()

	baseURL := hardcodedBaseURL
	token := hardcodedPAT
	mrURL := hardcodedMRURL

	cfg := gl.GitLabConfig{URL: baseURL, Token: token}
	provider, err := gl.New(cfg)
	if err != nil {
		log.Fatalf("failed to init gitlab provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1) Fetch MR details (connectivity + basic metadata)
	details, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		log.Fatalf("GetMergeRequestDetails failed: %v", err)
	}

	fmt.Println("== MR DETAILS ==")
	fmt.Printf("URL        : %s\n", details.URL)
	fmt.Printf("ID (IID)   : %s\n", details.ID)
	fmt.Printf("Title      : %s\n", details.Title)
	fmt.Printf("Author     : %s\n", details.Author)
	fmt.Printf("State      : %s\n", details.State)
	fmt.Printf("CreatedAt  : %s\n", details.CreatedAt)
	fmt.Printf("DiffRefs   : base=%s head=%s start=%s\n", details.DiffRefs.BaseSHA, details.DiffRefs.HeadSHA, details.DiffRefs.StartSHA)

	// 2) Fetch MR changes (sanity check: we can read diffs)
	diffs, err := provider.GetMergeRequestChanges(ctx, details.ID)
	if err != nil {
		log.Fatalf("GetMergeRequestChanges failed: %v", err)
	}

	fmt.Println("\n== MR CHANGES SUMMARY ==")
	fmt.Printf("Files changed: %d\n", len(diffs))
	max := len(diffs)
	if max > 5 {
		max = 5
	}
	for i := 0; i < max; i++ {
		d := diffs[i]
		fmt.Printf("- %s (hunks=%d, new=%v, deleted=%v, renamed=%v)\n", d.FilePath, len(d.Hunks), d.IsNew, d.IsDeleted, d.IsRenamed)
	}

	fmt.Println("\nConnection OK — fetched MR details and changes.")

	// 3) Build and emit Timeline and Comment Hierarchy
	httpClient := provider.GetHTTPClient()
	commits, err := httpClient.GetMergeRequestCommits(details.ProjectID, atoi(details.ID))
	if err != nil {
		log.Fatalf("GetMergeRequestCommits failed: %v", err)
	}
	discussions, err := httpClient.GetMergeRequestDiscussions(details.ProjectID, atoi(details.ID))
	if err != nil {
		log.Fatalf("GetMergeRequestDiscussions failed: %v", err)
	}

	timeline := rm.BuildTimeline(commits, discussions)
	tree := rm.BuildCommentTree(discussions)
	exportTimeline := rm.BuildExportTimeline(timeline)
	exportTree := rm.BuildExportCommentTree(tree)

	outDir := filepath.Join(".", "artifacts")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("failed to create artifacts dir: %v", err)
	}
	// Write deduped structures into canonical filenames
	mustWriteJSON(filepath.Join(outDir, "timeline.json"), exportTimeline)
	mustWriteJSON(filepath.Join(outDir, "comment_tree.json"), exportTree)
	fmt.Printf("Artifacts written to %s (timeline.json, comment_tree.json)\n", outDir)

	// 4) Clarify a specific comment: by author and content match
	targetAuthor := "Shrijith"
	targetContains := "Does this function warrant documentation?"
	var targetDiscussionID string
	var targetNoteID int
	var targetNotePosNewPath, targetNotePosOldPath string
	var targetNotePosHeadSHA, targetNotePosBaseSHA string
	var targetNotePosNewLine, targetNotePosOldLine int
	for _, d := range discussions {
		for _, n := range d.Notes {
			if n.System {
				continue
			}
			if n.Author.Name == targetAuthor && containsFold(n.Body, targetContains) {
				targetDiscussionID = d.ID
				targetNoteID = n.ID
				if n.Position != nil {
					targetNotePosNewPath = n.Position.NewPath
					targetNotePosOldPath = n.Position.OldPath
					targetNotePosHeadSHA = n.Position.HeadSHA
					targetNotePosBaseSHA = n.Position.BaseSHA
					targetNotePosNewLine = n.Position.NewLine
					targetNotePosOldLine = n.Position.OldLine
				}
				break
			}
		}
		if targetNoteID != 0 {
			break
		}
	}
	if targetNoteID == 0 {
		fmt.Println("Target comment not found; nothing to clarify.")
		return
	}
	// Mark with eyes emoji (unless dry-run)
	if !*dryRun {
		_ = httpClient.AwardEmojiOnMRNote(details.ProjectID, atoi(details.ID), targetNoteID, "eyes")
	} else {
		fmt.Printf("[dry-run] Would award :eyes: on note %d\n", targetNoteID)
	}

	// 5) Gather enhanced context with before/after comment demarcation
	// 5a) Find the timeline entry for this note to get its timestamp
	var targetTime time.Time
	for _, it := range timeline {
		if it.Kind == "comment" && it.Comment != nil && it.Comment.NoteID == fmt.Sprintf("%d", targetNoteID) {
			targetTime = it.CreatedAt
			break
		}
	}

	// 5b) Partition commits into before/after comment timestamp
	var beforeCommitLogs, afterCommitLogs []string
	for _, it := range timeline {
		if it.Kind == "commit" && it.Commit != nil {
			line := fmt.Sprintf("%s — %s", shortSHA(it.Commit.SHA), it.Commit.Title)
			if targetTime.IsZero() || !it.CreatedAt.After(targetTime) {
				beforeCommitLogs = append(beforeCommitLogs, line)
			} else {
				afterCommitLogs = append(afterCommitLogs, line)
			}
		}
	}
	// Limit before commits to last 8 entries (retain existing behavior)
	if len(beforeCommitLogs) > 8 {
		beforeCommitLogs = beforeCommitLogs[len(beforeCommitLogs)-8:]
	}

	// 5c) Determine SHAs: comment-time and current HEAD
	commentTimeSHA := targetNotePosHeadSHA
	baseSHA := targetNotePosBaseSHA
	if commentTimeSHA == "" {
		for _, ei := range exportTimeline.Items {
			if ei.Kind == "comment" && ei.ID == fmt.Sprintf("%d", targetNoteID) {
				commentTimeSHA = ei.PrevCommitSHA
				break
			}
		}
	}
	if baseSHA == "" && len(commits) > 0 {
		baseSHA = commits[len(commits)-1].ID // fallback: earliest MR commit
	}
	// Current HEAD SHA from MR details
	currentHeadSHA := details.DiffRefs.HeadSHA

	// 5d) Compute focused diff at comment time (existing logic)
	var commentTimeDiff string
	if commentTimeSHA != "" && baseSHA != "" {
		if codeDiffs, err := httpClient.CompareCommitsRaw(details.ProjectID, baseSHA, commentTimeSHA); err == nil {
			focusPath := firstNonEmpty(targetNotePosNewPath, targetNotePosOldPath)
			for _, cd := range codeDiffs {
				if cd.FilePath == focusPath || cd.OldFilePath == focusPath {
					var b strings.Builder
					fmt.Fprintf(&b, "--- a/%s b/%s\n", cd.OldFilePath, cd.FilePath)
					if len(cd.Hunks) > 0 {
						h := cd.Hunks[0].Content
						targetNew := targetNotePosNewLine
						targetOld := targetNotePosOldLine
						if hk := extractHunkForLine(h, targetOld, targetNew); hk != "" {
							annotated := annotateUnifiedDiffHunk(hk)
							if annotated != "" {
								b.WriteString(annotated)
							} else {
								b.WriteString(hk)
							}
						} else {
							b.WriteString(h)
						}
						if !strings.HasSuffix(b.String(), "\n") {
							b.WriteByte('\n')
						}
					}
					commentTimeDiff = b.String()
					break
				}
			}
		}
	}

	// 5e) Compute evolution diff from comment time to current HEAD
	var evolutionDiff string
	if commentTimeSHA != "" && currentHeadSHA != "" && commentTimeSHA != currentHeadSHA {
		if codeDiffs, err := httpClient.CompareCommitsRaw(details.ProjectID, commentTimeSHA, currentHeadSHA); err == nil {
			focusPath := firstNonEmpty(targetNotePosNewPath, targetNotePosOldPath)
			for _, cd := range codeDiffs {
				if cd.FilePath == focusPath || cd.OldFilePath == focusPath {
					var b strings.Builder
					fmt.Fprintf(&b, "--- comment-time (%s)\n+++ current HEAD (%s)\n", shortSHA(commentTimeSHA), shortSHA(currentHeadSHA))
					if len(cd.Hunks) > 0 {
						h := cd.Hunks[0].Content
						b.WriteString(h)
						if !strings.HasSuffix(b.String(), "\n") {
							b.WriteByte('\n')
						}
					}
					evolutionDiff = b.String()
					break
				}
			}
		}
	}

	// 5f) Enhanced thread context - capture full conversation and partition
	var beforeThreadContext, afterThreadContext []string
	for _, d := range discussions {
		if d.ID != targetDiscussionID {
			continue
		}
		for _, n := range d.Notes {
			if n.System {
				continue
			}
			ts := parseTimeBestEffort(n.CreatedAt)
			who := n.Author.Name
			entry := fmt.Sprintf("[%s] %s: %s", ts.Format(time.RFC3339), who, n.Body)

			if n.ID == targetNoteID {
				beforeThreadContext = append(beforeThreadContext, entry)
				break // target note goes in before context
			} else if targetTime.IsZero() || !ts.After(targetTime) {
				beforeThreadContext = append(beforeThreadContext, entry)
			} else {
				afterThreadContext = append(afterThreadContext, entry)
			}
		}
		// Continue processing to get all notes after target
		foundTarget := false
		for _, n := range d.Notes {
			if n.System {
				continue
			}
			if n.ID == targetNoteID {
				foundTarget = true
				continue
			}
			if foundTarget {
				ts := parseTimeBestEffort(n.CreatedAt)
				who := n.Author.Name
				entry := fmt.Sprintf("[%s] %s: %s", ts.Format(time.RFC3339), who, n.Body)
				afterThreadContext = append(afterThreadContext, entry)
			}
		}
		break
	}

	// 5g) Code excerpts at comment time and current state
	var commentTimeCodeExcerpt, currentCodeExcerpt string
	focusPath := firstNonEmpty(targetNotePosNewPath, targetNotePosOldPath)
	focusLine := targetNotePosNewLine
	if focusLine == 0 {
		focusLine = targetNotePosOldLine
	}

	// Code excerpt at comment time
	if commentTimeSHA != "" && focusPath != "" && focusLine > 0 {
		if raw, err := httpClient.GetFileRawAtRef(details.ProjectID, focusPath, commentTimeSHA); err == nil {
			commentTimeCodeExcerpt = renderCodeExcerptWithLineNumbers(raw, focusLine, 8)
		}
	}

	// Code excerpt at current HEAD
	if currentHeadSHA != "" && focusPath != "" && focusLine > 0 {
		if raw, err := httpClient.GetFileRawAtRef(details.ProjectID, focusPath, currentHeadSHA); err == nil {
			currentCodeExcerpt = renderCodeExcerptWithLineNumbers(raw, focusLine, 8)
		}
	}

	// 6) Build Gemini prompt with enhanced before/after context and synthesize clarification
	prompt := buildGeminiPromptEnhanced(
		// Target comment details
		targetAuthor,
		targetContains,
		firstNonEmpty(targetNotePosNewPath, targetNotePosOldPath),
		targetNotePosNewLine,
		targetNotePosOldLine,
		shortSHA(commentTimeSHA),
		targetTime,
		// Before comment context
		beforeCommitLogs,
		beforeThreadContext,
		commentTimeDiff,
		commentTimeCodeExcerpt,
		// After comment context
		afterCommitLogs,
		afterThreadContext,
		evolutionDiff,
		currentCodeExcerpt,
		// Current state
		shortSHA(currentHeadSHA),
	)
	synthesized := synthesizeClarification(prompt, commentTimeCodeExcerpt, targetAuthor, targetContains, firstNonEmpty(targetNotePosNewPath, targetNotePosOldPath), targetNotePosNewLine)
	// Dry run: print prompt and synthesized output, skip posting
	if *dryRun {
		fmt.Println("\n===== DRY RUN =====")
		fmt.Println("--- Prompt ---")
		fmt.Println(prompt)
		fmt.Println("--- Sample Output (dry-run, stub) ---")
		fmt.Println(synthesized)
		fmt.Println("===== END DRY RUN =====")
		return
	}
	if err := httpClient.ReplyToDiscussionNote(details.ProjectID, atoi(details.ID), targetDiscussionID, synthesized); err != nil {
		fmt.Printf("Posting synthesized reply failed: %v\n", err)
	} else {
		fmt.Println("Posted synthesized clarification reply.")
	}
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

// buildGeminiPrompt formats the context for Gemini
func buildGeminiPrompt(commitLogs []string, diff string, thread []string) string {
	// Kept for backward compatibility; route to rich prompt without code excerpt
	return buildGeminiPromptRich("", "", "", 0, 0, "", diff, "", thread, commitLogs)
}

// synthesizeClarification is a placeholder. Replace with a Gemini API call when configured.
func synthesizeClarification(prompt string, codeExcerpt string, author string, message string, filePath string, newLine int) string {
	// Heuristic structured output aligned with the prompt’s OUTPUT FORMAT
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

	var b strings.Builder
	b.WriteString("ResponseType: Answer\n\n")
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
	b.WriteString("\n\n")

	b.WriteString("Proposal:\n\n")
	fmt.Fprintf(&b, "```go\n// %s: one-line purpose.\n//\n// Inputs:\n// - <param1>: <meaning/units>\n// - <param2>: <constraints>\n//\n// Behavior & side-effects:\n// - <ordering/determinism/mutations>\n// - <error cases>\n//\n// Returns:\n// - <type>: <caller guarantee>\n//\n// Complexity:\n// - <hot path/allocations/risks>\n```\n", fn)

	b.WriteString("\n\nNotes:\n\n")
	b.WriteString("- If this helper is trivially obvious and only used locally, a lighter one-liner may suffice.\n\n")
	b.WriteString("- Happy to refine exact bullets if you share parameter names or the signature.\n")
	return b.String()
}

// annotateUnifiedDiffHunk adds pseudo line numbers to a single unified diff hunk for readability.
// It parses the @@ -a,b +c,d @@ header and then increments counts for context/added/removed lines.
func annotateUnifiedDiffHunk(hunk string) string {
	lines := strings.Split(hunk, "\n")
	if len(lines) == 0 {
		return hunk
	}
	var out []string
	var oldN, newN int
	headerParsed := false
	for i, ln := range lines {
		if i == 0 && strings.HasPrefix(ln, "@@ ") {
			// parse header
			// @@ -a,b +c,d @@
			oldN, newN = 0, 0
			// parse old
			if ix := strings.Index(ln, "-"); ix >= 0 {
				seg := ln[ix+1:]
				if j := strings.Index(seg, " "); j >= 0 {
					seg = seg[:j]
				}
				parts := strings.Split(seg, ",")
				if len(parts) > 0 {
					oldN = atoi(parts[0])
				}
			}
			// parse new
			if ix := strings.Index(ln, "+"); ix >= 0 {
				seg := ln[ix+1:]
				if j := strings.Index(seg, " "); j >= 0 {
					seg = seg[:j]
				}
				parts := strings.Split(seg, ",")
				if len(parts) > 0 {
					newN = atoi(parts[0])
				}
			}
			out = append(out, ln)
			headerParsed = true
			continue
		}
		if !headerParsed {
			out = append(out, ln)
			continue
		}
		if ln == "" {
			out = append(out, ln)
			continue
		}
		prefix := ln[0]
		switch prefix {
		case ' ':
			out = append(out, fmt.Sprintf("%6d %6d | %s", oldN, newN, ln))
			oldN++
			newN++
		case '+':
			out = append(out, fmt.Sprintf("%6s %6d | %s", "-", newN, ln))
			newN++
		case '-':
			out = append(out, fmt.Sprintf("%6d %6s | %s", oldN, "-", ln))
			oldN++
		default:
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// extractHunkForLine selects the unified diff hunk containing the target old/new line.
func extractHunkForLine(patch string, targetOld, targetNew int) string {
	lines := strings.Split(patch, "\n")
	var out []string
	var cur []string
	var newStart, newCount int
	var oldStart, oldCount int
	for _, ln := range lines {
		if strings.HasPrefix(ln, "@@ ") && strings.Contains(ln, "@@") {
			// flush previous hunk
			if len(cur) > 0 {
				if hunkContainsLine(oldStart, oldCount, targetOld) || hunkContainsLine(newStart, newCount, targetNew) {
					out = append(out, cur...)
				}
				cur = nil
			}
			cur = []string{ln}
			// parse header like: @@ -a,b +c,d @@
			// crude parse
			header := ln
			// find segments
			// default counts to 1 if missing
			oldStart, oldCount = 0, 0
			newStart, newCount = 0, 0
			// find '-' segment
			if i := strings.Index(header, "-"); i >= 0 {
				seg := header[i+1:]
				if j := strings.Index(seg, " "); j >= 0 {
					seg = seg[:j]
				}
				parts := strings.Split(seg, ",")
				if len(parts) > 0 {
					oldStart = atoi(parts[0])
				}
				if len(parts) > 1 {
					oldCount = atoi(parts[1])
				} else {
					oldCount = 1
				}
			}
			if i := strings.Index(header, "+"); i >= 0 {
				seg := header[i+1:]
				if j := strings.Index(seg, " "); j >= 0 {
					seg = seg[:j]
				}
				parts := strings.Split(seg, ",")
				if len(parts) > 0 {
					newStart = atoi(parts[0])
				}
				if len(parts) > 1 {
					newCount = atoi(parts[1])
				} else {
					newCount = 1
				}
			}
			continue
		}
		if cur != nil {
			cur = append(cur, ln)
		}
	}
	// flush last
	if len(cur) > 0 {
		if hunkContainsLine(oldStart, oldCount, targetOld) || hunkContainsLine(newStart, newCount, targetNew) {
			out = append(out, cur...)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func hunkContainsLine(start, count, target int) bool {
	if start == 0 || target == 0 {
		return false
	}
	end := start + count - 1
	return target >= start && target <= end
}

// renderCodeExcerptWithLineNumbers renders a window of lines around focusLine with line numbers.
func renderCodeExcerptWithLineNumbers(content string, focusLine, radius int) string {
	if radius <= 0 {
		radius = 6
	}
	lines := strings.Split(content, "\n")
	n := len(lines)
	if n == 0 || focusLine <= 0 {
		return ""
	}
	start := focusLine - radius
	if start < 1 {
		start = 1
	}
	end := focusLine + radius
	if end > n {
		end = n
	}
	var b strings.Builder
	b.WriteString("```\n")
	for i := start; i <= end; i++ {
		ln := lines[i-1]
		fmt.Fprintf(&b, "%5d | %s\n", i, ln)
	}
	b.WriteString("```\n")
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
