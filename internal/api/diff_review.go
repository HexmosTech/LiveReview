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
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

const diffReviewBypassKey = "lr-internal-2024"

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
	if err := ensureBypassKey(c); err != nil {
		return err
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

	rm := NewReviewManager(s.db)
	reviewRecord, err := rm.CreateReviewWithOrg(repoName, "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{"source": "diff-review"}, 1)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create review record"})
	}

	// Immediately mark as processing and persist preloaded changes for polling.
	_ = rm.UpdateReviewStatus(reviewRecord.ID, "processing")
	if err := rm.MergeReviewMetadata(reviewRecord.ID, map[string]interface{}{"preloaded_changes": modelDiffs}); err != nil {
		log.Printf("[WARN] failed to store preloaded_changes for review %d: %v", reviewRecord.ID, err)
	}

	aiConfig, err := s.getAIConfigFromDatabase(context.Background(), 1)
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

	go s.runDiffReview(reviewRequest, rm, reviewRecord.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"review_id": fmt.Sprintf("%d", reviewRecord.ID),
		"status":    "processing",
	})
}

// GetDiffReviewStatus returns processing status or completed results for a diff review.
func (s *Server) GetDiffReviewStatus(c echo.Context) error {
	if err := ensureBypassKey(c); err != nil {
		return err
	}

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
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":    reviewRecord.Status,
			"review_id": fmt.Sprintf("%d", reviewRecord.ID),
		})
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "completed",
		"review_id": fmt.Sprintf("%d", reviewRecord.ID),
		"summary":   result.Summary,
		"files":     files,
	})
}

// runDiffReview executes the review asynchronously and persists results.
func (s *Server) runDiffReview(request review.ReviewRequest, rm *ReviewManager, reviewID int64) {
	result := review.NewService(review.NewStandardProviderFactory(), review.NewStandardAIProviderFactory(), review.DefaultReviewConfig()).ProcessReview(context.Background(), request)

	status := "failed"
	summary := ""
	var comments []*models.ReviewComment

	if result != nil {
		if result.Success {
			status = "completed"
		}
		summary = result.Summary
		comments = result.Comments
	}

	if err := rm.UpdateReviewStatus(reviewID, status); err != nil {
		log.Printf("[WARN] failed to update review status for %d: %v", reviewID, err)
	}

	payload := diffReviewResult{Summary: summary, Comments: comments}
	if err := rm.MergeReviewMetadata(reviewID, map[string]interface{}{"review_result": payload}); err != nil {
		log.Printf("[WARN] failed to persist review_result for %d: %v", reviewID, err)
	}
}

func ensureBypassKey(c echo.Context) error {
	if c.Request().Header.Get("X-Bypass-Key") != diffReviewBypassKey {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	return nil
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
