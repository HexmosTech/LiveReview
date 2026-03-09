package aiconnectors

import (
	"database/sql"
	"testing"
)

func TestGetDefaultModelOpenAI(t *testing.T) {
	if got := GetDefaultModel(ProviderOpenAI); got != "o4-mini" {
		t.Fatalf("expected OpenAI default model o4-mini, got %q", got)
	}
}

func TestGetProviderModelsOpenAIIncludesO4MiniFirst(t *testing.T) {
	models := GetProviderModels(ProviderOpenAI)
	if len(models) == 0 {
		t.Fatal("expected OpenAI model list to be non-empty")
	}
	if models[0] != "o4-mini" {
		t.Fatalf("expected first OpenAI model to be o4-mini, got %q", models[0])
	}
}

func TestGetDefaultModelClaude(t *testing.T) {
	if got := GetDefaultModel(ProviderClaude); got != "claude-haiku-4-5-20251001" {
		t.Fatalf("expected Claude default model claude-haiku-4-5-20251001, got %q", got)
	}
}

func TestGetProviderModelsClaudeIncludesHaikuFirst(t *testing.T) {
	models := GetProviderModels(ProviderClaude)
	if len(models) == 0 {
		t.Fatal("expected Claude model list to be non-empty")
	}
	if models[0] != "claude-haiku-4-5-20251001" {
		t.Fatalf("expected first Claude model to be claude-haiku-4-5-20251001, got %q", models[0])
	}
}

func TestGetProviderModelsClaudeIncludesLegacyModels(t *testing.T) {
	models := GetProviderModels(ProviderClaude)
	set := make(map[string]struct{}, len(models))
	for _, m := range models {
		set[m] = struct{}{}
	}

	legacy := []string{
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}

	for _, model := range legacy {
		if _, ok := set[model]; !ok {
			t.Fatalf("expected legacy Claude model %q to remain available for compatibility", model)
		}
	}
}

func TestConnectorRecordGetConnectorOptionsDefaultsOpenAIModel(t *testing.T) {
	record := &ConnectorRecord{
		ProviderName: string(ProviderOpenAI),
		Provider:     ProviderOpenAI,
		ApiKey:       "sk-test",
		SelectedModel: sql.NullString{
			Valid: false,
		},
	}

	opts := record.GetConnectorOptions()
	if opts.ModelConfig.Model != "o4-mini" {
		t.Fatalf("expected default OpenAI model o4-mini, got %q", opts.ModelConfig.Model)
	}
}
