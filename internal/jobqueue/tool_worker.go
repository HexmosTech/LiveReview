package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/review"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/internal/toolclassifier"
	networktools "github.com/livereview/network/tools"
	"github.com/livereview/pkg/models"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
)

// ToolReviewJobArgs represents the arguments for executing a single static analysis tool via AWS Lambda.
// Kept temporarily so any legacy tool_review jobs already queued during a mixed-version deploy drain cleanly.
type ToolReviewJobArgs struct {
	ReviewID       int64                  `json:"review_id"`
	OrgID          int64                  `json:"org_id"`
	PlanCode       string                 `json:"plan_code"`
	ToolID         int64                  `json:"tool_id"`
	ToolName       string                 `json:"tool_name"`
	ToolCategory   string                 `json:"tool_category,omitempty"`
	LambdaARN      string                 `json:"lambda_arn"`
	DiffZipBase64  string                 `json:"diff_zip_base64"`
	Truncated      bool                   `json:"truncated,omitempty"`
	ProviderConfig *review.ProviderConfig `json:"provider_config,omitempty"`
	PostingID      string                 `json:"posting_id,omitempty"`
}

func (ToolReviewJobArgs) Kind() string { return "tool_review" }

// ToolReviewWorker executes a single legacy tool_review River job and drains in-flight rows cleanly.
type ToolReviewWorker struct {
	river.WorkerDefaults[ToolReviewJobArgs]
	db        *sql.DB
	awsCfg    aws.Config
	region    string
	accountID string
}

func (w *ToolReviewWorker) Timeout(job *river.Job[ToolReviewJobArgs]) time.Duration {
	return DefaultReviewJobTimeout
}

// loadAWSLambdaConfig loads AWS credentials and common Lambda settings (region, account ID)
// strictly from system_settings in the database (no env fallback).
func loadAWSLambdaConfig(db *sql.DB) (aws.Config, string, string) {
	var accessKey, secretKey, region, accountID string

	if db != nil {
		rows, qErr := db.Query("SELECT name, data FROM public.system_settings WHERE name IN ('aws_lambda', 'aws_credentials') ORDER BY CASE WHEN name = 'aws_lambda' THEN 1 ELSE 2 END")
		if qErr != nil {
			log.Printf("[WARN] loadAWSLambdaConfig: failed to load AWS settings from database: %v", qErr)
		} else {
			defer rows.Close()
			for rows.Next() {
				var name string
				var rawData []byte
				if scanErr := rows.Scan(&name, &rawData); scanErr != nil || len(rawData) == 0 {
					continue
				}
				var creds struct {
					AccessKeyID     string `json:"access_key_id"`
					SecretAccessKey string `json:"secret_access_key"`
					Access          string `json:"access"`
					Secret          string `json:"secret"`
					Region          string `json:"region"`
					AccountID       string `json:"account_id"`
				}
				if jsonErr := json.Unmarshal(rawData, &creds); jsonErr == nil {
					if creds.AccessKeyID != "" {
						accessKey = creds.AccessKeyID
					} else if creds.Access != "" {
						accessKey = creds.Access
					}
					if creds.SecretAccessKey != "" {
						secretKey = creds.SecretAccessKey
					} else if creds.Secret != "" {
						secretKey = creds.Secret
					}
					if creds.Region != "" {
						region = creds.Region
					}
					if creds.AccountID != "" {
						accountID = creds.AccountID
					}
				}
			}
		}
	}

	if region == "" || accessKey == "" || secretKey == "" {
		log.Printf("[WARN] loadAWSLambdaConfig: incomplete AWS Lambda settings in database (region_set=%v, credentials_set=%v)", region != "", accessKey != "" && secretKey != "")
	}

	awsCfg := aws.Config{
		Region: region,
	}
	if accessKey != "" && secretKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	}
	return awsCfg, region, accountID
}

// NewToolReviewWorker creates a new ToolReviewWorker with AWS config and common Lambda configuration
// (region, account ID) loaded strictly from system_settings in the database.
func NewToolReviewWorker(db *sql.DB) *ToolReviewWorker {
	awsCfg, region, accountID := loadAWSLambdaConfig(db)
	return &ToolReviewWorker{
		db:        db,
		awsCfg:    awsCfg,
		region:    region,
		accountID: accountID,
	}
}

type toolInvokeFunc func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error)

type toolClassifyFunc func(ctx context.Context, db *sql.DB, orgID int64, planCode license.PlanType, toolName string, findings []toolclassifier.RawToolFinding, logger *logging.ReviewLogger) ([]*models.ReviewComment, error)

type singleToolExecParams struct {
	ReviewID     int64
	OrgID        int64
	PlanCode     string
	ToolID       int64
	ToolName     string
	ToolCategory string
	LambdaARN    string
	PayloadBytes []byte
	Truncated    bool
	InvokeFn     toolInvokeFunc
	ClassifyFn   toolClassifyFunc
}

func resolveToolTargetARN(arn, workerRegion, cfgRegion, accountID string) string {
	if !strings.HasPrefix(arn, "arn:") && !strings.Contains(arn, "invalid") {
		region := workerRegion
		if region == "" {
			region = cfgRegion
		}
		if region != "" && accountID != "" {
			return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", region, accountID, arn)
		}
	}
	return arn
}

func normalizeTruncatedToolResult(outBytes []byte, truncated bool) []byte {
	if !truncated {
		return outBytes
	}
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(outBytes, &payloadMap); err == nil && payloadMap != nil {
		if _, hasFindings := payloadMap["findings"]; !hasFindings {
			payloadMap["findings"] = []interface{}{}
		}
		payloadMap["truncated"] = true
		if updated, err := json.Marshal(payloadMap); err == nil {
			return updated
		}
		return outBytes
	}
	var findingsList []interface{}
	if err := json.Unmarshal(outBytes, &findingsList); err == nil {
		payloadMap = map[string]interface{}{
			"exit_code": 0,
			"findings":  findingsList,
			"truncated": true,
		}
		if updated, err := json.Marshal(payloadMap); err == nil {
			return updated
		}
		return outBytes
	}
	payloadMap = map[string]interface{}{
		"exit_code":      1,
		"findings":       []interface{}{},
		"truncated":      true,
		"stderr":         string(outBytes),
		"failure_reason": "unmarshal lambda tool result failed",
	}
	if updated, err := json.Marshal(payloadMap); err == nil {
		return updated
	}
	return outBytes
}

func parseLambdaFindings(resultBytes []byte) ([]toolclassifier.RawToolFinding, error) {
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
	if jsonErr := json.Unmarshal(resultBytes, &altResp); jsonErr != nil {
		return nil, jsonErr
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

func recordSyntheticToolFailure(
	db *sql.DB,
	reviewID, orgID, toolID int64,
	toolName string,
	eventSink logging.EventSink,
	reason string,
) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if eventSink != nil {
		_ = eventSink.EmitLogEvent(cleanupCtx, reviewID, orgID, "error", fmt.Sprintf("Lambda execution for tool %s failed: %s", toolName, reason), "")
	}
	if db == nil {
		return
	}
	toolsStore := storagetools.NewToolsStore(db)
	failData := map[string]interface{}{
		"exit_code":      -1,
		"findings":       []interface{}{},
		"stderr":         reason,
		"failure_reason": reason,
	}
	failBytes, _ := json.Marshal(failData)
	if insertErr := toolsStore.InsertToolResultEvent(cleanupCtx, reviewID, orgID, toolID, toolName, failBytes); insertErr != nil {
		log.Printf("[ERROR] Failed to insert synthetic tool result event for tool=%s review=%d: %v", toolName, reviewID, insertErr)
	}
}

func emitToolBatchEventWithRetry(
	ctx context.Context,
	eventSink logging.EventSink,
	reviewID, orgID int64,
	toolName string,
	comments []*models.ReviewComment,
) {
	if eventSink == nil {
		return
	}
	// Ensure non-nil slice so empty results encode identically without surprises.
	if comments == nil {
		comments = []*models.ReviewComment{}
	}
	batchID := fmt.Sprintf("tool-%s", toolName)
	if emitBatchErr := eventSink.EmitBatchEvent(ctx, reviewID, orgID, batchID, "completed", 0, len(comments), comments); emitBatchErr != nil {
		log.Printf("[WARN] emitToolBatchEventWithRetry: failed to emit batch event for tool %s: %v, retrying once", toolName, emitBatchErr)
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if retryErr := eventSink.EmitBatchEvent(retryCtx, reviewID, orgID, batchID, "completed", 0, len(comments), comments); retryErr != nil {
			log.Printf("[ERROR] emitToolBatchEventWithRetry: retry failed to emit batch event for tool %s: %v", toolName, retryErr)
		}
		retryCancel()
	}
}

func markToolDispatchRunning(ctx context.Context, db *sql.DB, reviewID, orgID int64, toolName string) {
	if db == nil || reviewID <= 0 || orgID <= 0 || toolName == "" {
		return
	}
	// Update status on the single tool_dispatch row rather than inserting a second
	// event so tool_dispatch counts remain exact 1:1 per dispatched tool.
	_, err := db.ExecContext(ctx, `
		UPDATE public.review_events
		SET data = jsonb_set(COALESCE(data, '{}'::jsonb), '{status}', '"running"'::jsonb, true)
		WHERE review_id = $1 AND org_id = $2 AND event_type = 'tool_dispatch'
		  AND lower(data->>'tool_name') = lower($3)
	`, reviewID, orgID, toolName)
	if err != nil {
		log.Printf("[WARN] ToolRunner: failed to mark tool_dispatch running for tool=%s review=%d: %v", toolName, reviewID, err)
	}
}

// executeSingleToolInvocation invokes one AWS Lambda tool (with transient retry), persists
// tool_result, classifies findings within ClassifyTimeout, and emits a completed tool-<name>
// batch event even when 0 comments were found. It does NOT mutate reviews.metadata.review_result.
func executeSingleToolInvocation(
	ctx context.Context,
	db *sql.DB,
	awsCfg aws.Config,
	region, accountID string,
	params singleToolExecParams,
	cfg ToolsExecutionConfig,
	eventSink logging.EventSink,
) ([]*models.ReviewComment, bool, error) {
	startTime := time.Now()
	markToolDispatchRunning(ctx, db, params.ReviewID, params.OrgID, params.ToolName)
	if eventSink != nil {
		_ = eventSink.EmitLogEvent(ctx, params.ReviewID, params.OrgID, "info", fmt.Sprintf("Executing static analysis tool: %s", strings.ToUpper(params.ToolName)), "")
	}

	targetARN := resolveToolTargetARN(params.LambdaARN, region, awsCfg.Region, accountID)
	log.Printf("[INFO] ToolRunner: invoking Lambda %s (payload %d bytes) for tool=%s review=%d...", targetARN, len(params.PayloadBytes), params.ToolName, params.ReviewID)

	invokeFn := params.InvokeFn
	if invokeFn == nil {
		invokeFn = networktools.InvokeToolWithTimeout
	}

	var outBytes []byte
	var invokeErr error
	maxRetries := max(cfg.MaxRetries, 0)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		outBytes, invokeErr = invokeFn(ctx, awsCfg, targetARN, params.PayloadBytes, cfg.InvokeTimeout)
		if invokeErr == nil {
			break
		}
		if attempt >= maxRetries || !networktools.IsTransientToolInvokeError(invokeErr) || ctx.Err() != nil {
			break
		}
		log.Printf("[WARN] ToolRunner: transient Lambda error for tool=%s review=%d (attempt %d/%d), retrying in %v: %v",
			params.ToolName, params.ReviewID, attempt+1, maxRetries+1, cfg.RetryDelay, invokeErr)
		timer := time.NewTimer(cfg.RetryDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			invokeErr = ctx.Err()
		}
		if ctx.Err() != nil {
			break
		}
	}

	if invokeErr != nil {
		log.Printf("[ERROR] Lambda invocation for tool %s failed: %v", params.ToolName, invokeErr)
		recordSyntheticToolFailure(db, params.ReviewID, params.OrgID, params.ToolID, params.ToolName, eventSink, invokeErr.Error())
		return nil, false, nil
	}

	log.Printf("[INFO] ToolRunner: Lambda returned %d bytes in %v for tool=%s review=%d", len(outBytes), time.Since(startTime).Round(time.Millisecond), params.ToolName, params.ReviewID)
	if eventSink != nil {
		_ = eventSink.EmitLogEvent(ctx, params.ReviewID, params.OrgID, "success", fmt.Sprintf("Lambda execution completed for %s (%d bytes returned)", params.ToolName, len(outBytes)), "")
	}

	resultBytes := normalizeTruncatedToolResult(outBytes, params.Truncated)
	toolsStore := storagetools.NewToolsStore(db)
	if err := toolsStore.InsertToolResultEvent(ctx, params.ReviewID, params.OrgID, params.ToolID, params.ToolName, resultBytes); err != nil {
		if eventSink != nil {
			_ = eventSink.EmitLogEvent(ctx, params.ReviewID, params.OrgID, "error", fmt.Sprintf("Failed to store raw tool result event: %v", err), "")
		}
		return nil, false, fmt.Errorf("failed to store tool result event: %w", err)
	}

	rawList, jsonErr := parseLambdaFindings(resultBytes)
	classifiedComments := make([]*models.ReviewComment, 0)
	if jsonErr != nil {
		log.Printf("[WARN] ToolRunner: failed to unmarshal Lambda findings output for tool=%s review=%d: %v", params.ToolName, params.ReviewID, jsonErr)
	} else if len(rawList) > 0 {
		classifyFn := params.ClassifyFn
		if classifyFn == nil {
			classifyFn = toolclassifier.ClassifyToolResult
		}
		classifyTimeout := cfg.ClassifyTimeout
		if classifyTimeout <= 0 {
			classifyTimeout = DefaultToolClassifyTimeout
		}
		classifyCtx, classifyCancel := context.WithTimeout(ctx, classifyTimeout)
		classified, classErr := classifyFn(classifyCtx, db, params.OrgID, license.PlanType(params.PlanCode), params.ToolName, rawList, nil)
		classifyCancel()
		if classErr != nil {
			log.Printf("[WARN] ToolRunner: classification error for tool=%s review=%d: %v", params.ToolName, params.ReviewID, classErr)
		} else {
			for _, c := range classified {
				if c == nil {
					continue
				}
				if c.ToolName == "" {
					c.ToolName = params.ToolName
				}
				if c.ToolCategory == "" && params.ToolCategory != "" {
					c.ToolCategory = params.ToolCategory
				}
				if c.Source == "" {
					c.Source = "tool"
				}
				classifiedComments = append(classifiedComments, c)
			}
		}
	} else {
		log.Printf("[INFO] ToolRunner: tool=%s review=%d clean (0 findings)", params.ToolName, params.ReviewID)
	}

	// Always emit the completed batch event (even with 0 comments) so incremental
	// UI polling and River job retry hydration both have an exact signal that this
	// tool finished and was classified.
	emitToolBatchEventWithRetry(ctx, eventSink, params.ReviewID, params.OrgID, params.ToolName, classifiedComments)

	return classifiedComments, true, nil
}

func (w *ToolReviewWorker) Work(ctx context.Context, job *river.Job[ToolReviewJobArgs]) error {
	args := job.Args
	log.Printf("[INFO] ToolReviewWorker: starting legacy job tool=%s review=%d org=%d", args.ToolName, args.ReviewID, args.OrgID)

	eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
	payload := map[string]interface{}{
		"zip_file": args.DiffZipBase64,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Failed to marshal Lambda payload: %v", err), "")
		return fmt.Errorf("failed to marshal Lambda payload: %w", err)
	}

	cfg := DefaultToolsExecutionConfig()
	cfg.MaxRetries = 0 // River already manages retries for legacy ToolReviewWorker jobs.

	classifiedComments, ok, execErr := executeSingleToolInvocation(ctx, w.db, w.awsCfg, w.region, w.accountID, singleToolExecParams{
		ReviewID:     args.ReviewID,
		OrgID:        args.OrgID,
		PlanCode:     args.PlanCode,
		ToolID:       args.ToolID,
		ToolName:     args.ToolName,
		ToolCategory: args.ToolCategory,
		LambdaARN:    args.LambdaARN,
		PayloadBytes: payloadBytes,
		Truncated:    args.Truncated,
	}, cfg, eventSink)
	if execErr != nil {
		return execErr
	}

	if !ok {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = w.finalizeIfAllDone(cleanupCtx, args)
		return nil
	}

	if len(classifiedComments) > 0 {
		toolCommentsBytes, mErr := json.Marshal(classifiedComments)
		if mErr != nil {
			log.Printf("[ERROR] ToolReviewWorker: failed to marshal tool comments for review %d tool=%s: %v", args.ReviewID, args.ToolName, mErr)
			_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Failed to marshal %s findings: %v", args.ToolName, mErr), "")
		} else {
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
			if _, execErr := w.db.ExecContext(ctx, appendQuery, toolCommentsBytes, args.ReviewID); execErr != nil {
				log.Printf("[ERROR] ToolReviewWorker: failed to append tool comments to review %d metadata: %v", args.ReviewID, execErr)
			} else {
				_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "success", fmt.Sprintf("Appended %d %s findings to review #%d", len(classifiedComments), strings.ToUpper(args.ToolName), args.ReviewID), "")
			}
		}

		if args.ProviderConfig != nil && args.ProviderConfig.Type != "" && args.ProviderConfig.Type != "cli" && args.PostingID != "" {
			w.postToolCommentsToProvider(ctx, args, classifiedComments, eventSink)
		}
	}

	w.finalizeIfAllDone(ctx, args)
	return nil
}

// postToolCommentsToProvider delegates to reviewprocessor.PostToolCommentsToProvider.
func (w *ToolReviewWorker) postToolCommentsToProvider(
	ctx context.Context,
	args ToolReviewJobArgs,
	comments []*models.ReviewComment,
	eventSink *reviewprocessor.DatabaseEventSink,
) {
	cfg := DefaultToolsExecutionConfig()
	var sink logging.EventSink
	if eventSink != nil {
		sink = eventSink
	}
	reviewprocessor.PostToolCommentsToProvider(ctx, w.db, args.OrgID, args.ReviewID, comments, reviewprocessor.ToolPostConfig{
		ProviderConfig: args.ProviderConfig,
		PostingID:      args.PostingID,
		PacingDelay:    cfg.PostPacingDelay,
		MaxRetries:     cfg.PostMaxRetries,
	}, sink)
}

// finalizeIfAllDone checks if all dispatched tools have returned results and coordinates completion with the AI review.
func (w *ToolReviewWorker) finalizeIfAllDone(ctx context.Context, args ToolReviewJobArgs) error {
	var pendingJobs int
	err := w.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM public.review_events 
		WHERE review_id = $1 AND event_type = 'tool_dispatch' AND data->>'tool_name' NOT IN (
			SELECT data->>'tool_name' FROM public.review_events WHERE review_id = $1 AND event_type = 'tool_result'
		)`, args.ReviewID).Scan(&pendingJobs)
	if err != nil {
		log.Printf("[ERROR] Failed to count pending tool jobs for review %d: %v", args.ReviewID, err)
		return err
	}

	if pendingJobs > 0 {
		log.Printf("[INFO] ToolReviewWorker: review=%d tool completed, %d tool(s) still pending", args.ReviewID, pendingJobs)
		return nil
	}

	// All tool jobs are done! Check if AI review is also completed
	var currentStatus string
	var createdAt time.Time
	var aiDoneStr sql.NullString
	var metaSummary sql.NullString
	var failureReason sql.NullString
	qErr := w.db.QueryRowContext(ctx, `
		SELECT 
			status,
			created_at,
			metadata->>'ai_review_done',
			metadata->'review_result'->>'summary',
			metadata->>'failure_reason'
		FROM public.reviews WHERE id = $1`, args.ReviewID).Scan(&currentStatus, &createdAt, &aiDoneStr, &metaSummary, &failureReason)
	if qErr != nil {
		log.Printf("[ERROR] ToolReviewWorker: failed to query review %d for completion: %v", args.ReviewID, qErr)
		return qErr
	}

	// If the review has already reached a terminal status (completed, failed, cancelled), exit cleanly
	if currentStatus == "completed" || currentStatus == "failed" || currentStatus == "cancelled" {
		log.Printf("[INFO] ToolReviewWorker: review=%d already in terminal status %q", args.ReviewID, currentStatus)
		return nil
	}

	aiDone := aiDoneStr.Valid && (aiDoneStr.String == "true" || aiDoneStr.String == "1")
	// If AI review recorded a failure or failure_reason exists, treat as done (failed)
	if !aiDone && failureReason.Valid && failureReason.String != "" {
		aiDone = true
	}

	// Timeout safeguard: if review was created > 10 minutes ago and AI review never reported back, don't hang forever
	if !aiDone && time.Since(createdAt) > 10*time.Minute {
		log.Printf("[WARN] ToolReviewWorker: review=%d AI review timed out after %v without setting ai_review_done; completing review with available tool findings", args.ReviewID, time.Since(createdAt).Round(time.Second))
		aiDone = true
		if !failureReason.Valid || failureReason.String == "" {
			failureReason = sql.NullString{String: "AI review portion timed out", Valid: true}
		}
	}

	if !aiDone {
		log.Printf("[INFO] ToolReviewWorker: review=%d all static analysis tools done, waiting for AI review to complete", args.ReviewID)
		eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
		_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "info", "All static analysis tools completed. Waiting for AI review to finish...", "")
		return nil
	}

	// Both AI review and all tools are done! Complete the review atomically
	finalStatus := "completed"
	if failureReason.Valid && failureReason.String != "" {
		finalStatus = "failed"
	}

	res, updErr := w.db.ExecContext(ctx, `
		UPDATE public.reviews
		SET status = $1, completed_at = NOW()
		WHERE id = $2 AND status NOT IN ('completed', 'failed')`, finalStatus, args.ReviewID)
	if updErr != nil {
		log.Printf("[ERROR] Failed to update review %d status to %s: %v", args.ReviewID, finalStatus, updErr)
		return fmt.Errorf("failed to update review status: %w", updErr)
	}

	if rows, _ := res.RowsAffected(); rows > 0 {
		var commentCount int
		if err := w.db.QueryRowContext(ctx, `
			SELECT COALESCE(jsonb_array_length(metadata->'review_result'->'comments'), 0)
			FROM public.reviews WHERE id = $1`, args.ReviewID).Scan(&commentCount); err != nil || commentCount == 0 {
			if scanErr := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.ai_comments WHERE review_id = $1`, args.ReviewID).Scan(&commentCount); scanErr != nil {
				log.Printf("[WARN] ToolReviewWorker: failed to query ai_comments count for review %d: %v", args.ReviewID, scanErr)
			}
		}

		completionSummary := ""
		if metaSummary.Valid && metaSummary.String != "" {
			completionSummary = metaSummary.String
		} else {
			completionSummary = "### Review Completed\n\nAI review and static analysis tools completed."
		}

		failStr := ""
		if failureReason.Valid {
			failStr = failureReason.String
		}

		eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
		if emitErr := eventSink.EmitCompletionEvent(ctx, args.ReviewID, args.OrgID,
			completionSummary, commentCount, failStr); emitErr != nil {
			log.Printf("[ERROR] Failed to emit completion event for review %d: %v", args.ReviewID, emitErr)
		}

		log.Printf("[INFO] ToolReviewWorker: review=%d ALL tools and AI review completed, status=%s, total comments=%d", args.ReviewID, finalStatus, commentCount)
	}

	return nil
}
