package prompts

import (
	"context"
	"strings"
)

type repoRulesContextKey struct{}

// WithRepoRules returns a context carrying the repository's concatenated
// .lrc/rules/*.md instruction bundle (see internal/lrcconfig), so it can be
// spliced into AI prompts via BuildRepoRulesSection.
func WithRepoRules(ctx context.Context, rules string) context.Context {
	return context.WithValue(ctx, repoRulesContextKey{}, rules)
}

// RepoRulesFromContext returns the repository rules bundle stored in ctx, if
// any.
func RepoRulesFromContext(ctx context.Context) string {
	rules, _ := ctx.Value(repoRulesContextKey{}).(string)
	return rules
}

// BuildRepoRulesSection returns a "# Repository Rules" markdown section for
// the rules bundle stored in ctx, or "" if none is set.
func BuildRepoRulesSection(ctx context.Context) string {
	rules := strings.TrimSpace(RepoRulesFromContext(ctx))
	if rules == "" {
		return ""
	}
	return "# Repository Rules\n\n" + rules + "\n\n"
}
