package gitlab

import (
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestFormatGitLabComment_SanitizesMarkdownAndLinks(t *testing.T) {
	comment := &models.ReviewComment{
		Content:     "Use <img src=x onerror=alert(1)> and [click](javascript:alert(1))",
		Severity:    models.SeverityCritical,
		Suggestions: []string{"See [guide](https://example.com/guide)", "Try [bad](data:text/html,boom)"},
	}

	out := formatGitLabComment(comment)

	if strings.Contains(strings.ToLower(out), "<img") {
		t.Fatalf("expected html tag to be neutralized, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "javascript:") || strings.Contains(strings.ToLower(out), "data:") {
		t.Fatalf("expected unsafe schemes to be neutralized, got: %s", out)
	}
	if !strings.Contains(out, "[bad](#)") {
		t.Fatalf("expected unsafe suggestion link to be rewritten, got: %s", out)
	}
	if !strings.Contains(out, "[guide](https://example.com/guide)") {
		t.Fatalf("expected safe suggestion link to remain, got: %s", out)
	}
}
