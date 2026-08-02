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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/diffutil"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	"github.com/livereview/internal/prompts"
	reviewpkg "github.com/livereview/internal/review"
	"github.com/livereview/network/tools"
	"github.com/livereview/pkg/models"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
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

	var localDiffs []lib.LocalCodeDiff
	var lrcBundle lrcconfig.Bundle
	var toolRuleConfigs map[string]*lrcconfig.ToolRuleConfig

	// Parse repo-level tool configuration from zipBase64 if present
	if zipBase64 != "" {
		var parseErr error
		localDiffs, lrcBundle, parseErr = diffutil.ParseDiffZipBase64(zipBase64)
		if parseErr == nil {
			// Parse tool rule configs from policy/tools.toml or tools.toml
			toolRuleConfigs, _ = lrcconfig.ParseToolRuleConfigs(lrcBundle)

			existingMap := make(map[string]bool)
			for _, t := range enabledTools {
				existingMap[strings.ToLower(t.Name)] = true
			}

			// Add tools enabled via repo-level configuration tables (e.g. [gitleaks] enabled = true)
			for toolName, cfg := range toolRuleConfigs {
				if cfg != nil && cfg.Enabled != nil && *cfg.Enabled && !existingMap[toolName] {
					t, getErr := toolsStore.GetAvailableToolByName(ctx, toolName)
					if getErr == nil && t != nil {
						enabledTools = append(enabledTools, *t)
						existingMap[toolName] = true
						if logger != nil {
							logger.Log(fmt.Sprintf("Repo-level config (.lrc/policy/tools.toml) enabled tool %q", t.Name))
						}
					}
				}
			}
		}
	}

	// Filter enabled tools by per-tool path inclusion/exclusion rules
	var filteredTools []storagetools.AvailableTool
	for _, t := range enabledTools {
		toolNameLower := strings.ToLower(t.Name)
		cfg := toolRuleConfigs[toolNameLower]

		if lrcconfig.ShouldRunToolRuleForDiff(cfg, localDiffs) {
			filteredTools = append(filteredTools, t)
		} else if logger != nil {
			logger.Log(fmt.Sprintf("Tool %q skipped: no diff files matched trigger rules (.lrc/policy/tools.toml)", t.Name))
		}
	}
	enabledTools = filteredTools

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

			toolNameLower := strings.ToLower(t.Name)
			cfg := toolRuleConfigs[toolNameLower]

			toolRawDiff := rawDiff
			if cfg != nil && len(localDiffs) > 0 {
				filteredDiffs := lrcconfig.FilterLocalCodeDiffsForTool(cfg, localDiffs)
				if len(filteredDiffs) < len(localDiffs) {
					toolRawDiff = lrcconfig.FormatLocalDiffs(filteredDiffs)
				}
			}

			payloadMap := map[string]interface{}{
				"review_id": reviewID,
				"diff":      toolRawDiff,
				"zip_file":  zipBase64,
			}
			payloadBytes, err := json.Marshal(payloadMap)
			if err != nil {
				if logger != nil {
					logger.Log("[ERROR] Tool %s payload marshal failed: %v", t.Name, err)
				}
				return
			}

			if logger != nil {
				logger.Log("[TOOL %s] Invoking Lambda ARN: %s", t.Name, t.LambdaARN)
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

			var rawFindings []ToolFindingRaw
			var legacyLrcComments []struct {
				FilePath string `json:"filePath"`
				Line     int    `json:"line"`
				Content  string `json:"content"`
				Severity string `json:"severity"`
				Category string `json:"category"`
			}
			var exitCode int

			trimmedResp := strings.TrimSpace(string(respBytes))
			if strings.HasPrefix(trimmedResp, "[") {
				if err := json.Unmarshal(respBytes, &rawFindings); err != nil {
					if logger != nil {
						logger.Log("[ERROR] Tool %s raw array response unmarshal failed: %v", t.Name, err)
					}
				}
				if len(rawFindings) > 0 {
					exitCode = 1
				}
			} else {
				var rawResp struct {
					ExitCode int              `json:"exit_code"`
					Findings []ToolFindingRaw `json:"findings"`
					LiveReviewComments []struct {
						FilePath string `json:"filePath"`
						Line     int    `json:"line"`
						Content  string `json:"content"`
						Severity string `json:"severity"`
						Category string `json:"category"`
					} `json:"livereview_comments"`
				}
				if err := json.Unmarshal(respBytes, &rawResp); err != nil {
					if logger != nil {
						logger.Log("[ERROR] Tool %s response unmarshal failed: %v", t.Name, err)
					}
					return
				}
				rawFindings = rawResp.Findings
				legacyLrcComments = rawResp.LiveReviewComments
				exitCode = rawResp.ExitCode
			}

			// Immediately sanitize and redact secret fields in memory right after unmarshaling
			for idx := range rawFindings {
				rawFindings[idx].Secret = "[REDACTED]"
				rawFindings[idx].CodeSnippet = "[REDACTED]"
			}

			if logger != nil {
				logger.Log("[TOOL %s] Received %d raw findings, exit_code=%d", t.Name, len(rawFindings), exitCode)
			}

			// Process findings: classify raw findings concurrently with LLM or map legacy comments
			if len(rawFindings) > 0 {
				if logger != nil {
					logger.Log("[TOOL %s] Starting parallel LLM classification for %d findings...", t.Name, len(rawFindings))
				}
				var findWg sync.WaitGroup
				for _, f := range rawFindings {
					findWg.Add(1)
					go func(finding ToolFindingRaw) {
						defer findWg.Done()
						comment := classifyToolFindingWithLLM(ctx, db, orgID, t.Name, finding, logger)
						if comment != nil {
							toolMu.Lock()
							toolComments = append(toolComments, comment)
							toolMu.Unlock()
						}
					}(f)
				}
				findWg.Wait()
				if logger != nil {
					logger.Log("[TOOL %s] Completed LLM classification for %d findings.", t.Name, len(rawFindings))
				}
			} else if len(legacyLrcComments) > 0 {
				toolMu.Lock()
				for _, lrc := range legacyLrcComments {
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
						Source:   "tool",
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

type ToolFindingRaw struct {
	File        string `json:"file"`
	FilePath    string `json:"file_path"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	LineNumber  int    `json:"line_number"`
	Start       struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	Col         int    `json:"col"`
	Rule        string `json:"rule"`
	RuleID      string `json:"rule_id"`
	CheckID     string `json:"check_id"`
	Message     string `json:"message"`
	Extra       struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
	} `json:"extra"`
	Secret      string `json:"secret"`
	CodeSnippet string `json:"code_snippet"`
}

func (f ToolFindingRaw) GetFile() string {
	if f.Path != "" {
		return f.Path
	}
	if f.FilePath != "" {
		return f.FilePath
	}
	return f.File
}

func (f ToolFindingRaw) GetLine() int {
	if f.Start.Line > 0 {
		return f.Start.Line
	}
	if f.LineNumber > 0 {
		return f.LineNumber
	}
	return f.Line
}

func (f ToolFindingRaw) GetRule() string {
	if f.CheckID != "" {
		return f.CheckID
	}
	if f.RuleID != "" {
		return f.RuleID
	}
	return f.Rule
}

func (f ToolFindingRaw) GetMessage() string {
	if f.Extra.Message != "" {
		return f.Extra.Message
	}
	return f.Message
}

type ClassifiedToolResult struct {
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory"`
	Severity    string   `json:"severity"`
	Type        string   `json:"type"`
	Confidence  string   `json:"confidence"`
	Suggestions []string `json:"suggestions"`
	IsInternal  bool     `json:"isInternal"`
}

func classifyToolFindingWithLLM(
	ctx context.Context,
	db *sql.DB,
	orgID int64,
	toolName string,
	finding ToolFindingRaw,
	logger *logging.ReviewLogger,
) *models.ReviewComment {
	// 1. Fetch AI connector details for orgID from database
	var providerName, selectedModel, apiKey string
	err := db.QueryRowContext(ctx, `
		SELECT provider_name, COALESCE(selected_model, ''), api_key 
		FROM public.ai_connectors 
		WHERE org_id = $1 AND api_key != '' 
		ORDER BY id ASC LIMIT 1
	`, orgID).Scan(&providerName, &selectedModel, &apiKey)

	if err != nil || apiKey == "" {
		if logger != nil {
			logger.Log("[WARN] No active AI connector found for org_id=%d: %v", orgID, err)
		}
	}

	if selectedModel == "" {
		storage := aiconnectors.NewStorage(db)
		selectedModel = storage.GetDefaultModel(ctx, aiconnectors.Provider(providerName))
	}

	filePath := finding.GetFile()
	lineNum := finding.GetLine()
	ruleID := finding.GetRule()
	findingMsg := cleanFindingMessage(finding.GetMessage())

	// Default fallback values if LLM is unavailable
	defaultSeverity := models.SeverityCritical
	ruleLower := strings.ToLower(ruleID)
	msgLower := strings.ToLower(findingMsg)
	if strings.Contains(ruleLower, "info") || strings.Contains(msgLower, "info") {
		defaultSeverity = models.SeverityInfo
	} else if strings.Contains(ruleLower, "warn") {
		defaultSeverity = models.SeverityWarning
	}

	fallbackComment := &models.ReviewComment{
		FilePath:    filePath,
		Line:        lineNum,
		Content:     findingMsg,
		Severity:    defaultSeverity,
		Confidence:  "High",
		Type:        "Risk",
		Category:    "Security",
		Subcategory: "Secrets Management",
		Source:      "tool",
	}

	if apiKey == "" {
		if logger != nil {
			logger.Log("[WARN] No API key available for LLM classification of finding %s:%d, using fallback", filePath, lineNum)
		}
		return fallbackComment
	}

	// 2. Build prompt
	builder := prompts.NewPromptBuilder()
	promptInput := prompts.ToolFindingInput{
		ToolName:    toolName,
		RuleID:      ruleID,
		FilePath:    filePath,
		LineNumber:  lineNum,
		Message:     findingMsg,
		CodeSnippet: finding.CodeSnippet,
	}
	if promptInput.CodeSnippet == "" && finding.Secret != "" {
		promptInput.CodeSnippet = "secret = \"[REDACTED]\""
	}

	promptText := builder.BuildToolFindingClassificationPrompt(promptInput)

	if logger != nil {
		logger.Log("[CLASSIFY %s] %s:%d (%s) -> Calling LLM model %s", toolName, filePath, lineNum, ruleID, selectedModel)
	}

	// 3. Call Gemini / LLM model
	llmModel, errInit := googleai.New(ctx,
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel(selectedModel),
	)
	if errInit != nil {
		if logger != nil {
			logger.Log("[WARN] Failed to init LLM for classification: %v", errInit)
		}
		return fallbackComment
	}

	var respCall string
	for retry := 0; retry < 3; retry++ {
		resp, errCall := llms.GenerateFromSinglePrompt(ctx, llmModel, promptText,
			llms.WithTemperature(0.2),
			llms.WithMaxTokens(1500),
		)
		if errCall == nil && resp != "" {
			respCall = resp
			break
		}
		if errCall != nil && strings.Contains(errCall.Error(), "429") {
			time.Sleep(3 * time.Second)
			continue
		}
		if logger != nil {
			logger.Log("[WARN] LLM call error: %v", errCall)
		}
		break
	}

	if respCall == "" {
		return fallbackComment
	}

	// 4. Parse classification result
	cleanJSON := cleanJSONString(respCall)
	var classified ClassifiedToolResult
	if err := json.Unmarshal([]byte(cleanJSON), &classified); err != nil {
		if logger != nil {
			logger.Log("[WARN] Failed to parse LLM classification JSON: %v. Raw: %s", err, respCall)
		}
		return fallbackComment
	}

	// Map severity string to models.Severity
	sev := models.SeverityWarning
	switch strings.ToLower(classified.Severity) {
	case "critical":
		sev = models.SeverityCritical
	case "info":
		sev = models.SeverityInfo
	case "warning":
		sev = models.SeverityWarning
	}

	// Validate and normalize Category and Subcategory against closed taxonomy
	category, subcategory := ValidateAndNormalizeTaxonomy(classified.Category, classified.Subcategory)
	confidence := NormalizeConfidence(classified.Confidence)
	commentType := NormalizeType(classified.Type)

	if logger != nil {
		logger.Log("[CLASSIFY %s] %s:%d -> Category: %s / %s (Severity: %s, Confidence: %s, Type: %s)", toolName, filePath, lineNum, category, subcategory, sev, confidence, commentType)
	}

	return &models.ReviewComment{
		FilePath:    filePath,
		Line:        lineNum,
		Content:     findingMsg,
		Severity:    sev,
		Confidence:  confidence,
		Type:        commentType,
		Category:    category,
		Subcategory: subcategory,
		Source:      "tool",
	}
}

func cleanJSONString(s string) string {
	if idx := strings.Index(s, "{"); idx != -1 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "}"); idx != -1 {
		s = s[:idx+1]
	}
	return s
}

func cleanFindingMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if idx := strings.Index(msg, " (Match:"); idx != -1 {
		msg = strings.TrimSpace(msg[:idx])
	}
	if !strings.HasSuffix(msg, ".") && !strings.HasSuffix(msg, "!") {
		msg += "."
	}
	return msg
}

var ValidTaxonomyMap = map[string][]string{
	"Security":                {"Authentication", "Authorization", "Secrets Management", "Input Validation", "Injection Vulnerabilities", "Cryptography", "Dependency Vulnerabilities", "Data Exposure", "Session Management", "Security Logging & Auditing"},
	"Reliability":             {"Error Handling", "Fault Tolerance", "Retry Logic", "Timeout Management", "Resilience Patterns", "Availability Risks", "Data Integrity", "Race Conditions", "Resource Cleanup", "Failure Recovery"},
	"Correctness":             {"Logic Errors", "Edge Cases", "Data Validation", "State Management", "Concurrency Bugs", "Business Rule Violations", "Numerical Accuracy", "Null Handling", "Type Safety", "API Contract Violations"},
	"Performance":             {"Database Efficiency", "Algorithmic Complexity", "Memory Usage", "CPU Utilization", "Network Efficiency", "Caching", "Concurrency", "Resource Contention", "Rendering Performance", "Startup Performance"},
	"Cost":                    {"Cloud Resource Waste", "Infrastructure Overprovisioning", "Storage Optimization", "Database Cost Optimization", "Excessive API Usage", "Third-Party Service Costs", "Redundant Computation", "LLM Token Consumption", "Caching Opportunities", "Data Transfer Costs"},
	"Scalability":             {"Horizontal Scaling", "Vertical Scaling", "Distributed Systems", "Load Balancing", "Capacity Planning", "Bottleneck Risks", "Concurrency Limits", "Service Growth Constraints", "Database Scaling", "Queue Backpressure"},
	"Maintainability":         {"Code Complexity", "Readability", "Documentation", "Code Duplication", "Dead Code", "Naming Quality", "Testability", "Technical Debt", "Refactoring Opportunities", "Configuration Management", "UI/UX", "Accessibility"},
	"Architecture":            {"Separation of Concerns", "Modularity", "Coupling", "Cohesion", "Layering Violations", "Dependency Management", "Service Boundaries", "Domain Modeling", "API Design", "Extensibility"},
	"Developer Experience":    {"Testing", "CI/CD", "Build System", "Local Development", "Debuggability", "Observability", "Deployment Process", "Automation", "Developer Tooling", "Documentation Quality", "UI/UX", "Accessibility"},
	"Compliance & Governance": {"Privacy", "Regulatory Compliance", "Auditability", "Data Retention", "Data Residency", "Licensing", "Policy Enforcement", "Access Controls", "Change Management", "Governance Standards"},
}

var ValidTypes = map[string]string{
	"bug":            "Bug",
	"risk":           "Risk",
	"optimization":   "Optimization",
	"code smell":     "Code Smell",
	"best practice":  "Best Practice",
	"technical debt": "Technical Debt",
}

var ValidConfidences = map[string]string{
	"high":   "High",
	"medium": "Medium",
	"low":    "Low",
}

func ValidateAndNormalizeTaxonomy(rawCategory, rawSubcategory string) (string, string) {
	trimmedCat := strings.TrimSpace(rawCategory)
	trimmedSub := strings.TrimSpace(rawSubcategory)

	var matchedCategory string
	var allowedSubcategories []string

	// 1. Try matching rawCategory directly against top-level taxonomy categories
	for cat, subcats := range ValidTaxonomyMap {
		if strings.EqualFold(trimmedCat, cat) {
			matchedCategory = cat
			allowedSubcategories = subcats
			break
		}
	}

	// 2. If rawCategory is unrecognized, search all taxonomy subcategories to infer category from subcategory
	if matchedCategory == "" && trimmedSub != "" {
		for cat, subcats := range ValidTaxonomyMap {
			for _, sub := range subcats {
				if strings.EqualFold(trimmedSub, sub) {
					matchedCategory = cat
					allowedSubcategories = subcats
					trimmedSub = sub
					break
				}
			}
			if matchedCategory != "" {
				break
			}
		}
	}

	// 3. Fallback to Security only if no category could be matched or inferred
	if matchedCategory == "" {
		matchedCategory = "Security"
		allowedSubcategories = ValidTaxonomyMap["Security"]
	}

	// 4. Validate subcategory against allowedSubcategories of matchedCategory
	var matchedSubcategory string
	if trimmedSub != "" {
		for _, sub := range allowedSubcategories {
			if strings.EqualFold(trimmedSub, sub) {
				matchedSubcategory = sub
				break
			}
		}
	}

	// 5. Context-aware subcategory fallback if subcategory was empty or invalid
	if matchedSubcategory == "" {
		if matchedCategory == "Security" {
			matchedSubcategory = "Secrets Management"
		} else if len(allowedSubcategories) > 0 {
			matchedSubcategory = allowedSubcategories[0]
		}
	}

	return matchedCategory, matchedSubcategory
}

func NormalizeType(raw string) string {
	if val, ok := ValidTypes[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return val
	}
	return "Risk"
}

func NormalizeConfidence(raw string) string {
	if val, ok := ValidConfidences[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return val
	}
	return "High"
}
