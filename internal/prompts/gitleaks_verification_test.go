package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type GitleaksLambdaRawFinding struct {
	ToolName    string `json:"tool_name"`
	RuleID      string `json:"rule_id"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Secret      string `json:"secret"`
	Message     string `json:"message"`
	CodeSnippet string `json:"code_snippet"`
}

type PromptInputVerification struct {
	FindingID       string           `json:"finding_id"`
	RawToolFinding  ToolFindingInput `json:"raw_tool_finding"`
	GeneratedPrompt string           `json:"generated_classification_prompt"`
}

type ClassifiedTaxonomyOutput struct {
	FindingID      string           `json:"finding_id"`
	RawToolFinding ToolFindingInput `json:"raw_tool_finding"`
	ClassifiedJSON map[string]any   `json:"classified_taxonomy_result"`
}

func TestGitleaksToolVerificationFiles(t *testing.T) {
	// Target output directory: tests/test-tools-llm
	targetDir := os.Getenv("TEST_OUTPUT_DIR")
	if targetDir == "" {
		targetDir = filepath.Join("..", "..", "tests", "test-tools-llm")
	}
	err := os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	// 1. Raw Lambda Gitleaks findings detected from test_cases/gitleaks.txt
	rawLambdaFindings := []GitleaksLambdaRawFinding{
		{
			ToolName:    "gitleaks",
			RuleID:      "aws-access-token",
			FilePath:    "src/auth.py",
			LineNumber:  81,
			Secret:      "AKIAIOSFODNN7EXAMPLE",
			Message:     "AWS Access Key ID exposed in auth handler",
			CodeSnippet: "api_secret = 'AKIAIOSFODNN7EXAMPLE'",
		},
		{
			ToolName:    "gitleaks",
			RuleID:      "generic-api-key",
			FilePath:    "config/settings.json",
			LineNumber:  326,
			Secret:      "supersecretpassword123",
			Message:     "Hardcoded plaintext database password in settings",
			CodeSnippet: "\"database_password\": \"supersecretpassword123\"",
		},
		{
			ToolName:    "gitleaks",
			RuleID:      "slack-bot-token",
			FilePath:    "config/settings.json",
			LineNumber:  327,
			Secret:      "xoxb-1234-5678-abcdef",
			Message:     "Slack Bot OAuth Token exposed in settings",
			CodeSnippet: "\"slack_token\": \"xoxb-1234-5678-abcdef\"",
		},
		{
			ToolName:    "gitleaks",
			RuleID:      "private-key",
			FilePath:    "src/index.js",
			LineNumber:  425,
			Secret:      "-----BEGIN PRIVATE KEY-----",
			Message:     "Uncovered Unencrypted RSA Private Key",
			CodeSnippet: "const privateKey = '-----BEGIN PRIVATE KEY-----\\nMIIEvgIBADAN...'",
		},
	}

	// 1. Save tool-json.txt directly
	rawBytes, err := json.MarshalIndent(rawLambdaFindings, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "tool-json.txt"), rawBytes, 0644)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	apiKey := os.Getenv("GEMINI_API_KEY")
	selectedModel := os.Getenv("GEMINI_MODEL")
	if selectedModel == "" {
		selectedModel = "gemini-2.0-flash"
	}
	if apiKey == "" {
		t.Skip("Skipping live LLM verification test: GEMINI_API_KEY environment variable is not set")
	}

	// 2. Construct Prompts & Run Live LLM Classification for ALL 4 findings
	builder := NewPromptBuilder()
	var promptsList []PromptInputVerification
	var classifiedList []ClassifiedTaxonomyOutput

	var totalInputChars int
	var totalOutputChars int
	var inputSb strings.Builder

	for i, f := range rawLambdaFindings {
		if i > 0 {
			time.Sleep(3 * time.Second) // Rate limit pacing
		}

		findingID := filepath.Base(f.FilePath) + fmt.Sprintf("_L%d_%s", f.LineNumber, f.RuleID)

		input := ToolFindingInput{
			ToolName:    f.ToolName,
			RuleID:      f.RuleID,
			FilePath:    f.FilePath,
			LineNumber:  f.LineNumber,
			Message:     f.Message,
			CodeSnippet: f.CodeSnippet,
		}

		promptText := builder.BuildToolFindingClassificationPrompt(input)
		totalInputChars += len(promptText)

		promptsList = append(promptsList, PromptInputVerification{
			FindingID:       findingID,
			RawToolFinding:  input,
			GeneratedPrompt: promptText,
		})

		inputSb.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		inputSb.WriteString(fmt.Sprintf("[FINDING %d/%d] Prompt for %s:%d (%s)\n", i+1, len(rawLambdaFindings), f.FilePath, f.LineNumber, f.RuleID))
		inputSb.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		inputSb.WriteString(promptText)
		inputSb.WriteString("\n\n")

		// Execute live call to Gemini using provided API key
		var respCall string

		llmModel, errInit := googleai.New(ctx,
			googleai.WithAPIKey(apiKey),
			googleai.WithDefaultModel(selectedModel),
		)
		require.NoError(t, errInit, "failed to initialize googleai model")

		for retry := 0; retry < 5; retry++ {
			resp, errCall := llms.GenerateFromSinglePrompt(ctx, llmModel, promptText,
				llms.WithTemperature(0.2),
				llms.WithMaxTokens(2000),
			)
			if errCall == nil && resp != "" {
				respCall = resp
				t.Logf("Success for %s using model %s", findingID, selectedModel)
				break
			}
			if errCall != nil && strings.Contains(errCall.Error(), "429") {
				t.Logf("429 Rate Limit for %s. Retrying in 40s (attempt %d/5)...", findingID, retry+1)
				time.Sleep(40 * time.Second)
				continue
			}
			t.Logf("Call error: %v", errCall)
		}
		require.NotEmpty(t, respCall, fmt.Sprintf("empty response for finding %s", findingID))

		totalOutputChars += len(respCall)

		clean := cleanJSONString(respCall)
		var classified map[string]any
		err = json.Unmarshal([]byte(clean), &classified)
		require.NoError(t, err, fmt.Sprintf("failed to parse JSON from LLM: %s", respCall))

		classifiedList = append(classifiedList, ClassifiedTaxonomyOutput{
			FindingID:      findingID,
			RawToolFinding: input,
			ClassifiedJSON: classified,
		})
	}

	// 2. Save input.txt directly with token metrics
	totalInputTokens := (totalInputChars + 3) / 4
	var finalInputContent strings.Builder
	finalInputContent.WriteString(fmt.Sprintf("=== CUMULATIVE INPUT PROMPT TOKEN METRICS (%d FINDINGS) ===\n", len(rawLambdaFindings)))
	finalInputContent.WriteString(fmt.Sprintf("Total Findings Analyzed:  %d findings\n", len(rawLambdaFindings)))
	finalInputContent.WriteString(fmt.Sprintf("Total Character Count:    %d chars\n", totalInputChars))
	finalInputContent.WriteString(fmt.Sprintf("Estimated Input Tokens:   %d tokens (Avg ~%d tokens per prompt)\n", totalInputTokens, totalInputTokens/len(rawLambdaFindings)))
	finalInputContent.WriteString(fmt.Sprintf("==========================================================\n\n"))
	finalInputContent.WriteString(inputSb.String())

	err = os.WriteFile(filepath.Join(targetDir, "input.txt"), []byte(finalInputContent.String()), 0644)
	require.NoError(t, err)

	// 3. Save output.txt directly with token metrics & full classified JSON array
	totalOutputTokens := (totalOutputChars + 3) / 4
	totalTokens := totalInputTokens + totalOutputTokens
	fullReviewBaseline := 20000
	savingsPercent := float64(fullReviewBaseline-totalTokens) / float64(fullReviewBaseline) * 100.0

	classifiedJSONBytes, _ := json.MarshalIndent(classifiedList, "", "  ")

	var finalOutputContent strings.Builder
	finalOutputContent.WriteString(fmt.Sprintf("=== CUMULATIVE OUTPUT CLASSIFICATION TOKEN CONSUMPTION METRICS (%d FINDINGS) ===\n", len(rawLambdaFindings)))
	finalOutputContent.WriteString(fmt.Sprintf("Total Findings Analyzed: %d findings\n", len(rawLambdaFindings)))
	finalOutputContent.WriteString(fmt.Sprintf("Total Input Tokens:       %d tokens\n", totalInputTokens))
	finalOutputContent.WriteString(fmt.Sprintf("Total Output Tokens:      %d tokens\n", totalOutputTokens))
	finalOutputContent.WriteString(fmt.Sprintf("Total Tokens Consumed:    %d tokens (Avg ~%d tokens per finding)\n", totalTokens, totalTokens/len(rawLambdaFindings)))
	finalOutputContent.WriteString(fmt.Sprintf("Baseline Full Review:     %d tokens\n", fullReviewBaseline))
	finalOutputContent.WriteString(fmt.Sprintf("Token Efficiency Gain:    %.2f%% token savings!\n", savingsPercent))
	finalOutputContent.WriteString(fmt.Sprintf("=============================================================================\n\n"))
	finalOutputContent.WriteString(string(classifiedJSONBytes))

	err = os.WriteFile(filepath.Join(targetDir, "output.txt"), []byte(finalOutputContent.String()), 0644)
	require.NoError(t, err)
}

func cleanJSONString(s string) string {
	if idx := strings.Index(s, "{"); idx != -1 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "}"); idx != -1 {
		s = s[:idx+1]
	}
	return s
}
