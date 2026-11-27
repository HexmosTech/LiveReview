package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/cmd/mrmodel/lib"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	"github.com/livereview/internal/review"
	storagetools "github.com/livereview/storage/tools"
)

// ToolDispatchParams encapsulates parameters for dispatching static analysis tools.
type ToolDispatchParams struct {
	OrgID int64
	// RepositoryID is set ONLY for pull/merge request reviews so per-repo UI settings apply;
	// CLI diff and non-PR reviews leave this 0 to keep org + .lrc behavior.
	RepositoryID   int64
	ReviewID       int64
	PlanCode       string
	DiffZipBase64  string
	LocalDiffs     []lib.LocalCodeDiff
	LRCBundle      lrcconfig.Bundle
	Logger         *logging.ReviewLogger
	EventSink      logging.EventSink
	ProviderConfig *review.ProviderConfig
	PostingID      string
}

// DispatchTools inspects repository config and org tools, evaluates diff matchers,
// deducts credits, logs dispatch events, enqueues ToolReviewJob for each tool,
// and records tools_dispatched in review metadata.
func (jq *JobQueue) DispatchTools(ctx context.Context, p ToolDispatchParams) (int, error) {
	if !apimiddleware.IsCloudMode() {
		return 0, nil
	}

	if jq == nil || jq.db == nil {
		return 0, fmt.Errorf("database connection not available on JobQueue")
	}

	planCode := license.PlanType(p.PlanCode)
	if !license.IsToolsEligible(planCode) {
		return 0, nil
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
				logMsg("[WARN] DispatchTools: Failed to load repository tool settings (repo=%d org=%d): %v", p.RepositoryID, p.OrgID, repoErr)
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
			logMsg("[WARN] DispatchTools: Failed to fetch enabled tools for org %d: %v", p.OrgID, orgErr)
			if !hasPolicyFile {
				return 0, fmt.Errorf("failed to fetch enabled tools for org %d: %w", p.OrgID, orgErr)
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
			logMsg("[WARN] DispatchTools: Failed to load tools catalog for org %d: %v", p.OrgID, catErr)
			return 0, fmt.Errorf("failed to load tools catalog for org %d: %w", p.OrgID, catErr)
		}

		// 3. Precedence: Org Settings -> Repo UI Settings (custom mode, PR reviews only) -> .lrc/policy/tools.toml overlay:
		//   * Tool with enabled = true (or configured without enabled = false) runs even if disabled in baseline.
		//   * Tool with enabled = false does NOT run even if enabled in baseline.
		//   * Tool not mentioned in tools.toml inherits the baseline (repo custom when active, else org settings).
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
						logMsg("[WARN] DispatchTools: invalid config_json for tool %q: %v", t.Name, umErr)
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
		return 0, nil
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
		return 0, fmt.Errorf("failed to deduct tool credits: %w", err)
	}

	for _, tool := range enabledTools {
		toolJobArgs := ToolReviewJobArgs{
			ReviewID:       p.ReviewID,
			OrgID:          p.OrgID,
			PlanCode:       string(planCode),
			ToolID:         tool.ID,
			ToolName:       tool.Name,
			ToolCategory:   tool.Category,
			LambdaARN:      tool.LambdaARN,
			DiffZipBase64:  p.DiffZipBase64,
			ProviderConfig: p.ProviderConfig,
			PostingID:      p.PostingID,
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
