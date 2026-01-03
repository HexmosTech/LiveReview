package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/naming"
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

// diffReviewRequest models the incoming POST payload for diff reviews.
type diffReviewRequest struct {
	DiffZipBase64 string `json:"diff_zip_base64"`
	RepoName      string `json:"repo_name"`
}

// diffReviewResult holds persisted review output that is safe to marshal.
type diffReviewResult struct {
	Summary  string                  `json:"summary"`
	Comments []*models.ReviewComment `json:"comments"`
}

// DiffReview accepts a base64-encoded ZIP containing a unified diff and triggers a review.
func (s *Server) DiffReview(c echo.Context) error {
	// API key authentication is handled by middleware
	// Extract user and org context from middleware
	orgID := c.Get("org_id").(int64)
	userID := c.Get("user_id").(int64)
	log.Printf("[DiffReview] Extracted from context: userID=%d, orgID=%d", userID, orgID)

	// Fetch user info for author tracking
	var userEmail, authorName, authorUsername string
	var user models.User
	err := s.db.QueryRow(`
		SELECT id, email, first_name, last_name
		FROM users
		WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email, &user.FirstName, &user.LastName)

	if err == nil {
		userEmail = user.Email
		log.Printf("[DiffReview] User fetched: id=%d, email=%s, firstName=%v, lastName=%v",
			user.ID, user.Email, user.FirstName, user.LastName)

		// Build author name from first/last name if available
		if user.FirstName != nil && user.LastName != nil {
			authorName = strings.TrimSpace(*user.FirstName + " " + *user.LastName)
		} else if user.FirstName != nil {
			authorName = *user.FirstName
		} else if user.LastName != nil {
			authorName = *user.LastName
		}
		// Use email username as fallback for authorUsername
		if emailParts := strings.Split(user.Email, "@"); len(emailParts) > 0 {
			authorUsername = emailParts[0]
		}
		log.Printf("[DiffReview] Built author info: authorName='%s', authorUsername='%s'", authorName, authorUsername)
	} else {
		log.Printf("[DiffReview] ERROR fetching user: %v", err)
	}

	var req diffReviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.DiffZipBase64) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "diff_zip_base64 is required"})
	}

	localDiffs, err := parseDiffZipBase64(req.DiffZipBase64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to parse diff: %v", err)})
	}

	modelDiffs := convertLocalDiffs(localDiffs)
	repoName := strings.TrimSpace(req.RepoName)
	if repoName == "" {
		repoName = "cli-diff"
	}

	// Generate friendly name for CLI review
	friendlyName := naming.GenerateFriendlyName()
	log.Printf("[DiffReview] Generated friendlyName='%s'", friendlyName)

	rm := NewReviewManager(s.db)
	log.Printf("[DiffReview] Creating review with: repoName=%s, userEmail=%s, orgID=%d, friendlyName=%s, authorName=%s, authorUsername=%s",
		repoName, userEmail, orgID, friendlyName, authorName, authorUsername)
	reviewRecord, err := rm.CreateReviewWithOrg(repoName, "", "", "", "cli_diff", userEmail, "cli", nil, map[string]interface{}{"source": "diff-review"}, orgID, friendlyName, authorName, authorUsername)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create review record"})
	}

	// Immediately mark as processing and persist preloaded changes for polling.
	_ = rm.UpdateReviewStatus(reviewRecord.ID, "processing")
	if err := rm.MergeReviewMetadata(reviewRecord.ID, map[string]interface{}{"preloaded_changes": modelDiffs}); err != nil {
		log.Printf("[WARN] failed to store preloaded_changes for review %d: %v", reviewRecord.ID, err)
	}

	aiConfig, err := s.getAIConfigFromDatabase(context.Background(), orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to load AI config: %v", err)})
	}

	reviewRequest := review.ReviewRequest{
		URL:              fmt.Sprintf("cli-diff:%s", repoName),
		ReviewID:         fmt.Sprintf("%d", reviewRecord.ID),
		Provider:         review.ProviderConfig{Type: "cli", URL: "", Token: "", Config: map[string]interface{}{}},
		AI:               aiConfig,
		PreloadedChanges: modelDiffs,
	}

	go s.runDiffReview(reviewRequest, rm, reviewRecord.ID, orgID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"review_id":     fmt.Sprintf("%d", reviewRecord.ID),
		"status":        "processing",
		"friendly_name": friendlyName,
		"user_email":    userEmail,
	})
}

// GetDiffReviewStatus returns processing status or completed results for a diff review.
func (s *Server) GetDiffReviewStatus(c echo.Context) error {
	// API key authentication is handled by middleware

	reviewIDStr := c.Param("review_id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid review_id"})
	}

	rm := NewReviewManager(s.db)
	reviewRecord, err := rm.GetReview(reviewID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "review not found"})
	}

	if reviewRecord.Status != "completed" {
		meta := map[string]interface{}{}
		if len(reviewRecord.Metadata) > 0 {
			_ = json.Unmarshal(reviewRecord.Metadata, &meta)
		}

		failureReason, _ := meta["failure_reason"].(string)
		response := map[string]interface{}{
			"status":    reviewRecord.Status,
			"review_id": fmt.Sprintf("%d", reviewRecord.ID),
		}

		if reviewRecord.FriendlyName != nil {
			response["friendly_name"] = *reviewRecord.FriendlyName
		}

		if failureReason != "" {
			response["message"] = failureReason
		}

		return c.JSON(http.StatusOK, response)
	}

	meta := map[string]interface{}{}
	if len(reviewRecord.Metadata) > 0 {
		_ = json.Unmarshal(reviewRecord.Metadata, &meta)
	}

	preloaded, err := decodePreloadedChanges(meta)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to decode preloaded changes: %v", err)})
	}

	result, err := decodeReviewResult(meta)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to decode review result: %v", err)})
	}

	files := buildDiffFiles(preloaded, result.Comments)

	response := map[string]interface{}{
		"status":    "completed",
		"review_id": fmt.Sprintf("%d", reviewRecord.ID),
		"summary":   result.Summary,
		"files":     files,
	}

	// Include friendly_name if available
	if reviewRecord.FriendlyName != nil {
		response["friendly_name"] = *reviewRecord.FriendlyName
	}

	// Include ai_summary_title if available
	if aiSummaryTitle, ok := meta["ai_summary_title"].(string); ok && aiSummaryTitle != "" {
		response["ai_summary_title"] = aiSummaryTitle
	}

	return c.JSON(http.StatusOK, response)
}

// runDiffReview executes the review asynchronously and persists results.
func (s *Server) runDiffReview(request review.ReviewRequest, rm *ReviewManager, reviewID int64, orgID int64) {
	// Initialize logger with event sink for UI visibility
	logger, err := logging.StartReviewLoggingWithIDs(fmt.Sprintf("%d", reviewID), reviewID, orgID)
	if err != nil {
		log.Printf("[ERROR] Failed to start logging for review %d: %v", reviewID, err)
	}

	if logger != nil {
		// Attach event sink so logs go to review_events table for UI
		eventSink := NewDatabaseEventSink(s.db)
		logger.SetEventSink(eventSink)
		logger.LogSection("CLI DIFF REVIEW STARTED")
		logger.Log("Review ID: %d", reviewID)
		logger.Log("Organization ID: %d", orgID)
		logger.Log("Processing diff from CLI...")
	}

	// Mark as in progress
	_ = rm.UpdateReviewStatus(reviewID, "in_progress")

	if logger != nil {
		logger.LogSection("PROCESSING REVIEW")
		logger.Log("Analyzing changes and generating comments...")
	}

	result := review.NewService(review.NewStandardProviderFactory(), review.NewStandardAIProviderFactory(), review.DefaultReviewConfig()).ProcessReview(context.Background(), request)

	status := "failed"
	summary := ""
	var comments []*models.ReviewComment
	failureReason := ""

	if result != nil {
		if result.Success {
			status = "completed"
			if logger != nil {
				logger.LogSection("REVIEW COMPLETED")
				logger.Log("Successfully generated %d comments", len(result.Comments))
			}
		} else {
			if result.Error != nil {
				failureReason = result.Error.Error()
			}
			if failureReason == "" {
				failureReason = "review processing encountered errors"
			}
			if logger != nil {
				logger.LogSection("REVIEW FAILED")
				logger.Log("Review processing encountered errors: %s", failureReason)
			}
		}
		summary = result.Summary
		comments = result.Comments
	} else {
		failureReason = "review processing returned no result"
		if logger != nil {
			logger.LogSection("REVIEW FAILED")
			logger.Log("Review processing returned no result")
		}
	}

	if err := rm.UpdateReviewStatus(reviewID, status); err != nil {
		log.Printf("[WARN] failed to update review status for %d: %v", reviewID, err)
	}

	payload := diffReviewResult{Summary: summary, Comments: comments}
	meta := map[string]interface{}{"review_result": payload}
	if failureReason != "" {
		meta["failure_reason"] = failureReason
	}
	if err := rm.MergeReviewMetadata(reviewID, meta); err != nil {
		log.Printf("[WARN] failed to persist review_result for %d: %v", reviewID, err)
	}

	// Persist AI summary title for later display (extract first heading only)
	if summary != "" {
		title := extractFirstHeading(summary)
		if title != "" {
			if err := rm.MergeReviewMetadata(reviewID, map[string]interface{}{"ai_summary_title": title}); err != nil {
				log.Printf("[WARN] failed to persist ai_summary_title for %d: %v", reviewID, err)
			}
		}
	}
}

// extractFirstHeading extracts the first markdown heading (# ...) from a markdown text
func extractFirstHeading(markdown string) string {
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Remove the # characters and leading/trailing whitespace
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			return heading
		}
	}
	return ""
}

// parseDiffZipBase64 decodes the client payload (base64 zip containing a unified diff)
// into parsed local diffs without touching the database. This is used by the handler
// and contract-style unit tests to keep the input/output surface consistent.
func parseDiffZipBase64(encoded string) ([]lib.LocalCodeDiff, error) {
	zipBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode diff_zip_base64: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "lr-diff-review-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "diff.zip")
	if err := os.WriteFile(zipPath, zipBytes, 0600); err != nil {
		return nil, fmt.Errorf("failed to persist uploaded zip: %w", err)
	}

	extractedFiles, err := extractZip(zipPath, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract zip: %w", err)
	}
	if len(extractedFiles) == 0 {
		return nil, fmt.Errorf("zip archive contained no files")
	}

	diffContent, err := os.ReadFile(extractedFiles[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read extracted diff: %w", err)
	}

	parser := lib.NewLocalParser()
	localDiffs, err := parser.Parse(string(diffContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}

	return localDiffs, nil
}

func extractZip(zipPath, dest string) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var extracted []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		cleaned := filepath.Clean(f.Name)
		targetPath := filepath.Join(dest, cleaned)
		if !strings.HasPrefix(targetPath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return extracted, err
		}
		rc, err := f.Open()
		if err != nil {
			return extracted, err
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			_ = rc.Close()
			return extracted, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			_ = rc.Close()
			return extracted, err
		}
		out.Close()
		_ = rc.Close()

		extracted = append(extracted, targetPath)
	}
	return extracted, nil
}

func convertLocalDiffs(localDiffs []lib.LocalCodeDiff) []*models.CodeDiff {
	converted := make([]*models.CodeDiff, 0, len(localDiffs))
	for _, ld := range localDiffs {
		converted = append(converted, convertLocalToModelDiff(ld))
	}
	return converted
}

func convertLocalToModelDiff(local lib.LocalCodeDiff) *models.CodeDiff {
	hunks := make([]models.DiffHunk, 0, len(local.Hunks))
	for _, h := range local.Hunks {
		hunks = append(hunks, convertLocalHunk(h))
	}

	filePath := local.NewPath
	if strings.TrimSpace(filePath) == "" {
		filePath = local.OldPath
	}

	return &models.CodeDiff{
		FilePath:    filePath,
		OldContent:  "",
		NewContent:  "",
		Hunks:       hunks,
		CommitID:    "",
		FileType:    filepath.Ext(filePath),
		IsDeleted:   false,
		IsNew:       false,
		IsRenamed:   false,
		OldFilePath: local.OldPath,
	}
}

func convertLocalHunk(h lib.LocalDiffHunk) models.DiffHunk {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStartLine, h.OldLineCount, h.NewStartLine, h.NewLineCount))
	if strings.TrimSpace(h.HeaderText) != "" {
		buf.WriteByte(' ')
		buf.WriteString(strings.TrimSpace(h.HeaderText))
	}
	buf.WriteByte('\n')

	for _, line := range h.Lines {
		prefix := " "
		switch line.LineType {
		case "added":
			prefix = "+"
		case "deleted":
			prefix = "-"
		}
		buf.WriteString(prefix)
		buf.WriteString(line.Content)
		buf.WriteByte('\n')
	}

	content := strings.TrimSuffix(buf.String(), "\n")
	return models.DiffHunk{
		OldStartLine: h.OldStartLine,
		OldLineCount: h.OldLineCount,
		NewStartLine: h.NewStartLine,
		NewLineCount: h.NewLineCount,
		Content:      content,
	}
}

func decodePreloadedChanges(meta map[string]interface{}) ([]models.CodeDiff, error) {
	raw, ok := meta["preloaded_changes"]
	if !ok {
		return nil, fmt.Errorf("preloaded_changes missing")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var diffs []models.CodeDiff
	if err := json.Unmarshal(data, &diffs); err != nil {
		return nil, err
	}
	return diffs, nil
}

func decodeReviewResult(meta map[string]interface{}) (diffReviewResult, error) {
	raw, ok := meta["review_result"]
	if !ok {
		return diffReviewResult{}, fmt.Errorf("review_result missing")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return diffReviewResult{}, err
	}
	var res diffReviewResult
	if err := json.Unmarshal(data, &res); err != nil {
		return diffReviewResult{}, err
	}
	return res, nil
}

func buildDiffFiles(diffs []models.CodeDiff, comments []*models.ReviewComment) []map[string]interface{} {
	files := make([]map[string]interface{}, 0, len(diffs))
	for _, diff := range diffs {
		matched := filterCommentsForFile(diff.FilePath, diff.Hunks, comments)
		files = append(files, map[string]interface{}{
			"file_path": diff.FilePath,
			"hunks":     marshalHunks(diff.Hunks),
			"comments":  matched,
		})
	}
	return files
}

func marshalHunks(hunks []models.DiffHunk) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(hunks))
	for _, h := range hunks {
		out = append(out, map[string]interface{}{
			"old_start_line": h.OldStartLine,
			"old_line_count": h.OldLineCount,
			"new_start_line": h.NewStartLine,
			"new_line_count": h.NewLineCount,
			"content":        h.Content,
		})
	}
	return out
}

func filterCommentsForFile(filePath string, hunks []models.DiffHunk, comments []*models.ReviewComment) []map[string]interface{} {
	matched := []map[string]interface{}{}
	for _, comment := range comments {
		if comment == nil || strings.TrimSpace(comment.FilePath) != strings.TrimSpace(filePath) {
			continue
		}
		if !lineWithinHunks(comment.Line, hunks) {
			log.Printf("[WARN] comment line %d is out of range for file %s", comment.Line, filePath)
		}
		matched = append(matched, map[string]interface{}{
			"line":     comment.Line,
			"content":  comment.Content,
			"severity": string(comment.Severity),
			"category": comment.Category,
		})
	}
	return matched
}

func lineWithinHunks(line int, hunks []models.DiffHunk) bool {
	for _, h := range hunks {
		start := h.NewStartLine
		end := h.NewStartLine + h.NewLineCount
		if line >= start && line <= end {
			return true
		}
	}
	return false
}
