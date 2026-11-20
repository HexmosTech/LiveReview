package reviewprocessor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ComputeToolSummary computes a ToolSummary from rawToolEvent events, multiplierMap, and an optional reference time.
// This implements and tests the tool summary calculation logic used by ReviewEventsRepo.GetToolSummary.
func ComputeToolSummary(events []rawToolEvent, multiplierMap map[string]float64, now ...time.Time) *ToolSummary {
	if len(events) == 0 {
		return nil
	}

	currentTime := time.Now()
	if len(now) > 0 && !now[0].IsZero() {
		currentTime = now[0]
	}

	type toolDispatchData struct {
		ToolID   int64  `json:"tool_id"`
		ToolName string `json:"tool_name"`
		Status   string `json:"status"`
	}
	type toolFinding struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Message string `json:"message"`
	}
	type toolResultData struct {
		ToolID        int64         `json:"tool_id"`
		ToolName      string        `json:"tool_name"`
		ExitCode      int           `json:"exit_code"`
		Findings      []toolFinding `json:"findings"`
		LinesOfCode   int           `json:"lines_of_code"`
		Stderr        string        `json:"stderr"`
		Truncated     bool          `json:"truncated,omitempty"`
		FailureReason string        `json:"failure_reason,omitempty"`
	}

	dispatchedMap := make(map[string]time.Time)
	dispatchedOrder := make([]string, 0)
	resultsMap := make(map[string]toolResultData)

	for _, ev := range events {
		if ev.EventType == "tool_dispatch" {
			var d toolDispatchData
			if err := json.Unmarshal(ev.Data, &d); err == nil && d.ToolName != "" {
				if _, exists := dispatchedMap[d.ToolName]; !exists {
					dispatchedMap[d.ToolName] = ev.CreatedAt
					dispatchedOrder = append(dispatchedOrder, d.ToolName)
				}
			}
		} else if ev.EventType == "tool_result" {
			var res toolResultData
			if err := json.Unmarshal(ev.Data, &res); err == nil && res.ToolName != "" {
				resultsMap[res.ToolName] = res
				if _, exists := dispatchedMap[res.ToolName]; !exists {
					dispatchedMap[res.ToolName] = ev.CreatedAt
					dispatchedOrder = append(dispatchedOrder, res.ToolName)
				}
			}
		}
	}

	toolBreakdown := make([]ToolBreakdownItem, 0, len(dispatchedOrder))
	totalComments := 0
	toolsExecuted := 0
	totalCostUSD := 0.0

	for _, toolName := range dispatchedOrder {
		res, hasResult := resultsMap[toolName]
		dispatchTime := dispatchedMap[toolName]

		mult, exists := multiplierMap[strings.ToLower(toolName)]
		if !exists || mult <= 0 {
			mult = 1.0
		}
		totalCostUSD += mult

		item := ToolBreakdownItem{
			ToolName:    toolName,
			CreditsUsed: mult,
		}

		if hasResult {
			toolsExecuted++
			commentCount := len(res.Findings)
			item.CommentsGenerated = commentCount
			totalComments += commentCount

			if commentCount > 0 {
				if res.Truncated {
					item.Status = "partial"
				} else {
					item.Status = "completed"
				}
			} else if res.Truncated {
				item.Status = "partial"
			} else if res.ExitCode != 0 {
				item.Status = "failed"
			} else {
				item.Status = "clean"
			}
		} else {
			if currentTime.Sub(dispatchTime) > 10*time.Second {
				item.Status = "running"
			} else {
				item.Status = "pending"
			}
		}
		toolBreakdown = append(toolBreakdown, item)
	}

	return &ToolSummary{
		ToolsExecuted:          toolsExecuted,
		TotalCommentsGenerated: totalComments,
		TotalCostUsd:           &totalCostUSD,
		ToolBreakdown:          toolBreakdown,
	}
}

// ComputeToolSummary is an alias/wrapper for GetToolSummary on ReviewEventsRepo.
func (r *ReviewEventsRepo) ComputeToolSummary(ctx context.Context, reviewID, orgID int64) (*ToolSummary, error) {
	return r.GetToolSummary(ctx, reviewID, orgID)
}

// ComputeToolSummaryFromReviewEvents computes ToolSummary from a slice of ReviewEvent pointers.
func ComputeToolSummaryFromReviewEvents(events []*ReviewEvent, multiplierMap map[string]float64, now ...time.Time) *ToolSummary {
	rawEvents := make([]rawToolEvent, 0, len(events))
	for _, ev := range events {
		if ev != nil && (ev.EventType == "tool_dispatch" || ev.EventType == "tool_result") {
			rawEvents = append(rawEvents, rawToolEvent{
				EventType: ev.EventType,
				Data:      ev.Data,
				CreatedAt: ev.Timestamp,
			})
		}
	}
	return ComputeToolSummary(rawEvents, multiplierMap, now...)
}

func makeDispatchEvent(toolName string, t time.Time) rawToolEvent {
	data, _ := json.Marshal(map[string]interface{}{
		"tool_id":   1,
		"tool_name": toolName,
		"status":    "dispatched",
	})
	return rawToolEvent{
		EventType: "tool_dispatch",
		Data:      data,
		CreatedAt: t,
	}
}

func makeResultEvent(toolName string, exitCode int, findingsCount int, truncated bool, t time.Time) rawToolEvent {
	findings := make([]map[string]interface{}, findingsCount)
	for i := 0; i < findingsCount; i++ {
		findings[i] = map[string]interface{}{
			"file":    fmt.Sprintf("file_%d.go", i),
			"line":    i + 1,
			"message": fmt.Sprintf("Issue %d", i+1),
		}
	}
	data, _ := json.Marshal(map[string]interface{}{
		"tool_id":       1,
		"tool_name":     toolName,
		"exit_code":     exitCode,
		"findings":      findings,
		"lines_of_code": 100,
		"stderr":        "",
		"truncated":     truncated,
	})
	return rawToolEvent{
		EventType: "tool_result",
		Data:      data,
		CreatedAt: t,
	}
}

func testDatabaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
}

func TestComputeToolSummary(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

	t.Run("E1_ExitCode1_WithFindings_Completed", func(t *testing.T) {
		// E1: Tool with ExitCode=1 and 2 findings -> Status = "completed" (NOT "failed").
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-5*time.Second)),
			makeResultEvent("ruff", 1, 2, false, now.Add(-3*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 1, summary.ToolsExecuted)
		assert.Equal(t, 2, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "completed", summary.ToolBreakdown[0].Status, "Tool with exit code 1 and findings must be completed, NOT failed")
		assert.Equal(t, 2, summary.ToolBreakdown[0].CommentsGenerated)
	})

	t.Run("E2_ExitCode1_ZeroFindings_Failed", func(t *testing.T) {
		// E2: Tool with ExitCode=1 and 0 findings -> Status = "failed".
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-5*time.Second)),
			makeResultEvent("ruff", 1, 0, false, now.Add(-3*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 1, summary.ToolsExecuted)
		assert.Equal(t, 0, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "failed", summary.ToolBreakdown[0].Status, "Tool with exit code 1 and 0 findings must be failed")
		assert.Equal(t, 0, summary.ToolBreakdown[0].CommentsGenerated)
	})

	t.Run("E3_ExitCode0_ZeroFindings_Clean", func(t *testing.T) {
		// E3: Tool with ExitCode=0 and 0 findings -> Status = "clean".
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-5*time.Second)),
			makeResultEvent("ruff", 0, 0, false, now.Add(-3*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 1, summary.ToolsExecuted)
		assert.Equal(t, 0, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "clean", summary.ToolBreakdown[0].Status, "Tool with exit code 0 and 0 findings must be clean")
		assert.Equal(t, 0, summary.ToolBreakdown[0].CommentsGenerated)
	})

	t.Run("E4_Truncated_WithFindings_Partial", func(t *testing.T) {
		// E4: Tool with Truncated=true and findings -> Status = "partial".
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-5*time.Second)),
			makeResultEvent("ruff", 0, 3, true, now.Add(-3*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 1, summary.ToolsExecuted)
		assert.Equal(t, 3, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "partial", summary.ToolBreakdown[0].Status, "Tool with Truncated=true and findings must be partial")
		assert.Equal(t, 3, summary.ToolBreakdown[0].CommentsGenerated)
	})

	t.Run("E5_Truncated_ZeroFindings_Partial", func(t *testing.T) {
		// E5: Tool with Truncated=true and 0 findings -> Status = "partial".
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-5*time.Second)),
			makeResultEvent("ruff", 0, 0, true, now.Add(-3*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 1, summary.ToolsExecuted)
		assert.Equal(t, 0, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "partial", summary.ToolBreakdown[0].Status, "Tool with Truncated=true and 0 findings must be partial")
		assert.Equal(t, 0, summary.ToolBreakdown[0].CommentsGenerated)
	})

	t.Run("E6_Pending_DispatchedRecently", func(t *testing.T) {
		// E6: Tool pending vs running (based on dispatch time) -> pending (<= 10s)
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-4*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 0, summary.ToolsExecuted)
		assert.Equal(t, 0, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "pending", summary.ToolBreakdown[0].Status, "Tool dispatched <= 10s ago must be pending")
	})

	t.Run("E7_Running_DispatchedOver10sAgo", func(t *testing.T) {
		// E7: Tool running when dispatched more than 10s ago
		events := []rawToolEvent{
			makeDispatchEvent("ruff", now.Add(-15*time.Second)),
		}
		summary := ComputeToolSummary(events, nil, now)
		require.NotNil(t, summary)
		assert.Equal(t, 0, summary.ToolsExecuted)
		assert.Equal(t, 0, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "running", summary.ToolBreakdown[0].Status, "Tool dispatched > 10s ago must be running")
	})

	t.Run("Combined_MultiTool_Summary", func(t *testing.T) {
		// Comprehensive check with all statuses in one summary
		events := []rawToolEvent{
			makeDispatchEvent("tool1_e1", now.Add(-20*time.Second)),
			makeResultEvent("tool1_e1", 1, 2, false, now.Add(-18*time.Second)),

			makeDispatchEvent("tool2_e2", now.Add(-19*time.Second)),
			makeResultEvent("tool2_e2", 1, 0, false, now.Add(-17*time.Second)),

			makeDispatchEvent("tool3_e3", now.Add(-18*time.Second)),
			makeResultEvent("tool3_e3", 0, 0, false, now.Add(-16*time.Second)),

			makeDispatchEvent("tool4_e4", now.Add(-17*time.Second)),
			makeResultEvent("tool4_e4", 1, 5, true, now.Add(-15*time.Second)),

			makeDispatchEvent("tool5_e5", now.Add(-16*time.Second)),
			makeResultEvent("tool5_e5", 0, 0, true, now.Add(-14*time.Second)),

			makeDispatchEvent("tool6_pending", now.Add(-5*time.Second)),

			makeDispatchEvent("tool7_running", now.Add(-25*time.Second)),
		}

		multipliers := map[string]float64{
			"tool1_e1": 1.5,
			"tool2_e2": 2.0,
		}

		summary := ComputeToolSummary(events, multipliers, now)
		require.NotNil(t, summary)
		assert.Equal(t, 5, summary.ToolsExecuted)
		assert.Equal(t, 7, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 7)

		assert.Equal(t, "completed", summary.ToolBreakdown[0].Status)
		assert.Equal(t, 1.5, summary.ToolBreakdown[0].CreditsUsed)

		assert.Equal(t, "failed", summary.ToolBreakdown[1].Status)
		assert.Equal(t, 2.0, summary.ToolBreakdown[1].CreditsUsed)

		assert.Equal(t, "clean", summary.ToolBreakdown[2].Status)
		assert.Equal(t, 1.0, summary.ToolBreakdown[2].CreditsUsed)

		assert.Equal(t, "partial", summary.ToolBreakdown[3].Status)
		assert.Equal(t, 1.0, summary.ToolBreakdown[3].CreditsUsed)

		assert.Equal(t, "partial", summary.ToolBreakdown[4].Status)
		assert.Equal(t, 1.0, summary.ToolBreakdown[4].CreditsUsed)

		assert.Equal(t, "pending", summary.ToolBreakdown[5].Status)
		assert.Equal(t, 1.0, summary.ToolBreakdown[5].CreditsUsed)

		assert.Equal(t, "running", summary.ToolBreakdown[6].Status)
		assert.Equal(t, 1.0, summary.ToolBreakdown[6].CreditsUsed)
	})

	t.Run("DB_Integration_GetToolSummary", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping DB integration test in short mode")
		}

		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = testDatabaseURL()
		}

		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			t.Skipf("Skipping DB test: unable to open postgres connection: %v", err)
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			t.Skipf("Skipping DB test: database not pingable: %v", err)
		}

		orgID := int64(1)
		var reviewID int64
		err = db.QueryRowContext(ctx, `
			INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, "test/events-summary-repo", "main", "abc12345", "https://test.com/pr/summary", "created", "manual", "test@example.com", "test", orgID).Scan(&reviewID)
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM public.review_events WHERE review_id = $1", reviewID)
			_, _ = db.ExecContext(context.Background(), "DELETE FROM public.reviews WHERE id = $1", reviewID)
		})

		repo := NewReviewEventsRepo(db)

		// Insert DB events for E1-E7
		insertRaw := func(eventType string, data interface{}, createdAt time.Time) {
			b, mErr := json.Marshal(data)
			require.NoError(t, mErr)
			_, err = db.ExecContext(ctx, `
				INSERT INTO public.review_events (review_id, org_id, ts, event_type, data)
				VALUES ($1, $2, $3, $4, $5)
			`, reviewID, orgID, createdAt, eventType, b)
			require.NoError(t, err)
		}

		dbNow := time.Now()

		// E1: ExitCode=1, 2 findings -> completed
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 1, "tool_name": "db_tool1", "status": "dispatched"}, dbNow.Add(-60*time.Second))
		insertRaw("tool_result", map[string]interface{}{
			"tool_id":   1,
			"tool_name": "db_tool1",
			"exit_code": 1,
			"findings": []map[string]interface{}{
				{"file": "a.go", "line": 1, "message": "msg1"},
				{"file": "b.go", "line": 2, "message": "msg2"},
			},
			"lines_of_code": 50,
			"truncated":     false,
		}, dbNow.Add(-55*time.Second))

		// E2: ExitCode=1, 0 findings -> failed
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 2, "tool_name": "db_tool2", "status": "dispatched"}, dbNow.Add(-50*time.Second))
		insertRaw("tool_result", map[string]interface{}{
			"tool_id":       2,
			"tool_name":     "db_tool2",
			"exit_code":     1,
			"findings":      []map[string]interface{}{},
			"lines_of_code": 50,
			"truncated":     false,
		}, dbNow.Add(-45*time.Second))

		// E3: ExitCode=0, 0 findings -> clean
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 3, "tool_name": "db_tool3", "status": "dispatched"}, dbNow.Add(-40*time.Second))
		insertRaw("tool_result", map[string]interface{}{
			"tool_id":       3,
			"tool_name":     "db_tool3",
			"exit_code":     0,
			"findings":      []map[string]interface{}{},
			"lines_of_code": 50,
			"truncated":     false,
		}, dbNow.Add(-35*time.Second))

		// E4: Truncated=true, findings -> partial
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 4, "tool_name": "db_tool4", "status": "dispatched"}, dbNow.Add(-30*time.Second))
		insertRaw("tool_result", map[string]interface{}{
			"tool_id":   4,
			"tool_name": "db_tool4",
			"exit_code": 0,
			"findings": []map[string]interface{}{
				{"file": "c.go", "line": 10, "message": "msg3"},
			},
			"lines_of_code": 50,
			"truncated":     true,
		}, dbNow.Add(-25*time.Second))

		// E5: Truncated=true, 0 findings -> partial
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 5, "tool_name": "db_tool5", "status": "dispatched"}, dbNow.Add(-20*time.Second))
		insertRaw("tool_result", map[string]interface{}{
			"tool_id":       5,
			"tool_name":     "db_tool5",
			"exit_code":     0,
			"findings":      []map[string]interface{}{},
			"lines_of_code": 50,
			"truncated":     true,
		}, dbNow.Add(-15*time.Second))

		// E6: Tool pending (dispatched recently, <= 10s)
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 6, "tool_name": "db_tool6", "status": "dispatched"}, dbNow.Add(-2*time.Second))

		// E7: Tool running (dispatched > 10s ago)
		insertRaw("tool_dispatch", map[string]interface{}{"tool_id": 7, "tool_name": "db_tool7", "status": "dispatched"}, dbNow.Add(-30*time.Second))

		summary, err := repo.ComputeToolSummary(ctx, reviewID, orgID)
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, 5, summary.ToolsExecuted)
		assert.Equal(t, 3, summary.TotalCommentsGenerated)
		require.Len(t, summary.ToolBreakdown, 7)

		statusMap := make(map[string]string)
		for _, item := range summary.ToolBreakdown {
			statusMap[item.ToolName] = item.Status
		}

		assert.Equal(t, "completed", statusMap["db_tool1"], "E1 DB: ExitCode=1 with findings must be completed")
		assert.Equal(t, "failed", statusMap["db_tool2"], "E2 DB: ExitCode=1 with 0 findings must be failed")
		assert.Equal(t, "clean", statusMap["db_tool3"], "E3 DB: ExitCode=0 with 0 findings must be clean")
		assert.Equal(t, "partial", statusMap["db_tool4"], "E4 DB: Truncated with findings must be partial")
		assert.Equal(t, "partial", statusMap["db_tool5"], "E5 DB: Truncated with 0 findings must be partial")
		assert.Equal(t, "pending", statusMap["db_tool6"], "E6 DB: Dispatched <= 10s ago must be pending")
		assert.Equal(t, "running", statusMap["db_tool7"], "E7 DB: Dispatched > 10s ago must be running")
	})
}
