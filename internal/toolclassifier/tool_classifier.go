package toolclassifier

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/aiselection"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

// TaxonomyTuple represents a deterministic classification tuple
type TaxonomyTuple struct {
	Category    string
	Subcategory string
	Severity    string
	Confidence  string
	Type        string
}

// KnownDeterministicRules maps (tool_name + ":" + rule_id) -> taxonomy classification for 0 LLM token cost.
var KnownDeterministicRules = map[string]TaxonomyTuple{
	// Secrets Detection
	"gitleaks:generic-api-key":   {"Security", "Secrets Management", "critical", "High", "Risk"},
	"gitleaks:aws-access-key":    {"Security", "Secrets Management", "critical", "High", "Risk"},
	"gitleaks:aws-access-token":  {"Security", "Secrets Management", "critical", "High", "Risk"},
	"gitleaks:private-key":        {"Security", "Secrets Management", "critical", "High", "Risk"},
	"gitleaks:github-pat":         {"Security", "Secrets Management", "critical", "High", "Risk"},
	"gitleaks:slack-webhook":      {"Security", "Secrets Management", "critical", "High", "Risk"},
	"detect-secrets:secret-key":   {"Security", "Secrets Management", "critical", "High", "Risk"},
	"trufflehog:credential":      {"Security", "Secrets Management", "critical", "High", "Risk"},

	// Security & SAST
	"semgrep:sql-injection":      {"Security", "Injection Vulnerabilities", "critical", "High", "Bug"},
	"bandit:b303":                {"Security", "Cryptography", "warning", "High", "Risk"},
	"bandit:b101":                {"Security", "Input Validation", "warning", "High", "Risk"},
	"brakeman:command_injection": {"Security", "Injection Vulnerabilities", "critical", "High", "Bug"},

	// Linters & Static Analysis
	"hadolint:dl3006":       {"Maintainability", "Configuration Management", "warning", "Medium", "Best Practice"},
	"shellcheck:sc2086":     {"Reliability", "Fault Tolerance", "info", "Medium", "Code Smell"},
	"ruff:e501":             {"Maintainability", "Code Complexity", "info", "High", "Code Smell"},
	"ruff:f401":             {"Maintainability", "Dead Code", "warning", "High", "Code Smell"},
	"eslint:no-unused-vars": {"Maintainability", "Dead Code", "warning", "High", "Code Smell"},
}

// RawToolFinding represents an individual finding inside a Lambda response
type RawToolFinding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col,omitempty"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ClassifyToolResult classifies raw findings using deterministic static rules first, and Helper LLM batch call for unresolved findings.
func ClassifyToolResult(
	ctx context.Context,
	db *sql.DB,
	orgID int64,
	planCode license.PlanType,
	toolName string,
	findings []RawToolFinding,
	logger *logging.ReviewLogger,
) ([]*models.ReviewComment, error) {
	if len(findings) == 0 {
		return nil, nil
	}

	comments := make([]*models.ReviewComment, len(findings))
	var unresolvedIndices []int

	// Step 1: Check deterministic static rule mappings (0 LLM Tokens)
	for idx, finding := range findings {
		ruleKey := strings.ToLower(toolName + ":" + finding.Rule)
		rawRuleKey := strings.ToLower(finding.Rule)
		if tuple, ok := KnownDeterministicRules[ruleKey]; ok {
			cat, subcat := ValidateAndNormalizeTaxonomy(tuple.Category, tuple.Subcategory)
			comments[idx] = &models.ReviewComment{
				FilePath:    finding.File,
				Line:        finding.Line,
				Content:     finding.Message,
				Severity:    parseSeverity(tuple.Severity),
				Category:    cat,
				Subcategory: subcat,
				Confidence:  NormalizeConfidence(tuple.Confidence),
				Type:        NormalizeType(tuple.Type),
			}
			if logger != nil {
				logger.Log("[TOOL %s] Deterministic classification (0 tokens) for rule %s: %s / %s", toolName, finding.Rule, cat, subcat)
			}
		} else if tuple, ok := KnownDeterministicRules[rawRuleKey]; ok {
			cat, subcat := ValidateAndNormalizeTaxonomy(tuple.Category, tuple.Subcategory)
			comments[idx] = &models.ReviewComment{
				FilePath:    finding.File,
				Line:        finding.Line,
				Content:     finding.Message,
				Severity:    parseSeverity(tuple.Severity),
				Category:    cat,
				Subcategory: subcat,
				Confidence:  NormalizeConfidence(tuple.Confidence),
				Type:        NormalizeType(tuple.Type),
			}
			if logger != nil {
				logger.Log("[TOOL %s] Deterministic classification (0 tokens) for rule %s: %s / %s", toolName, finding.Rule, cat, subcat)
			}
		} else {
			unresolvedIndices = append(unresolvedIndices, idx)
		}
	}

	if len(unresolvedIndices) == 0 {
		return comments, nil
	}

	// Step 2: Resolve Helper AI model configuration
	selection, err := aiselection.GetReviewAISelection(ctx, db, orgID, planCode)
	var helperConfig review.AIConfig
	if err == nil && selection.HelperEnabled && selection.Helper != nil {
		helperConfig = *selection.Helper
	} else if err == nil {
		helperConfig = selection.Leader
	} else {
		return applyFallbackComments(findings, comments, unresolvedIndices), nil
	}

	// Step 3: Construct compact pipe-delimited batch prompt (~120 tokens)
	type CompactFindingInput struct {
		Index   int    `json:"i"`
		Rule    string `json:"r"`
		Message string `json:"m"`
	}

	compactItems := make([]CompactFindingInput, 0, len(unresolvedIndices))
	for _, origIdx := range unresolvedIndices {
		compactItems = append(compactItems, CompactFindingInput{
			Index:   origIdx,
			Rule:    findings[origIdx].Rule,
			Message: findings[origIdx].Message,
		})
	}

	payloadJSON, _ := json.Marshal(compactItems)

	prompt := fmt.Sprintf(`Classify static analysis tool findings into LiveReview taxonomy.
Categories: Security, Performance, Maintainability, Reliability, Compliance, Architecture.
Subcategories: Secrets Management, Injection Vulnerabilities, Cryptography, Input Validation, Dead Code, Configuration Management, Code Complexity, Fault Tolerance, Data Exposure.

Format response strictly as JSON array of pipe-delimited strings:
["index|category|subcategory|severity_code|confidence_code|type_code"]
Severity: c=critical, w=warning, i=info
Confidence: H=High, M=Medium, L=Low
Type: B=Bug, R=Risk, O=Optimization, S=Smell, P=Practice, D=Debt

Findings:
%s`, string(payloadJSON))

	options, _, err := connectorOptionsFromAIConfig(helperConfig, len(compactItems))
	if err == nil {
		connector, connErr := aiconnectors.NewConnector(ctx, options)
		if connErr == nil {
			respStr, inputTokens, outputTokens, callErr := connector.CallWithUsage(ctx, prompt)
			if callErr == nil {
				fmt.Printf("[INFO] [TOOL %s] Taxonomy LLM classification token usage: %d input tokens, %d output tokens (%d total LLM tokens)\n", toolName, inputTokens, outputTokens, inputTokens+outputTokens)
				if logger != nil {
					logger.Log("[TOOL %s] Taxonomy LLM classification token usage: %d input tokens, %d output tokens (%d total LLM tokens)", toolName, inputTokens, outputTokens, inputTokens+outputTokens)
				}

				parsedLines := parsePipeDelimitedResponse(respStr)
				for _, line := range parsedLines {
					parts := strings.Split(line, "|")
					if len(parts) >= 6 {
						var origIdx int
						if _, scanErr := fmt.Sscanf(parts[0], "%d", &origIdx); scanErr == nil && origIdx >= 0 && origIdx < len(findings) {
							cat, subcat := ValidateAndNormalizeTaxonomy(parts[1], parts[2])
							comments[origIdx] = &models.ReviewComment{
								FilePath:    findings[origIdx].File,
								Line:        findings[origIdx].Line,
								Content:     findings[origIdx].Message,
								Severity:    parseSeverityCode(parts[3]),
								Category:    cat,
								Subcategory: subcat,
								Confidence:  parseConfidenceCode(parts[4]),
								Type:        parseTypeCode(parts[5]),
							}
						}
					}
				}
			}
		}
	}

	return applyFallbackComments(findings, comments, unresolvedIndices), nil
}

func parsePipeDelimitedResponse(raw string) []string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var lines []string
	if err := json.Unmarshal([]byte(cleaned), &lines); err != nil {
		log.Printf("[WARN] parsePipeDelimitedResponse: failed to unmarshal pipe-delimited response: %v", err)
	}
	return lines
}

func applyFallbackComments(findings []RawToolFinding, comments []*models.ReviewComment, unresolved []int) []*models.ReviewComment {
	for _, origIdx := range unresolved {
		if comments[origIdx] == nil {
			comments[origIdx] = &models.ReviewComment{
				FilePath:    findings[origIdx].File,
				Line:        findings[origIdx].Line,
				Content:     findings[origIdx].Message,
				Severity:    models.SeverityWarning,
				Category:    "Security",
				Subcategory: "Secrets Management",
				Confidence:  "High",
				Type:        "Risk",
			}
		}
	}
	return comments
}

func connectorOptionsFromAIConfig(config review.AIConfig, commentCount int) (aiconnectors.ConnectorOptions, string, error) {
	providerName, _ := config.Config["provider_name"].(string)
	providerType, ok := config.Config["ai_provider_type"].(string)
	if !ok || strings.TrimSpace(providerType) == "" {
		providerType = providerName
	}
	if strings.TrimSpace(providerType) == "" {
		return aiconnectors.ConnectorOptions{}, "", fmt.Errorf("AI provider type is missing")
	}

	baseURL, _ := config.Config["base_url"].(string)
	projectID, _ := config.Config["gcp_project_id"].(string)
	location, _ := config.Config["gcp_location"].(string)
	awsAccessKeyID, _ := config.Config["aws_access_key_id"].(string)
	awsRegion, _ := config.Config["aws_region"].(string)

	return aiconnectors.ConnectorOptions{
		Provider:       aiconnectors.Provider(providerType),
		APIKey:         config.APIKey,
		BaseURL:        aiconnectors.ResolveBaseURLForProviderName(providerType, baseURL),
		GCPProjectID:   projectID,
		GCPLocation:    location,
		AWSAccessKeyID: awsAccessKeyID,
		AWSRegion:      awsRegion,
		ModelConfig: aiconnectors.ModelConfig{
			Temperature: config.Temperature,
			MaxTokens:   2048,
			Model:       config.Model,
		},
	}, strings.TrimSpace(providerName), nil
}

func parseSeverity(raw string) models.CommentSeverity {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return models.SeverityCritical
	case "info":
		return models.SeverityInfo
	default:
		return models.SeverityWarning
	}
}

func parseSeverityCode(code string) models.CommentSeverity {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "c":
		return models.SeverityCritical
	case "i":
		return models.SeverityInfo
	default:
		return models.SeverityWarning
	}
}

func parseConfidenceCode(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "H":
		return "High"
	case "L":
		return "Low"
	default:
		return "Medium"
	}
}

func parseTypeCode(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "B":
		return "Bug"
	case "R":
		return "Risk"
	case "O":
		return "Optimization"
	case "S":
		return "Code Smell"
	case "P":
		return "Best Practice"
	case "D":
		return "Technical Debt"
	default:
		return "Risk"
	}
}

var ValidTaxonomyMap = map[string][]string{
	"Security":                {"Authentication", "Authorization", "Secrets Management", "Input Validation", "Injection Vulnerabilities", "Cryptography", "Dependency Vulnerabilities", "Data Exposure", "Session Management", "Security Logging & Auditing"},
	"Reliability":             {"Error Handling", "Fault Tolerance", "Retry Logic", "Timeout Management", "Resilience Patterns", "Availability Risks", "Data Integrity", "Race Conditions", "Resource Cleanup", "Failure Recovery"},
	"Correctness":             {"Logic Errors", "Edge Cases", "Data Validation", "State Management", "Concurrency Bugs", "Business Rule Violations", "Numerical Accuracy", "Null Handling", "Type Safety", "API Contract Violations"},
	"Performance":             {"Database Efficiency", "Algorithmic Complexity", "Memory Usage", "CPU Utilization", "Network Efficiency", "Caching", "Concurrency", "Resource Contention", "Rendering Performance", "Startup Performance"},
	"Cost":                    {"Cloud Resource Waste", "Infrastructure Overprovisioning", "Storage Optimization", "Database Cost Optimization", "Excessive API Usage", "Third-Party Service Costs", "Redundant Computation", "LLM Token Consumption", "Caching Opportunities", "Data Transfer Costs"},
	"Scalability":             {"Horizontal Scaling", "Vertical Scaling", "Distributed Systems", "Load Balancing", "Capacity Planning", "Bottleneck Risks", "Concurrency Limits", "Service Growth Constraints", "Database Scaling", "Queue Backpressure"},
	"Maintainability":         {"Code Complexity", "Readability", "Documentation", "Code Duplication", "Dead Code", "Naming Quality", "Testability", "Technical Debt", "Refactoring Opportunities", "Configuration Management", "UI/UX", "Accessibility"},
	"Architecture":            {"Separation of Concerns", "Modularity", "Coupling", "Cohesion", "Layering Violations", "Dependency Management", "Service Boundaries", "Domain Modeling", "API Design", "Extensibility"},
	"Developer Experience":    {"Testing", "CI/CD", "Build System", "Local Development", "Debuggability", "Observability", "Deployment Process", "Automation", "Developer Tooling", "Documentation Quality", "UI/UX", "Accessibility"},
	"Compliance & Governance": {"Privacy", "Regulatory Compliance", "Auditability", "Data Retention", "Data Residency", "Licensing", "Policy Enforcement", "Access Controls", "Change Management", "Governance Standards"},
}

var ValidTypes = map[string]string{
	"bug":            "Bug",
	"risk":           "Risk",
	"optimization":   "Optimization",
	"code smell":     "Code Smell",
	"best practice":  "Best Practice",
	"technical debt": "Technical Debt",
}

var ValidConfidences = map[string]string{
	"high":   "High",
	"medium": "Medium",
	"low":    "Low",
}

func ValidateAndNormalizeTaxonomy(rawCategory, rawSubcategory string) (string, string) {
	trimmedCat := strings.TrimSpace(rawCategory)
	trimmedSub := strings.TrimSpace(rawSubcategory)

	var matchedCategory string
	var allowedSubcategories []string

	for cat, subcats := range ValidTaxonomyMap {
		if strings.EqualFold(trimmedCat, cat) {
			matchedCategory = cat
			allowedSubcategories = subcats
			break
		}
	}

	if matchedCategory == "" && trimmedSub != "" {
		for cat, subcats := range ValidTaxonomyMap {
			for _, sub := range subcats {
				if strings.EqualFold(trimmedSub, sub) {
					matchedCategory = cat
					allowedSubcategories = subcats
					trimmedSub = sub
					break
				}
			}
			if matchedCategory != "" {
				break
			}
		}
	}

	if matchedCategory == "" {
		matchedCategory = "Security"
		allowedSubcategories = ValidTaxonomyMap["Security"]
	}

	var matchedSubcategory string
	if trimmedSub != "" {
		for _, sub := range allowedSubcategories {
			if strings.EqualFold(trimmedSub, sub) {
				matchedSubcategory = sub
				break
			}
		}
	}

	if matchedSubcategory == "" {
		if matchedCategory == "Security" {
			matchedSubcategory = "Secrets Management"
		} else if len(allowedSubcategories) > 0 {
			matchedSubcategory = allowedSubcategories[0]
		}
	}

	return matchedCategory, matchedSubcategory
}

func NormalizeType(raw string) string {
	if val, ok := ValidTypes[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return val
	}
	return "Risk"
}

func NormalizeConfidence(raw string) string {
	if val, ok := ValidConfidences[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return val
	}
	return "High"
}
