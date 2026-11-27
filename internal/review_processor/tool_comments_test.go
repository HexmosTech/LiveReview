package reviewprocessor

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livereview/internal/providers"
	reviewpkg "github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
	"go.uber.org/goleak"
)

type rpMockDBState struct {
	mu           sync.Mutex
	postedTools  map[string]bool
	skippedTools map[string]bool
	reviewStatus string
	reviewResult []byte
	eventTypes   []string
}

type rpMockDriver struct {
	mu    sync.Mutex
	state *rpMockDBState
}

func (d *rpMockDriver) Open(name string) (driver.Conn, error) {
	return &rpMockConn{driver: d}, nil
}

type rpMockConn struct {
	driver *rpMockDriver
}

func (c *rpMockConn) Prepare(query string) (driver.Stmt, error) {
	return &rpMockStmt{conn: c, query: query}, nil
}
func (c *rpMockConn) Close() error              { return nil }
func (c *rpMockConn) Begin() (driver.Tx, error) { return &rpMockTx{}, nil }

type rpMockTx struct{}

func (t *rpMockTx) Commit() error   { return nil }
func (t *rpMockTx) Rollback() error { return nil }

type rpMockRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *rpMockRows) Columns() []string { return r.cols }
func (r *rpMockRows) Close() error     { return nil }
func (r *rpMockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

type rpMockResult struct{ rows int64 }

func (r rpMockResult) LastInsertId() (int64, error) { return 0, nil }
func (r rpMockResult) RowsAffected() (int64, error) { return r.rows, nil }

func (c *rpMockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.driver.mu.Lock()
	state := c.driver.state
	c.driver.mu.Unlock()
	state.mu.Lock()
	defer state.mu.Unlock()

	if strings.Contains(query, "tools_posted_to_provider") {
		rawPosted, _ := json.Marshal(state.postedTools)
		if strings.Contains(query, "tools_skipped_on_provider") {
			rawSkipped, _ := json.Marshal(state.skippedTools)
			return &rpMockRows{
				cols: []string{"posted", "skipped"},
				rows: [][]driver.Value{{rawPosted, rawSkipped}},
			}, nil
		}
		return &rpMockRows{
			cols: []string{"coalesce"},
			rows: [][]driver.Value{{rawPosted}},
		}, nil
	}
	if strings.Contains(query, "UPDATE reviews SET status =") && len(args) > 0 {
		state.reviewStatus = fmt.Sprint(args[0].Value)
		return &rpMockRows{
			cols: []string{"org_id"},
			rows: [][]driver.Value{{int64(1)}},
		}, nil
	}
	return &rpMockRows{cols: []string{"id"}, rows: nil}, nil
}

func (c *rpMockConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.driver.mu.Lock()
	state := c.driver.state
	c.driver.mu.Unlock()
	state.mu.Lock()
	defer state.mu.Unlock()

	if (strings.Contains(query, "tools_posted_to_provider") || strings.Contains(query, "ARRAY[$1::text]")) && len(args) >= 2 {
		sectionKey, _ := args[0].Value.(string)
		toolKey, _ := args[1].Value.(string)
		switch sectionKey {
		case "tools_skipped_on_provider":
			if toolKey != "" {
				state.skippedTools[toolKey] = true
			}
			return rpMockResult{rows: 1}, nil
		case "tools_posted_to_provider":
			if toolKey != "" {
				state.postedTools[toolKey] = true
			}
			return rpMockResult{rows: 1}, nil
		}
	}
	if strings.Contains(query, "tools_posted_to_provider") && len(args) > 0 {
		if toolKey, ok := args[0].Value.(string); ok {
			state.postedTools[toolKey] = true
		}
		return rpMockResult{rows: 1}, nil
	}
	if strings.Contains(query, "INSERT INTO public.review_events") {
		for _, a := range args {
			if a.Ordinal == 4 {
				if s, ok := a.Value.(string); ok {
					state.eventTypes = append(state.eventTypes, s)
				}
			}
		}
		return rpMockResult{rows: 1}, nil
	}
	if strings.Contains(query, "UPDATE public.reviews") && len(args) > 0 {
		var arg0Str string
		switch v := args[0].Value.(type) {
		case []byte:
			arg0Str = string(v)
		case string:
			arg0Str = v
		}
		if strings.Contains(arg0Str, "review_result") {
			state.reviewResult = []byte(arg0Str)
		}
		if strings.Contains(query, "status =") && len(args) >= 3 {
			state.reviewStatus = fmt.Sprint(args[2].Value)
		}
		return rpMockResult{rows: 1}, nil
	}
	return rpMockResult{rows: 1}, nil
}

type rpMockStmt struct {
	conn  *rpMockConn
	query string
}

func (s *rpMockStmt) Close() error  { return nil }
func (s *rpMockStmt) NumInput() int { return -1 }
func (s *rpMockStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *rpMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

var (
	rpDriver   rpMockDriver
	rpDriverMu sync.Mutex
)

func init() {
	sql.Register("rp-toolcomments-mock", &rpDriver)
}

func setupRPMockDB(t *testing.T) (*sql.DB, *rpMockDBState) {
	t.Helper()
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() {
		goleak.VerifyNone(t, baseline)
	})
	rpDriverMu.Lock()
	state := &rpMockDBState{
		postedTools:  make(map[string]bool),
		skippedTools: make(map[string]bool),
	}
	rpDriver.mu.Lock()
	rpDriver.state = state
	rpDriver.mu.Unlock()

	db, err := sql.Open("rp-toolcomments-mock", "")
	if err != nil {
		rpDriverMu.Unlock()
		t.Fatalf("open rp mock db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		rpDriver.mu.Lock()
		rpDriver.state = nil
		rpDriver.mu.Unlock()
		rpDriverMu.Unlock()
	})
	return db, state
}

type stubToolCommentProvider struct {
	mu           sync.Mutex
	posted       []*models.ReviewComment
	rateLimitHit bool
}

func (p *stubToolCommentProvider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	return nil, nil
}
func (p *stubToolCommentProvider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	return nil, nil
}
func (p *stubToolCommentProvider) PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error {
	return nil
}
func (p *stubToolCommentProvider) Name() string                             { return "github" }
func (p *stubToolCommentProvider) Configure(config map[string]interface{}) error { return nil }
func (p *stubToolCommentProvider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.Contains(comment.Content, "422-line-not-in-diff") {
		return fmt.Errorf("GitHub line comment failed: 422 Unprocessable Entity")
	}
	if strings.Contains(comment.Content, "transient-fail-on-first-run") {
		return fmt.Errorf("500 temporary server error")
	}
	if strings.Contains(comment.Content, "trigger-rate-limit") && !p.rateLimitHit {
		p.rateLimitHit = true
		return &providers.RateLimitedError{
			Provider:   "github",
			RetryAfter: 10 * time.Millisecond,
		}
	}
	p.posted = append(p.posted, comment)
	return nil
}

func TestPostToolCommentsWithProvider_PerCommentMarkersPartialFailureAnd429Retry(t *testing.T) {
	db, state := setupRPMockDB(t)
	prov := &stubToolCommentProvider{}

	// Simulate partial batch failure: comment 1 (422 permanent) & comment 2 (succeeds after 429) get marked,
	// while comment 3 (transient failure) does NOT get marked and retries cleanly next run without duplicating comment 2.
	comments := []*models.ReviewComment{
		{ToolName: "ruff", RuleID: "E999", FilePath: "a.py", Line: 1, Content: "422-line-not-in-diff"},
		{ToolName: "ruff", RuleID: "F401", FilePath: "a.py", Line: 5, Content: "trigger-rate-limit"},
		{ToolName: "ruff", RuleID: "E501", FilePath: "a.py", Line: 8, Content: "transient-fail-on-first-run"},
		{ToolName: "gitleaks", RuleID: "generic-api-key", FilePath: "b.py", Line: 9, Content: "hardcoded key"},
	}

	cfg := ToolPostConfig{
		PostingID:   "org/repo/1",
		PacingDelay: 1 * time.Millisecond,
		MaxRetries:  1,
	}

	postToolCommentsWithProvider(context.Background(), db, 1, 100, comments, cfg, "github", prov, nil)

	prov.mu.Lock()
	firstRunCount := len(prov.posted)
	prov.mu.Unlock()
	if firstRunCount != 2 {
		t.Fatalf("expected 2 posted comments (ruff line 5 + gitleaks line 9; 422 skipped and line 8 failed), got %d", firstRunCount)
	}

	state.mu.Lock()
	ruffLine1Posted := state.postedTools[commentPostMarkerKey(comments[0])]
	ruffLine1Skipped := state.skippedTools[commentPostMarkerKey(comments[0])]
	ruffLine5Marked := state.postedTools[commentPostMarkerKey(comments[1])]
	ruffLine8Marked := state.postedTools[commentPostMarkerKey(comments[2])]
	gitleaksMarked := state.postedTools[commentPostMarkerKey(comments[3])]
	state.mu.Unlock()
	if ruffLine1Posted || !ruffLine1Skipped {
		t.Fatalf("expected permanent 422 comment marked skipped (not posted), got posted=%v skipped=%v", ruffLine1Posted, ruffLine1Skipped)
	}
	if !ruffLine5Marked || !gitleaksMarked {
		t.Fatalf("expected succeeded comments marked per-comment, got ruffL5=%v gitleaks=%v", ruffLine5Marked, gitleaksMarked)
	}
	if ruffLine8Marked {
		t.Fatalf("expected failed ruff line 8 NOT to be marked as posted")
	}

	// Retry: Tier-3 rewording of line 5 content with same RuleID must keep the exact same marker
	// and skip re-posting, while transient line 8 now succeeds.
	retryComments := []*models.ReviewComment{
		comments[0],
		{ToolName: "ruff", RuleID: "F401", FilePath: "./a.py", Line: 5, Content: "reworded by Tier-3 retry"},
		{ToolName: "ruff", RuleID: "E501", FilePath: "a.py", Line: 8, Content: "now-succeeds-on-retry"},
		comments[3],
	}
	postToolCommentsWithProvider(context.Background(), db, 1, 100, retryComments, cfg, "github", prov, nil)

	prov.mu.Lock()
	secondRunCount := len(prov.posted)
	prov.mu.Unlock()
	if secondRunCount != 3 {
		t.Fatalf("expected ONLY previously unposted comment posted on retry (total 3, not 5), got %d", secondRunCount)
	}
}

func TestCommentPostMarkerKey_StableAcrossContentRewording(t *testing.T) {
	first := &models.ReviewComment{
		ToolName: "Semgrep",
		RuleID:   "python.lang.security.sqli",
		FilePath: "./src/db/query.py",
		Line:     42,
		Content:  "Raw SQL string formatting",
	}
	reworded := &models.ReviewComment{
		ToolName: "semgrep",
		RuleID:   "PYTHON.LANG.SECURITY.SQLI",
		FilePath: "/src/db/query.py",
		Line:     42,
		Content:  "SQL injection risk (reclassified text on retry)",
	}
	differentLine := &models.ReviewComment{
		ToolName: "semgrep",
		RuleID:   "python.lang.security.sqli",
		FilePath: "src/db/query.py",
		Line:     43,
		Content:  "Raw SQL string formatting",
	}
	if k1, k2 := commentPostMarkerKey(first), commentPostMarkerKey(reworded); k1 != k2 {
		t.Fatalf("expected identical marker key across Tier-3 content rewording and path/case differences: %s != %s", k1, k2)
	}
	if commentPostMarkerKey(first) == commentPostMarkerKey(differentLine) {
		t.Fatalf("expected distinct marker keys for different line numbers")
	}
}

func TestClassifyCommentPostError_RealProviderShapes(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		err      error
		want     commentErrorKind
	}{
		{"github 422 permanent", "github", errors.New("GitHub line comment failed: 422 Unprocessable Entity"), commentErrPermanentSkip},
		{"gitlab 400 permanent", "gitlab", errors.New("API request failed with status 400: line_code can't be blank"), commentErrPermanentSkip},
		{"gitea 400 permanent", "gitea", errors.New("gitea POST failed with status 400: invalid line"), commentErrPermanentSkip},
		{"ado 400 permanent", "azuredevops", errors.New("azure devops POST failed with status 400: invalid threadContext"), commentErrPermanentSkip},
		{"github 403 auth abort", "github", errors.New("GitHub line comment failed: 403 Forbidden"), commentErrAuthAbort},
		{"gitlab 401 auth abort", "gitlab", errors.New("API request failed with status 401: Unauthorized"), commentErrAuthAbort},
		{"github 403 secondary rate limit transient", "github", errors.New("GitHub line comment failed: 403 Forbidden: You have exceeded a secondary rate limit"), commentErrTransient},
		{"typed RateLimitedError transient", "github", &providers.RateLimitedError{Provider: "github", RetryAfter: 5 * time.Second}, commentErrTransient},
		{"gitlab 429 transient", "gitlab", errors.New("API request failed with status 429: Too Many Requests"), commentErrTransient},
		{"gitlab 502 transient", "gitlab", errors.New("API request failed with status 502: Bad Gateway"), commentErrTransient},
		{"503 transient", "github", errors.New("503 Service Unavailable"), commentErrTransient},
		{"per-attempt deadline exceeded transient", "github", context.DeadlineExceeded, commentErrTransient},
	}

	parentCtx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCommentPostError(parentCtx, tc.err, tc.provider); got != tc.want {
				t.Fatalf("classifyCommentPostError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDedupeToolCommentsAgainstAI_ExcludesAILineOverlapBeforePosting(t *testing.T) {
	ai := []*models.ReviewComment{
		{FilePath: "a.py", Line: 10, Content: "AI flagged unused import", Source: "ai"},
	}
	tools := []*models.ReviewComment{
		{ToolName: "ruff", RuleID: "F401", FilePath: "./a.py", Line: 10, Content: "unused import", Source: "tool"},
		{ToolName: "bandit", RuleID: "B608", FilePath: "b.py", Line: 20, Content: "SQL injection", Source: "tool"},
	}
	forPosting, merged := DedupeToolCommentsAgainstAI(ai, tools)
	if len(forPosting) != 1 || forPosting[0].ToolName != "bandit" {
		t.Fatalf("expected AI-overlapped ruff comment filtered BEFORE PR posting, got: %+v", forPosting)
	}
	if len(merged) != 2 || merged[0].Source != "ai" || merged[1].ToolName != "bandit" {
		t.Fatalf("expected PR posting and stored review_result to match cleanly, got: %+v", merged)
	}
}

func TestMergeAndDedupeReviewComments(t *testing.T) {
	ai := []*models.ReviewComment{
		{FilePath: "a.py", Line: 10, Content: "AI finding", Source: "ai"},
		{FilePath: "b.py", Line: 20, Content: "Unused import", Source: "tool", ToolName: "ruff"},
	}
	tools := []*models.ReviewComment{
		{FilePath: "b.py", Line: 20, Content: " Unused import ", Source: "tool", ToolName: "RUFF"},
		{FilePath: "c.py", Line: 30, Content: "New tool finding", Source: "tool", ToolName: "gitleaks"},
	}
	merged := MergeAndDedupeReviewComments(ai, tools)
	if len(merged) != 3 {
		t.Fatalf("expected 3 deduplicated comments, got %d", len(merged))
	}
	if merged[0].Content != "AI finding" || merged[2].ToolName != "gitleaks" {
		t.Fatalf("unexpected merge order/contents: %+v", merged)
	}
}

func TestProcessManualReviewWithTools_AIFailureAwaitsAndPreservesRunningTools(t *testing.T) {
	db, state := setupRPMockDB(t)

	var waitFinished atomic.Bool
	doneCh := make(chan []*models.ReviewComment, 1)
	go func() {
		time.Sleep(35 * time.Millisecond)
		doneCh <- []*models.ReviewComment{
			{
				FilePath: "app.py",
				Line:     7,
				Content:  "unused import sys",
				Source:   "tool",
				ToolName: "ruff",
				RuleID:   "F401",
			},
		}
	}()

	req := reviewpkg.ReviewRequest{
		URL:      "", // triggers immediate AI review failure without network I/O
		ReviewID: "555",
		Provider: reviewpkg.ProviderConfig{Type: "github", URL: "https://github.com/org/repo"},
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	opts := &ManualToolsOptions{
		WaitForTools: func(ctx context.Context) []*models.ReviewComment {
			res := <-doneCh
			waitFinished.Store(true)
			return res
		},
		PostConfig: ToolPostConfig{
			ProviderConfig: &reviewpkg.ProviderConfig{Type: "github", URL: "https://github.com/org/repo"},
			PostingID:      "org/repo/555",
		},
	}

	err = ProcessManualReviewWithTools(context.Background(), db, 1, "team", nil, "u@example.com", 555, string(reqBytes), opts, nil)
	if err != nil {
		t.Fatalf("ProcessManualReviewWithTools returned error: %v", err)
	}
	if !waitFinished.Load() {
		t.Fatalf("expected WaitForTools to finish even after AI failure")
	}

	state.mu.Lock()
	status := state.reviewStatus
	metaRaw := string(state.reviewResult)
	postedCount := len(state.postedTools)
	skippedCount := len(state.skippedTools)
	state.mu.Unlock()

	if status != "failed" {
		t.Fatalf("expected review status 'failed' when AI failed, got %q", status)
	}
	if !strings.Contains(metaRaw, "unused import sys") {
		t.Fatalf("expected persisted review_result metadata to preserve inline tool findings on AI failure, got: %s", metaRaw)
	}
	if postedCount != 0 || skippedCount != 0 {
		t.Fatalf("expected NO PR comments posted or marked when AI review failed, got posted=%d skipped=%d", postedCount, skippedCount)
	}
}
