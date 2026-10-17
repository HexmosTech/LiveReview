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
	outBytes, err := networktools.InvokeTool(ctx, w.awsCfg, args.LambdaARN, payloadBytes)
	if err != nil {
		log.Printf("[ERROR] Lambda invocation for tool %s failed: %v", args.ToolName, err)
		_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "error", fmt.Sprintf("Lambda execution for tool %s failed: %v", args.ToolName, err), "")
		// Write synthetic failure event
		toolsStore := storagetools.NewToolsStore(w.db)
		failJSON := fmt.Sprintf(`{"exit_code": -1, "findings": [], "stderr": %q}`, err.Error())
		if insertErr := toolsStore.InsertToolResultEvent(ctx, args.ReviewID, args.OrgID, args.ToolID, args.ToolName, []byte(failJSON)); insertErr != nil {
			log.Printf("[ERROR] Failed to insert synthetic tool result event: %v", insertErr)
		}
		w.finalizeIfAllDone(ctx, args)
		return err
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
			rm := reviewprocessor.NewReviewManager(w.db)
			stored := 0
			for _, c := range classifiedComments {
				if c == nil {
					continue
				}
				cMap := map[string]interface{}{
					"content":     c.Content,
					"severity":    string(c.Severity),
					"category":    c.Category,
					"subcategory": c.Subcategory,
					"confidence":  c.Confidence,
					"type":        c.Type,
					"suggestions": c.Suggestions,
					"source":      "tool",
					"tool_name":   args.ToolName,
				}
				commentType := "line_comment"
				var linePtr *int
				if c.Line > 0 {
					lVal := c.Line
					linePtr = &lVal
				} else {
					commentType = "file_comment"
				}

				var pathPtr *string
				if strings.TrimSpace(c.FilePath) != "" {
					pVal := c.FilePath
					pathPtr = &pVal
				}

				_, err := rm.AddAIComment(args.ReviewID, commentType, cMap, pathPtr, linePtr, args.OrgID)
				if err != nil {
					log.Printf("[ERROR] Failed to add AI comment for review %d: %v", args.ReviewID, err)
				} else {
					stored++
				}
			}
			_ = eventSink.EmitLogEvent(ctx, args.ReviewID, args.OrgID, "success", fmt.Sprintf("Persisted %d/%d classified taxonomy comments to review #%d", stored, len(classifiedComments), args.ReviewID), "")
			log.Printf("[INFO] ToolReviewWorker: tool=%s review=%d stored %d/%d classified taxonomy comments", args.ToolName, args.ReviewID, stored, len(classifiedComments))
		}
	} else {
		log.Printf("[INFO] ToolReviewWorker: tool=%s review=%d no findings to classify", args.ToolName, args.ReviewID)
	}

	w.finalizeIfAllDone(ctx, args)
	return nil
}

// finalizeIfAllDone checks if all dispatched tools have returned results and marks the review completed.
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

	if pendingJobs == 0 {
		if _, updErr := w.db.ExecContext(ctx, `UPDATE public.reviews SET status = 'completed', completed_at = NOW() WHERE id = $1`, args.ReviewID); updErr != nil {
			log.Printf("[ERROR] Failed to update review %d status to completed: %v", args.ReviewID, updErr)
			return fmt.Errorf("failed to update review status to completed: %w", updErr)
		}

		// Count persisted comments and emit completion event
		var commentCount int
		if scanErr := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.ai_comments WHERE review_id = $1`, args.ReviewID).Scan(&commentCount); scanErr != nil {
			log.Printf("[WARN] Failed to query comment count for completed review %d: %v", args.ReviewID, scanErr)
		}

		eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
		if emitErr := eventSink.EmitCompletionEvent(ctx, args.ReviewID, args.OrgID,
			"### Static Analysis Tools Review Only\n\nAI review skipped due to --tools flag.",
			commentCount, ""); emitErr != nil {
			log.Printf("[ERROR] Failed to emit completion event for review %d: %v", args.ReviewID, emitErr)
		}

		log.Printf("[INFO] ToolReviewWorker: review=%d ALL tools done, status=completed, comments=%d", args.ReviewID, commentCount)
	}
	return nil
}
