package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/livereview/cmd/mrmodel/lib"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	"github.com/livereview/internal/review"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/pkg/models"
	storagetools "github.com/livereview/storage/tools"
)

// ToolDispatchParams encapsulates parameters for executing static analysis tools
// concurrently alongside the AI review within a single review worker job.
type ToolDispatchParams struct {
	OrgID int64
	// RepositoryID is set ONLY for pull/merge request reviews so per-repo UI settings apply;
	// CLI diff and non-PR reviews leave this 0 to keep org + .lrc behavior.
	RepositoryID   int64
	ReviewID       int64
	PlanCode       string
	DiffZipBase64  string
	Truncated      bool
	LocalDiffs     []lib.LocalCodeDiff
	LRCBundle      lrcconfig.Bundle
	Logger         *logging.ReviewLogger
	EventSink      logging.EventSink
	ProviderConfig *review.ProviderConfig
	PostingID      string
}

// ToolsRunHandle coordinates the inline async fan-out of SAST tool goroutines
// started for a single review. Callers must always Cancel/Wait (via defer) so no
// tool goroutines ever outlive the parent River worker invocation.
type ToolsRunHandle struct {
	dispatchedCount int
	cancel          context.CancelFunc
	done            chan struct{}

	mu       sync.Mutex
	comments []*models.ReviewComment
}

func closedToolsRunHandle(dispatchedCount int) *ToolsRunHandle {
	done := make(chan struct{})
	close(done)
	return &ToolsRunHandle{
		dispatchedCount: dispatchedCount,
		done:            done,
	}
}

// DispatchedCount returns the number of enabled tools that matched the diff.
func (h *ToolsRunHandle) DispatchedCount() int {
	if h == nil {
		return 0
	}
	return h.dispatchedCount
}

// Cancel cancels the tool phase context if still running. Safe on nil receiver.
func (h *ToolsRunHandle) Cancel() {
	if h != nil && h.cancel != nil {
		h.cancel()
	}
}

// Wait blocks until every spawned tool goroutine completes (or exits after
// cancellation/panic recovery) and returns all classified tool comments.
func (h *ToolsRunHandle) Wait() []*models.ReviewComment {
	if h == nil || h.done == nil {
		return nil
	}
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.comments) == 0 {
		return nil
	}
	out := make([]*models.ReviewComment, len(h.comments))
	copy(out, h.comments)
	return out
}

func (h *ToolsRunHandle) appendComments(comments []*models.ReviewComment) {
	if len(comments) == 0 {
		return
	}
	h.mu.Lock()
	h.comments = append(h.comments, comments...)
	h.mu.Unlock()
}

type previousToolRunState struct {
	dispatchedTools      map[string]bool
	succeededToolResults map[string]bool
	completedToolBatches map[string][]*models.ReviewComment
}

func loadPreviousToolRunState(ctx context.Context, db *sql.DB, reviewID, orgID int64) previousToolRunState {
	state := previousToolRunState{
		dispatchedTools:      make(map[string]bool),
		succeededToolResults: make(map[string]bool),
		completedToolBatches: make(map[string][]*models.ReviewComment),
	}
	if db == nil || reviewID <= 0 || orgID <= 0 {
		return state
	}

	rows, err := db.QueryContext(ctx, `
		SELECT event_type, COALESCE(batch_id, ''), data
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2
		  AND event_type IN ('tool_dispatch', 'tool_result', 'batch')
		ORDER BY ts ASC, id ASC
	`, reviewID, orgID)
	if err != nil {
		log.Printf("[WARN] StartToolsAsync: failed to load prior tool events for review=%d org=%d: %v", reviewID, orgID, err)
		return state
	}
	defer rows.Close()

	for rows.Next() {
		var eventType, batchID string
		var rawData []byte
		if scanErr := rows.Scan(&eventType, &batchID, &rawData); scanErr != nil {
			log.Printf("[WARN] StartToolsAsync: failed to scan prior event row for review=%d org=%d: %v", reviewID, orgID, scanErr)
			continue
		}
		switch eventType {
		case "tool_dispatch":
			var d struct {
				ToolName string `json:"tool_name"`
			}
			if uErr := json.Unmarshal(rawData, &d); uErr != nil {
				log.Printf("[WARN] StartToolsAsync: malformed tool_dispatch event for review=%d: %v", reviewID, uErr)
			} else if strings.TrimSpace(d.ToolName) != "" {
				state.dispatchedTools[strings.ToLower(strings.TrimSpace(d.ToolName))] = true
			}
		case "tool_result":
			var res struct {
				ToolName      string `json:"tool_name"`
				ExitCode      int    `json:"exit_code"`
				FailureReason string `json:"failure_reason"`
			}
			if uErr := json.Unmarshal(rawData, &res); uErr != nil {
				log.Printf("[WARN] StartToolsAsync: malformed tool_result event for review=%d: %v", reviewID, uErr)
			} else if strings.TrimSpace(res.ToolName) != "" {
				key := strings.ToLower(strings.TrimSpace(res.ToolName))
				// Treat exit_code == -1 or non-empty failure_reason as failure so
				// transient Lambda failures are re-run on River job retry.
				state.succeededToolResults[key] = res.ExitCode != -1 && strings.TrimSpace(res.FailureReason) == ""
			}
		case "batch":
			if !strings.HasPrefix(batchID, "tool-") {
				continue
			}
			var batchData struct {
				Status   string                  `json:"status"`
				Comments []*models.ReviewComment `json:"comments"`
			}
			if uErr := json.Unmarshal(rawData, &batchData); uErr != nil {
				log.Printf("[WARN] StartToolsAsync: malformed batch %q event for review=%d: %v", batchID, reviewID, uErr)
			} else if batchData.Status == "completed" {
				toolNameKey := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(batchID, "tool-")))
				if toolNameKey != "" {
					hydrated := make([]*models.ReviewComment, 0, len(batchData.Comments))
					for _, c := range batchData.Comments {
						if c != nil {
							if c.ToolName == "" {
								c.ToolName = strings.TrimPrefix(batchID, "tool-")
							}
							if c.Source == "" {
								c.Source = "tool"
							}
							hydrated = append(hydrated, c)
						}
					}
					state.completedToolBatches[toolNameKey] = hydrated
				}
			}
		}
	}

	return state
}

// DispatchTools starts inline SAST tools and awaits their completion, returning the count
// of matched tools. Retained for backwards compatibility with callers/tests.
func (jq *JobQueue) DispatchTools(ctx context.Context, p ToolDispatchParams) (int, error) {
	handle, err := jq.StartToolsAsync(ctx, p)
	if err != nil {
		return 0, err
	}
	defer handle.Cancel()
	_ = handle.Wait()
	return handle.DispatchedCount(), nil
}

// StartToolsAsync evaluates repo/org tool settings, matches diff rules, deducts
// credits idempotently, records tool_dispatch events, and launches all active SAST
// Lambda calls concurrently inside goroutines bounded by per-review and global
// semaphores. It never enqueues separate River tool_review jobs.
func (jq *JobQueue) StartToolsAsync(ctx context.Context, p ToolDispatchParams) (*ToolsRunHandle, error) {
	if !apimiddleware.IsCloudMode() {
		return closedToolsRunHandle(0), nil
	}

	if jq == nil || jq.db == nil {
		return nil, fmt.Errorf("database connection not available on JobQueue")
	}

	planCode := license.PlanType(p.PlanCode)
	if !license.IsToolsEligible(planCode) {
		return closedToolsRunHandle(0), nil
	}

	logMsg := func(format string, v ...interface{}) {
		if p.Logger != nil {
			p.Logger.Log(format, v...)
		} else {
			log.Printf(format, v...)
		}
	}

	toolsStore := storagetools.NewToolsStore(jq.db)

	var toolRuleConfigs map[string]*lrcconfig.ToolRuleConfig
	var hasPolicyFile bool
	var parseErr error
	if p.LRCBundle.Files != nil {
		if _, ok := p.LRCBundle.Files["policy/tools.toml"]; ok {
			hasPolicyFile = true
		} else if _, ok := p.LRCBundle.Files["tools.toml"]; ok {
			hasPolicyFile = true
		}

		if hasPolicyFile {
			toolRuleConfigs, parseErr = lrcconfig.ParseToolRuleConfigs(p.LRCBundle)
			if parseErr != nil {
				logMsg("[WARN] Failed to parse tool rule configs from .lrc bundle: %v", parseErr)
			}
		}
	}

	var enabledTools []storagetools.AvailableTool

	// 1. Resolve baseline: org settings by default, overridden when a PR review's repository sets mode='custom'.
	baselineEnabledMap := make(map[string]bool)
	usingRepoCustom := false
	var repoCustomTools map[string]storagetools.OrgToolConfig

	if p.RepositoryID > 0 {
		repoSettings, repoErr := toolsStore.GetRepoToolSettings(ctx, p.OrgID, p.RepositoryID)
		if repoErr != nil {
			if !errors.Is(repoErr, sql.ErrNoRows) {
				logMsg("[WARN] StartToolsAsync: Failed to load repository tool settings (repo=%d org=%d): %v", p.RepositoryID, p.OrgID, repoErr)
			}
		} else if repoSettings.Mode == storagetools.RepoToolModeCustom {
			usingRepoCustom = true
			repoCustomTools = repoSettings.Tools
			for k, cfg := range repoSettings.Tools {
				if cfg.Enabled {
					baselineEnabledMap[strings.ToLower(k)] = true
				}
			}
		}
	}

	var orgTools []storagetools.AvailableTool
	if !usingRepoCustom {
		var orgErr error
		orgTools, orgErr = toolsStore.GetEnabledToolsForOrg(ctx, p.OrgID)
		if orgErr != nil {
			logMsg("[WARN] StartToolsAsync: Failed to fetch enabled tools for org %d: %v", p.OrgID, orgErr)
			if !hasPolicyFile {
				return nil, fmt.Errorf("failed to fetch enabled tools for org %d: %w", p.OrgID, orgErr)
			}
		}
		for _, t := range orgTools {
			baselineEnabledMap[strings.ToLower(t.Name)] = true
		}
	}

	if !hasPolicyFile && !usingRepoCustom {
		enabledTools = orgTools
	} else {
		// 2. Load the full tools catalog so repo UI custom mode and/or .lrc policy can resolve over it
		catalog, catErr := toolsStore.LoadCatalog(ctx)
		if catErr != nil {
			logMsg("[WARN] StartToolsAsync: Failed to load tools catalog for org %d: %v", p.OrgID, catErr)
			return nil, fmt.Errorf("failed to load tools catalog for org %d: %w", p.OrgID, catErr)
		}

		// 3. Precedence: Org Settings -> Repo UI Settings (custom mode, PR reviews only) -> .lrc/policy/tools.toml overlay:
		for _, t := range catalog {
			toolNameLower := strings.ToLower(t.Name)
			shouldRun := baselineEnabledMap[toolNameLower]
			if usingRepoCustom && !shouldRun {
				if cfgByID, ok := repoCustomTools[fmt.Sprintf("%d", t.ID)]; ok {
					shouldRun = cfgByID.Enabled
				}
			}

			var cfg *lrcconfig.ToolRuleConfig
			if usingRepoCustom {
				if repoCfg, ok := storagetools.LookupToolConfig(repoCustomTools, t); ok && len(repoCfg.ConfigJSON) > 0 {
					var rule storagetools.RepoToolRuleConfig
					if umErr := json.Unmarshal(repoCfg.ConfigJSON, &rule); umErr != nil {
						logMsg("[WARN] StartToolsAsync: invalid config_json for tool %q: %v", t.Name, umErr)
					} else if len(rule.Include) > 0 || len(rule.Exclude) > 0 {
						cfg = &lrcconfig.ToolRuleConfig{
							Include: rule.Include,
							Exclude: rule.Exclude,
						}
					}
				}
			}
			if hasPolicyFile && parseErr == nil {
				if c, configured := toolRuleConfigs[toolNameLower]; configured {
					cfg = c
					if c.Enabled != nil {
						shouldRun = *c.Enabled
					} else {
						shouldRun = true
					}
				}
			}

			if !shouldRun {
				continue
			}

			if lrcconfig.ShouldRunToolRuleForDiff(cfg, p.LocalDiffs) {
				enabledTools = append(enabledTools, t)
			} else {
				logMsg("Tool %q skipped: no diff files matched trigger rules", t.Name)
			}
		}
	}

	if len(enabledTools) == 0 {
		return closedToolsRunHandle(0), nil
	}

	breakdown := make([]storagetools.ToolCharge, 0, len(enabledTools))
	for _, t := range enabledTools {
		breakdown = append(breakdown, storagetools.ToolCharge{
			ToolName:   t.Name,
			Multiplier: t.Multiplier,
		})
	}

	creditStore := storagetools.NewCreditStore(jq.db)
	if err := creditStore.DeductCredits(ctx, p.OrgID, p.ReviewID, breakdown, planCode); err != nil {
		logMsg("[WARN] Failed to deduct tool credits for review %d: %v. Proceeding with AI review only.", p.ReviewID, err)
		if p.EventSink != nil {
			_ = p.EventSink.EmitLogEvent(ctx, p.ReviewID, p.OrgID, "warn", fmt.Sprintf("Static analysis skipped due to credit deduction failure: %v", err), "")
		}
		return nil, fmt.Errorf("failed to deduct tool credits: %w", err)
	}

	// Load prior event state so retries hydrate succeeded+classified tools without
	// re-invoking Lambda, while re-running any tool that previously failed or stopped mid-write.
	prevState := loadPreviousToolRunState(ctx, jq.db, p.ReviewID, p.OrgID)

	toolsDispatched := 0
	for _, tool := range enabledTools {
		toolsDispatched++
		key := strings.ToLower(strings.TrimSpace(tool.Name))
		// Only deduplicate inserting another tool_dispatch event row; execution
		// filtering below still re-runs any tool without a succeeded tool_result.
		if prevState.dispatchedTools[key] {
			continue
		}

		dispatchPayload := map[string]interface{}{
			"tool_id":   tool.ID,
			"tool_name": tool.Name,
			"status":    "pending",
		}
		dispatchJSON, marshalErr := json.Marshal(dispatchPayload)
		if marshalErr != nil {
			logMsg("[WARN] StartToolsAsync: failed to marshal tool_dispatch payload for tool=%s: %v", tool.Name, marshalErr)
			continue
		}

		if _, execErr := jq.db.ExecContext(ctx, `
			INSERT INTO public.review_events (review_id, org_id, event_type, level, data)
			VALUES ($1, $2, 'tool_dispatch', 'info', $3)`,
			p.ReviewID, p.OrgID, dispatchJSON); execErr != nil {
			logMsg("[WARN] StartToolsAsync: failed to insert tool_dispatch event for tool=%s review=%d: %v", tool.Name, p.ReviewID, execErr)
		}
	}

	// Persist tools_dispatched count directly into review metadata
	if _, err := jq.db.ExecContext(ctx, `
		UPDATE public.reviews
		SET metadata = jsonb_set(
			COALESCE(metadata, '{}'::jsonb),
			'{tools_dispatched}',
			to_jsonb($1::int)
		)
		WHERE id = $2
	`, toolsDispatched, p.ReviewID); err != nil {
		logMsg("[WARN] StartToolsAsync: failed to persist tools_dispatched for review=%d: %v", p.ReviewID, err)
	}

	payloadBytes, marshalErr := json.Marshal(map[string]interface{}{
		"zip_file": p.DiffZipBase64,
	})
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to marshal shared Lambda payload: %w", marshalErr)
	}

	eventSink := p.EventSink
	if eventSink == nil {
		eventSink = reviewprocessor.NewDatabaseEventSink(jq.db)
	}

	toolsCfg, globalSem := jq.ensureToolsRuntime()
	perReviewCap := max(toolsCfg.MaxConcurrentPerReview, 1)
	perReviewSem := make(chan struct{}, perReviewCap)
	awsCfg, region, accountID := loadAWSLambdaConfig(jq.db)

	toolsToRun := make([]storagetools.AvailableTool, 0, len(enabledTools))
	hydratedTotal := 0
	var initialHydrated []*models.ReviewComment
	for _, tool := range enabledTools {
		key := strings.ToLower(strings.TrimSpace(tool.Name))
		if prevState.succeededToolResults[key] {
			if cachedComments, ok := prevState.completedToolBatches[key]; ok {
				initialHydrated = append(initialHydrated, cachedComments...)
				hydratedTotal++
				continue
			}
		}
		toolsToRun = append(toolsToRun, tool)
	}

	if hydratedTotal > 0 {
		log.Printf("[INFO] StartToolsAsync: review=%d hydrated %d already-completed tool(s) (%d comments) on retry", p.ReviewID, hydratedTotal, len(initialHydrated))
	}

	if len(toolsToRun) == 0 {
		h := closedToolsRunHandle(toolsDispatched)
		h.comments = initialHydrated
		return h, nil
	}

	phaseTimeout := toolsCfg.ToolPhaseTimeout
	if phaseTimeout <= 0 {
		phaseTimeout = DefaultToolPhaseTimeout
	}
	toolPhaseCtx, cancel := context.WithTimeout(ctx, phaseTimeout)
	done := make(chan struct{})
	handle := &ToolsRunHandle{
		dispatchedCount: toolsDispatched,
		cancel:          cancel,
		done:            done,
		comments:        initialHydrated,
	}

	db := jq.db
	invokeOverride := jq.toolInvokeOverride
	classifyOverride := jq.toolClassifyOverride
	planCodeStr := string(planCode)
	orgID := p.OrgID
	reviewID := p.ReviewID
	truncated := p.Truncated

	go func() {
		defer close(done)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] StartToolsAsync coordinator panic recovered for review=%d: %v", reviewID, r)
			}
		}()

		var wg sync.WaitGroup
		for _, tool := range toolsToRun {
			wg.Add(1)
			go func(t storagetools.AvailableTool) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[ERROR] StartToolsAsync goroutine panic recovered for tool=%s review=%d: %v", t.Name, reviewID, r)
						recordSyntheticToolFailure(db, reviewID, orgID, t.ID, t.Name, eventSink, fmt.Sprintf("internal panic: %v", r))
					}
				}()

				// Always acquire per-review capacity FIRST, then process-wide global capacity,
				// so goroutines waiting on per-review limits do not hold global slots idle.
				select {
				case perReviewSem <- struct{}{}:
					defer func() { <-perReviewSem }()
				case <-toolPhaseCtx.Done():
					recordSyntheticToolFailure(db, reviewID, orgID, t.ID, t.Name, eventSink, fmt.Sprintf("tool execution aborted waiting for per-review slot: %v", toolPhaseCtx.Err()))
					return
				}

				select {
				case globalSem <- struct{}{}:
					defer func() { <-globalSem }()
				case <-toolPhaseCtx.Done():
					recordSyntheticToolFailure(db, reviewID, orgID, t.ID, t.Name, eventSink, fmt.Sprintf("tool execution aborted waiting for global concurrency slot: %v", toolPhaseCtx.Err()))
					return
				}

				comments, ok, execErr := executeSingleToolInvocation(
					toolPhaseCtx,
					db,
					awsCfg,
					region,
					accountID,
					singleToolExecParams{
						ReviewID:     reviewID,
						OrgID:        orgID,
						PlanCode:     planCodeStr,
						ToolID:       t.ID,
						ToolName:     t.Name,
						ToolCategory: t.Category,
						LambdaARN:    t.LambdaARN,
						PayloadBytes: payloadBytes,
						Truncated:    truncated,
						InvokeFn:     invokeOverride,
						ClassifyFn:   classifyOverride,
					},
					toolsCfg,
					eventSink,
				)
				if execErr != nil {
					log.Printf("[WARN] StartToolsAsync: tool=%s review=%d execution error: %v", t.Name, reviewID, execErr)
					recordSyntheticToolFailure(db, reviewID, orgID, t.ID, t.Name, eventSink, execErr.Error())
					return
				}
				if ok && len(comments) > 0 {
					handle.appendComments(comments)
				}
			}(tool)
		}

		wg.Wait()
	}()

	return handle, nil
}
