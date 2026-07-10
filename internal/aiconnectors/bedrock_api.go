package aiconnectors

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// isLangchainSupportedBedrockModel reports whether langchaingo's llms/bedrock package
// (which LiveReview uses to talk to Bedrock) can actually complete a request for this model.
// That package detects the model family from substrings in the model ID and only implements
// request/response translation for ai21, amazon, nova, anthropic, cohere, and meta - anything
// else (e.g. DeepSeek, Mistral, Stability AI, Writer) fails at call time with "unsupported
// provider".
//
// Nova used to be excluded here too: langchaingo always sends an empty `system` block when
// there's no system message, which Nova's schema rejects. LangchainProvider now always
// includes a real system message for Bedrock calls (see callBedrockDirect), which avoids that
// bug, so Nova models are safe to list again.
//
// Cohere Rerank models aren't chat/text-generation models at all (they score document
// relevance for a query) and would be sent a text-completion request they can't answer, so
// they stay excluded.
func isLangchainSupportedBedrockModel(modelID string) bool {
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "rerank") {
		return false
	}
	switch {
	case strings.Contains(lower, "ai21"),
		strings.Contains(lower, "amazon"),
		strings.Contains(lower, "anthropic"),
		strings.Contains(lower, "cohere"),
		strings.Contains(lower, "meta"):
		return true
	default:
		return false
	}
}

// BedrockModel represents a foundation model available to the given AWS account/region.
type BedrockModel struct {
	ModelID  string `json:"model_id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// FetchBedrockModels lists the foundation models available for the given AWS credentials and
// region via Bedrock's control-plane ListFoundationModels API. Unlike Ollama's model list (which
// is instance-specific but public within that instance), Bedrock's catalog is account/region
// specific, so this is fetched on demand with the connector's own credentials rather than synced
// globally - the same design already used for FetchOllamaModels.
func FetchBedrockModels(ctx context.Context, accessKeyID string, secretAccessKey string, region string) ([]BedrockModel, error) {
	if region == "" {
		return nil, fmt.Errorf("region is required to list Bedrock foundation models")
	}

	cfg, err := LoadBedrockAWSConfig(ctx, accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for Bedrock: %w", err)
	}

	client := bedrock.NewFromConfig(cfg)
	out, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByOutputModality: types.ModelModalityText,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list Bedrock foundation models: %w", err)
	}

	models := make([]BedrockModel, 0, len(out.ModelSummaries))
	for _, summary := range out.ModelSummaries {
		if summary.ModelLifecycle != nil && summary.ModelLifecycle.Status == types.FoundationModelLifecycleStatusLegacy {
			continue
		}

		var modelID, name, provider string
		if summary.ModelId != nil {
			modelID = *summary.ModelId
		}
		if summary.ModelName != nil {
			name = *summary.ModelName
		}
		if summary.ProviderName != nil {
			provider = *summary.ProviderName
		}
		if modelID == "" {
			continue
		}
		if !isLangchainSupportedBedrockModel(modelID) {
			continue
		}

		models = append(models, BedrockModel{
			ModelID:  modelID,
			Name:     name,
			Provider: provider,
		})
	}

	return models, nil
}
