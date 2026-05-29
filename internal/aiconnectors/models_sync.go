package aiconnectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// OpenRouterArchitecture represents the model modality information in OpenRouter response
type OpenRouterArchitecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// OpenRouterModel represents a model in the OpenRouter API response
type OpenRouterModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Architecture  OpenRouterArchitecture `json:"architecture"`
	ContextLength int                    `json:"context_length"`
	RawJSON       json.RawMessage        `json:"-"`
}

// UnmarshalJSON custom unmarshaler to capture the raw JSON of each model
func (m *OpenRouterModel) UnmarshalJSON(data []byte) error {
	type Alias OpenRouterModel
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.RawJSON = json.RawMessage(data)
	return nil
}

// OpenRouterResponse represents the OpenRouter API response wrapper
type OpenRouterResponse struct {
	Data []OpenRouterModel `json:"data"`
}

// MappedModel represents a parsed model ready to be stored in DB
type MappedModel struct {
	ModelID   string
	Provider  string
	Name      string
	IsDefault bool
	Metadata  json.RawMessage
}

// Default models mapped by provider for setting default selection flag in DB
var defaultProviderModels = map[string]string{
	"openai":     "gpt-mini-latest",
	"claude":     "claude-sonnet-4.6",
	"gemini":     "gemini-2.5-flash",
	"deepseek":   "deepseek-v4-flash",
	"cohere":     "command-a",
	"openrouter": "deepseek/deepseek-v4-flash",
}

// RunOpenRouterModelSyncScheduler starts the dynamic model catalog sync scheduler
func RunOpenRouterModelSyncScheduler(ctx context.Context, db *sql.DB, interval time.Duration) {
	log.Info().Msg("Starting dynamic AI models sync scheduler")

	// Initial sync immediately on boot
	if err := SyncOpenRouterModels(ctx, db); err != nil {
		log.Error().Err(err).Msg("Initial dynamic AI models sync failed")
	} else {
		log.Info().Msg("Initial dynamic AI models sync successful")
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("Dynamic AI models sync scheduler stopped")
				return
			case <-ticker.C:
				log.Info().Msg("Running periodic dynamic AI models sync")
				if err := SyncOpenRouterModels(ctx, db); err != nil {
					log.Error().Err(err).Msg("Periodic dynamic AI models sync failed")
				} else {
					log.Info().Msg("Periodic dynamic AI models sync successful")
				}
			}
		}
	}()
}

// SyncOpenRouterModels fetches the latest models from OpenRouter and upserts them
func SyncOpenRouterModels(ctx context.Context, db *sql.DB) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch models from OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenRouter API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var openrouterResp OpenRouterResponse
	if err := json.Unmarshal(bodyBytes, &openrouterResp); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	var mappedModels []MappedModel

	// Process each model and map it to our supported providers
	for _, model := range openrouterResp.Data {
		id := model.ID

		// Exclude models with context windows smaller than 32k tokens
		if model.ContextLength < 32768 {
			continue
		}

		idLower := strings.ToLower(id)
		nameLower := strings.ToLower(model.Name)

		// Exclude models that contain image, audio, or lyria in their ID or Name
		if strings.Contains(idLower, "image") || strings.Contains(nameLower, "image") ||
			strings.Contains(idLower, "audio") || strings.Contains(nameLower, "audio") ||
			strings.Contains(idLower, "lyria") || strings.Contains(nameLower, "lyria") {
			continue
		}

		// Ensure the model supports "text" input modalities if list is present
		if len(model.Architecture.InputModalities) > 0 {
			hasText := false
			for _, modality := range model.Architecture.InputModalities {
				if modality == "text" {
					hasText = true
					break
				}
			}
			if !hasText {
				continue
			}
		}

		// Ensure the model supports "text" output modalities if list is present
		if len(model.Architecture.OutputModalities) > 0 {
			hasText := false
			for _, modality := range model.Architecture.OutputModalities {
				if modality == "text" {
					hasText = true
					break
				}
			}
			if !hasText {
				continue
			}
		}

		name := model.Name

		var provider string
		var cleanModelID string

		switch {
		case strings.HasPrefix(id, "openai/"):
			provider = "openai"
			cleanModelID = strings.TrimPrefix(id, "openai/")
		case strings.HasPrefix(id, "google/"):
			provider = "gemini"
			cleanModelID = strings.TrimPrefix(id, "google/")
		case strings.HasPrefix(id, "anthropic/"):
			provider = "claude"
			cleanModelID = strings.TrimPrefix(id, "anthropic/")
		case strings.HasPrefix(id, "deepseek/"):
			provider = "deepseek"
			cleanModelID = strings.TrimPrefix(id, "deepseek/")
		case strings.HasPrefix(id, "cohere/"):
			provider = "cohere"
			cleanModelID = strings.TrimPrefix(id, "cohere/")
		}

		// 1. If mapped to a native provider, store it for that provider
		if provider != "" {
			isDefault := defaultProviderModels[provider] == cleanModelID
			mappedModels = append(mappedModels, MappedModel{
				ModelID:   cleanModelID,
				Provider:  provider,
				Name:      name,
				IsDefault: isDefault,
				Metadata:  model.RawJSON,
			})
		}

		// 2. Also map supported models (and free/popular models) to the openrouter provider
		// We include any openai, gemini, claude, deepseek, or cohere models in the OpenRouter options.
		if provider != "" || id == "deepseek/deepseek-r1-0528:free" {
			isDefault := defaultProviderModels["openrouter"] == id
			mappedModels = append(mappedModels, MappedModel{
				ModelID:   id, // For OpenRouter, we use the full OpenRouter ID
				Provider:  "openrouter",
				Name:      fmt.Sprintf("OpenRouter: %s", name),
				IsDefault: isDefault,
				Metadata:  model.RawJSON,
			})
		}
	}

	if len(mappedModels) == 0 {
		return fmt.Errorf("no supported models found in OpenRouter API response")
	}

	// Begin transaction to safely upsert models
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate existing models first (soft-delete approach so we don't break existing connectors)
	_, err = tx.ExecContext(ctx, "UPDATE ai_models SET is_active = false")
	if err != nil {
		return fmt.Errorf("failed to reset model states: %w", err)
	}

	upsertQuery := `
		INSERT INTO ai_models (model_id, provider, name, is_active, is_default, metadata, updated_at)
		VALUES ($1, $2, $3, true, $4, $5, NOW())
		ON CONFLICT (model_id) DO UPDATE
		SET name = EXCLUDED.name,
		    provider = EXCLUDED.provider,
		    is_active = true,
		    is_default = EXCLUDED.is_default,
		    metadata = EXCLUDED.metadata,
		    updated_at = NOW()
	`

	stmt, err := tx.PrepareContext(ctx, upsertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare upsert query: %w", err)
	}
	defer stmt.Close()

	for _, model := range mappedModels {
		_, err := stmt.ExecContext(ctx, model.ModelID, model.Provider, model.Name, model.IsDefault, model.Metadata)
		if err != nil {
			log.Warn().Err(err).Str("model_id", model.ModelID).Msg("Failed to upsert model")
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Info().Int("count", len(mappedModels)).Msg("Dynamic AI models catalog synchronized successfully")
	return nil
}
