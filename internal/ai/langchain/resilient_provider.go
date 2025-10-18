package langchain

import (
	"fmt"

	"github.com/livereview/internal/llm"
	"github.com/livereview/pkg/models"
)

// ResilientLangchainProvider wraps the existing LangchainProvider with JSON repair capabilities
type ResilientLangchainProvider struct {
	*LangchainProvider // Embed the base provider
	reviewID           int64
	orgID              int64
}

// NewResilientLangchainProvider creates a resilient wrapper around the existing LangChain provider
func NewResilientLangchainProvider(baseProvider *LangchainProvider, reviewID, orgID int64) *ResilientLangchainProvider {
	return &ResilientLangchainProvider{
		LangchainProvider: baseProvider,
		reviewID:          reviewID,
		orgID:             orgID,
	}
}

// RepairAndParseJSON provides enhanced JSON repair for LLM responses
func (rp *ResilientLangchainProvider) RepairAndParseJSON(response string, diffs []models.CodeDiff, batchID string) (*ParsedResult, error) {
	// Try our resilient JSON processing first
	var target interface{}
	processorResult, err := llm.ProcessLLMResponse(response, &target, rp.LangchainProvider.logger)

	// Log JSON repair event if repair was performed
	if processorResult.RepairStats.WasRepaired {
		fmt.Printf("[LANGCHAIN JSON REPAIR] Review %d Batch %s: %d strategies used, %d errors fixed, %v repair time\n",
			rp.reviewID, batchID, len(processorResult.RepairStats.RepairStrategies),
			processorResult.RepairStats.ErrorsFixed, processorResult.RepairStats.RepairTime)
	}

	if err != nil {
		// Fall back to the original parsing method
		fmt.Printf("[LANGCHAIN FALLBACK] Resilient parsing failed, falling back to original method: %v\n", err)
		return rp.LangchainProvider.parseResponse(response, diffs)
	}

	// If resilient parsing succeeded, try the original parser with the repaired JSON
	originalResult, originalErr := rp.LangchainProvider.parseResponse(processorResult.RepairedJSON, diffs)

	if originalErr != nil {
		// Even with JSON repair, we couldn't parse - return graceful fallback
		fmt.Printf("[LANGCHAIN FALLBACK] Even repaired JSON failed to parse: %v\n", originalErr)
		return rp.LangchainProvider.fallbackParsedResult(response, diffs, "resilient parsing failed: "+originalErr.Error()), nil
	}

	return originalResult, nil
}

// EnableJSONRepairForProvider is a simple helper to wrap an existing provider with JSON repair
func EnableJSONRepairForProvider(provider *LangchainProvider, reviewID, orgID int64) *ResilientLangchainProvider {
	return NewResilientLangchainProvider(provider, reviewID, orgID)
}
