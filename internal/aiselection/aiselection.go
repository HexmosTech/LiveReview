// Package aiselection resolves an org's configured AI connector(s) into a review.AIConfig,
// independent of the HTTP server. It exists so both the API layer (manual/webhook review
// triggers) and background workers (e.g. scheduled reviews) can resolve "which AI model
// should review this org's code" the same way, without internal/jobqueue importing
// internal/api (which would create an import cycle).
package aiselection

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/review"
	storageaiconnectors "github.com/livereview/storage/aiconnectors"
)

// Selection holds the resolved leader (and optional helper) AI configuration for a review.
type Selection struct {
	Leader        review.AIConfig
	Helper        *review.AIConfig
	HelperEnabled bool
	HelperMode    string
}

// GetReviewAISelection resolves the org's configured Leader (and, if enabled, Helper) AI
// connector into review.AIConfig, following the same plan-based selection rules used for
// manual/webhook-triggered reviews.
func GetReviewAISelection(ctx context.Context, db *sql.DB, orgID int64, planCode license.PlanType) (*Selection, error) {
	storage := aiconnectors.NewStorage(db)
	leaderConnectors, err := storage.GetConnectorsByRole(ctx, orgID, storageaiconnectors.AIConnectorRoleLeader)
	if err != nil {
		return nil, fmt.Errorf("failed to get Leader AI connectors: %w", err)
	}

	leaderConfig, err := selectLeaderAIConfig(ctx, db, leaderConnectors, planCode)
	if err != nil {
		return nil, err
	}

	settingsStore := storageaiconnectors.NewReviewAISettingsStore(db)
	settings, err := settingsStore.GetByOrgID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get review AI settings: %w", err)
	}

	selection := &Selection{
		Leader:        leaderConfig,
		HelperEnabled: settings.HelperEnabled,
		HelperMode:    settings.HelperMode,
	}

	if !settings.HelperEnabled {
		return selection, nil
	}

	helperConnectors, err := storage.GetConnectorsByRole(ctx, orgID, storageaiconnectors.AIConnectorRoleHelper)
	if err != nil {
		return nil, fmt.Errorf("failed to get Helper AI connectors: %w", err)
	}
	if len(helperConnectors) == 0 {
		// Adaptive Review is on but no helper connector is configured yet.
		// Degrade to leader-only instead of failing the review.
		selection.HelperEnabled = false
		return selection, nil
	}
	helperConfig, err := selectHelperAIConfig(ctx, db, helperConnectors)
	if err != nil {
		return nil, err
	}
	selection.Helper = &helperConfig

	return selection, nil
}

func selectLeaderAIConfig(ctx context.Context, db *sql.DB, connectors []*aiconnectors.ConnectorRecord, planCode license.PlanType) (review.AIConfig, error) {
	if planCode == "" {
		planCode = license.PlanFree30K
	}

	if planCode == license.PlanFree30K {
		var byokConnector *aiconnectors.ConnectorRecord
		for _, c := range connectors {
			if c.ProviderName != aidefault.ProviderName {
				byokConnector = c
				break
			}
		}
		if byokConnector == nil {
			return review.AIConfig{}, fmt.Errorf("the Free plan requires you to configure your own LLM API key (BYOK) for your organization.")
		}
		return buildBYOKAIConfig(ctx, db, byokConnector, "byok_required")
	}

	if planCode == license.PlanTeam32USD {
		if len(connectors) > 0 {
			connector := connectors[0]
			if connector.ProviderName == aidefault.ProviderName {
				return buildDefaultAIConfig(ctx, db, connector)
			}
			return buildBYOKAIConfig(ctx, db, connector, "byok_override")
		}
		return buildHostedAutoAIConfig(ctx, db)
	}

	if len(connectors) > 0 {
		return buildBYOKAIConfig(ctx, db, connectors[0], "byok_optional")
	}
	return buildHostedAutoAIConfig(ctx, db)
}

func selectHelperAIConfig(ctx context.Context, db *sql.DB, connectors []*aiconnectors.ConnectorRecord) (review.AIConfig, error) {
	if len(connectors) == 0 {
		// Defensive: callers should already have routed around this via the
		// empty-helperConnectors check in GetReviewAISelection.
		return review.AIConfig{}, fmt.Errorf("helper model is enabled but no Helper AI connector is configured")
	}
	connector := connectors[0]
	if connector.ProviderName == aidefault.ProviderName {
		return buildDefaultAIConfig(ctx, db, connector)
	}
	return buildBYOKAIConfig(ctx, db, connector, "helper_connector")
}

func buildDefaultAIConfig(ctx context.Context, db *sql.DB, record *aiconnectors.ConnectorRecord) (review.AIConfig, error) {
	tier := record.GetSelectedModel()
	if tier == "" {
		tier = "default"
	}
	options, err := aidefault.ResolveConnectorOptions(ctx, db, tier)
	if err != nil {
		return review.AIConfig{}, fmt.Errorf("failed to resolve managed AI options for tier %s: %w", tier, err)
	}

	configMap := map[string]interface{}{
		"provider_name":       record.ProviderName,
		"ai_provider_type":    string(options.Provider),
		"connector_name":      record.ConnectorName,
		"display_order":       record.DisplayOrder,
		"ai_execution_mode":   "managed_default",
		"ai_execution_source": "internal",
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      options.APIKey,
		Model:       options.ModelConfig.Model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func buildBYOKAIConfig(ctx context.Context, db *sql.DB, connector *aiconnectors.ConnectorRecord, executionMode string) (review.AIConfig, error) {
	if connector == nil {
		return review.AIConfig{}, fmt.Errorf("connector is required for BYOK mode")
	}

	aiType := "langchain" // We always use langchain as the AI type

	var model string
	if connector.SelectedModel.Valid && connector.SelectedModel.String != "" {
		model = connector.SelectedModel.String
	} else {
		storage := aiconnectors.NewStorage(db)
		model = storage.GetDefaultModel(ctx, connector.Provider)
		if model == "" {
			return review.AIConfig{}, fmt.Errorf("no active default model configured in database for provider %s", connector.ProviderName)
		}
	}

	configMap := map[string]interface{}{
		"provider_name":       connector.ProviderName,
		"connector_name":      connector.ConnectorName,
		"display_order":       connector.DisplayOrder,
		"ai_execution_mode":   executionMode,
		"ai_execution_source": "connector",
	}

	if connector.GCPProjectID.Valid && connector.GCPProjectID.String != "" {
		configMap["gcp_project_id"] = connector.GCPProjectID.String
	}
	if connector.GCPLocation.Valid && connector.GCPLocation.String != "" {
		configMap["gcp_location"] = connector.GCPLocation.String
	}
	if connector.AWSAccessKeyID.Valid && connector.AWSAccessKeyID.String != "" {
		configMap["aws_access_key_id"] = connector.AWSAccessKeyID.String
	}
	if connector.AWSRegion.Valid && connector.AWSRegion.String != "" {
		configMap["aws_region"] = connector.AWSRegion.String
	}

	baseURL := ""
	if connector.BaseURL.Valid && connector.BaseURL.String != "" {
		baseURL = connector.BaseURL.String
	}
	baseURL = aiconnectors.ResolveBaseURLForProviderName(connector.ProviderName, baseURL)

	if baseURL != "" {
		configMap["base_url"] = baseURL
	}

	return review.AIConfig{
		Type:        aiType,
		APIKey:      connector.ApiKey,
		Model:       model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func buildHostedAutoAIConfig(ctx context.Context, db *sql.DB) (review.AIConfig, error) {
	providerName := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_PROVIDER"))
	if providerName == "" {
		providerName = "gemini"
	}

	model := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_MODEL"))
	if model == "" {
		storage := aiconnectors.NewStorage(db)
		model = storage.GetDefaultModel(ctx, aiconnectors.Provider(providerName))
		if model == "" {
			return review.AIConfig{}, fmt.Errorf("no active default model configured in database for hosted provider %s", providerName)
		}
	}

	apiKey := ""
	switch providerName {
	case "gemini":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_GEMINI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		}
	case "openai":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENAI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	case "deepseek":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_DEEPSEEK_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
		}
	case "openrouter":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENROUTER_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
		}
	case "claude":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_CLAUDE_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
	case "ollama":
		// Ollama does not require API key.
	default:
		return review.AIConfig{}, fmt.Errorf("unsupported hosted auto provider: %s", providerName)
	}

	if providerName != "ollama" && apiKey == "" {
		return review.AIConfig{}, fmt.Errorf("hosted auto provider '%s' is configured without API key; set LIVEREVIEW_HOSTED_*_API_KEY", providerName)
	}

	configMap := map[string]interface{}{
		"provider_name":       providerName,
		"connector_name":      "Hosted Auto",
		"display_order":       -1,
		"ai_execution_mode":   "hosted_auto",
		"ai_execution_source": "platform",
	}

	baseURL := aiconnectors.ResolveBaseURLForProviderName(providerName, strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_BASE_URL")))
	if baseURL != "" {
		configMap["base_url"] = baseURL
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      apiKey,
		Model:       model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}
