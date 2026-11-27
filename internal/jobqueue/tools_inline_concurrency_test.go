package jobqueue

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/lib/pq"
	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/internal/toolclassifier"
	networktools "github.com/livereview/network/tools"
	"github.com/livereview/pkg/models"
	"go.uber.org/goleak"
)

type recordingToolEventSink struct {
	mu      sync.Mutex
	batches []struct {
		BatchID  string
		Status   string
		Comments []*models.ReviewComment
	}
	logs []string
}

func (s *recordingToolEventSink) EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error {
	return nil
}

func (s *recordingToolEventSink) EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, level+":"+message)
	return nil
}

func (s *recordingToolEventSink) EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var typed []*models.ReviewComment
	if raw, err := json.Marshal(comments); err == nil {
		_ = json.Unmarshal(raw, &typed)
	}
	s.batches = append(s.batches, struct {
		BatchID  string
		Status   string
		Comments []*models.ReviewComment
	}{BatchID: batchID, Status: status, Comments: typed})
	return nil
}

func (s *recordingToolEventSink) EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error {
	return nil
}

func (s *recordingToolEventSink) EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error {
	return nil
}

func setupInlineToolJobQueue(t *testing.T, cfg *QueueConfig, invokeFn toolInvokeFunc) (*JobQueue, *mockDBState) {
	t.Helper()
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() {
		goleak.VerifyNone(t, baseline)
	})
	mockDB, state := setupMockDB(t)
	return &JobQueue{
		db:                 mockDB,
		config:             cfg,
		toolInvokeOverride: invokeFn,
	}, state
}

func TestStartToolsAsync_ConcurrentExecution_NoLeaksAndRaceFree(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	var active atomic.Int32
	var maxActive atomic.Int32
	var totalInvocations atomic.Int32

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 3,
		MaxConcurrentGlobal:    2, // stricter than per-review to verify two-tier throttling
		ToolPhaseTimeout:       5 * time.Second,
		InvokeTimeout:          2 * time.Second,
		ClassifyTimeout:        2 * time.Second,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		cur := active.Add(1)
		for {
			prev := maxActive.Load()
			if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer active.Add(-1)
		totalInvocations.Add(1)

		// Verify shared read-only payload unmarshals intact concurrently.
		var body struct {
			ZipFile string `json:"zip_file"`
		}
		if err := json.Unmarshal(payload, &body); err != nil || body.ZipFile == "" {
			t.Errorf("invalid payload in concurrent invocation for %s: err=%v", arn, err)
		}

		time.Sleep(15 * time.Millisecond)

		switch {
		case strings.Contains(arn, "ruff"):
			return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":4,"rule":"F401","message":"unused import os"}]}`), nil
		case strings.Contains(arn, "gitleaks"):
			return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":8,"rule":"generic-api-key","message":"hardcoded key"}]}`), nil
		default:
			return []byte(`{"exit_code":0,"findings":[]}`), nil
		}
	})

	state.orgSettingsJSON = []byte(`{
		"ruff": {"enabled": true},
		"gitleaks": {"enabled": true},
		"semgrep": {"enabled": true},
		"eslint": {"enabled": true}
	}`)

	sink := &recordingToolEventSink{}
	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9101,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "app.py"}},
		EventSink:     sink,
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	if got := handle.DispatchedCount(); got != 4 {
		t.Fatalf("expected 4 tools dispatched, got %d", got)
	}
	if got := totalInvocations.Load(); got != 4 {
		t.Fatalf("expected 4 Lambda invocations, got %d", got)
	}
	if got := maxActive.Load(); got > 2 {
		t.Fatalf("expected max concurrency bounded by global cap 2, observed %d", got)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 classified comments (ruff + gitleaks), got %d", len(comments))
	}
	// Every tool (including 0-finding semgrep and eslint) must emit its batch event.
	sink.mu.Lock()
	batchCount := len(sink.batches)
	sink.mu.Unlock()
	if batchCount != 4 {
		t.Fatalf("expected 4 completed tool batch events (including 0-finding tools), got %d", batchCount)
	}
}

func TestStartToolsAsync_PanickingToolRecoveredAndIsolated(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 4,
		MaxConcurrentGlobal:    4,
		ToolPhaseTimeout:       5 * time.Second,
		InvokeTimeout:          1 * time.Second,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		if strings.Contains(arn, "gitleaks") {
			panic("simulated tool panic")
		}
		return []byte(`{"exit_code":1,"findings":[{"file":"main.py","line":2,"rule":"F401","message":"unused import"}]}`), nil
	})

	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)
	sink := &recordingToolEventSink{}

	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9102,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "main.py"}},
		EventSink:     sink,
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	if len(comments) != 1 || comments[0].ToolName != "ruff" {
		t.Fatalf("expected surviving ruff finding after gitleaks panic, got %+v", comments)
	}

	state.mu.Lock()
	resultEvents := append([]string(nil), state.resultEvents...)
	state.mu.Unlock()

	foundPanicResult := false
	for _, raw := range resultEvents {
		if strings.Contains(raw, `"tool_name":"gitleaks"`) && strings.Contains(raw, `"exit_code":-1`) && strings.Contains(raw, "internal panic") {
			foundPanicResult = true
		}
	}
	if !foundPanicResult {
		t.Fatalf("expected synthetic exit_code=-1 failure result for panicking tool, got results=%v", resultEvents)
	}
}

func TestStartToolsAsync_HungLambdaBoundedByInvokeTimeout(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 4,
		MaxConcurrentGlobal:    4,
		ToolPhaseTimeout:       3 * time.Second,
		InvokeTimeout:          50 * time.Millisecond,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		if strings.Contains(arn, "gitleaks") {
			attemptCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			<-attemptCtx.Done()
			return nil, &networktools.ToolInvokeError{
				Transient: ctx.Err() == nil,
				Message:   "http request failed",
				Err:       attemptCtx.Err(),
			}
		}
		return []byte(`{"exit_code":1,"findings":[{"file":"main.py","line":3,"rule":"F401","message":"unused import"}]}`), nil
	})

	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)

	start := time.Now()
	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9103,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "main.py"}},
		EventSink:     &recordingToolEventSink{},
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	elapsed := time.Since(start)
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("hung Lambda was not bounded by InvokeTimeout: took %v", elapsed)
	}
	if len(comments) != 1 || comments[0].ToolName != "ruff" {
		t.Fatalf("expected ruff comment despite hung gitleaks, got %+v", comments)
	}
}

func TestStartToolsAsync_TransientRetryAndZeroFindingBatchEvent(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	var attempts atomic.Int32
	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 2,
		MaxConcurrentGlobal:    2,
		ToolPhaseTimeout:       3 * time.Second,
		InvokeTimeout:          1 * time.Second,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             1,
		RetryDelay:             10 * time.Millisecond,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		n := attempts.Add(1)
		if n == 1 {
			return nil, &networktools.ToolInvokeError{
				StatusCode: http.StatusServiceUnavailable,
				Transient:  true,
				Message:    "http 503",
			}
		}
		return []byte(`{"exit_code":0,"findings":[]}`), nil
	})

	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}}`)
	sink := &recordingToolEventSink{}

	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9104,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "main.py"}},
		EventSink:     sink,
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(comments))
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts (1 transient retry), got %d", attempts.Load())
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.batches) != 1 || sink.batches[0].BatchID != "tool-ruff" || sink.batches[0].Status != "completed" {
		t.Fatalf("expected completed tool-ruff batch event even with 0 comments, got %+v", sink.batches)
	}
}

func TestStartToolsAsync_RetryHydrationSkipsSucceededAndRerunsFailed(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	var invokedARNs []string
	var invokeMu sync.Mutex

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 4,
		MaxConcurrentGlobal:    4,
		ToolPhaseTimeout:       3 * time.Second,
		InvokeTimeout:          1 * time.Second,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		invokeMu.Lock()
		invokedARNs = append(invokedARNs, arn)
		invokeMu.Unlock()
		return []byte(`{"exit_code":1,"findings":[{"file":"secret.py","line":10,"rule":"generic-api-key","message":"API key exposed"}]}`), nil
	})

	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)
	state.priorReviewEvents = [][]driver.Value{
		{"tool_dispatch", "", []byte(`{"tool_id":1,"tool_name":"ruff","status":"pending"}`)},
		{"tool_dispatch", "", []byte(`{"tool_id":2,"tool_name":"gitleaks","status":"pending"}`)},
		{"tool_result", "", []byte(`{"tool_name":"ruff","exit_code":1,"failure_reason":""}`)},
		{"tool_result", "", []byte(`{"tool_name":"gitleaks","exit_code":-1,"failure_reason":"lambda timeout"}`)},
		{"batch", "tool-ruff", []byte(`{"status":"completed","comments":[{"filePath":"old.py","line":5,"content":"cached ruff finding","severity":"warning","toolName":"ruff","source":"tool"}]}`)},
	}

	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9105,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "secret.py"}},
		EventSink:     &recordingToolEventSink{},
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	invokeMu.Lock()
	called := append([]string(nil), invokedARNs...)
	invokeMu.Unlock()

	if len(called) != 1 || !strings.Contains(called[0], "gitleaks") {
		t.Fatalf("expected only previously failed tool gitleaks to re-run, called=%v", called)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 merged comments (1 hydrated from ruff + 1 rerun from gitleaks), got %d: %+v", len(comments), comments)
	}
}

func TestQueueConfig_RescuerThresholdPinnedAndBoundsClamped(t *testing.T) {
	cfg := DefaultQueueConfig()
	if got, wantMin := cfg.EffectiveRescueStuckJobsAfter(), 2*cfg.EffectiveReviewJobTimeout(); got < wantMin {
		t.Fatalf("RescueStuckJobsAfter %v must be >= 2x ReviewJobTimeout %v", got, cfg.EffectiveReviewJobTimeout())
	}
	if got := cfg.EffectiveRescueStuckJobsAfter(); got <= cfg.EffectiveRiverJobTimeout() {
		t.Fatalf("RescueStuckJobsAfter %v must exceed client JobTimeout %v", got, cfg.EffectiveRiverJobTimeout())
	}

	zeroCfg := ToolsExecutionConfig{
		MaxConcurrentPerReview: 0,
		MaxConcurrentGlobal:    -5,
		ToolPhaseTimeout:       30 * time.Minute,
	}.Normalize(10 * time.Minute)
	if zeroCfg.MaxConcurrentPerReview < 1 {
		t.Fatalf("MaxConcurrentPerReview must clamp >= 1, got %d", zeroCfg.MaxConcurrentPerReview)
	}
	if zeroCfg.MaxConcurrentGlobal < 1 {
		t.Fatalf("MaxConcurrentGlobal must clamp >= 1, got %d", zeroCfg.MaxConcurrentGlobal)
	}
	if zeroCfg.ToolPhaseTimeout >= 10*time.Minute {
		t.Fatalf("ToolPhaseTimeout %v must stay below ReviewJobTimeout 10m", zeroCfg.ToolPhaseTimeout)
	}
}

func TestStartToolsAsync_PanicReleasesBothSemaphoreSlots(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 1, // Capacity 1: a leaked per-review or global slot deadlocks the second tool.
		MaxConcurrentGlobal:    1,
		ToolPhaseTimeout:       3 * time.Second,
		InvokeTimeout:          1 * time.Second,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             0,
	}

	var calls atomic.Int32
	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		if calls.Add(1) == 1 {
			panic("first tool panic must still release both semaphores")
		}
		return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":4,"rule":"ruff:F401","message":"unused import"}]}`), nil
	})
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)

	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9106,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "app.py"}},
		EventSink:     &recordingToolEventSink{},
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	if calls.Load() != 2 {
		t.Fatalf("expected second tool to run after first tool panic released capacity-1 slots, calls=%d", calls.Load())
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 finding from surviving tool, got %d", len(comments))
	}
}

func TestStartToolsAsync_HungClassifierBoundedByClassifyTimeout(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 2,
		MaxConcurrentGlobal:    2,
		ToolPhaseTimeout:       3 * time.Second,
		InvokeTimeout:          1 * time.Second,
		ClassifyTimeout:        50 * time.Millisecond,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		if strings.Contains(arn, "gitleaks") {
			return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":6,"rule":"unknown-rule","message":"hung classifier target"}]}`), nil
		}
		return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":2,"rule":"F401","message":"unused import"}]}`), nil
	})
	jq.toolClassifyOverride = func(ctx context.Context, db *sql.DB, orgID int64, planCode license.PlanType, toolName string, findings []toolclassifier.RawToolFinding, logger *logging.ReviewLogger) ([]*models.ReviewComment, error) {
		if toolName == "gitleaks" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return toolclassifier.ClassifyToolResult(ctx, db, orgID, planCode, toolName, findings, logger)
	}
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)
	sink := &recordingToolEventSink{}

	start := time.Now()
	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9107,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "app.py"}},
		EventSink:     sink,
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	comments := handle.Wait()
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("hung classifier was not bounded by ClassifyTimeout: took %v", elapsed)
	}
	if len(comments) != 1 || comments[0].ToolName != "ruff" {
		t.Fatalf("expected ruff finding despite hung gitleaks classifier, got %+v", comments)
	}

	sink.mu.Lock()
	batchCount := len(sink.batches)
	sink.mu.Unlock()
	if batchCount != 2 {
		t.Fatalf("expected batch events emitted for both tools (including degraded hung classifier), got %d", batchCount)
	}
}

func TestStartToolsAsync_RealDB_Concurrent15ToolsAndRetryHydration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-DB inline concurrency test in -short mode")
	}
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB inline concurrency test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}

	ctx := context.Background()

	// Seed isolated org, billing state, 15 enabled tools, and review row.
	var orgID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO orgs (name) VALUES ('inline-sast-15tools-race-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM tool_credit_ledger WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM org_billing_state WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO org_billing_state (org_id, current_plan_code, billing_period_start, billing_period_end, tool_credits_used_month, tool_credits_limit_month)
		VALUES ($1, 'team_32usd', NOW() - interval '1 day', NOW() + interval '29 days', 0, 50000)
	`, orgID); err != nil {
		t.Fatalf("insert org_billing_state: %v", err)
	}

	orgTools := make(map[string]map[string]bool)
	var seededToolNames []string
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("racetool_%02d", i)
		seededToolNames = append(seededToolNames, name)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO available_tools (name, description, lambda_arn, multiplier, use_case, category)
			VALUES ($1, $2, $3, 1.0, 'test', 'SAST')
			ON CONFLICT (name) DO NOTHING
		`, name, "race test tool", "mock-lambda-"+name); err != nil {
			t.Fatalf("insert available_tool %s: %v", name, err)
		}
		orgTools[name] = map[string]bool{"enabled": true}
	}
	t.Cleanup(func() {
		for _, name := range seededToolNames {
			_, _ = db.Exec(`DELETE FROM available_tools WHERE name = $1`, name)
		}
	})

	settingsJSON, _ := json.Marshal(orgTools)
	if _, err := db.ExecContext(ctx, `UPDATE orgs SET settings = $1 WHERE id = $2`, settingsJSON, orgID); err != nil {
		t.Fatalf("update orgs settings: %v", err)
	}

	var reviewID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO reviews (repository, status, org_id, metadata)
		VALUES ('acme/race-15tools', 'in_progress', $1, '{}'::jsonb) RETURNING id
	`, orgID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}

	// Only include the 15 race test tools in policy TOML so org baseline doesn't pull other catalog rows.
	var policyBuf strings.Builder
	for _, name := range seededToolNames {
		fmt.Fprintf(&policyBuf, "[%s]\nenabled = true\n", name)
	}
	// Disable any existing real catalog tools not in our test set by leaving orgSettings empty and enabling via .lrc/policy/tools.toml.
	if _, err := db.ExecContext(ctx, `UPDATE orgs SET settings = '{}'::jsonb WHERE id = $1`, orgID); err != nil {
		t.Fatalf("clear orgs settings: %v", err)
	}

	var failOnce sync.Map
	var firstRunCalls atomic.Int32
	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 15,
		MaxConcurrentGlobal:    30,
		ToolPhaseTimeout:       15 * time.Second,
		InvokeTimeout:          2 * time.Second,
		ClassifyTimeout:        2 * time.Second,
		MaxRetries:             0,
	}

	jq := &JobQueue{
		db:     db,
		config: cfg,
		toolInvokeOverride: func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
			firstRunCalls.Add(1)
			// Simulate transient-less fatal failure on racetool_15 during attempt 1 so retry re-runs ONLY racetool_15.
			if strings.HasSuffix(arn, "racetool_15") {
				if _, loaded := failOnce.LoadOrStore("failed", true); !loaded {
					return nil, &networktools.ToolInvokeError{StatusCode: 400, Transient: false, Message: "simulated attempt-1 failure"}
				}
			}
			// Known deterministic rule so ClassifyToolResult does not require live LLM keys.
			return []byte(`{"exit_code":1,"findings":[{"file":"main.py","line":1,"rule":"ruff:f401","message":"unused import"}]}`), nil
		},
	}

	params := ToolDispatchParams{
		OrgID:         orgID,
		ReviewID:      reviewID,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "main.py"}},
		EventSink:     reviewprocessor.NewDatabaseEventSink(db),
	}
	params.LRCBundle.Files = map[string][]byte{
		"policy/tools.toml": []byte(policyBuf.String()),
	}

	// Attempt 1: 14 succeed, racetool_15 records synthetic failure (exit_code=-1).
	h1, err := jq.StartToolsAsync(ctx, params)
	if err != nil {
		t.Fatalf("attempt 1 StartToolsAsync failed: %v", err)
	}
	comments1 := h1.Wait()
	h1.Cancel()
	if firstRunCalls.Load() != 15 {
		t.Fatalf("expected 15 Lambda calls on attempt 1, got %d", firstRunCalls.Load())
	}
	if len(comments1) != 14 {
		t.Fatalf("expected 14 comments on attempt 1, got %d", len(comments1))
	}

	// Attempt 2 (retry): 14 succeeded tools hydrate from Postgres review_events without Lambda; ONLY racetool_15 re-runs!
	firstRunCalls.Store(0)
	h2, err := jq.StartToolsAsync(ctx, params)
	if err != nil {
		t.Fatalf("attempt 2 StartToolsAsync failed: %v", err)
	}
	comments2 := h2.Wait()
	h2.Cancel()
	if firstRunCalls.Load() != 1 {
		t.Fatalf("expected ONLY 1 failed tool to re-run on retry (14 hydrated from DB), got %d Lambda calls", firstRunCalls.Load())
	}
	if len(comments2) != 15 {
		t.Fatalf("expected all 15 comments (14 hydrated + 1 rerun) on retry, got %d", len(comments2))
	}
}

type notifyingToolEventSink struct {
	recordingToolEventSink
	onBatch chan string
}

func (s *notifyingToolEventSink) EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error {
	_ = s.recordingToolEventSink.EmitBatchEvent(ctx, reviewID, orgID, batchID, status, tokenEstimate, fileCount, comments)
	if s.onBatch != nil {
		s.onBatch <- batchID
	}
	return nil
}

func TestStartToolsAsync_FastToolEmitsUIBatchBeforeSlowToolCompletes(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	slowGate := make(chan struct{})
	batchReady := make(chan string, 2)

	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 4,
		MaxConcurrentGlobal:    4,
		ToolPhaseTimeout:       5 * time.Second,
		InvokeTimeout:          2 * time.Second,
		ClassifyTimeout:        1 * time.Second,
		MaxRetries:             0,
	}

	jq, state := setupInlineToolJobQueue(t, cfg, func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
		if strings.Contains(arn, "gitleaks") {
			select {
			case <-slowGate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":8,"rule":"generic-api-key","message":"hardcoded key"}]}`), nil
		}
		return []byte(`{"exit_code":1,"findings":[{"file":"app.py","line":2,"rule":"F401","message":"unused import"}]}`), nil
	})
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)
	sink := &notifyingToolEventSink{onBatch: batchReady}

	handle, err := jq.StartToolsAsync(context.Background(), ToolDispatchParams{
		OrgID:         1,
		ReviewID:      9108,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "app.py"}},
		EventSink:     sink,
	})
	if err != nil {
		t.Fatalf("StartToolsAsync failed: %v", err)
	}
	defer handle.Cancel()

	// Fast tool (ruff) MUST stream its UI batch event BEFORE slow tool (gitleaks) finishes.
	select {
	case batchID := <-batchReady:
		if batchID != "tool-ruff" {
			t.Fatalf("expected fast tool batch tool-ruff first, got %s", batchID)
		}
	case <-time.After(2 * time.Second):
		close(slowGate)
		t.Fatal("timed out waiting for fast tool-ruff UI batch while slow tool gitleaks was blocked")
	}

	close(slowGate)

	comments := handle.Wait()
	if len(comments) != 2 {
		t.Fatalf("expected 2 total tool comments after Wait, got %d", len(comments))
	}
}

func TestLoad_10ConcurrentReviews15Tools_DBPoolAndLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-DB load test in -short mode")
	}
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")

	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-DB inline concurrency test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	// Match production database.NewDB() connection pool limits.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}

	ctx := context.Background()

	var orgID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO orgs (name, settings) VALUES ('inline-sast-loadtest-org', '{}'::jsonb) RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	const numReviews = 10
	const numTools = 15

	var seededToolNames []string
	for i := 1; i <= numTools; i++ {
		name := fmt.Sprintf("loadtool_%02d", i)
		seededToolNames = append(seededToolNames, name)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO available_tools (name, description, lambda_arn, multiplier, use_case, category)
			VALUES ($1, $2, $3, 1.0, 'test', 'SAST')
			ON CONFLICT (name) DO NOTHING
		`, name, "load test tool", "mock-lambda-"+name); err != nil {
			t.Fatalf("insert available_tool %s: %v", name, err)
		}
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM tool_credit_ledger WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM org_billing_state WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
		for _, name := range seededToolNames {
			_, _ = db.Exec(`DELETE FROM available_tools WHERE name = $1`, name)
		}
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO org_billing_state (org_id, current_plan_code, billing_period_start, billing_period_end, tool_credits_used_month, tool_credits_limit_month)
		VALUES ($1, 'team_32usd', NOW() - interval '1 day', NOW() + interval '29 days', 0, 50000)
	`, orgID); err != nil {
		t.Fatalf("insert org_billing_state: %v", err)
	}

	var policyBuf strings.Builder
	for _, name := range seededToolNames {
		fmt.Fprintf(&policyBuf, "[%s]\nenabled = true\n", name)
	}
	policyBytes := []byte(policyBuf.String())

	reviewIDs := make([]int64, numReviews)
	for i := 0; i < numReviews; i++ {
		if err := db.QueryRowContext(ctx, `
			INSERT INTO reviews (repository, status, org_id, metadata)
			VALUES ($1, 'in_progress', $2, '{}'::jsonb) RETURNING id
		`, fmt.Sprintf("acme/loadtest-%02d", i+1), orgID).Scan(&reviewIDs[i]); err != nil {
			t.Fatalf("insert review %d: %v", i, err)
		}
	}

	var activeCalls, peakCalls, totalCalls atomic.Int32
	cfg := DefaultQueueConfig()
	cfg.ToolsExecution = ToolsExecutionConfig{
		MaxConcurrentPerReview: 15,
		MaxConcurrentGlobal:    30,
		ToolPhaseTimeout:       30 * time.Second,
		InvokeTimeout:          5 * time.Second,
		ClassifyTimeout:        5 * time.Second,
		MaxRetries:             0,
	}

	jq := &JobQueue{
		db:     db,
		config: cfg,
		toolInvokeOverride: func(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
			cur := activeCalls.Add(1)
			defer activeCalls.Add(-1)
			for {
				prev := peakCalls.Load()
				if cur <= prev || peakCalls.CompareAndSwap(prev, cur) {
					break
				}
			}
			totalCalls.Add(1)
			select {
			case <-time.After(25 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(`{"exit_code":1,"findings":[{"file":"main.py","line":1,"rule":"ruff:f401","message":"unused import"}]}`), nil
		},
	}

	statsBefore := db.Stats()
	latencies := make([]time.Duration, numReviews)
	errs := make([]error, numReviews)
	commentCounts := make([]int, numReviews)

	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	var wg sync.WaitGroup
	for i := 0; i < numReviews; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			startBarrier.Wait()
			t0 := time.Now()
			params := ToolDispatchParams{
				OrgID:         orgID,
				ReviewID:      reviewIDs[i],
				PlanCode:      string(license.PlanTeam),
				DiffZipBase64: "UEsDBBQAAAAIA...",
				LocalDiffs:    []lib.LocalCodeDiff{{NewPath: "main.py"}},
				EventSink:     reviewprocessor.NewDatabaseEventSink(db),
			}
			params.LRCBundle.Files = map[string][]byte{
				"policy/tools.toml": policyBytes,
			}
			h, err := jq.StartToolsAsync(ctx, params)
			if err != nil {
				errs[i] = err
				return
			}
			comments := h.Wait()
			latencies[i] = time.Since(t0)
			commentCounts[i] = len(comments)
			h.Cancel()
		}()
	}

	loadStart := time.Now()
	startBarrier.Done()
	wg.Wait()
	wallElapsed := time.Since(loadStart)
	statsAfter := db.Stats()

	for i := 0; i < numReviews; i++ {
		if errs[i] != nil {
			t.Fatalf("review %d StartToolsAsync failed: %v", i, errs[i])
		}
		if commentCounts[i] != numTools {
			t.Fatalf("review %d expected %d comments, got %d", i, numTools, commentCounts[i])
		}
	}
	if totalCalls.Load() != numReviews*numTools {
		t.Fatalf("expected %d total mock tool invocations, got %d", numReviews*numTools, totalCalls.Load())
	}
	if peakCalls.Load() > 30 {
		t.Fatalf("global semaphore exceeded cap 30: peak=%d", peakCalls.Load())
	}

	var totalLat, minLat, maxLat time.Duration
	for i, d := range latencies {
		totalLat += d
		if i == 0 || d < minLat {
			minLat = d
		}
		if d > maxLat {
			maxLat = d
		}
	}
	waitCountDelta := statsAfter.WaitCount - statsBefore.WaitCount
	waitDurDelta := statsAfter.WaitDuration - statsBefore.WaitDuration
	t.Logf("10x15 load test: totalCalls=%d peakGlobalConcurrency=%d wall=%s wait(min=%s avg=%s max=%s) db.Stats(MaxOpen=%d Open=%d WaitCountDelta=%d WaitDurationDelta=%s)",
		totalCalls.Load(),
		peakCalls.Load(),
		wallElapsed,
		minLat,
		totalLat/time.Duration(numReviews),
		maxLat,
		statsAfter.MaxOpenConnections,
		statsAfter.OpenConnections,
		waitCountDelta,
		waitDurDelta,
	)
}
