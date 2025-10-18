package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/learnings"
	"github.com/stretchr/testify/require"
)

func TestFetchRelevantLearningsAndPromptIntegration(t *testing.T) {
	t.Helper()

	env, err := loadEnvFile("../../.env")
	require.NoError(t, err)

	dbURL, ok := env["DATABASE_URL"]
	require.True(t, ok, "DATABASE_URL not found in .env")
	require.NotEmpty(t, dbURL, "DATABASE_URL is empty")

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})
	require.NoError(t, db.Ping())

	store := learnings.NewPostgresStore(db)
	svc := learnings.NewService(store)
	srv := &Server{learningsService: svc}
	processor := &UnifiedProcessorV2Impl{server: srv}

	uniqueToken := fmt.Sprintf("lrprompttest-%d", time.Now().UnixNano())
	repoID := "integration/test-repo"
	title := fmt.Sprintf("Integration prompt guidance %s", uniqueToken)
	body := fmt.Sprintf("This learning %s ensures prompts include DB backed guidance.", uniqueToken)
	learning := &learnings.Learning{
		ShortID:    fmt.Sprintf("LRTEST-%d", time.Now().UnixNano()),
		OrgID:      1,
		Scope:      learnings.ScopeRepo,
		RepoID:     repoID,
		Title:      title,
		Body:       body,
		Tags:       []string{"integration", "prompt"},
		Status:     learnings.StatusActive,
		Confidence: 1,
		Simhash:    int64(learnings.Simhash64(strings.Join([]string{title, body}, " | "))),
		SourceURLs: []string{"https://example.com/integration-prompts"},
		SourceContext: &learnings.SourceContext{
			Provider:   "gitlab",
			Repository: repoID,
			PRNumber:   42,
			ThreadID:   uniqueToken,
		},
	}

	ctx := context.Background()
	require.NoError(t, store.Create(ctx, learning))
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DELETE FROM learnings WHERE short_id = $1", learning.ShortID)
		if err != nil {
			t.Fatalf("failed to cleanup learning: %v", err)
		}
	})

	relevant, err := processor.fetchRelevantLearnings(ctx, learning.OrgID, repoID, []string{"prompt_builder.go"}, fmt.Sprintf("Need guidance %s", uniqueToken), "")
	require.NoError(t, err)
	require.NotEmpty(t, relevant, "expected seeded learning to be returned")

	found := false
	for _, item := range relevant {
		if item.ShortID == learning.ShortID {
			found = true
			break
		}
	}
	require.True(t, found, "seeded learning %s not present", learning.ShortID)

	prompt := "Base prompt body"
	combined := processor.appendLearningsToPrompt(prompt, relevant)
	require.Contains(t, combined, learning.ShortID)
	require.Contains(t, combined, learning.Title)
	require.Contains(t, combined, "=== Org learnings ===")

	t.Logf("Seeded learning short_id=%s org_id=%d repo_id=%s", learning.ShortID, learning.OrgID, learning.RepoID)
	t.Logf("Rendered prompt including learnings:\n%s", combined)
}
