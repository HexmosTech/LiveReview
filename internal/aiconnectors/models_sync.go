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
	"openai":     "gpt-5.5",
	"claude":     "claude-sonnet-4.6",
	"gemini":     "gemini-2.5-flash",
	"deepseek":   "deepseek-v4-flash",
	"cohere":     "command-a",
	"openrouter": "deepseek/deepseek-v4-flash",
	"atlas":      "deepseek-ai/deepseek-v4-flash",
}

// RunOpenRouterModelSyncScheduler starts the dynamic model catalog sync scheduler
func RunOpenRouterModelSyncScheduler(ctx context.Context, db *sql.DB, interval time.Duration) {
	// Initial sync immediately on boot
	if err := SyncOpenRouterModels(ctx, db); err != nil {
		log.Error().Err(err).Msg("OpenRouter models sync failed")
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := SyncOpenRouterModels(ctx, db); err != nil {
					log.Error().Err(err).Msg("OpenRouter models sync failed")
				}
			}
		}
	}()
}

// SyncOpenRouterModels fetches the latest models from OpenRouter and upserts them
func SyncOpenRouterModels(ctx context.Context, db *sql.DB) error {
	log.Info().Msg("OpenRouter models sync started")

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

		// 2. Map ALL eligible synced models to the openrouter provider without any prefix restrictions
		isDefault := defaultProviderModels["openrouter"] == id
		mappedModels = append(mappedModels, MappedModel{
			ModelID:   id, // For OpenRouter, we use the full OpenRouter ID
			Provider:  "openrouter",
			Name:      fmt.Sprintf("OpenRouter: %s", name),
			IsDefault: isDefault,
			Metadata:  model.RawJSON,
		})
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

	log.Info().Int("count", len(mappedModels)).Msg("OpenRouter models sync completed")
	return nil
}

// AtlasModel represents a model returned from the Atlas Cloud models API.
type AtlasModel struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	RawJSON json.RawMessage `json:"-"`
}

// UnmarshalJSON custom unmarshaler to capture the raw JSON of each Atlas model
func (m *AtlasModel) UnmarshalJSON(data []byte) error {
	type Alias AtlasModel
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

// AtlasResponse represents the OpenAI-compatible response wrapper for Atlas Cloud.
type AtlasResponse struct {
	Data []AtlasModel `json:"data"`
}

// RunAtlasModelSyncScheduler starts the dynamic model catalog sync scheduler for Atlas Cloud
func RunAtlasModelSyncScheduler(ctx context.Context, db *sql.DB, interval time.Duration) {
	// Initial sync immediately on boot
	if err := SyncAtlasModels(ctx, db); err != nil {
		log.Error().Err(err).Msg("Atlas Cloud models sync failed")
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := SyncAtlasModels(ctx, db); err != nil {
					log.Error().Err(err).Msg("Atlas Cloud models sync failed")
				}
			}
		}
	}()
}

// SyncAtlasModels fetches the latest models from Atlas Cloud and upserts them
func SyncAtlasModels(ctx context.Context, db *sql.DB) error {
	log.Info().Msg("Atlas Cloud models sync started")

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.atlascloud.ai/v1/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch models from Atlas Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Atlas Cloud API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var atlasResp AtlasResponse
	if err := json.Unmarshal(bodyBytes, &atlasResp); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	var mappedModels []MappedModel

	// Process each model and map it to the atlas provider
	for _, model := range atlasResp.Data {
		id := model.ID
		idLower := strings.ToLower(id)

		// Filter out non-text/media generation models
		if strings.Contains(idLower, "video") || strings.Contains(idLower, "image") ||
			strings.Contains(idLower, "flux") || strings.Contains(idLower, "kling") ||
			strings.Contains(idLower, "stable-diffusion") || strings.Contains(idLower, "sd") ||
			strings.Contains(idLower, "seed") || strings.Contains(idLower, "wan") ||
			strings.Contains(idLower, "audio") || strings.Contains(idLower, "lyria") {
			continue
		}

		name := model.ID // Use model ID as display name or human readable representation
		// If ID contains slashes like 'deepseek-ai/DeepSeek-V3', split and capitalize
		parts := strings.Split(id, "/")
		if len(parts) > 1 {
			name = parts[len(parts)-1]
		}

		isDefault := defaultProviderModels["atlas"] == id
		mappedModels = append(mappedModels, MappedModel{
			ModelID:   id,
			Provider:  "atlas",
			Name:      fmt.Sprintf("Atlas: %s", name),
			IsDefault: isDefault,
			Metadata:  model.RawJSON,
		})
	}

	if len(mappedModels) == 0 {
		return fmt.Errorf("no supported models found in Atlas Cloud API response")
	}

	// Begin transaction to safely upsert models
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate existing atlas models first
	_, err = tx.ExecContext(ctx, "UPDATE ai_models SET is_active = false WHERE provider = 'atlas'")
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

	log.Info().Int("count", len(mappedModels)).Msg("Atlas Cloud models sync completed")
	return nil
}
