package langchain

import "testing"

func TestEffectiveTemperatureOpenAIOSeriesUsesOne(t *testing.T) {
	t.Parallel()

	p := &LangchainProvider{
		providerType:   "openai",
		modelName:      "o4-mini",
		temperature:    0.4,
		temperatureSet: true,
	}

	if got := p.effectiveTemperature(); got != 1.0 {
		t.Fatalf("expected temperature 1.0 for OpenAI o-series, got %v", got)
	}
}

func TestEffectiveTemperatureNonOpenAIDefault(t *testing.T) {
	t.Parallel()

	p := &LangchainProvider{
		providerType:   "gemini",
		modelName:      "gemini-2.5-flash",
		temperature:    0,
		temperatureSet: false,
	}

	if got := p.effectiveTemperature(); got != 0.4 {
		t.Fatalf("expected default temperature 0.4, got %v", got)
	}
}

func TestEffectiveTemperatureNonOpenAIUsesConfigured(t *testing.T) {
	t.Parallel()

	p := &LangchainProvider{
		providerType:   "gemini",
		modelName:      "gemini-2.5-flash",
		temperature:    0.7,
		temperatureSet: true,
	}

	if got := p.effectiveTemperature(); got != 0.7 {
		t.Fatalf("expected configured temperature 0.7, got %v", got)
	}
}

func TestEffectiveTemperatureExplicitZeroIsPreserved(t *testing.T) {
	t.Parallel()

	p := &LangchainProvider{
		providerType:   "gemini",
		modelName:      "gemini-2.5-flash",
		temperature:    0,
		temperatureSet: true,
	}

	if got := p.effectiveTemperature(); got != 0 {
		t.Fatalf("expected explicit zero temperature to be preserved, got %v", got)
	}
}

func TestNormalizeTemperatureBounds(t *testing.T) {
	t.Parallel()

	if got := normalizeTemperature(-1, true); got != 0 {
		t.Fatalf("expected negative temperature to clamp to 0, got %v", got)
	}

	if got := normalizeTemperature(3, true); got != 2 {
		t.Fatalf("expected oversized temperature to clamp to 2, got %v", got)
	}
}

func TestIsOpenAISeriesModelAcceptsODigit(t *testing.T) {
	t.Parallel()

	if !isOpenAISeriesModel("o2-preview") {
		t.Fatal("expected o2-preview to be treated as OpenAI o-series")
	}
}
