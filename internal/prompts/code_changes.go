package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/livereview/internal/aisanitize"
	"github.com/livereview/pkg/models"
	"github.com/rs/zerolog/log"
)

// BuildCodeChangesSection returns the markdown section for code changes
// identical to what the legacy PromptBuilder used to append.
func BuildCodeChangesSection(diffs []*models.CodeDiff) string {
	return BuildCodeChangesSectionWithContext(context.Background(), diffs)
}

// BuildCodeChangesSectionWithContext returns the markdown section for code changes
// while preserving caller context for sanitization operations.
func BuildCodeChangesSectionWithContext(ctx context.Context, diffs []*models.CodeDiff) string {
	var b strings.Builder
	var aggregate aisanitize.SanitizationReport
	b.WriteString("# Code Changes\n\n")
	for _, diff := range diffs {
		safeFilePath, filePathReport := aisanitize.SanitizeFilePathFragment(diff.FilePath)
		aggregateSanitizationReport(&aggregate, filePathReport)
		b.WriteString(fmt.Sprintf("%s%s\n", FilePrefix, safeFilePath))

		if diff.IsNew {
			b.WriteString(NewFileMarker + "\n")
		} else if diff.IsDeleted {
			b.WriteString(DeletedFileMarker + "\n")
		} else if diff.IsRenamed {
			safeOldPath, oldPathReport := aisanitize.SanitizeFilePathFragment(diff.OldFilePath)
			aggregateSanitizationReport(&aggregate, oldPathReport)
			b.WriteString(fmt.Sprintf("%s%s%s\n", RenamedFilePrefix, safeOldPath, RenamedFileSuffix))
		}
		b.WriteString("\n")

		for _, hunk := range diff.Hunks {
			safeHunk, hunkReport := aisanitize.SanitizeDiffHunk(ctx, hunk.Content)
			aggregateSanitizationReport(&aggregate, hunkReport)
			b.WriteString("```diff\n")
			b.WriteString(safeHunk)
			b.WriteString("\n```\n\n")
		}
	}

	if aggregate.PIIRedactError {
		log.Warn().
			Bool("pii_redact_error", aggregate.PIIRedactError).
			Bool("sanitized", aggregate.Sanitized).
			Int("secrets_redacted", aggregate.SecretsRedacted).
			Msg("Prompt input sanitization encountered PII redaction errors")
	} else if aggregate.Sanitized {
		log.Debug().
			Bool("sanitized", aggregate.Sanitized).
			Int("secrets_redacted", aggregate.SecretsRedacted).
			Bool("pii_redacted", aggregate.PIIRedacted).
			Msg("Prompt input sanitization applied")
	}

	return b.String()
}

func aggregateSanitizationReport(aggregate *aisanitize.SanitizationReport, current aisanitize.SanitizationReport) {
	if current.RiskScore > aggregate.RiskScore {
		aggregate.RiskScore = current.RiskScore
		aggregate.RiskBand = current.RiskBand
	}
	aggregate.Sanitized = aggregate.Sanitized || current.Sanitized
	aggregate.PIIRedacted = aggregate.PIIRedacted || current.PIIRedacted
	aggregate.PIIRedactError = aggregate.PIIRedactError || current.PIIRedactError
	aggregate.SecretsRedacted += current.SecretsRedacted
	aggregate.DetectedTypes = append(aggregate.DetectedTypes, current.DetectedTypes...)
}
