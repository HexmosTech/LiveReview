package langchain

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/aisanitize"
	"github.com/livereview/internal/llm"
	"github.com/livereview/internal/logging"
	"github.com/livereview/pkg/models"
)

// Enhanced parseResponse that integrates with our JSON repair system
func (p *LangchainProvider) parseResponseWithRepair(ctx context.Context, response string, diffs []models.CodeDiff, reviewID, orgID int64, batchID string, logger *logging.ReviewLogger) (*ParsedResult, error) {
	// First try the original parsing
	originalResult, originalErr := p.parseResponse(response, diffs)

	// If original parsing succeeded, return it
	if originalErr == nil {
		if err := applyPostOutputSanitizeToParsedResult(ctx, originalResult, batchID, logger); err != nil {
			log.Printf("[LANGCHAIN OUTPUT SANITIZE] batch=%s sanitize_error=%v", batchID, err)
			if logger != nil {
				logger.Log("Output sanitization encountered internal errors for batch %s: %v", batchID, err)
			}
		}
		return originalResult, nil
	}

	// Original parsing failed - try our JSON repair system
	if logger != nil {
		logger.Log("Original parsing failed: %v. Attempting JSON repair...", originalErr)
	}
	log.Printf("[LANGCHAIN] Original parsing failed: %v. Attempting JSON repair...", originalErr)

	// Try our resilient JSON processing
	var target interface{}
	processorResult, err := llm.ProcessLLMResponse(response, &target, logger)

	// Log JSON repair event if repair was performed
	if processorResult.RepairStats.WasRepaired {
		repairMsg := fmt.Sprintf("Review %d Batch %s: %d strategies used, %d errors fixed, %v repair time",
			reviewID, batchID, len(processorResult.RepairStats.RepairStrategies),
			processorResult.RepairStats.ErrorsFixed, processorResult.RepairStats.RepairTime)
		if logger != nil {
			logger.Log("JSON REPAIR: %s", repairMsg)
		}
		log.Printf("[LANGCHAIN JSON REPAIR] %s", repairMsg)

		// Log the strategies used
		if len(processorResult.RepairStats.RepairStrategies) > 0 {
			strategyMsg := fmt.Sprintf("Strategies: %s", strings.Join(processorResult.RepairStats.RepairStrategies, ", "))
			if logger != nil {
				logger.Log("JSON REPAIR: %s", strategyMsg)
			}
			log.Printf("[LANGCHAIN JSON REPAIR] %s", strategyMsg)
		}
	}

	if err != nil {
		// Even JSON repair failed - return the original fallback
		if logger != nil {
			logger.Log("JSON repair also failed: %v. Using graceful fallback.", err)
		}
		log.Printf("[LANGCHAIN FALLBACK] JSON repair also failed: %v. Using graceful fallback.", err)
		return p.fallbackParsedResult(response, diffs, "both original and repair parsing failed: "+err.Error()), nil
	}

	// JSON repair succeeded - try parsing the repaired JSON
	repairedResult, repairedErr := p.parseResponse(processorResult.RepairedJSON, diffs)

	if repairedErr != nil {
		// Even with repaired JSON, parsing failed
		if logger != nil {
			logger.Log("Repaired JSON still failed to parse: %v. Using graceful fallback.", repairedErr)
		}
		log.Printf("[LANGCHAIN FALLBACK] Repaired JSON still failed to parse: %v. Using graceful fallback.", repairedErr)
		return p.fallbackParsedResult(response, diffs, "repaired JSON parse failed: "+repairedErr.Error()), nil
	}

	// Success! Repaired JSON parsed correctly
	if err := applyPostOutputSanitizeToParsedResult(ctx, repairedResult, batchID, logger); err != nil {
		log.Printf("[LANGCHAIN OUTPUT SANITIZE] batch=%s sanitize_error=%v", batchID, err)
		if logger != nil {
			logger.Log("Output sanitization encountered internal errors for batch %s: %v", batchID, err)
		}
	}
	successMsg := fmt.Sprintf("JSON repair successful - parsed response with %d comments", len(repairedResult.Comments))
	if logger != nil {
		logger.Log("SUCCESS: %s", successMsg)
	}
	log.Printf("[LANGCHAIN SUCCESS] %s", successMsg)
	return repairedResult, nil
}

func applyPostOutputSanitizeToParsedResult(ctx context.Context, parsed *ParsedResult, batchID string, logger *logging.ReviewLogger) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic during output sanitization: %v", recovered)
		}
	}()

	if parsed == nil {
		return nil
	}

	changed := false
	secretsRedacted := 0
	piiRedacted := false
	internalErrors := 0

	for i := range parsed.TechnicalSummaries {
		orig := parsed.TechnicalSummaries[i].Summary
		sanitized, report, sanitizeErr := sanitizePostflightSafe(ctx, orig)
		if sanitizeErr != nil {
			internalErrors++
			continue
		}
		if sanitized != orig {
			parsed.TechnicalSummaries[i].Summary = sanitized
			changed = true
		}
		secretsRedacted += report.SecretsRedacted
		piiRedacted = piiRedacted || report.PIIRedacted
		if report.PIIRedactError {
			internalErrors++
		}
	}

	for _, comment := range parsed.Comments {
		if comment == nil {
			continue
		}

		orig := comment.Content
		sanitized, report, sanitizeErr := sanitizePostflightSafe(ctx, orig)
		if sanitizeErr != nil {
			internalErrors++
			continue
		}
		if sanitized != orig {
			comment.Content = sanitized
			changed = true
		}
		secretsRedacted += report.SecretsRedacted
		piiRedacted = piiRedacted || report.PIIRedacted
		if report.PIIRedactError {
			internalErrors++
		}

		for idx, suggestion := range comment.Suggestions {
			sanitizedSuggestion, suggestionReport, sanitizeErr := sanitizePostflightSafe(ctx, suggestion)
			if sanitizeErr != nil {
				internalErrors++
				continue
			}
			if sanitizedSuggestion != suggestion {
				comment.Suggestions[idx] = sanitizedSuggestion
				changed = true
			}
			secretsRedacted += suggestionReport.SecretsRedacted
			piiRedacted = piiRedacted || suggestionReport.PIIRedacted
			if suggestionReport.PIIRedactError {
				internalErrors++
			}
		}
	}

	if changed || internalErrors > 0 {
		log.Printf("[LANGCHAIN OUTPUT SANITIZE] batch=%s sanitized=%t secrets_redacted=%d pii_redacted=%t internal_errors=%d", batchID, changed, secretsRedacted, piiRedacted, internalErrors)
		if logger != nil {
			logger.Log("Output sanitization summary for batch %s (sanitized=%t secrets_redacted=%d pii_redacted=%t internal_errors=%d)", batchID, changed, secretsRedacted, piiRedacted, internalErrors)
		}
	}

	if internalErrors > 0 {
		return fmt.Errorf("output sanitization encountered %d internal error(s)", internalErrors)
	}

	return nil
}

func sanitizePostflightSafe(ctx context.Context, value string) (sanitized string, report aisanitize.SanitizationReport, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic in SanitizationPostflight: %v", recovered)
			sanitized = value
		}
	}()

	sanitized, report = aisanitize.SanitizationPostflight(ctx, value)
	return sanitized, report, nil
}

// EnableJSONRepair modifies an existing LangChain provider to use JSON repair
// This is a simple way to add resiliency without major refactoring
func (p *LangchainProvider) EnableJSONRepair(reviewID, orgID int64) {
	// This would be used to enable JSON repair on the existing provider
	// For now, it's just a marker - the actual integration would happen
	// by replacing calls to parseResponse with parseResponseWithRepair
	log.Printf("[LANGCHAIN] JSON repair enabled for review %d (org %d)", reviewID, orgID)
}
