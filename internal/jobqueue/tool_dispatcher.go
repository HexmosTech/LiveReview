package jobqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	storagetools "github.com/livereview/storage/tools"
)

// ToolDispatchParams encapsulates parameters for dispatching static analysis tools.
type ToolDispatchParams struct {
	OrgID         int64
	ReviewID      int64
	PlanCode      string
	DiffZipBase64 string
	LocalDiffs   []lib.LocalCodeDiff
	LRCBundle     lrcconfig.Bundle
	Logger        *logging.ReviewLogger
	EventSink     logging.EventSink
}

// DispatchTools inspects repository config and org tools, evaluates diff matchers,
// deducts credits, logs dispatch events, enqueues ToolReviewJob for each tool,
// and records tools_dispatched in review metadata.
func (jq *JobQueue) DispatchTools(ctx context.Context, p ToolDispatchParams) (int, error) {
	if jq == nil || jq.db == nil {
		return 0, fmt.Errorf("database connection not available on JobQueue")
	}

	logMsg := func(format string, v ...interface{}) {
		if p.Logger != nil {
			p.Logger.Log(format, v...)
		} else {
			log.Printf(format, v...)
		}
	}

	var toolsDispatched int
	toolsStore := storagetools.NewToolsStore(jq.db)
	enabledTools, err := toolsStore.GetEnabledToolsForOrg(ctx, p.OrgID)
	if err != nil {
		logMsg("[WARN] DispatchTools: Failed to fetch enabled tools for org %d: %v", p.OrgID, err)
	}

	var toolRuleConfigs map[string]*lrcconfig.ToolRuleConfig
	if p.LRCBundle.Files != nil {
		var parseErr error
		toolRuleConfigs, parseErr = lrcconfig.ParseToolRuleConfigs(p.LRCBundle)
		if parseErr != nil {
			logMsg("[WARN] Failed to parse tool rule configs from .lrc bundle: %v", parseErr)
		}
	}

	existingMap := make(map[string]bool)
	for _, t := range enabledTools {
		existingMap[strings.ToLower(t.Name)] = true
	}

	for toolName, cfg := range toolRuleConfigs {
		if cfg != nil && cfg.Enabled != nil && *cfg.Enabled && !existingMap[toolName] {
			t, err := toolsStore.GetAvailableToolByName(ctx, toolName)
			if err != nil {
				logMsg("[WARN] Failed to lookup available tool %q: %v", toolName, err)
				continue
			}
			if t != nil {
				enabledTools = append(enabledTools, *t)
				existingMap[toolName] = true
				logMsg("Repo-level config (.lrc/policy/tools.toml) enabled tool %q", t.Name)
			}
		}
	}

	var filteredTools []storagetools.AvailableTool
	for _, t := range enabledTools {
		toolNameLower := strings.ToLower(t.Name)
		cfg := toolRuleConfigs[toolNameLower]

		if lrcconfig.ShouldRunToolRuleForDiff(cfg, p.LocalDiffs) {
			filteredTools = append(filteredTools, t)
		} else {
			logMsg("Tool %q skipped: no diff files matched trigger rules (.lrc/policy/tools.toml)", t.Name)
		}
	}
	enabledTools = filteredTools

	if len(enabledTools) == 0 {
		return 0, nil
	}

	var totalMultiplier float64
	for _, t := range enabledTools {
		totalMultiplier += t.Multiplier
	}

	planCode := license.PlanType(p.PlanCode)
	if planCode == "" {
		planCode = license.PlanFree30K
	}

	creditStore := storagetools.NewCreditStore(jq.db)
	if err := creditStore.DeductCredits(ctx, p.OrgID, p.ReviewID, totalMultiplier, planCode); err != nil {
		logMsg("[WARN] Failed to deduct tool credits for review %d: %v. Proceeding with AI review only.", p.ReviewID, err)
		if p.EventSink != nil {
			_ = p.EventSink.EmitLogEvent(ctx, p.ReviewID, p.OrgID, "warn", fmt.Sprintf("Static analysis skipped due to credit deduction failure: %v", err), "")
		}
		return 0, fmt.Errorf("failed to deduct tool credits: %w", err)
	}

	for _, tool := range enabledTools {
		toolJobArgs := ToolReviewJobArgs{
			ReviewID:      p.ReviewID,
			OrgID:         p.OrgID,
			PlanCode:      string(planCode),
			ToolID:        tool.ID,
			ToolName:      tool.Name,
			LambdaARN:     tool.LambdaARN,
			DiffZipBase64: p.DiffZipBase64,
		}
		if err := jq.QueueToolReviewJob(ctx, toolJobArgs); err != nil {
			logMsg("[ERROR] Failed to queue tool job for %s: %v", tool.Name, err)
			continue
		}

		toolsDispatched++
		logMsg("Enqueued tool job for %s (Lambda ARN: %s)", tool.Name, tool.LambdaARN)

		dispatchPayload := map[string]interface{}{
			"tool_id":   tool.ID,
			"tool_name": tool.Name,
			"status":    "pending",
		}
		dispatchJSON, marshalErr := json.Marshal(dispatchPayload)
		if marshalErr != nil {
			logMsg("[WARN] DispatchTools: failed to marshal tool_dispatch payload for tool=%s: %v", tool.Name, marshalErr)
			continue
		}

		if _, execErr := jq.db.ExecContext(ctx, `
			INSERT INTO public.review_events (review_id, org_id, event_type, level, data)
			VALUES ($1, $2, 'tool_dispatch', 'info', $3)`,
			p.ReviewID, p.OrgID, dispatchJSON); execErr != nil {
			logMsg("[WARN] DispatchTools: failed to insert tool_dispatch event for tool=%s review=%d: %v", tool.Name, p.ReviewID, execErr)
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
		logMsg("[WARN] DispatchTools: failed to persist tools_dispatched for review=%d: %v", p.ReviewID, err)
	}

	return toolsDispatched, nil
}
