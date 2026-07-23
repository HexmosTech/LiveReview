package jobqueue

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/livereview/internal/diffutil"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	reviewpkg "github.com/livereview/internal/review"
	"github.com/livereview/network/tools"
	"github.com/livereview/pkg/models"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
)

// ToolReviewOrchestratorJobArgs represents the arguments for orchestrating tool reviews
type ToolReviewOrchestratorJobArgs struct {
	ReviewID        int64   `json:"review_id"`
	OrgID           int64   `json:"org_id"`
	PRURL           string  `json:"pr_url"`
	ConnectorID     int64   `json:"connector_id"`
	Provider        string  `json:"provider"`
	TotalMultiplier float64 `json:"total_multiplier"`
}

// Kind returns the job kind for River
func (ToolReviewOrchestratorJobArgs) Kind() string {
	return "tool_review_orchestrator"
}

// ToolReviewOrchestratorWorker handles the entire tool review orchestration pipeline
type ToolReviewOrchestratorWorker struct {
	river.WorkerDefaults[ToolReviewOrchestratorJobArgs]
	db     *sql.DB
	awsCfg aws.Config
}

// Work performs the full tool review pipeline (diff fetch, credit deduct, tool invocation, comment post)
func (w *ToolReviewOrchestratorWorker) Work(ctx context.Context, job *river.Job[ToolReviewOrchestratorJobArgs]) error {
	args := job.Args

	log.Printf("[INFO] ToolReviewOrchestrator: starting for review=%d, org=%d, provider=%s", args.ReviewID, args.OrgID, args.Provider)
	
	logger, err := logging.StartReviewLoggingWithIDs(strconv.FormatInt(args.ReviewID, 10), args.ReviewID, args.OrgID)
	if err != nil {
		log.Printf("[WARN] ToolReviewOrchestrator: failed to start review logger: %v", err)
	}
	if logger != nil {
		defer logger.Close()
		logger.LogSection("ORCHESTRATOR STARTED")
		logger.Log("Tool Review Orchestrator initialized for review ID %d", args.ReviewID)
	}

	// 1. Fetch enabled tools
	toolsStore := storagetools.NewToolsStore(w.db)
	enabledTools, err := toolsStore.GetEnabledToolsForOrg(ctx, args.OrgID)
	if err != nil {
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
		if logger != nil {
			logger.EmitReviewFailure(fmt.Errorf("failed to fetch enabled tools: %w", err))
		}
		return fmt.Errorf("failed to fetch enabled tools: %w", err)
	}
	if len(enabledTools) == 0 {
		log.Printf("[INFO] ToolReviewOrchestrator: No enabled tools for org %d", args.OrgID)
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "completed", args.ReviewID)
		return nil
	}

	// Credit check and deduction is handled during ExecuteToolsForReview.

	// 3. Fetch Connection and Diff from Provider
	providerFactory := reviewpkg.NewStandardProviderFactory()
	
	// Fetch connection details to build ProviderConfig
	var tokenNS sql.NullString
	var patToken sql.NullString
	var tokenType sql.NullString
	var providerURL sql.NullString
	
	err = w.db.QueryRowContext(ctx, `SELECT access_token, pat_token, token_type, provider_url FROM integration_tokens WHERE id = $1 AND org_id = $2`, args.ConnectorID, args.OrgID).Scan(&tokenNS, &patToken, &tokenType, &providerURL)
	if err != nil {
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
		if logger != nil {
			logger.EmitReviewFailure(fmt.Errorf("failed to get integration token: %w", err))
		}
		return fmt.Errorf("failed to get integration token: %w", err)
	}
	
	actualToken := tokenNS.String
	if tokenType.Valid && tokenType.String == "PAT" && patToken.Valid && patToken.String != "" {
		actualToken = patToken.String
	}
	
	providerConfigMap := map[string]interface{}{}
	if tokenType.Valid && tokenType.String == "PAT" && patToken.Valid && patToken.String != "" {
		providerConfigMap["pat_token"] = patToken.String
		if strings.HasPrefix(args.Provider, "bitbucket") {
			providerConfigMap["repo_url"] = args.PRURL
		}
	}
	
	provConfig := reviewpkg.ProviderConfig{
		Type:   args.Provider,
		URL:    providerURL.String,
		Token:  actualToken,
		Config: providerConfigMap,
	}

	providerInstance, err := providerFactory.CreateProvider(ctx, provConfig)
	if err != nil {
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
		if logger != nil {
			logger.EmitReviewFailure(fmt.Errorf("failed to create provider: %w", err))
		}
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Resolve PR ID
	prID := fmt.Sprintf("%d", args.ReviewID)
	parsedURL, err := neturl.Parse(args.PRURL)
	if err == nil {
		parts := strings.Split(parsedURL.Path, "/")
		if strings.HasPrefix(args.Provider, "github") && len(parts) >= 5 && parts[3] == "pull" {
			prID = parts[1] + "/" + parts[2] + "/" + parts[4]
		} else if strings.HasPrefix(args.Provider, "bitbucket") && len(parts) >= 5 && parts[3] == "pull-requests" {
			prID = parts[1] + "/" + parts[2] + "/" + parts[4]
		}
	}

	mrDetails, err := providerInstance.GetMergeRequestDetails(ctx, args.PRURL)
	if err == nil && mrDetails != nil {
		prID = mrDetails.ID
		if args.Provider == "github" {
			u, parseErr := neturl.Parse(mrDetails.URL)
			if parseErr == nil {
				parts := strings.Split(u.Path, "/")
				if len(parts) >= 5 && parts[3] == "pull" {
					prID = parts[1] + "/" + parts[2] + "/" + parts[4]
				}
			}
		} else if args.Provider == "bitbucket" {
			u, parseErr := neturl.Parse(mrDetails.URL)
			if parseErr == nil {
				parts := strings.Split(u.Path, "/")
				if len(parts) >= 5 && parts[3] == "pull-requests" {
					prID = parts[1] + "/" + parts[2] + "/" + parts[4]
				}
			}
		}
		
		// Update review metadata (Issue #5)
		authorName := mrDetails.AuthorName
		if authorName == "" {
			authorName = mrDetails.Author
		}
		authorUsername := mrDetails.AuthorUsername
		if authorUsername == "" {
			authorUsername = mrDetails.Author
		}
		
		_, dbErr := w.db.ExecContext(ctx, `
			UPDATE public.reviews
			SET repository = COALESCE(NULLIF($1, ''), repository),
			    branch = COALESCE(NULLIF($2, ''), branch),
			    mr_title = COALESCE(NULLIF($3, ''), mr_title),
			    author_name = COALESCE(NULLIF($4, ''), author_name),
			    author_username = COALESCE(NULLIF($5, ''), author_username)
			WHERE id = $6
		`, mrDetails.RepositoryURL, mrDetails.SourceBranch, mrDetails.Title, authorName, authorUsername, args.ReviewID)
		
		if dbErr != nil {
			log.Printf("[WARN] ToolReviewOrchestrator: failed to update review metadata for review=%d: %v", args.ReviewID, dbErr)
		}
	}

	changes, err := providerInstance.GetMergeRequestChanges(ctx, prID)
	if err != nil {
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
		if logger != nil {
			logger.EmitReviewFailure(fmt.Errorf("failed to get MR changes: %w", err))
		}
		return fmt.Errorf("failed to get MR changes: %w", err)
	}

	rawDiff := reviewpkg.FormatDiffs(changes)
	if rawDiff == "" {
		log.Printf("[INFO] ToolReviewOrchestrator: empty diff for review %d", args.ReviewID)
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "completed", args.ReviewID)
		return nil
	}

	// 4. Run Tools
	toolComments, err := ExecuteToolsForReview(ctx, w.db, w.awsCfg, args.OrgID, args.ReviewID, rawDiff, "", logger)
	if err != nil {
		_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
		if logger != nil {
			logger.EmitReviewFailure(fmt.Errorf("failed to execute tools: %w", err))
		}
		return fmt.Errorf("failed to execute tools: %w", err)
	}

	// 5. Post Comments to Provider (inline on file:line when available)
	if len(toolComments) > 0 {
		postErr := providerInstance.PostComments(ctx, prID, toolComments)
		if postErr != nil {
			log.Printf("[ERROR] Failed to post static analysis comments to PR: %v", postErr)
			_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "failed", args.ReviewID)
			if logger != nil {
				logger.EmitReviewFailure(fmt.Errorf("failed to post comments to PR: %w", postErr))
			}
			return fmt.Errorf("failed to post static analysis comments to PR: %w", postErr)
		}
	}

	// 6. Finalize
	_, _ = w.db.ExecContext(ctx, "UPDATE public.reviews SET status = $1 WHERE id = $2", "completed", args.ReviewID)
	log.Printf("[INFO] ToolReviewOrchestrator: completed review=%d", args.ReviewID)
	if logger != nil {
		logger.EmitReviewCompletion(len(toolComments), "Tool static analysis complete")
	}

	return nil
}

// ExecuteToolsForReview runs the enabled static analysis tools for the given review.
// It checks/deducts credits, invokes the tool lambdas in parallel, inserts the tool result events,
// and returns the parsed review comments.
func ExecuteToolsForReview(
	ctx context.Context,
	db *sql.DB,
	awsCfg aws.Config,
	orgID int64,
	reviewID int64,
	rawDiff string,
	zipBase64 string,
	logger *logging.ReviewLogger,
) ([]*models.ReviewComment, error) {
	toolsStore := storagetools.NewToolsStore(db)
	enabledTools, err := toolsStore.GetEnabledToolsForOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch enabled tools: %w", err)
	}

	// Parse repo-level tool configuration from zipBase64 if present (.lrc/tools.toml)
	if zipBase64 != "" {
		_, lrcBundle, parseErr := diffutil.ParseDiffZipBase64(zipBase64)
		if parseErr == nil {
			repoTools, toolTomlErr := lrcconfig.ParseToolConfig(lrcBundle)
			if toolTomlErr == nil && len(repoTools) > 0 {
				existingMap := make(map[string]bool)
				for _, t := range enabledTools {
					existingMap[strings.ToLower(t.Name)] = true
				}
				for toolName, enabled := range repoTools {
					toolNameLower := strings.ToLower(toolName)
					if enabled && !existingMap[toolNameLower] {
						t, getErr := toolsStore.GetAvailableToolByName(ctx, toolNameLower)
						if getErr == nil && t != nil {
							enabledTools = append(enabledTools, *t)
							existingMap[toolNameLower] = true
							if logger != nil {
								logger.Log(fmt.Sprintf("Repo-level config (.lrc/tools.toml) enabled tool %q", t.Name))
							}
						}
					}
				}
			}
		}
	}

	if len(enabledTools) == 0 {
		return nil, nil
	}

	var totalMultiplier float64
	for _, t := range enabledTools {
		totalMultiplier += t.Multiplier
	}

	creditStore := storagetools.NewCreditStore(db)

	// Fetch plan code for this org from the review record or org_billing_state.
	var planCodeStr string
	_ = db.QueryRowContext(ctx,
		`SELECT COALESCE(metadata->>'plan_code', '') FROM public.reviews WHERE id = $1`,
		reviewID,
	).Scan(&planCodeStr)
	if planCodeStr == "" {
		_ = db.QueryRowContext(ctx,
			`SELECT current_plan_code FROM public.org_billing_state WHERE org_id = $1`,
			orgID,
		).Scan(&planCodeStr)
	}
	planCode := license.PlanType(planCodeStr)
	if !license.IsToolsEligible(planCode) {
		return nil, fmt.Errorf("tools not available on plan %q — skipping tool execution", planCode)
	}

	err = creditStore.DeductCredits(ctx, orgID, reviewID, totalMultiplier, planCode)
	if err != nil {
		return nil, fmt.Errorf("failed to deduct credits: %w", err)
	}

	if zipBase64 == "" && rawDiff != "" {
		var buf bytes.Buffer
		zipWriter := zip.NewWriter(&buf)
		if f, err := zipWriter.Create("diff.txt"); err == nil {
			_, _ = f.Write([]byte(rawDiff))
		}
		_ = zipWriter.Close()
		zipBase64 = base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	var wg sync.WaitGroup
	var toolMu sync.Mutex
	var toolComments []*models.ReviewComment

	for _, tool := range enabledTools {
		wg.Add(1)
		go func(t storagetools.AvailableTool) {
			defer wg.Done()

			payloadMap := map[string]interface{}{
				"review_id": reviewID,
				"diff":      rawDiff,
				"zip_file":  zipBase64,
			}
			payloadBytes, err := json.Marshal(payloadMap)
			if err != nil {
				if logger != nil {
					logger.Log("[ERROR] Tool %s payload marshal failed: %v", t.Name, err)
				}
				return
			}

			respBytes, err := tools.InvokeTool(ctx, awsCfg, t.LambdaARN, payloadBytes)
			if err != nil {
				if logger != nil {
					logger.Log("[ERROR] Tool %s lambda invocation failed: %v", t.Name, err)
				}
				return
			}

			if err := toolsStore.InsertToolResultEvent(ctx, reviewID, orgID, t.ID, t.Name, respBytes); err != nil {
				if logger != nil {
					logger.Log("[ERROR] Tool %s failed to store result event: %v", t.Name, err)
				}
			}

			var result struct {
				LiveReviewComments []struct {
					FilePath string `json:"filePath"`
					Line     int    `json:"line"`
					Content  string `json:"content"`
					Severity string `json:"severity"`
					Category string `json:"category"`
				} `json:"livereview_comments"`
			}
			if err := json.Unmarshal(respBytes, &result); err != nil {
				if logger != nil {
					logger.Log("[ERROR] Tool %s response unmarshal failed: %v", t.Name, err)
				}
				return
			}

			if len(result.LiveReviewComments) > 0 {
				toolMu.Lock()
				for _, lrc := range result.LiveReviewComments {
					severity := models.SeverityWarning
					if lrc.Severity == "critical" {
						severity = models.SeverityCritical
					} else if lrc.Severity == "info" {
						severity = models.SeverityInfo
					}
					comment := &models.ReviewComment{
						FilePath: lrc.FilePath,
						Line:     lrc.Line,
						Content:  lrc.Content,
						Severity: severity,
						Category: "tool-generated",
					}
					toolComments = append(toolComments, comment)
				}
				toolMu.Unlock()
			}
		}(tool)
	}
	wg.Wait()

	return toolComments, nil
}
