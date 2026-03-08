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
