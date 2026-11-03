package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/livereview/internal/license"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/internal/toolclassifier"
	networktools "github.com/livereview/network/tools"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
)

// ToolReviewJobArgs represents the arguments for executing a single static analysis tool via AWS Lambda.
type ToolReviewJobArgs struct {
	ReviewID      int64  `json:"review_id"`
	OrgID         int64  `json:"org_id"`
	PlanCode      string `json:"plan_code"`
	ToolID        int64  `json:"tool_id"`
	ToolName      string `json:"tool_name"`
	LambdaARN     string `json:"lambda_arn"`
	DiffZipBase64 string `json:"diff_zip_base64"`
}

func (ToolReviewJobArgs) Kind() string { return "tool_review" }

// ToolReviewWorker executes a single static analysis tool via AWS Lambda and stores classified findings.
type ToolReviewWorker struct {
	river.WorkerDefaults[ToolReviewJobArgs]
	db     *sql.DB
	awsCfg aws.Config
}

func (w *ToolReviewWorker) Timeout(job *river.Job[ToolReviewJobArgs]) time.Duration {
	return 10 * time.Minute
}

// NewToolReviewWorker creates a new ToolReviewWorker with AWS config loaded from env.
func NewToolReviewWorker(db *sql.DB) *ToolReviewWorker {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	var awsCfg aws.Config
	var err error
	if accessKey != "" && secretKey != "" {
		awsCfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else {
		awsCfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(region),
		)
	}
	if err != nil {
		log.Printf("[WARN] ToolReviewWorker: failed to load AWS config: %v. Creating fallback config.", err)
		awsCfg = aws.Config{Region: region}
		if accessKey != "" && secretKey != "" {
			awsCfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		}
	}
	return &ToolReviewWorker{db: db, awsCfg: awsCfg}
}

func (w *ToolReviewWorker) Work(ctx context.Context, job *river.Job[ToolReviewJobArgs]) error {
	args := job.Args
	startTime := time.Now()

	log.Printf("[INFO] ToolReviewWorker: starting tool=%s review=%d org=%d", args.ToolName, args.ReviewID, args.OrgID)

	eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
	_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "info", fmt.Sprintf("Executing static analysis tool: %s", strings.ToUpper(args.ToolName)), "")

	// Build Lambda payload
	payload := map[string]interface{}{
		"zip_file": args.DiffZipBase64,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Failed to marshal Lambda payload: %v", err), "")
		return fmt.Errorf("failed to marshal Lambda payload: %w", err)
	}

	log.Printf("[INFO] ToolReviewWorker: invoking Lambda %s (payload %d bytes) for tool=%s review=%d...", args.LambdaARN, len(payloadBytes), args.ToolName, args.ReviewID)
	// Invoke Lambda
	outBytes, invokeErr := networktools.InvokeTool(ctx, w.awsCfg, args.LambdaARN, payloadBytes)
	if invokeErr != nil {
		log.Printf("[ERROR] Lambda invocation for tool %s failed: %v", args.ToolName, invokeErr)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		_ = eventSink.EmitLogEvent(cleanupCtx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Lambda execution for tool %s failed: %v", args.ToolName, invokeErr), "")
		// Write synthetic failure event
		toolsStore := storagetools.NewToolsStore(w.db)
		failJSON := fmt.Sprintf(`{"exit_code": -1, "findings": [], "stderr": %q}`, invokeErr.Error())
		if insertErr := toolsStore.InsertToolResultEvent(cleanupCtx, args.ReviewID, args.OrgID, args.ToolID, args.ToolName, []byte(failJSON)); insertErr != nil {
			log.Printf("[ERROR] Failed to insert synthetic tool result event: %v", insertErr)
		}
		_ = w.finalizeIfAllDone(cleanupCtx, args)
		return nil
	}

	log.Printf("[INFO] ToolReviewWorker: Lambda returned %d bytes in %v for tool=%s review=%d", len(outBytes), time.Since(startTime).Round(time.Millisecond), args.ToolName, args.ReviewID)
	_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "success", fmt.Sprintf("Lambda execution completed for %s (%d bytes returned)", args.ToolName, len(outBytes)), "")

	// Store raw tool_result event
	toolsStore := storagetools.NewToolsStore(w.db)
	if err := toolsStore.InsertToolResultEvent(ctx, args.ReviewID, args.OrgID, args.ToolID, args.ToolName, outBytes); err != nil {
		_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Failed to store raw tool result event: %v", err), "")
		return fmt.Errorf("failed to store tool result event: %w", err)
	}

	// Parse findings and classify using deterministic rule map + taxonomy
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
	if jsonErr := json.Unmarshal(outBytes, &altResp); jsonErr != nil {
		log.Printf("[WARN] ToolReviewWorker: failed to unmarshal Lambda findings output for tool=%s review=%d: %v", args.ToolName, args.ReviewID, jsonErr)
	} else if len(altResp.Findings) > 0 {
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
		classifiedComments, classErr := toolclassifier.ClassifyToolResult(ctx, w.db, args.OrgID, license.PlanType(args.PlanCode), args.ToolName, rawList, nil)
		if classErr == nil && len(classifiedComments) > 0 {
			toolComments := make([]map[string]interface{}, 0, len(classifiedComments))
			for _, c := range classifiedComments {
				if c == nil {
					continue
				}
				toolComments = append(toolComments, map[string]interface{}{
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
					"tool_name":   args.ToolName,
				})
			}

			if len(toolComments) > 0 {
				toolCommentsBytes, err := json.Marshal(toolComments)
				if err != nil {
					log.Printf("[ERROR] ToolReviewWorker: failed to marshal tool comments for review %d tool=%s: %v", args.ReviewID, args.ToolName, err)
					_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Failed to marshal %s findings: %v", args.ToolName, err), "")
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
						_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "success", fmt.Sprintf("Appended %d %s findings to review #%d", len(toolComments), strings.ToUpper(args.ToolName), args.ReviewID), "")
						log.Printf("[INFO] ToolReviewWorker: tool=%s review=%d appended %d comments to review_result", args.ToolName, args.ReviewID, len(toolComments))
					}
				}
			}
		}
	} else {
		log.Printf("[INFO] ToolReviewWorker: tool=%s review=%d no findings to classify", args.ToolName, args.ReviewID)
	}

	w.finalizeIfAllDone(ctx, args)
	return nil
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
		WHERE id = $2 AND status != 'completed'`, finalStatus, args.ReviewID)
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
