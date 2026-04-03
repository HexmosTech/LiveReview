package aisanitize

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/HexmosTech/deidentify"
	"github.com/mdombrov-33/go-promptguard/detector"
	"github.com/rs/zerolog/log"
	"github.com/zricethezav/gitleaks/v8/detect"
)

const (
	defaultMediumRiskThreshold = 0.70
	defaultHighRiskThreshold   = 0.85
)

type RiskBand string

const (
	RiskBandLow    RiskBand = "low"
	RiskBandMedium RiskBand = "medium"
	RiskBandHigh   RiskBand = "high"
)

type SanitizationReport struct {
	RiskScore       float64
	RiskBand        RiskBand
	DetectedTypes   []string
	Sanitized       bool
	PIIRedacted     bool
	PIIRedactError  bool
	SecretsRedacted int
}

var (
	mediumRiskThreshold = sanitizeThresholdValue(
		getEnvFloat("LIVEREVIEW_SANITIZER_MEDIUM_THRESHOLD", defaultMediumRiskThreshold),
		defaultMediumRiskThreshold,
	)
	highRiskThreshold = sanitizeThresholdValue(
		getEnvFloat("LIVEREVIEW_SANITIZER_HIGH_THRESHOLD", defaultHighRiskThreshold),
		defaultHighRiskThreshold,
	)

	guard = detector.New(
		detector.WithThreshold(mediumRiskThreshold),
		detector.WithNormalizationMode(detector.ModeBalanced),
		detector.WithDelimiterMode(detector.ModeBalanced),
		detector.WithMaxInputLength(200000),
	)

	deidentifyOnce    sync.Once
	deidentifier      *deidentify.Deidentifier
	deidentifierError error
	gitleaksOnce      sync.Once
	gitleaksDetector  *detect.Detector
	gitleaksError     error

	zeroWidthChars = regexp.MustCompile(`[\x{200B}-\x{200D}\x{FEFF}]`)

	roleTokensReplacer = strings.NewReplacer(
		"<|system|>", "<blocked-system-token>",
		"<|assistant|>", "<blocked-assistant-token>",
		"<|user|>", "<blocked-user-token>",
		"<|im_start|>", "<blocked-im-start>",
		"<|im_end|>", "<blocked-im-end>",
	)

	instructionBoundaryPattern = regexp.MustCompile(`(?i)(---\s*end\s*system\s*---|ignore\s+previous\s+instructions|show\s+me\s+your\s+system\s+prompt|repeat\s+everything\s+above)`)

	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bapi[_-]?key\b\s*[:=]\s*["']?[A-Fa-f0-9]{24,}["']?`),
		regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`(?i)xox[baprs]-[A-Za-z0-9-]{10,}`),
	}

	piiPatterns = []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		{pattern: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), replacement: "REDACTED_EMAIL"},
		{pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), replacement: "REDACTED_SSN"},
	}
)

func getEnvFloat(name string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func sanitizeThresholdValue(value, fallback float64) float64 {
	if value <= 0 || value > 1 {
		return fallback
	}
	return value
}

func initDeidentifier() error {
	deidentifyOnce.Do(func() {
		secret, err := deidentify.GenerateSecretKey()
		if err != nil || secret == "" {
			secret, err = generateProcessScopedKey()
			if err != nil {
				deidentifierError = fmt.Errorf("failed to create deidentify secret key: %w", err)
				return
			}
			if secret == "" {
				deidentifierError = fmt.Errorf("failed to create deidentify secret key: empty fallback key")
				return
			}
		}
		deidentifier = deidentify.NewDeidentifier(secret)
	})
	return deidentifierError
}

func generateProcessScopedKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func initGitleaksDetector() error {
	gitleaksOnce.Do(func() {
		detector, err := detect.NewDetectorDefaultConfig()
		if err != nil {
			gitleaksError = fmt.Errorf("failed to initialize gitleaks detector: %w", err)
			return
		}
		gitleaksDetector = detector
	})
	return gitleaksError
}

func isCommentLikeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, " ") {
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "+- "))
	}

	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "--")
}

func bandForScore(score float64) RiskBand {
	if score >= highRiskThreshold {
		return RiskBandHigh
	}
	if score >= mediumRiskThreshold {
		return RiskBandMedium
	}
	return RiskBandLow
}

func sanitizeControls(text string) string {
	out := zeroWidthChars.ReplaceAllString(text, "")
	out = roleTokensReplacer.Replace(out)
	out = instructionBoundaryPattern.ReplaceAllString(out, "[neutralized-instruction]")
	return out
}

func redactSecrets(text string) (string, int) {
	count := 0
	out := text
	for _, pattern := range secretPatterns {
		matches := pattern.FindAllStringIndex(out, -1)
		if len(matches) == 0 {
			continue
		}
		count += len(matches)
		out = pattern.ReplaceAllString(out, "REDACTED_SECRET")
	}

	if err := initGitleaksDetector(); err != nil {
		log.Warn().Err(err).Msg("gitleaks detector initialization failed; continuing with regex-only redaction")
		return out, count
	}

	if gitleaksDetector == nil {
		return out, count
	}

	findings := gitleaksDetector.DetectString(out)
	for _, finding := range findings {
		replaced := false
		if finding.Secret != "" {
			newOut := strings.ReplaceAll(out, finding.Secret, "REDACTED_SECRET")
			replaced = replaced || newOut != out
			out = newOut
		}
		if finding.Match != "" {
			newOut := strings.ReplaceAll(out, finding.Match, "REDACTED_SECRET")
			replaced = replaced || newOut != out
			out = newOut
		}
		if replaced {
			count++
		}
	}

	return out, count
}

func redactPIIFallback(text string) (string, int) {
	count := 0
	out := text
	for _, p := range piiPatterns {
		matches := p.pattern.FindAllStringIndex(out, -1)
		if len(matches) == 0 {
			continue
		}
		count += len(matches)
		out = p.pattern.ReplaceAllString(out, p.replacement)
	}
	return out, count
}

func detectPromptRisk(ctx context.Context, input string) detector.Result {
	if strings.TrimSpace(input) == "" {
		return detector.Result{Safe: true, RiskScore: 0}
	}
	return guard.Detect(ctx, input)
}

func SanitizationPreflight(ctx context.Context, input string) (string, SanitizationReport) {
	result := detectPromptRisk(ctx, input)
	band := bandForScore(result.RiskScore)

	report := SanitizationReport{
		RiskScore:     result.RiskScore,
		RiskBand:      band,
		DetectedTypes: make([]string, 0, len(result.DetectedPatterns)),
	}
	for _, p := range result.DetectedPatterns {
		report.DetectedTypes = append(report.DetectedTypes, p.Type)
	}

	out := input
	controlled := sanitizeControls(out)
	if controlled != out {
		report.Sanitized = true
	}
	out = controlled

	var secretCount int
	out, secretCount = redactSecrets(out)
	report.SecretsRedacted = secretCount
	if secretCount > 0 {
		report.Sanitized = true
	}

	return out, report
}

// SanitizationPostflight applies non-blocking output redaction for user-visible model text.
// This reuses the natural-language sanitizer so output handling is deterministic and consistent
// with existing controls.
func SanitizationPostflight(ctx context.Context, output string) (string, SanitizationReport) {
	out, report := SanitizeNaturalLanguageFragment(ctx, output)
	outMarkdown, markdownChanged := sanitizeMarkdownForOutput(out)
	if markdownChanged {
		report.Sanitized = true
	}
	return outMarkdown, report
}

func SanitizeCodeLikeFragment(ctx context.Context, input string) (string, SanitizationReport) {
	return SanitizationPreflight(ctx, input)
}

func SanitizeFilePathFragment(input string) (string, SanitizationReport) {
	out := sanitizeControls(input)
	report := SanitizationReport{RiskBand: RiskBandLow}
	if out != input {
		report.Sanitized = true
	}
	return out, report
}

func SanitizeNaturalLanguageFragment(ctx context.Context, input string) (string, SanitizationReport) {
	out, report := SanitizationPreflight(ctx, input)

	if err := initDeidentifier(); err != nil {
		report.PIIRedactError = true
	}
	if deidentifier != nil && strings.TrimSpace(out) != "" {
		// Skip names due to regex bug in name identification causing false positives
		redacted, err := deidentifier.Text(out, deidentify.TextOptions{SkipNames: true})
		if err == nil {
			if redacted != out {
				report.PIIRedacted = true
				report.Sanitized = true
			}
			out = redacted
		} else {
			report.PIIRedactError = true
		}
	}

	outPII, piiFallbackCount := redactPIIFallback(out)
	if piiFallbackCount > 0 {
		report.PIIRedacted = true
		report.Sanitized = true
	}
	out = outPII

	return out, report
}

func SanitizeDiffHunk(ctx context.Context, hunk string) (string, SanitizationReport) {
	lines := strings.Split(hunk, "\n")

	combined := SanitizationReport{RiskBand: RiskBandLow}
	for i, line := range lines {
		var (
			sanitized string
			report    SanitizationReport
		)

		if isCommentLikeLine(line) {
			sanitized, report = SanitizeNaturalLanguageFragment(ctx, line)
		} else {
			sanitized, report = SanitizeCodeLikeFragment(ctx, line)
		}
		lines[i] = sanitized

		if report.RiskScore > combined.RiskScore {
			combined.RiskScore = report.RiskScore
			combined.RiskBand = report.RiskBand
		}
		combined.Sanitized = combined.Sanitized || report.Sanitized
		combined.PIIRedacted = combined.PIIRedacted || report.PIIRedacted
		combined.PIIRedactError = combined.PIIRedactError || report.PIIRedactError
		combined.SecretsRedacted += report.SecretsRedacted
		combined.DetectedTypes = append(combined.DetectedTypes, report.DetectedTypes...)
	}

	return strings.Join(lines, "\n"), combined
}

func IsCloudProvider(provider string) bool {
	switch strings.ToLower(provider) {
	case "openai", "gemini", "claude", "deepseek", "openrouter":
		return true
	default:
		return false
	}
}
