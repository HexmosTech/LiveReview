package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	reviewpkg "github.com/livereview/internal/review"
)

type ReviewAccountingStageResponse struct {
	Stage          string   `json:"stage"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	PricingVersion string   `json:"pricingVersion,omitempty"`
	InputTokens    *int64   `json:"inputTokens,omitempty"`
	OutputTokens   *int64   `json:"outputTokens,omitempty"`
	CostUSD        *float64 `json:"costUsd,omitempty"`
}

func buildReviewAIMetadata(request *reviewpkg.ReviewRequest, result *reviewpkg.ReviewResult) map[string]interface{} {
	if request == nil || result == nil {
		return map[string]interface{}{}
	}

	meta := map[string]interface{}{
		"helper_enabled": request.HelperEnabled,
		"helper_mode":    strings.TrimSpace(request.HelperMode),
	}

	stages := make([]map[string]interface{}, 0, 2)
	if result.LeaderUsage != nil {
		stages = append(stages, stageUsageToMetadata(result.LeaderUsage))
	}
	if result.HelperUsage != nil {
		stages = append(stages, stageUsageToMetadata(result.HelperUsage))
	}
	if len(stages) > 0 {
		meta["stage_breakdown"] = stages
	}

	for k, v := range aiExecutionMetadataForRole("leader", request.AI.Config) {
		meta[k] = v
	}
	if request.HelperAI != nil {
		for k, v := range aiExecutionMetadataForRole("helper", request.HelperAI.Config) {
			meta[k] = v
		}
	}

	return meta
}

func stageUsageToMetadata(usage *reviewpkg.AIStageUsage) map[string]interface{} {
	meta := map[string]interface{}{
		"stage":           usage.Stage,
		"provider":        usage.Provider,
		"model":           usage.Model,
		"pricing_version": usage.PricingVersion,
	}
	if usage.InputTokens != nil {
		meta["input_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		meta["output_tokens"] = *usage.OutputTokens
	}
	if usage.CostUSD != nil {
		meta["cost_usd"] = *usage.CostUSD
	}
	return meta
}

func aiExecutionMetadataForRole(role string, config map[string]interface{}) map[string]interface{} {
	meta := map[string]interface{}{}
	if len(config) == 0 {
		return meta
	}
	prefix := strings.TrimSpace(role)
	if prefix == "" {
		prefix = "ai"
	} else {
		prefix = prefix + "_ai"
	}
	if mode, ok := config["ai_execution_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		meta[prefix+"_execution_mode"] = strings.TrimSpace(mode)
	}
	if source, ok := config["ai_execution_source"].(string); ok && strings.TrimSpace(source) != "" {
		meta[prefix+"_execution_source"] = strings.TrimSpace(source)
	}
	if provider, ok := config["provider_name"].(string); ok && strings.TrimSpace(provider) != "" {
		meta[prefix+"_provider_name"] = strings.TrimSpace(provider)
	}
	if connectorName, ok := config["connector_name"].(string); ok && strings.TrimSpace(connectorName) != "" {
		meta[prefix+"_connector_name"] = strings.TrimSpace(connectorName)
	}
	return meta
}

func loadReviewMetadata(ctx context.Context, db *sql.DB, orgID int64, reviewID int64) (map[string]interface{}, error) {
	var raw []byte
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(metadata, '{}') FROM reviews WHERE id = $1 AND org_id = $2`, reviewID, orgID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load review metadata: %w", err)
	}
	meta := map[string]interface{}{}
	if len(raw) == 0 {
		return meta, nil
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("decode review metadata: %w", err)
	}
	return meta, nil
}

func parseReviewAIStageBreakdown(meta map[string]interface{}) []ReviewAccountingStageResponse {
	rawStages, ok := meta["stage_breakdown"]
	if !ok {
		return nil
	}
	stageList, ok := rawStages.([]interface{})
	if !ok {
		return nil
	}
	result := make([]ReviewAccountingStageResponse, 0, len(stageList))
	for _, rawStage := range stageList {
		stageMap, ok := rawStage.(map[string]interface{})
		if !ok {
			continue
		}
		stage := ReviewAccountingStageResponse{
			Stage:          readStringValue(stageMap, "stage"),
			Provider:       readStringValue(stageMap, "provider"),
			Model:          readStringValue(stageMap, "model"),
			PricingVersion: readStringValue(stageMap, "pricing_version"),
			InputTokens:    readInt64Value(stageMap, "input_tokens"),
			OutputTokens:   readInt64Value(stageMap, "output_tokens"),
			CostUSD:        readFloat64Value(stageMap, "cost_usd"),
		}
		if strings.TrimSpace(stage.Stage) == "" {
			continue
		}
		result = append(result, stage)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func readStringValue(values map[string]interface{}, key string) string {
	v, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func readInt64Value(values map[string]interface{}, key string) *int64 {
	switch value := values[key].(type) {
	case float64:
		v := int64(value)
		return &v
	case int64:
		v := value
		return &v
	case int:
		v := int64(value)
		return &v
	default:
		return nil
	}
}

func readFloat64Value(values map[string]interface{}, key string) *float64 {
	switch value := values[key].(type) {
	case float64:
		v := value
		return &v
	case int64:
		v := float64(value)
		return &v
	case int:
		v := float64(value)
		return &v
	default:
		return nil
	}
}
