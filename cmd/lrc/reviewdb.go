package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// reviewSession represents a single review iteration stored in the DB.
type reviewSession struct {
	ID        int64     `json:"id"`
	TreeHash  string    `json:"tree_hash"`
	Branch    string    `json:"branch"`
	Action    string    `json:"action"` // "reviewed", "skipped", "vouched"
	Timestamp time.Time `json:"timestamp"`
	DiffFiles string    `json:"diff_files"` // JSON-encoded []attestationFileEntry
	ReviewID  string    `json:"review_id"`  // API review ID, if applicable
}

// attestationFileEntry is a slim representation of a file diff for storage
// (no Content field — just line ranges).
type attestationFileEntry struct {
	FilePath string                `json:"file_path"`
	Hunks    []attestationHunkRange `json:"hunks"`
}

// attestationHunkRange stores just the line-range info from a hunk.
type attestationHunkRange struct {
	OldStartLine int `json:"old_start_line"`
	OldLineCount int `json:"old_line_count"`
	NewStartLine int `json:"new_start_line"`
	NewLineCount int `json:"new_line_count"`
}

// coverageResult holds computed coverage statistics.
type coverageResult struct {
	Iterations       int     `json:"iterations"`
	PriorAICovPct    float64 `json:"prior_ai_coverage_pct"`
	CoveredLines     int     `json:"covered_lines"`
	TotalLines       int     `json:"total_lines"`
	PriorReviewCount int     `json:"prior_review_count"` // count of "reviewed" sessions
}

const reviewDBSchema = `
CREATE TABLE IF NOT EXISTS review_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tree_hash TEXT NOT NULL,
    branch TEXT NOT NULL,
    action TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    diff_files TEXT,
    review_id TEXT
);
CREATE INDEX IF NOT EXISTS idx_review_sessions_branch ON review_sessions(branch);
CREATE INDEX IF NOT EXISTS idx_review_sessions_tree ON review_sessions(tree_hash);
`

// reviewDBPath returns the path to the review database under .git/lrc/.
func reviewDBPath() (string, error) {
	gitDir, err := resolveGitDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git dir: %w", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", fmt.Errorf("failed to absolutize git dir: %w", err)
		}
	}
	lrcDir := filepath.Join(gitDir, "lrc")
	if err := os.MkdirAll(lrcDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create lrc directory: %w", err)
	}
	return filepath.Join(lrcDir, "reviews.db"), nil
}

// openReviewDB opens (or creates) the SQLite review database.
func openReviewDB() (*sql.DB, error) {
	dbPath, err := reviewDBPath()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open review database: %w", err)
	}

	if _, err := db.Exec(reviewDBSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize review database schema: %w", err)
	}

	return db, nil
}

// currentBranch returns the current git branch name, or "HEAD" if detached.
func currentBranch() string {
	out, err := exec.Command("git", "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine branch (detached HEAD?): %v\n", err)
		return "HEAD"
	}
	return strings.TrimSpace(string(out))
}

// filesToEntries converts parsed diff file results to slim attestation entries
// (strips Content from hunks to keep DB rows small).
func filesToEntries(files []diffReviewFileResult) []attestationFileEntry {
	entries := make([]attestationFileEntry, len(files))
	for i, f := range files {
		hunks := make([]attestationHunkRange, len(f.Hunks))
		for j, h := range f.Hunks {
			hunks[j] = attestationHunkRange{
				OldStartLine: h.OldStartLine,
				OldLineCount: h.OldLineCount,
				NewStartLine: h.NewStartLine,
				NewLineCount: h.NewLineCount,
			}
		}
		entries[i] = attestationFileEntry{
			FilePath: f.FilePath,
			Hunks:    hunks,
		}
	}
	return entries
}

// insertReviewSession inserts a new review session into the database.
func insertReviewSession(db *sql.DB, treeHash, branch, action string, files []attestationFileEntry, reviewID string) error {
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("failed to marshal diff files: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO review_sessions (tree_hash, branch, action, timestamp, diff_files, review_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		treeHash, branch, action, time.Now().UTC().Format(time.RFC3339), string(filesJSON), reviewID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert review session: %w", err)
	}
	return nil
}

// countIterations returns the total number of review sessions for the given branch.
func countIterations(db *sql.DB, branch string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM review_sessions WHERE branch = ?`, branch).Scan(&count)
	return count, err
}

// getPriorReviewedSessions returns all "reviewed" sessions for the branch,
// ordered by timestamp ascending.
func getPriorReviewedSessions(db *sql.DB, branch string) ([]reviewSession, error) {
	rows, err := db.Query(
		`SELECT id, tree_hash, branch, action, timestamp, diff_files, review_id
		 FROM review_sessions
		 WHERE branch = ? AND action = 'reviewed'
		 ORDER BY timestamp ASC`,
		branch,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []reviewSession
	for rows.Next() {
		var s reviewSession
		var ts, diffFiles, reviewID string
		if err := rows.Scan(&s.ID, &s.TreeHash, &s.Branch, &s.Action, &ts, &diffFiles, &reviewID); err != nil {
			return nil, err
		}
		parsedTime, parseErr := time.Parse(time.RFC3339, ts)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed timestamp %q in review session %d: %v\n", ts, s.ID, parseErr)
		}
		s.Timestamp = parsedTime
		s.DiffFiles = diffFiles
		s.ReviewID = reviewID
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// cleanupReviewSessions deletes all sessions for the given branch.
// Called after a successful commit to start fresh.
func cleanupReviewSessions(db *sql.DB, branch string) (int64, error) {
	result, err := db.Exec(`DELETE FROM review_sessions WHERE branch = ?`, branch)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// cleanupAllSessions deletes ALL sessions from the database.
func cleanupAllSessions(db *sql.DB) (int64, error) {
	result, err := db.Exec(`DELETE FROM review_sessions`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// computePriorCoverage calculates how many lines in the current diff were
// already AI-reviewed in prior iterations (for the same branch).
//
// The algorithm:
//  1. Get all "reviewed" sessions for the current branch
//  2. For each prior session, compute which of the current diff's new-side lines
//     were already covered by that review (i.e., lines that haven't changed since)
//  3. Accumulate coverage across all prior sessions (union of covered lines)
//  4. Return iteration count and coverage percentage
func computePriorCoverage(db *sql.DB, branch, currentTreeHash string, currentFiles []attestationFileEntry) (coverageResult, error) {
	result := coverageResult{}

	// Count total iterations (all actions)
	totalIter, err := countIterations(db, branch)
	if err != nil {
		return result, err
	}
	result.Iterations = totalIter + 1 // +1 for the current one being recorded

	// Get prior "reviewed" sessions
	priorSessions, err := getPriorReviewedSessions(db, branch)
	if err != nil {
		return result, err
	}
	result.PriorReviewCount = len(priorSessions)

	if len(priorSessions) == 0 || len(currentFiles) == 0 {
		// No prior AI reviews or no files in current diff — 0% coverage
		result.TotalLines = countTotalNewLines(currentFiles)
		return result, nil
	}

	// Build set of current file paths for quick lookup
	currentFileSet := make(map[string][]attestationHunkRange)
	for _, f := range currentFiles {
		currentFileSet[f.FilePath] = f.Hunks
	}

	// Total new-side lines in the current diff
	result.TotalLines = countTotalNewLines(currentFiles)
	if result.TotalLines == 0 {
		return result, nil
	}

	// coveredLines tracks which (file, line) pairs are covered by prior reviews.
	// Key: "filepath:linenum"
	coveredLines := make(map[string]bool)

	for _, session := range priorSessions {
		if session.TreeHash == currentTreeHash {
			// Same tree — all current lines are covered by this review
			for _, f := range currentFiles {
				markAllNewLines(coveredLines, f)
			}
			continue
		}

		// Find what changed between the prior reviewed tree and the current tree
		changedFiles, err := diffTreeFiles(session.TreeHash, currentTreeHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping review session %d: could not diff trees %s..%s: %v\n", session.ID, session.TreeHash, currentTreeHash, err)
			continue
		}

		changedFileSet := make(map[string]bool)
		for _, cf := range changedFiles {
			changedFileSet[cf] = true
		}

		// Parse the prior session's diff files
		var priorFiles []attestationFileEntry
		if session.DiffFiles != "" {
			if umErr := json.Unmarshal([]byte(session.DiffFiles), &priorFiles); umErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: malformed diff_files in review session %d: %v\n", session.ID, umErr)
				continue
			}
		}

		priorFileMap := make(map[string][]attestationHunkRange)
		for _, pf := range priorFiles {
			priorFileMap[pf.FilePath] = pf.Hunks
		}

		for _, cf := range currentFiles {
			if !changedFileSet[cf.FilePath] {
				// File didn't change since prior review — all new-side lines are covered
				markAllNewLines(coveredLines, cf)
			} else {
				// File changed — compute line-level overlap
				if priorHunks, ok := priorFileMap[cf.FilePath]; ok {
					markOverlappingLines(coveredLines, cf.FilePath, cf.Hunks, priorHunks, session.TreeHash, currentTreeHash)
				}
			}
		}
	}

	result.CoveredLines = len(coveredLines)
	if result.TotalLines > 0 {
		result.PriorAICovPct = float64(result.CoveredLines) / float64(result.TotalLines) * 100
	}

	return result, nil
}

// countTotalNewLines returns the sum of all new-side line counts across all hunks.
func countTotalNewLines(files []attestationFileEntry) int {
	total := 0
	for _, f := range files {
		for _, h := range f.Hunks {
			total += h.NewLineCount
		}
	}
	return total
}

// markAllNewLines marks all new-side lines for a file as covered.
func markAllNewLines(covered map[string]bool, f attestationFileEntry) {
	for _, h := range f.Hunks {
		for line := h.NewStartLine; line < h.NewStartLine+h.NewLineCount; line++ {
			covered[fmt.Sprintf("%s:%d", f.FilePath, line)] = true
		}
	}
}

// diffTreeFiles returns the list of file paths that changed between two tree objects.
func diffTreeFiles(tree1, tree2 string) ([]string, error) {
	out, err := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", tree1, tree2).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree failed: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// markOverlappingLines marks lines in the current file that were covered by a prior
// review, accounting for changes between the two trees. For lines in the current
// diff that fall entirely within unchanged regions relative to the prior review,
// they're considered covered.
func markOverlappingLines(covered map[string]bool, filePath string, currentHunks, priorHunks []attestationHunkRange, priorTree, currentTree string) {
	// Get the detailed diff between the two trees for this specific file
	interTreeDiff, err := diffTreeFileHunks(priorTree, currentTree, filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not diff %s between trees %s..%s: %v\n", filePath, priorTree[:8], currentTree[:8], err)
		return
	}

	// Build a set of line ranges that changed between the two trees (new-side)
	changedRanges := make([]lineRange, 0, len(interTreeDiff))
	for _, h := range interTreeDiff {
		changedRanges = append(changedRanges, lineRange{
			Start: h.NewStartLine,
			End:   h.NewStartLine + h.NewLineCount - 1,
		})
	}

	// For each line in the current diff's hunks, if that line is NOT in any
	// inter-tree changed range, it was present in the prior reviewed tree and
	// is therefore covered.
	for _, h := range currentHunks {
		for line := h.NewStartLine; line < h.NewStartLine+h.NewLineCount; line++ {
			if !lineInRanges(line, changedRanges) {
				covered[fmt.Sprintf("%s:%d", filePath, line)] = true
			}
		}
	}
}

type lineRange struct {
	Start, End int
}

func lineInRanges(line int, ranges []lineRange) bool {
	for _, r := range ranges {
		if line >= r.Start && line <= r.End {
			return true
		}
	}
	return false
}

// diffTreeFileHunks returns parsed hunk ranges for changes in a specific file
// between two tree objects.
func diffTreeFileHunks(tree1, tree2, filePath string) ([]attestationHunkRange, error) {
	out, err := exec.Command("git", "diff", tree1, tree2, "--", filePath).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s %s -- %s failed: %w", tree1, tree2, filePath, err)
	}

	return parseHunkRangesFromDiff(string(out)), nil
}

// parseHunkRangesFromDiff extracts hunk line ranges from raw diff output.
func parseHunkRangesFromDiff(diffStr string) []attestationHunkRange {
	re := regexp.MustCompile(`@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)
	matches := re.FindAllStringSubmatch(diffStr, -1)

	hunks := make([]attestationHunkRange, 0, len(matches))
	for _, m := range matches {
		oldStart, err := strconv.Atoi(m[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed hunk old_start %q: %v\n", m[1], err)
			continue
		}
		oldCount := 1
		if m[2] != "" {
			parsed, err := strconv.Atoi(m[2])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: malformed hunk old_count %q: %v\n", m[2], err)
				continue
			}
			oldCount = parsed
		}
		newStart, err := strconv.Atoi(m[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed hunk new_start %q: %v\n", m[3], err)
			continue
		}
		newCount := 1
		if m[4] != "" {
			parsed, err := strconv.Atoi(m[4])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: malformed hunk new_count %q: %v\n", m[4], err)
				continue
			}
			newCount = parsed
		}
		hunks = append(hunks, attestationHunkRange{
			OldStartLine: oldStart,
			OldLineCount: oldCount,
			NewStartLine: newStart,
			NewLineCount: newCount,
		})
	}
	return hunks
}

// recordAndComputeCoverage is a convenience function that opens the DB,
// records the session, computes coverage, and returns the result.
// It is the main entry point for all review actions (reviewed/skipped/vouched).
func recordAndComputeCoverage(action string, parsedFiles []diffReviewFileResult, reviewID string, verbose bool) (coverageResult, error) {
	db, err := openReviewDB()
	if err != nil {
		if verbose {
			fmt.Printf("Warning: could not open review DB: %v (coverage tracking disabled)\n", err)
		}
		return coverageResult{Iterations: 1}, nil
	}
	defer db.Close()

	treeHash, err := currentTreeHash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine current tree hash: %v (coverage tracking disabled)\n", err)
		return coverageResult{Iterations: 1}, nil
	}

	branch := currentBranch()
	entries := filesToEntries(parsedFiles)

	// Compute coverage BEFORE inserting current session
	cov, err := computePriorCoverage(db, branch, treeHash, entries)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: coverage computation failed: %v\n", err)
		}
		cov = coverageResult{Iterations: 1}
	}

	// For "reviewed" action, the current review covers 100% of the lines it touches
	// The coverage % reflects how much was ALREADY covered by PRIOR reviews
	// (not including the current one)

	// Insert the current session
	if err := insertReviewSession(db, treeHash, branch, action, entries, reviewID); err != nil {
		if verbose {
			fmt.Printf("Warning: failed to record review session: %v\n", err)
		}
	}

	return cov, nil
}

// runReviewDBCleanup deletes all review sessions for the current branch.
// Called from the post-commit hook via "lrc review-cleanup".
func runReviewDBCleanup(verbose bool) error {
	db, err := openReviewDB()
	if err != nil {
		if verbose {
			fmt.Printf("Warning: could not open review DB for cleanup: %v\n", err)
		}
		return nil
	}
	defer db.Close()

	branch := currentBranch()
	affected, err := cleanupReviewSessions(db, branch)
	if err != nil {
		return fmt.Errorf("failed to clean up review sessions: %w", err)
	}
	if verbose && affected > 0 {
		fmt.Printf("lrc: cleaned up %d review session(s) for branch %s\n", affected, branch)
	}
	return nil
}
