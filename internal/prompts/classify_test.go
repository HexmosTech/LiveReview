package prompts

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Embedded input JSON for testing static tool finding prompts
const rawToolFindingsInputJSON = `[
  {
    "tool_name": "gitleaks",
    "rule_id": "aws-access-token",
    "file_path": "backend/config/aws.go",
    "line_number": 14,
    "message": "Uncovered secret: AKIAIOSFODNN7EXAMPLE",
    "code_snippet": "const AWSKey = \"AKIAIOSFODNN7EXAMPLE\""
  },
  {
    "tool_name": "bandit",
    "rule_id": "B303",
    "file_path": "services/crypto.py",
    "line_number": 18,
    "message": "Use of MD5 insecure hash function",
    "code_snippet": "hashlib.md5(password.encode()).hexdigest()"
  },
  {
    "tool_name": "golangci-lint",
    "rule_id": "errcheck",
    "file_path": "storage/db.go",
    "line_number": 88,
    "message": "Error return value of 'file.Close' is not checked",
    "code_snippet": "defer file.Close()"
  },
  {
    "tool_name": "eslint",
    "rule_id": "no-eval",
    "file_path": "src/components/DynamicScript.tsx",
    "line_number": 34,
    "message": "eval can be harmful.",
    "code_snippet": "const result = eval(userCodeInput);"
  },
  {
    "tool_name": "ruff",
    "rule_id": "F841",
    "file_path": "controllers/user.py",
    "line_number": 102,
    "message": "Local variable 'temp_res' is assigned to but never used",
    "code_snippet": "temp_res = calculate_stats(user_id)"
  },
  {
    "tool_name": "actionlint",
    "rule_id": "expression",
    "file_path": ".github/workflows/deploy.yml",
    "line_number": 25,
    "message": "Unsanitized input in run step: github.event.issue.title can lead to script injection",
    "code_snippet": "run: echo \"${{ github.event.issue.title }}\""
  }
]`

// Embedded expected output JSON for taxonomy classification verification
const rawClassifiedTaxonomyOutputJSON = `[
  {
    "tool_name": "gitleaks",
    "category": "Security",
    "subcategory": "Secrets Management",
    "severity": "critical",
    "type": "Risk",
    "confidence": "High",
    "suggestions": ["Remove hardcoded AWS key and fetch it from environment variables or AWS Secrets Manager."],
    "isInternal": false
  },
  {
    "tool_name": "bandit",
    "category": "Security",
    "subcategory": "Cryptography",
    "severity": "critical",
    "type": "Risk",
    "confidence": "High",
    "suggestions": ["Replace MD5 with a secure hashing algorithm like SHA-256 or bcrypt for password hashing."],
    "isInternal": false
  },
  {
    "tool_name": "golangci-lint",
    "category": "Reliability",
    "subcategory": "Error Handling",
    "severity": "warning",
    "type": "Code Smell",
    "confidence": "High",
    "suggestions": ["Check and log the error returned by file.Close() to prevent silent write/close failures."],
    "isInternal": false
  },
  {
    "tool_name": "eslint",
    "category": "Security",
    "subcategory": "Injection Vulnerabilities",
    "severity": "critical",
    "type": "Risk",
    "confidence": "High",
    "suggestions": ["Avoid eval(); parse input safely or use structured JSON evaluation."],
    "isInternal": false
  },
  {
    "tool_name": "ruff",
    "category": "Maintainability",
    "subcategory": "Dead Code",
    "severity": "info",
    "type": "Code Smell",
    "confidence": "High",
    "suggestions": ["Remove unused variable 'temp_res' or use '_' if side effects are required."],
    "isInternal": true
  },
  {
    "tool_name": "actionlint",
    "category": "Security",
    "subcategory": "Injection Vulnerabilities",
    "severity": "critical",
    "type": "Risk",
    "confidence": "High",
    "suggestions": ["Pass event title via environment variable 'TITLE: ${{ github.event.issue.title }}' instead of inline script execution."],
    "isInternal": false
  }
]`

func TestToolFindingClassificationPrompt_EmbeddedJSON(t *testing.T) {
	var findings []ToolFindingInput
	err := json.Unmarshal([]byte(rawToolFindingsInputJSON), &findings)
	require.NoError(t, err, "unmarshal rawToolFindingsInputJSON failed")

	var expectedClassifications []map[string]any
	err = json.Unmarshal([]byte(rawClassifiedTaxonomyOutputJSON), &expectedClassifications)
	require.NoError(t, err, "unmarshal rawClassifiedTaxonomyOutputJSON failed")
	require.Len(t, expectedClassifications, len(findings))

	builder := NewPromptBuilder()

	for i, f := range findings {
		prompt := builder.BuildToolFindingClassificationPrompt(f)
		assert.Contains(t, prompt, f.ToolName)
		assert.Contains(t, prompt, f.RuleID)
		assert.Contains(t, prompt, f.FilePath)
		assert.Contains(t, prompt, "TAXONOMY CLASSIFICATION RULES")
		assert.Contains(t, prompt, "COMMENT CLASSIFICATION")

		expected := expectedClassifications[i]

		// Verify expected classification fields are well-formed in the embedded JSON
		assert.Equal(t, f.ToolName, expected["tool_name"],
			"finding[%d] tool_name mismatch", i)

		category, ok := expected["category"].(string)
		assert.True(t, ok && category != "", "finding[%d] expected non-empty category", i)

		subcategory, ok := expected["subcategory"].(string)
		assert.True(t, ok && subcategory != "", "finding[%d] expected non-empty subcategory", i)

		severity, ok := expected["severity"].(string)
		assert.True(t, ok, "finding[%d] severity must be a string", i)
		assert.Contains(t, []string{"critical", "warning", "info"}, severity,
			"finding[%d] severity must be one of critical/warning/info", i)

		typVal, ok := expected["type"].(string)
		assert.True(t, ok, "finding[%d] type must be a string", i)
		assert.Contains(t, []string{"Bug", "Risk", "Optimization", "Code Smell", "Best Practice", "Technical Debt"}, typVal,
			"finding[%d] type must be a valid taxonomy type", i)

		confidence, ok := expected["confidence"].(string)
		assert.True(t, ok, "finding[%d] confidence must be a string", i)
		assert.Contains(t, []string{"High", "Medium", "Low"}, confidence,
			"finding[%d] confidence must be High/Medium/Low", i)

		_, hasIsInternal := expected["isInternal"]
		assert.True(t, hasIsInternal, "finding[%d] must have isInternal field", i)
	}
}
