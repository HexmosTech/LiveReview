package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/cmd/mrmodel/lib"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/naming"
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
	"github.com/livereview/storage/archive"
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

const (
	maxExtractedFileBytes  = 25 << 20  // 25 MiB per extracted file
	maxExtractedTotalBytes = 200 << 20 // 200 MiB across all extracted files
)

// DiffReview accepts a base64-encoded ZIP containing a unified diff and triggers a review.
func (s *Server) DiffReview(c echo.Context) error {
	// API key authentication is handled by middleware
	// Extract user and org context from middleware
	orgID := c.Get("org_id").(int64)
	userID := c.Get("user_id").(int64)
	actorUserID := userID
	log.Printf("[DiffReview] Extracted from context: userID=%d, orgID=%d", userID, orgID)

	// Fetch user info for author tracking
	var userEmail, authorName, authorUsername string
	user, err := archive.DiffReviewLoadUser(s.db, userID)

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
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.DiffZipBase64) == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "diff_zip_base64 is required")
	}

	localDiffs, err := parseDiffZipBase64(req.DiffZipBase64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("failed to parse diff: %v", err))
	}
	billableLOC := CalculateEffectiveDiffLOCFromLocalDiffs(localDiffs)
	c.Set(EnvelopeOperationTypeContextKey, "diff_review")
	c.Set(EnvelopeTriggerSourceContextKey, "api")
	c.Set(EnvelopeOperationBillableLOCContextKey, billableLOC)

	planCode := license.PlanFree30K
	if planCtx, ok := c.Get(apimiddleware.PlanContextKey).(apimiddleware.PlanContext); ok && planCtx.PlanType != "" {
		planCode = planCtx.PlanType
	}

	accountingService := license.NewLOCAccountingService(s.db)
	preflightResult, err := accountingService.CheckPreflight(context.Background(), license.LOCPreflightInput{
		OrgID:       orgID,
		RequiredLOC: billableLOC,
		PlanCode:    planCode,
	})
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed quota preflight: %v", err))
	}
	applyPreflightToEnvelopeContext(c, preflightResult)

	if preflightResult.Blocked {
		errorCode := "quota_exceeded"
		errorMessage := "monthly LOC quota exceeded for this operation"
		if preflightResult.BlockReason == "trial_readonly" {
			errorCode = "trial_readonly"
			errorMessage = "trial period ended; review operations are read-only until plan update"
		}
		return JSONWithEnvelope(c, http.StatusForbidden, map[string]interface{}{
			"error":        errorMessage,
			"error_code":   errorCode,
			"required_loc": billableLOC,
		})
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
	preflightMeta := map[string]interface{}{
		"source":                 "diff-review",
		"operation_billable_loc": billableLOC,
		"loc_used_month":         preflightResult.LOCUsedMonth,
		"loc_remaining_month":    preflightResult.LOCRemainingMonth,
		"usage_percent":          preflightResult.UsagePercent,
		"threshold_state":        preflightResult.ThresholdState,
		"blocked":                preflightResult.Blocked,
		"trial_readonly":         preflightResult.TrialReadOnly,
		"billing_period_start":   preflightResult.BillingPeriodStart.Format(time.RFC3339),
		"billing_period_end":     preflightResult.BillingPeriodEnd.Format(time.RFC3339),
		"reset_at":               preflightResult.BillingPeriodEnd.Format(time.RFC3339),
	}
	reviewRecord, err := rm.CreateReviewWithOrg(repoName, "", "", "", "cli_diff", userEmail, "cli", nil, preflightMeta, orgID, friendlyName, authorName, authorUsername)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to create review record")
	}
	operationID := fmt.Sprintf("diff-review:%d", reviewRecord.ID)
	idempotencyKey := operationID
	c.Set(EnvelopeOperationIDContextKey, operationID)
	c.Set(EnvelopeIdempotencyKeyContextKey, idempotencyKey)

	// Immediately mark as processing and persist preloaded changes for polling.
	_ = rm.UpdateReviewStatus(reviewRecord.ID, "processing")
	if err := rm.MergeReviewMetadata(reviewRecord.ID, map[string]interface{}{"preloaded_changes": modelDiffs, "operation_billable_loc": billableLOC}); err != nil {
		log.Printf("[WARN] failed to store preloaded_changes for review %d: %v", reviewRecord.ID, err)
	}

	aiConfig, err := s.getAIConfigFromDatabase(context.Background(), orgID, planCode)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load AI config: %v", err))
	}

	reviewRequest := review.ReviewRequest{
		URL:              fmt.Sprintf("cli-diff:%s", repoName),
		ReviewID:         fmt.Sprintf("%d", reviewRecord.ID),
		Provider:         review.ProviderConfig{Type: "cli", URL: "", Token: "", Config: map[string]interface{}{}},
		AI:               aiConfig,
		PreloadedChanges: modelDiffs,
	}

	go s.runDiffReview(reviewRequest, rm, reviewRecord.ID, orgID, billableLOC, actorUserID, userEmail)

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
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
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid review_id")
	}

	rm := NewReviewManager(s.db)
	reviewRecord, err := rm.GetReview(reviewID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "review not found")
	}

	if reviewRecord.Status != "completed" {
		meta := map[string]interface{}{}
		if len(reviewRecord.Metadata) > 0 {
			_ = json.Unmarshal(reviewRecord.Metadata, &meta)
		}
		applyEnvelopeUsageFromMetadata(c, meta)
		if v, ok := readOperationBillableLOC(meta); ok {
			c.Set(EnvelopeOperationTypeContextKey, "diff_review")
			c.Set(EnvelopeTriggerSourceContextKey, "api")
			c.Set(EnvelopeOperationBillableLOCContextKey, v)
		}
		if v, ok := readStringMeta(meta, "accounted_at"); ok {
			c.Set(EnvelopeAccountedAtContextKey, v)
		}
		if v, ok := readStringMeta(meta, "operation_id"); ok {
			c.Set(EnvelopeOperationIDContextKey, v)
		}
		if v, ok := readStringMeta(meta, "idempotency_key"); ok {
			c.Set(EnvelopeIdempotencyKeyContextKey, v)
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

		return JSONWithEnvelope(c, http.StatusOK, response)
	}

	meta := map[string]interface{}{}
	if len(reviewRecord.Metadata) > 0 {
		_ = json.Unmarshal(reviewRecord.Metadata, &meta)
	}
	applyEnvelopeUsageFromMetadata(c, meta)
	if v, ok := readOperationBillableLOC(meta); ok {
		c.Set(EnvelopeOperationTypeContextKey, "diff_review")
		c.Set(EnvelopeTriggerSourceContextKey, "api")
		c.Set(EnvelopeOperationBillableLOCContextKey, v)
	}
	if v, ok := readStringMeta(meta, "accounted_at"); ok {
		c.Set(EnvelopeAccountedAtContextKey, v)
	}
	if v, ok := readStringMeta(meta, "operation_id"); ok {
		c.Set(EnvelopeOperationIDContextKey, v)
	}
	if v, ok := readStringMeta(meta, "idempotency_key"); ok {
		c.Set(EnvelopeIdempotencyKeyContextKey, v)
	}

	preloaded, err := decodePreloadedChanges(meta)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to decode preloaded changes: %v", err))
	}

	result, err := decodeReviewResult(meta)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to decode review result: %v", err))
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

	return JSONWithEnvelope(c, http.StatusOK, response)
}

// runDiffReview executes the review asynchronously and persists results.
func (s *Server) runDiffReview(request review.ReviewRequest, rm *ReviewManager, reviewID int64, orgID int64, billableLOC int64, actorUserID int64, actorEmail string) {
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
			accountedAt := time.Now().UTC().Format(time.RFC3339)
			accountingService := license.NewLOCAccountingService(s.db)
			operationID := fmt.Sprintf("diff-review:%d", reviewID)
			idempotencyKey := operationID
			var actorUserIDPtr *int64
			if actorUserID > 0 {
				resolvedActorUserID := actorUserID
				actorUserIDPtr = &resolvedActorUserID
			}
			if err := accountingService.AccountSuccess(context.Background(), license.LOCAccountSuccessInput{
				OrgID:          orgID,
				ReviewID:       reviewID,
				ActorUserID:    actorUserIDPtr,
				ActorEmail:     strings.TrimSpace(actorEmail),
				OperationType:  "diff_review",
				TriggerSource:  "api",
				OperationID:    operationID,
				IdempotencyKey: idempotencyKey,
				BillableLOC:    billableLOC,
				Provider:       result.Provider,
				Model:          result.Model,
				PricingVersion: result.PricingVersion,
				InputTokens:    result.InputTokens,
				OutputTokens:   result.OutputTokens,
				CostUSD:        result.CostUSD,
			}); err != nil {
				log.Printf("[WARN] failed success-only accounting for review %d: %v", reviewID, err)
			} else {
				meta := map[string]interface{}{"accounted_at": accountedAt, "operation_id": operationID, "idempotency_key": idempotencyKey}
				for k, v := range aiExecutionMetadataFromConfig(request.AI.Config) {
					meta[k] = v
				}
				if err := rm.MergeReviewMetadata(reviewID, meta); err != nil {
					log.Printf("[WARN] failed to store accounted_at for review %d: %v", reviewID, err)
				}
			}
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

	tempDir, err := archive.DiffReviewCreateTempWorkspace()
	if err != nil {
		return nil, fmt.Errorf("failed to create temp workspace: %w", err)
	}
	defer func() {
		if cleanupErr := archive.DiffReviewRemoveWorkspace(tempDir); cleanupErr != nil {
			log.Printf("[WARN] failed to clean up temp workspace %q: %v", tempDir, cleanupErr)
		}
	}()

	zipPath := filepath.Join(tempDir, "diff.zip")
	if err := archive.DiffReviewWriteUploadedZip(zipPath, zipBytes); err != nil {
		return nil, fmt.Errorf("failed to persist uploaded zip: %w", err)
	}

	extractedFiles, err := extractZip(zipPath, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract zip: %w", err)
	}
	if len(extractedFiles) == 0 {
		return nil, fmt.Errorf("zip archive contained no files")
	}

	diffContent, err := archive.DiffReviewReadExtractedDiff(extractedFiles[0])
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
	var totalExtracted int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if int64(f.UncompressedSize64) > maxExtractedFileBytes {
			return extracted, fmt.Errorf("zip entry too large: %s", f.Name)
		}
		if totalExtracted+int64(f.UncompressedSize64) > maxExtractedTotalBytes {
			return extracted, fmt.Errorf("zip exceeds maximum extracted size")
		}
		cleaned := filepath.Clean(f.Name)
		targetPath := filepath.Join(dest, cleaned)
		if !strings.HasPrefix(targetPath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path %s", f.Name)
		}
		if err := archive.DiffReviewEnsureParentDir(targetPath); err != nil {
			return extracted, err
		}
		rc, err := f.Open()
		if err != nil {
			return extracted, err
		}

		out, err := archive.DiffReviewOpenExtractedFile(targetPath, f.Mode())
		if err != nil {
			_ = rc.Close()
			return extracted, err
		}
		written, err := io.CopyN(out, rc, maxExtractedFileBytes+1)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			out.Close()
			_ = rc.Close()
			return extracted, err
		}
		if written > maxExtractedFileBytes {
			out.Close()
			_ = rc.Close()
			return extracted, fmt.Errorf("zip entry exceeds per-file limit: %s", f.Name)
		}
		totalExtracted += written
		if totalExtracted > maxExtractedTotalBytes {
			out.Close()
			_ = rc.Close()
			return extracted, fmt.Errorf("zip exceeds maximum extracted size")
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

func readOperationBillableLOC(meta map[string]interface{}) (int64, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta["operation_billable_loc"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func readStringMeta(meta map[string]interface{}, key string) (string, bool) {
	if meta == nil {
		return "", false
	}
	v, ok := meta[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

func applyPreflightToEnvelopeContext(c echo.Context, result license.LOCPreflightResult) {
	c.Set(EnvelopeLOCUsedMonthContextKey, result.LOCUsedMonth)
	c.Set(EnvelopeLOCRemainMonthContextKey, result.LOCRemainingMonth)
	c.Set(EnvelopeUsagePercentContextKey, result.UsagePercent)
	c.Set(EnvelopeThresholdStateContextKey, result.ThresholdState)
	c.Set(EnvelopeBlockedContextKey, result.Blocked)
	c.Set(EnvelopeTrialReadOnlyContextKey, result.TrialReadOnly)
	c.Set(EnvelopeBillingPeriodStartContextKey, result.BillingPeriodStart.Format(time.RFC3339))
	c.Set(EnvelopeBillingPeriodEndContextKey, result.BillingPeriodEnd.Format(time.RFC3339))
	c.Set(EnvelopeResetAtContextKey, result.BillingPeriodEnd.Format(time.RFC3339))
}

func applyEnvelopeUsageFromMetadata(c echo.Context, meta map[string]interface{}) {
	if v, ok := readInt64Meta(meta, "loc_used_month"); ok {
		c.Set(EnvelopeLOCUsedMonthContextKey, v)
	}
	if v, ok := readInt64Meta(meta, "loc_remaining_month"); ok {
		c.Set(EnvelopeLOCRemainMonthContextKey, v)
	}
	if v, ok := readIntMeta(meta, "usage_percent"); ok {
		c.Set(EnvelopeUsagePercentContextKey, v)
	}
	if v, ok := readStringMeta(meta, "threshold_state"); ok {
		c.Set(EnvelopeThresholdStateContextKey, v)
	}
	if v, ok := readBoolMeta(meta, "blocked"); ok {
		c.Set(EnvelopeBlockedContextKey, v)
	}
	if v, ok := readBoolMeta(meta, "trial_readonly"); ok {
		c.Set(EnvelopeTrialReadOnlyContextKey, v)
	}
	if v, ok := readStringMeta(meta, "billing_period_start"); ok {
		c.Set(EnvelopeBillingPeriodStartContextKey, v)
	}
	if v, ok := readStringMeta(meta, "billing_period_end"); ok {
		c.Set(EnvelopeBillingPeriodEndContextKey, v)
	}
	if v, ok := readStringMeta(meta, "reset_at"); ok {
		c.Set(EnvelopeResetAtContextKey, v)
	}
}

func readInt64Meta(meta map[string]interface{}, key string) (int64, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func readIntMeta(meta map[string]interface{}, key string) (int, bool) {
	v, ok := readInt64Meta(meta, key)
	if !ok {
		return 0, false
	}
	return int(v), true
}

func readBoolMeta(meta map[string]interface{}, key string) (bool, bool) {
	if meta == nil {
		return false, false
	}
	v, ok := meta[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
