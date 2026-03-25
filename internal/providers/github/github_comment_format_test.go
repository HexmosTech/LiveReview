package github

import (
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestFormatGitHubComment_SanitizesMarkdownAndLinks(t *testing.T) {
	comment := &models.ReviewComment{
		Content:     "Use <script>alert(1)</script> and [click](javascript:alert(1))",
		Severity:    models.SeverityWarning,
		Suggestions: []string{"Open [safe](https://example.com)", "Run [bad](javascript:alert(2))"},
	}

	out := formatGitHubComment(comment)

	if strings.Contains(strings.ToLower(out), "<script") {
		t.Fatalf("expected script tag to be neutralized, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "javascript:") {
		t.Fatalf("expected javascript links to be neutralized, got: %s", out)
	}
	if !strings.Contains(out, "[bad](#)") {
		t.Fatalf("expected unsafe suggestion link to be rewritten, got: %s", out)
	}
	if !strings.Contains(out, "[safe](https://example.com)") {
		t.Fatalf("expected safe suggestion link to remain, got: %s", out)
	}
}
