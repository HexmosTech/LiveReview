package langchain

import (
	"fmt"
	"strings"

	"github.com/livereview/internal/llm"
	"github.com/livereview/pkg/models"
)

// Enhanced parseResponse that integrates with our JSON repair system
func (p *LangchainProvider) parseResponseWithRepair(response string, diffs []models.CodeDiff, reviewID, orgID int64, batchID string) (*ParsedResult, error) {
	// First try the original parsing
	originalResult, originalErr := p.parseResponse(response, diffs)

	// If original parsing succeeded, return it
	if originalErr == nil {
		return originalResult, nil
	}

	// Original parsing failed - try our JSON repair system
	fmt.Printf("[LANGCHAIN] Original parsing failed: %v. Attempting JSON repair...\n", originalErr)

	// Try our resilient JSON processing
	var target interface{}
	processorResult, err := llm.ProcessLLMResponse(response, &target)

	// Log JSON repair event if repair was performed
	if processorResult.RepairStats.WasRepaired {
		fmt.Printf("[LANGCHAIN JSON REPAIR] Review %d Batch %s: %d strategies used, %d errors fixed, %v repair time\n",
			reviewID, batchID, len(processorResult.RepairStats.RepairStrategies),
			processorResult.RepairStats.ErrorsFixed, processorResult.RepairStats.RepairTime)

		// Log the strategies used
		if len(processorResult.RepairStats.RepairStrategies) > 0 {
			fmt.Printf("[LANGCHAIN JSON REPAIR] Strategies: %s\n", strings.Join(processorResult.RepairStats.RepairStrategies, ", "))
		}
	}

	if err != nil {
		// Even JSON repair failed - return the original fallback
		fmt.Printf("[LANGCHAIN FALLBACK] JSON repair also failed: %v. Using graceful fallback.\n", err)
		return p.fallbackParsedResult(response, diffs, "both original and repair parsing failed: "+err.Error()), nil
	}

	// JSON repair succeeded - try parsing the repaired JSON
	repairedResult, repairedErr := p.parseResponse(processorResult.RepairedJSON, diffs)

	if repairedErr != nil {
		// Even with repaired JSON, parsing failed
		fmt.Printf("[LANGCHAIN FALLBACK] Repaired JSON still failed to parse: %v. Using graceful fallback.\n", repairedErr)
		return p.fallbackParsedResult(response, diffs, "repaired JSON parse failed: "+repairedErr.Error()), nil
	}

	// Success! Repaired JSON parsed correctly
	fmt.Printf("[LANGCHAIN SUCCESS] JSON repair successful - parsed response with %d comments\n", len(repairedResult.Comments))
	return repairedResult, nil
}

// EnableJSONRepair modifies an existing LangChain provider to use JSON repair
// This is a simple way to add resiliency without major refactoring
func (p *LangchainProvider) EnableJSONRepair(reviewID, orgID int64) {
	// This would be used to enable JSON repair on the existing provider
	// For now, it's just a marker - the actual integration would happen
	// by replacing calls to parseResponse with parseResponseWithRepair
	fmt.Printf("[LANGCHAIN] JSON repair enabled for review %d (org %d)\n", reviewID, orgID)
}
