package bitbucket

import (
	"testing"

	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestFormatBitbucketComment_WithToolName(t *testing.T) {
	c := &models.ReviewComment{
		Content:  "Detected a generic API key: potentially exposing access to sensitive operations.",
		Severity: models.SeverityCritical,
		ToolName: "gitleaks",
	}

	formatted := formatBitbucketComment(c)
	expected := "**Severity: critical** **Tool: gitleaks**\n\nDetected a generic API key: potentially exposing access to sensitive operations."
	assert.Equal(t, expected, formatted)
}

func TestFormatBitbucketComment_WithoutToolName(t *testing.T) {
	c := &models.ReviewComment{
		Content:  "potential deadlock if tests run in parallel",
		Severity: models.SeverityWarning,
		Suggestions: []string{
			"Restructure mock to use a single mutex or use sync.Map",
			"Add t.Parallel() protection and ensure setupMockDB is serialized",
		},
	}

	formatted := formatBitbucketComment(c)
	expected := "**Severity: warning**\n\npotential deadlock if tests run in parallel\n\n**Suggestions:**\n1. Restructure mock to use a single mutex or use sync.Map\n2. Add t.Parallel() protection and ensure setupMockDB is serialized\n"
	assert.Equal(t, expected, formatted)
}
