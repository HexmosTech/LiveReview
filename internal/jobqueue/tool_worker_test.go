package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/lib/pq"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/toolclassifier"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
)

// helper to seed a test org and review with customizable status and metadata
func seedToolWorkerTestOrgAndReview(t *testing.T, db *sql.DB, status string, metadata map[string]interface{}) (int64, int64) {
	t.Helper()
	var orgID int64
	orgName := fmt.Sprintf("tool-worker-test-org-%d", time.Now().UnixNano())
	if err := db.QueryRow(`INSERT INTO public.orgs (name) VALUES ($1) RETURNING id`, orgName).Scan(&orgID); err != nil {
		t.Fatalf("insert test org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM public.dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM public.review_events WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM public.reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM public.orgs WHERE id = $1`, orgID)
	})

	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var reviewID int64
	if err := db.QueryRow(`
		INSERT INTO public.reviews (repository, branch, commit_hash, status, org_id, metadata, created_at)
		VALUES ('test/repo', 'main', 'test-commit', $1, $2, $3::jsonb, NOW()) RETURNING id`,
		status, orgID, metaBytes).Scan(&reviewID); err != nil {
		t.Fatalf("insert test review: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM public.review_events WHERE review_id = $1`, reviewID)
		_, _ = db.Exec(`DELETE FROM public.reviews WHERE id = $1`, reviewID)
	})
	return orgID, reviewID
}

// insertTestReviewEvent is a test helper to insert dispatch or result events
func insertTestReviewEvent(t *testing.T, db *sql.DB, reviewID, orgID int64, eventType, level string, data interface{}) {
	t.Helper()
	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO public.review_events (review_id, org_id, event_type, level, data)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, reviewID, orgID, eventType, level, dataBytes)
	if err != nil {
		t.Fatalf("insert review event: %v", err)
	}
}

// parseLambdaFindingsTest replicates the Lambda findings parsing logic from ToolReviewWorker.Work.
func parseLambdaFindingsTest(outBytes []byte) ([]toolclassifier.RawToolFinding, error) {
	var altResp struct {
		Findings []struct {
			Rule        string `json:"rule"`
			RuleID      string `json:"rule_id"`
			CheckID     string `json:"check_id"`
			Message     string `json:"message"`
			Description string `json:"description"`
			File        string `json:"file"`
			Path        string `json:"path"`
			Line        int    `json:"line"`
			Start       struct {
				Line int `json:"line"`
			} `json:"start"`
			Extra struct {
				Message string `json:"message"`
			} `json:"extra"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(outBytes, &altResp); err != nil {
		return nil, err
	}
	rawList := make([]toolclassifier.RawToolFinding, len(altResp.Findings))
	for i, f := range altResp.Findings {
		rule := f.Rule
		if rule == "" {
			rule = f.RuleID
		}
		if rule == "" {
			rule = f.CheckID
		}
		msg := f.Message
		if msg == "" {
			msg = f.Description
		}
		if msg == "" {
			msg = f.Extra.Message
		}
		file := f.File
		if file == "" {
			file = f.Path
		}
		line := f.Line
		if line == 0 && f.Start.Line > 0 {
			line = f.Start.Line
		}
		rawList[i] = toolclassifier.RawToolFinding{
			File:    file,
			Line:    line,
			Rule:    rule,
			Message: msg,
		}
	}
	return rawList, nil
}

// Master test running W1-W5 suites
func TestToolReviewWorker(t *testing.T) {
	t.Run("W1_SyntheticFailureEvent", testToolReviewWorker_W1_SyntheticFailure)
	t.Run("W2_AlternateFindings_RuleIDAndDescription", testToolReviewWorker_W2_AlternateFindings_RuleIDAndDescription)
	t.Run("W3_AlternateFindings_CheckIDAndExtraMessage", testToolReviewWorker_W3_AlternateFindings_CheckIDAndExtraMessage)
	t.Run("W4_Finalize_PendingToolsCheck", testToolReviewWorker_W4_Finalize_PendingToolsCheck)
	t.Run("W5_Finalize_AllToolsDone", testToolReviewWorker_W5_Finalize_AllToolsDone)
}

// TestToolReviewWorker_W1_SyntheticFailure verifies that when Lambda invocation fails,
// the worker writes a synthetic failure event containing exit_code: -1 and failure_reason/stderr.
func TestToolReviewWorker_W1_SyntheticFailure(t *testing.T) {
	testToolReviewWorker_W1_SyntheticFailure(t)
}

func testToolReviewWorker_W1_SyntheticFailure(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
		"ai_review_done": "false",
	})

	worker := &ToolReviewWorker{
		db:     db,
		awsCfg: aws.Config{Region: "us-east-1"},
	}

	job := &river.Job[ToolReviewJobArgs]{
		Args: ToolReviewJobArgs{
			ReviewID:      reviewID,
			OrgID:         orgID,
			PlanCode:      "team_plan",
			ToolID:        1,
			ToolName:      "ruff",
			LambdaARN:     "invalid-arn-format",
			DiffZipBase64: "dGVzdA==",
		},
	}

	// Work should handle the Lambda error gracefully and return nil (not fail the River job)
	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected worker.Work to return nil on handled lambda failure, got: %v", err)
	}

	// Verify synthetic tool_result event was inserted
	var eventType, level string
	var rawData []byte
	err = db.QueryRowContext(ctx, `
		SELECT event_type, level, data
		FROM public.review_events
		WHERE review_id = $1 AND event_type = 'tool_result'
		ORDER BY id DESC LIMIT 1
	`, reviewID).Scan(&eventType, &level, &rawData)
	if err != nil {
		t.Fatalf("failed to query synthetic tool_result event: %v", err)
	}

	if eventType != "tool_result" {
		t.Errorf("expected event_type 'tool_result', got %q", eventType)
	}

	var payload struct {
		ToolID        int64       `json:"tool_id"`
		ToolName      string      `json:"tool_name"`
		ExitCode      int         `json:"exit_code"`
		Stderr        string      `json:"stderr"`
		FailureReason string      `json:"failure_reason"`
		Findings      interface{} `json:"findings"`
	}
	if err := json.Unmarshal(rawData, &payload); err != nil {
		t.Fatalf("failed to unmarshal tool_result payload: %v (raw: %s)", err, string(rawData))
	}

	// Requirement: Verify synthetic failure event payload contains exit_code: -1 and failure_reason
	if payload.ExitCode != -1 {
		t.Errorf("expected exit_code: -1, got %d", payload.ExitCode)
	}

	reason := payload.FailureReason
	if reason == "" {
		reason = payload.Stderr
	}
	if reason == "" {
		t.Errorf("expected failure_reason or stderr to contain failure details, got empty")
	} else if !strings.Contains(reason, "invalid lambda ARN") {
		t.Errorf("expected failure reason to describe lambda error, got %q", reason)
	}

	if payload.ToolName != "ruff" {
		t.Errorf("expected tool_name 'ruff', got %q", payload.ToolName)
	}

	// Also verify that an error log event was emitted
	var logMsg string
	err = db.QueryRowContext(ctx, `
		SELECT data->>'message'
		FROM public.review_events
		WHERE review_id = $1 AND event_type = 'log' AND level = 'error'
		ORDER BY id DESC LIMIT 1
	`, reviewID).Scan(&logMsg)
	if err != nil {
		t.Fatalf("failed to query error log event: %v", err)
	}
	if !strings.Contains(logMsg, "Lambda execution for tool ruff failed") {
		t.Errorf("expected error log message to contain lambda failure details, got %q", logMsg)
	}

	// Also verify storagetools.GetToolResultsForReview retrieves the result
	toolsStore := storagetools.NewToolsStore(db)
	results, err := toolsStore.GetToolResultsForReview(ctx, reviewID)
	if err != nil {
		t.Fatalf("GetToolResultsForReview failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(results))
	}
	if results[0].ExitCode != -1 {
		t.Errorf("expected GetToolResultsForReview ExitCode -1, got %d", results[0].ExitCode)
	}
}

// TestToolReviewWorker_W2_AlternateFindings_RuleIDAndDescription verifies Lambda result
// parsing handles alternate findings structures with rule_id, description, path, and start.line.
func TestToolReviewWorker_W2_AlternateFindings_RuleIDAndDescription(t *testing.T) {
	testToolReviewWorker_W2_AlternateFindings_RuleIDAndDescription(t)
}

func testToolReviewWorker_W2_AlternateFindings_RuleIDAndDescription(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
		"ai_review_done": "false",
	})

	// Sample Lambda response using rule_id, description, path, and start.line
	lambdaOutput := []byte(`{
		"exit_code": 1,
		"findings": [
			{
				"rule_id": "F401",
				"description": "module 'os' imported but unused",
				"path": "internal/api/handler.py",
				"start": {
					"line": 15
				}
			}
		]
	}`)

	rawList, err := parseLambdaFindingsTest(lambdaOutput)
	if err != nil {
		t.Fatalf("failed to parse lambda findings: %v", err)
	}

	if len(rawList) != 1 {
		t.Fatalf("expected 1 raw finding, got %d", len(rawList))
	}

	finding := rawList[0]
	if finding.Rule != "F401" {
		t.Errorf("expected Rule 'F401' extracted from rule_id, got %q", finding.Rule)
	}
	if finding.Message != "module 'os' imported but unused" {
		t.Errorf("expected Message extracted from description, got %q", finding.Message)
	}
	if finding.File != "internal/api/handler.py" {
		t.Errorf("expected File extracted from path, got %q", finding.File)
	}
	if finding.Line != 15 {
		t.Errorf("expected Line 15 extracted from start.line, got %d", finding.Line)
	}

	// Verify classification works with the extracted findings
	classifiedComments, classErr := toolclassifier.ClassifyToolResult(ctx, db, orgID, license.PlanFree, "ruff", rawList, nil)
	if classErr != nil {
		t.Fatalf("ClassifyToolResult failed: %v", classErr)
	}
	if len(classifiedComments) == 0 {
		t.Fatalf("expected at least 1 classified comment for ruff rule F401")
	}

	c := classifiedComments[0]
	if c.FilePath != "internal/api/handler.py" {
		t.Errorf("expected comment FilePath 'internal/api/handler.py', got %q", c.FilePath)
	}
	if c.Line != 15 {
		t.Errorf("expected comment Line 15, got %d", c.Line)
	}

	// Verify appending tool comments to review metadata in database
	toolComments := []map[string]interface{}{
		{
			"file_path":   c.FilePath,
			"line":        c.Line,
			"content":     c.Content,
			"severity":    string(c.Severity),
			"category":    c.Category,
			"subcategory": c.Subcategory,
			"confidence":  c.Confidence,
			"type":        c.Type,
			"suggestions": c.Suggestions,
			"source":      "tool",
			"tool_name":   "ruff",
		},
	}
	commentsBytes, err := json.Marshal(toolComments)
	if err != nil {
		t.Fatalf("marshal tool comments: %v", err)
	}

	appendQuery := `
		UPDATE public.reviews
		SET metadata = jsonb_set(
			COALESCE(metadata, '{}'::jsonb),
			'{review_result}',
			COALESCE(metadata->'review_result', '{}'::jsonb) || jsonb_build_object(
				'comments',
				COALESCE(metadata->'review_result'->'comments', '[]'::jsonb) || $1::jsonb
			)
		)
		WHERE id = $2
	`
	if _, execErr := db.ExecContext(ctx, appendQuery, commentsBytes, reviewID); execErr != nil {
		t.Fatalf("failed to append tool comments to review metadata: %v", execErr)
	}

	// Query metadata back and verify comments array
	var storedCommentsCount int
	err = db.QueryRowContext(ctx, `
		SELECT jsonb_array_length(metadata->'review_result'->'comments')
		FROM public.reviews WHERE id = $1
	`, reviewID).Scan(&storedCommentsCount)
	if err != nil {
		t.Fatalf("failed to query metadata comments count: %v", err)
	}
	if storedCommentsCount != 1 {
		t.Errorf("expected 1 comment stored in metadata->review_result->comments, got %d", storedCommentsCount)
	}
}

// TestToolReviewWorker_W3_AlternateFindings_CheckIDAndExtraMessage verifies Lambda result
// parsing handles check_id and extra.message alternate structures and verifies precedence.
func TestToolReviewWorker_W3_AlternateFindings_CheckIDAndExtraMessage(t *testing.T) {
	testToolReviewWorker_W3_AlternateFindings_CheckIDAndExtraMessage(t)
}

func testToolReviewWorker_W3_AlternateFindings_CheckIDAndExtraMessage(t *testing.T) {
	// 1. Alternate structure with check_id and extra.message
	lambdaOutput1 := []byte(`{
		"exit_code": 1,
		"findings": [
			{
				"check_id": "CKV_AWS_21",
				"extra": {
					"message": "Ensure all S3 buckets have versioning enabled"
				},
				"file": "terraform/storage.tf",
				"line": 42
			}
		]
	}`)

	rawList1, err := parseLambdaFindingsTest(lambdaOutput1)
	if err != nil {
		t.Fatalf("failed to parse lambda findings: %v", err)
	}
	if len(rawList1) != 1 {
		t.Fatalf("expected 1 raw finding, got %d", len(rawList1))
	}
	if rawList1[0].Rule != "CKV_AWS_21" {
		t.Errorf("expected Rule 'CKV_AWS_21' from check_id, got %q", rawList1[0].Rule)
	}
	if rawList1[0].Message != "Ensure all S3 buckets have versioning enabled" {
		t.Errorf("expected Message from extra.message, got %q", rawList1[0].Message)
	}
	if rawList1[0].File != "terraform/storage.tf" {
		t.Errorf("expected File 'terraform/storage.tf', got %q", rawList1[0].File)
	}
	if rawList1[0].Line != 42 {
		t.Errorf("expected Line 42, got %d", rawList1[0].Line)
	}

	// 2. Precedence test: primary 'rule' takes precedence over 'rule_id' and 'check_id',
	// and 'message' takes precedence over 'description' and 'extra.message'.
	lambdaOutput2 := []byte(`{
		"findings": [
			{
				"rule": "PRIMARY_RULE",
				"rule_id": "SECONDARY_RULE_ID",
				"check_id": "TERTIARY_CHECK_ID",
				"message": "PRIMARY_MSG",
				"description": "SECONDARY_DESC",
				"extra": { "message": "TERTIARY_EXTRA_MSG" },
				"file": "primary.go",
				"path": "secondary.go",
				"line": 100,
				"start": { "line": 200 }
			}
		]
	}`)

	rawList2, err := parseLambdaFindingsTest(lambdaOutput2)
	if err != nil {
		t.Fatalf("failed to parse precedence test output: %v", err)
	}
	if len(rawList2) != 1 {
		t.Fatalf("expected 1 raw finding, got %d", len(rawList2))
	}
	f2 := rawList2[0]
	if f2.Rule != "PRIMARY_RULE" {
		t.Errorf("expected primary rule to win, got %q", f2.Rule)
	}
	if f2.Message != "PRIMARY_MSG" {
		t.Errorf("expected primary message to win, got %q", f2.Message)
	}
	if f2.File != "primary.go" {
		t.Errorf("expected primary file to win, got %q", f2.File)
	}
	if f2.Line != 100 {
		t.Errorf("expected primary line to win, got %d", f2.Line)
	}
}

// TestToolReviewWorker_W4_Finalize_PendingToolsCheck verifies finalize logic correctly
// checks all dispatched tools before setting review status, ensuring review remains in_progress
// when some tools are still pending.
func TestToolReviewWorker_W4_Finalize_PendingToolsCheck(t *testing.T) {
	testToolReviewWorker_W4_Finalize_PendingToolsCheck(t)
}

func testToolReviewWorker_W4_Finalize_PendingToolsCheck(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
		"ai_review_done": "true", // AI review is already marked done
	})

	worker := &ToolReviewWorker{
		db:     db,
		awsCfg: aws.Config{Region: "us-east-1"},
	}

	// Dispatch TWO tools: ruff and osv
	insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
		"tool_id":   1,
		"tool_name": "ruff",
		"status":    "dispatched",
	})
	insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
		"tool_id":   3,
		"tool_name": "osv",
		"status":    "dispatched",
	})

	// Record tool_result for ONLY ruff (osv is still pending)
	insertTestReviewEvent(t, db, reviewID, orgID, "tool_result", "info", map[string]interface{}{
		"tool_id":   1,
		"tool_name": "ruff",
		"exit_code": 0,
		"findings":  []interface{}{},
	})

	// Run finalize check for ruff
	err := worker.finalizeIfAllDone(ctx, ToolReviewJobArgs{
		ReviewID: reviewID,
		OrgID:    orgID,
		ToolName: "ruff",
	})
	if err != nil {
		t.Fatalf("finalizeIfAllDone failed: %v", err)
	}

	// Verify that review status was NOT updated to 'completed' because osv is still pending
	var currentStatus string
	var completedAt sql.NullTime
	err = db.QueryRowContext(ctx, `SELECT status, completed_at FROM public.reviews WHERE id = $1`, reviewID).Scan(&currentStatus, &completedAt)
	if err != nil {
		t.Fatalf("query review status: %v", err)
	}

	if currentStatus != "in_progress" {
		t.Errorf("expected review status to remain 'in_progress' while tools are pending, got %q", currentStatus)
	}
	if completedAt.Valid {
		t.Errorf("expected completed_at to be NULL while tools are pending, got %v", completedAt.Time)
	}

	// Verify no completion event was emitted
	var completionCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM public.review_events
		WHERE review_id = $1 AND event_type = 'completion'
	`, reviewID).Scan(&completionCount)
	if err != nil {
		t.Fatalf("query completion count: %v", err)
	}
	if completionCount != 0 {
		t.Errorf("expected 0 completion events while tools are pending, got %d", completionCount)
	}
}

// TestToolReviewWorker_W5_Finalize_AllToolsDone verifies finalize logic completes the review
// once all dispatched tools have recorded results and handles terminal status, failure_reason,
// and completion event emission.
func TestToolReviewWorker_W5_Finalize_AllToolsDone(t *testing.T) {
	testToolReviewWorker_W5_Finalize_AllToolsDone(t)
}

func testToolReviewWorker_W5_Finalize_AllToolsDone(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	t.Run("AllToolsAndAIDone_CompletesReview", func(t *testing.T) {
		orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
			"ai_review_done": "true",
			"review_result": map[string]interface{}{
				"summary":  "Test review summary",
				"comments": []interface{}{map[string]interface{}{"content": "test comment"}},
			},
		})

		worker := &ToolReviewWorker{
			db:     db,
			awsCfg: aws.Config{Region: "us-east-1"},
		}

		// Dispatch 2 tools
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "status": "dispatched",
		})
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
			"tool_id": 3, "tool_name": "osv", "status": "dispatched",
		})

		// Both tools return results
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_result", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "exit_code": 0, "findings": []interface{}{},
		})
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_result", "info", map[string]interface{}{
			"tool_id": 3, "tool_name": "osv", "exit_code": 0, "findings": []interface{}{},
		})

		// Finalize
		err := worker.finalizeIfAllDone(ctx, ToolReviewJobArgs{
			ReviewID: reviewID,
			OrgID:    orgID,
			ToolName: "osv",
		})
		if err != nil {
			t.Fatalf("finalizeIfAllDone failed: %v", err)
		}

		// Status must now be 'completed' with completed_at set
		var currentStatus string
		var completedAt sql.NullTime
		err = db.QueryRowContext(ctx, `SELECT status, completed_at FROM public.reviews WHERE id = $1`, reviewID).Scan(&currentStatus, &completedAt)
		if err != nil {
			t.Fatalf("query review status: %v", err)
		}
		if currentStatus != "completed" {
			t.Errorf("expected review status 'completed', got %q", currentStatus)
		}
		if !completedAt.Valid {
			t.Errorf("expected completed_at to be populated, got NULL")
		}

		// Completion event should have been emitted
		var rawCompData []byte
		err = db.QueryRowContext(ctx, `
			SELECT data
			FROM public.review_events
			WHERE review_id = $1 AND event_type = 'completion'
			ORDER BY id DESC LIMIT 1
		`, reviewID).Scan(&rawCompData)
		if err != nil {
			t.Fatalf("failed to query completion event: %v", err)
		}

		var compData struct {
			ResultSummary  string `json:"resultSummary"`
			ResultSummary2 string `json:"result_summary"`
			CommentCount   int    `json:"commentCount"`
			CommentCount2  int    `json:"comment_count"`
		}
		if err := json.Unmarshal(rawCompData, &compData); err != nil {
			t.Fatalf("failed to unmarshal completion event: %v", err)
		}

		resSummary := compData.ResultSummary
		if resSummary == "" {
			resSummary = compData.ResultSummary2
		}
		if resSummary != "Test review summary" {
			t.Errorf("expected completion result_summary 'Test review summary', got %q", resSummary)
		}

		cCount := compData.CommentCount
		if cCount == 0 {
			cCount = compData.CommentCount2
		}
		if cCount != 1 {
			t.Errorf("expected comment_count 1, got %d", cCount)
		}
	})

	t.Run("FailureReason_SetsStatusFailed", func(t *testing.T) {
		orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
			"failure_reason": "AI review processor crashed",
		})

		worker := &ToolReviewWorker{
			db:     db,
			awsCfg: aws.Config{Region: "us-east-1"},
		}

		insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "status": "dispatched",
		})
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_result", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "exit_code": 0, "findings": []interface{}{},
		})

		err := worker.finalizeIfAllDone(ctx, ToolReviewJobArgs{
			ReviewID: reviewID,
			OrgID:    orgID,
			ToolName: "ruff",
		})
		if err != nil {
			t.Fatalf("finalizeIfAllDone failed: %v", err)
		}

		var currentStatus string
		err = db.QueryRowContext(ctx, `SELECT status FROM public.reviews WHERE id = $1`, reviewID).Scan(&currentStatus)
		if err != nil {
			t.Fatalf("query review status: %v", err)
		}
		if currentStatus != "failed" {
			t.Errorf("expected review status 'failed' when failure_reason is present, got %q", currentStatus)
		}

		// Verify error_summary in completion event
		var rawCompData []byte
		err = db.QueryRowContext(ctx, `
			SELECT data
			FROM public.review_events
			WHERE review_id = $1 AND event_type = 'completion'
			ORDER BY id DESC LIMIT 1
		`, reviewID).Scan(&rawCompData)
		if err != nil {
			t.Fatalf("failed to query completion event: %v", err)
		}

		var failCompData struct {
			ErrorSummary  string `json:"errorSummary"`
			ErrorSummary2 string `json:"error_summary"`
		}
		if err := json.Unmarshal(rawCompData, &failCompData); err != nil {
			t.Fatalf("failed to unmarshal completion event: %v", err)
		}

		errSummary := failCompData.ErrorSummary
		if errSummary == "" {
			errSummary = failCompData.ErrorSummary2
		}
		if errSummary != "AI review processor crashed" {
			t.Errorf("expected error_summary 'AI review processor crashed', got %q", errSummary)
		}
	})

	t.Run("TerminalStatus_NoOp", func(t *testing.T) {
		orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "completed", map[string]interface{}{
			"ai_review_done": "true",
		})

		worker := &ToolReviewWorker{
			db:     db,
			awsCfg: aws.Config{Region: "us-east-1"},
		}

		err := worker.finalizeIfAllDone(ctx, ToolReviewJobArgs{
			ReviewID: reviewID,
			OrgID:    orgID,
			ToolName: "ruff",
		})
		if err != nil {
			t.Fatalf("finalizeIfAllDone failed: %v", err)
		}

		var currentStatus string
		err = db.QueryRowContext(ctx, `SELECT status FROM public.reviews WHERE id = $1`, reviewID).Scan(&currentStatus)
		if err != nil {
			t.Fatalf("query review status: %v", err)
		}
		if currentStatus != "completed" {
			t.Errorf("expected status 'completed' to remain unchanged, got %q", currentStatus)
		}
	})

	t.Run("AIDoneFalse_WaitsForAIReview", func(t *testing.T) {
		orgID, reviewID := seedToolWorkerTestOrgAndReview(t, db, "in_progress", map[string]interface{}{
			"ai_review_done": "false",
		})

		worker := &ToolReviewWorker{
			db:     db,
			awsCfg: aws.Config{Region: "us-east-1"},
		}

		// Tool is done
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_dispatch", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "status": "dispatched",
		})
		insertTestReviewEvent(t, db, reviewID, orgID, "tool_result", "info", map[string]interface{}{
			"tool_id": 1, "tool_name": "ruff", "exit_code": 0, "findings": []interface{}{},
		})

		err := worker.finalizeIfAllDone(ctx, ToolReviewJobArgs{
			ReviewID: reviewID,
			OrgID:    orgID,
			ToolName: "ruff",
		})
		if err != nil {
			t.Fatalf("finalizeIfAllDone failed: %v", err)
		}

		// Status should still be 'in_progress' because AI review is waiting
		var currentStatus string
		err = db.QueryRowContext(ctx, `SELECT status FROM public.reviews WHERE id = $1`, reviewID).Scan(&currentStatus)
		if err != nil {
			t.Fatalf("query review status: %v", err)
		}
		if currentStatus != "in_progress" {
			t.Errorf("expected review status to remain 'in_progress' waiting for AI review, got %q", currentStatus)
		}

		// Log event waiting for AI review should have been emitted
		var waitingLogCount int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM public.review_events
			WHERE review_id = $1 AND event_type = 'log' AND data->>'message' LIKE '%Waiting for AI review%'
		`, reviewID).Scan(&waitingLogCount)
		if err != nil {
			t.Fatalf("query waiting log: %v", err)
		}
		if waitingLogCount == 0 {
			t.Errorf("expected a log event indicating waiting for AI review")
		}
	})
}
