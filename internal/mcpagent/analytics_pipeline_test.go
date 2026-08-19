package mcpagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/livereview/internal/database"
	"github.com/livereview/internal/logging"
	storageanalytics "github.com/livereview/storage/analytics"
	"github.com/tmc/langchaingo/llms"
)

// scriptedProvider replays canned model replies in order, so the pipeline can
// be exercised with the real guard, the real executor and the real database
// while only the model is faked. Everything that produces a number is real.
type scriptedProvider struct {
	replies []string
	calls   int
	prompts []string
}

func (p *scriptedProvider) Complete(_ context.Context, history []HistoryEntry, _ []llms.Tool, _ ...llms.CallOption) (string, TokenUsage, error) {
	if len(history) > 0 {
		if content, ok := history[len(history)-1]["content"].(string); ok {
			p.prompts = append(p.prompts, content)
		}
	}
	if p.calls >= len(p.replies) {
		return "", TokenUsage{}, fmt.Errorf("scripted provider exhausted after %d calls", p.calls)
	}
	reply := p.replies[p.calls]
	p.calls++
	return reply, TokenUsage{}, nil
}

func (p *scriptedProvider) Describe() string                       { return "scripted/test" }
func (p *scriptedProvider) FormatTools(_ []MCPToolDef) []llms.Tool { return nil }

func testAgent(t *testing.T, orgID int64, replies ...string) (*Agent, *scriptedProvider, *sql.DB) {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// The guard's table catalog is now auto-generated from the live dbctx
	// index (see livisql.CatalogFor/mcpagent.orgScopedColumns) rather than a
	// static Go list, so a real index has to exist for a real guard to
	// recognize any table at all. InitSchemaIndex is a sync.Once - later
	// calls across other tests in this package are no-ops.
	if dsn, err := database.LoadDatabaseURL(); err == nil {
		InitSchemaIndex(dsn)
	}
	// schemaIndex() itself only waits schemaIndexWaitTimeout (3s, sized for
	// interactive chat latency) - a first build against 56 tables can take
	// longer than that, so wait on the real Ready() channel here instead of
	// skipping on a slow but otherwise healthy build. schemaIdx is this
	// package's own unexported singleton (schema_index.go); accessible
	// directly since this test file lives in the same package.
	if schemaIdx != nil {
		select {
		case <-schemaIdx.Ready():
		case <-time.After(30 * time.Second):
		}
	}
	if schemaIndex() == nil {
		t.Skip("skipping: dbctx schema index not available")
	}

	prov := &scriptedProvider{replies: replies}
	agent := &Agent{
		provider:   prov,
		mcpSession: &MCPSession{OrgID: orgID, UserRole: "member", OrgName: "TestOrg"},
		maxSteps:   5,
	}
	agent.analytics = storageanalytics.NewAdHocStore(db)
	return agent, prov, db
}

// orgWithCompletedReviews picks an org that actually has completed reviews so
// the assertions compare against real data.
func orgWithCompletedReviews(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var orgID int64
	err := db.QueryRow(`SELECT org_id FROM reviews WHERE status = 'completed'
		GROUP BY org_id ORDER BY count(*) DESC LIMIT 1`).Scan(&orgID)
	if err != nil {
		t.Skipf("skipping: no completed reviews available: %v", err)
	}
	return orgID
}

// The original bug, end to end: the model asks for monthly counts and the
// numbers in the rendered chart must equal what the database holds. The old
// path answered 51 and 16 where the database held 55 and 18, because it counted
// rows itself. Here it never sees a row.
func TestAnalyticsPipelineProducesExactCounts(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS m FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1) t`, orgID)
	dataSQL := fmt.Sprintf(`SELECT to_char(date_trunc('month', completed_at), 'YYYY-MM') AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1 ORDER BY 1`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		fmt.Sprintf(`{"analytics_plan":[{"id":"r1","question":"reviews per month","count_sql":%q}]}`, countSQL),
		fmt.Sprintf(`{"response_type":"chart","title":"Reviews by Month","description":"d","query":"q","data_sql":%q,"mark":"bar","encoding":{"x":{"field":"month","type":"ordinal"},"y":{"field":"review_count","type":"quantitative"}}}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan, ok := parseAnalyticsPlan(agent.provider.(*scriptedProvider).replies[0])
	if !ok {
		t.Fatal("test fixture is not a valid plan")
	}
	// Consume the plan reply so the finalize reply is next.
	agent.provider.(*scriptedProvider).calls = 1

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "how many reviews per month?", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected a chart, got %d artifacts", len(artifacts))
	}

	// Ground truth straight from the database.
	want := map[string]int64{}
	rows, err := db.Query(`SELECT to_char(date_trunc('month', completed_at), 'YYYY-MM'), count(*)
		FROM reviews WHERE status = 'completed' AND org_id = $1 GROUP BY 1`, orgID)
	if err != nil {
		t.Fatalf("ground truth query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var month string
		var n int64
		if err := rows.Scan(&month, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		want[month] = n
	}

	var payload struct {
		Reports []struct {
			Spec struct {
				Data struct {
					Values []map[string]any `json:"values"`
				} `json:"data"`
			} `json:"spec"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("response is not the expected reports envelope: %v\n%s", err, text)
	}
	if len(payload.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(payload.Reports))
	}

	got := map[string]int64{}
	for _, row := range payload.Reports[0].Spec.Data.Values {
		month, _ := row["month"].(string)
		n, _ := row["review_count"].(float64)
		got[month] = int64(n)
	}
	if len(got) != len(want) {
		t.Fatalf("month count mismatch: got %v want %v", got, want)
	}
	for month, n := range want {
		if got[month] != n {
			t.Fatalf("month %s: chart shows %d, database holds %d (full: got=%v want=%v)", month, got[month], n, got, want)
		}
	}
	t.Logf("verified %d months against the database: %v", len(got), got)
}

// Zero rows must produce a sentence, and must not run a data query at all.
func TestAnalyticsPipelineZeroRowsShortCircuits(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	// A predicate that cannot match anything.
	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM reviews WHERE status = 'completed' AND completed_at < '1971-01-01' AND org_id = %d`, orgID)
	prov := &scriptedProvider{replies: []string{
		`{"response_type":"no_data","text":"TestOrg completed no reviews in that period."}`,
	}}
	agent.provider = prov

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "reviews before 1971", CountSQL: countSQL}}

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "any reviews before 1971?", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(artifacts))
	}
	if strings.Contains(text, "\"reports\"") {
		t.Fatalf("zero rows produced a chart envelope: %s", text)
	}
	if !strings.Contains(text, "no reviews") {
		t.Fatalf("expected a no-data sentence, got: %s", text)
	}
	// Exactly one call: the no-data phrasing. No finalize, no data query.
	if prov.calls != 1 {
		t.Fatalf("expected 1 LLM call for the zero-row path, got %d", prov.calls)
	}
}

// A rejected query gets one repair attempt, and the budget is enforced.
func TestAnalyticsPipelineRepairsThenGivesUp(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	prov := &scriptedProvider{replies: []string{
		// repair attempt: still invalid
		`SELECT count(*) AS n FROM public.reviews`,
	}}
	agent.provider = prov

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "bad query", CountSQL: `SELECT count(*) FROM secret_table`}}

	text, _, _, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "q", "", clog)
	if err != nil {
		t.Fatalf("a failing report must not fail the turn: %v", err)
	}
	if !strings.Contains(text, "could not") {
		t.Fatalf("expected a graceful degradation message, got: %s", text)
	}
	// One repair attempt, then stop: the budget is 2 attempts total.
	if prov.calls != 1 {
		t.Fatalf("expected exactly 1 repair call, got %d", prov.calls)
	}
	if len(prov.prompts) > 0 && !strings.Contains(prov.prompts[0], "not available") {
		t.Logf("repair hint delivered: %s", prov.prompts[0])
	}
}

// A layered chart (trend + rolling average) must assemble a "layer" array in
// the spec rather than a flat mark/encoding pair, and survive normalization
// and the triviality check the same way a flat chart does.
func TestAnalyticsPipelineProducesLayeredChart(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS m FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1) t`, orgID)
	dataSQL := fmt.Sprintf(`SELECT to_char(date_trunc('month', completed_at), 'YYYY-MM') AS month, count(*) AS review_count, count(*) AS rolling_avg FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1 ORDER BY 1`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		fmt.Sprintf(`{"response_type":"chart","title":"Reviews by Month","description":"d","query":"q","data_sql":%q,
			"layer":[
				{"mark":"line","encoding":{"x":{"field":"month","type":"ordinal"},"y":{"field":"review_count","type":"quantitative"}}},
				{"mark":"line","encoding":{"x":{"field":"month","type":"ordinal"},"y":{"field":"rolling_avg","type":"quantitative"}}}
			]}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "reviews per month with rolling average", CountSQL: countSQL}}

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "reviews per month with rolling average", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected a chart, got %d artifacts", len(artifacts))
	}

	var payload struct {
		Reports []struct {
			Spec struct {
				Mark  string           `json:"mark"`
				Layer []map[string]any `json:"layer"`
			} `json:"spec"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("response is not the expected reports envelope: %v\n%s", err, text)
	}
	if len(payload.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(payload.Reports))
	}
	spec := payload.Reports[0].Spec
	if spec.Mark != "" {
		t.Fatalf("layered chart should not carry a top-level mark, got %q", spec.Mark)
	}
	if len(spec.Layer) != 2 {
		t.Fatalf("expected 2 layers in the spec, got %d: %s", len(spec.Layer), text)
	}
}

// A faceted chart (one small panel per author) must assemble a "facet"+"spec"
// pair in the rendered spec rather than a flat mark/encoding pair, and
// survive normalization the same way a flat or layered chart does.
func TestAnalyticsPipelineProducesFacetedChart(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(DISTINCT author_username) AS n FROM reviews WHERE status = 'completed' AND org_id = %d`, orgID)
	dataSQL := fmt.Sprintf(`SELECT author_username, repository, count(*) AS n FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1, 2`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		fmt.Sprintf(`{"response_type":"chart","title":"Reviews by Repository, per Author","description":"d","query":"q","data_sql":%q,
			"facet":{"field":"author_username","type":"nominal","columns":4},
			"spec":{"mark":"bar","encoding":{"x":{"field":"repository","type":"nominal"},"y":{"field":"n","type":"quantitative"}}}}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "reviews by repository, one panel per author", CountSQL: countSQL}}

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "reviews by repository, one panel per author", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected a chart, got %d artifacts", len(artifacts))
	}

	var payload struct {
		Reports []struct {
			Spec struct {
				Mark  string         `json:"mark"`
				Facet map[string]any `json:"facet"`
				Spec  map[string]any `json:"spec"`
			} `json:"spec"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("response is not the expected reports envelope: %v\n%s", err, text)
	}
	if len(payload.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(payload.Reports))
	}
	spec := payload.Reports[0].Spec
	if spec.Mark != "" {
		t.Fatalf("faceted chart should not carry a top-level mark, got %q", spec.Mark)
	}
	if spec.Facet == nil || spec.Spec == nil {
		t.Fatalf("expected both facet and spec in the rendered chart, got facet=%v spec=%v", spec.Facet, spec.Spec)
	}
}

// A layer referencing a field the query never selects must degrade to CSV,
// the same safety net a flat chart already has.
func TestAnalyticsPipelineLayeredChartMissingFieldFallsBackToCSV(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM reviews WHERE status = 'completed' AND org_id = %d`, orgID)
	dataSQL := fmt.Sprintf(`SELECT id, repository, status FROM reviews WHERE status = 'completed' AND org_id = %d ORDER BY id LIMIT 5`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		fmt.Sprintf(`{"response_type":"chart","title":"Bad layer","description":"d","query":"q","data_sql":%q,
			"layer":[{"mark":"line","encoding":{"x":{"field":"id","type":"ordinal"},"y":{"field":"does_not_exist","type":"quantitative"}}}]}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "bad layer field", CountSQL: countSQL}}

	_, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "bad layer field", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected the missing field to fall back to a CSV artifact, got %d artifacts", len(artifacts))
	}
}

// An unparseable finalize reply (e.g. truncated mid-JSON, as seen with
// Gemini's reasoning eating into its output budget) gets one repair
// attempt, the same budget the SQL phases already give a rejected
// statement, and must still produce a chart rather than giving up.
func TestAnalyticsPipelineFinalizeRepairsAfterBadJSON(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS m FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1) t`, orgID)
	dataSQL := fmt.Sprintf(`SELECT to_char(date_trunc('month', completed_at), 'YYYY-MM') AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' AND org_id = %d GROUP BY 1 ORDER BY 1`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		// attempt 1: cut off mid-object, as if the model's output budget ran out.
		`{"response_type": "chart", "title": "Reviews by Month", "description": "d", "query`,
		// attempt 2 (repair): valid.
		fmt.Sprintf(`{"response_type":"chart","title":"Reviews by Month","description":"d","query":"q","data_sql":%q,"mark":"bar","encoding":{"x":{"field":"month","type":"ordinal"},"y":{"field":"review_count","type":"quantitative"}}}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "reviews per month", CountSQL: countSQL}}

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "reviews per month", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected a chart, got %d artifacts", len(artifacts))
	}
	if !strings.Contains(text, "\"reports\"") {
		t.Fatalf("expected the repair attempt to still produce a chart, got: %s", text)
	}
}

// Two bad finalize replies in a row exhaust the repair budget and degrade
// gracefully rather than erroring the turn.
func TestAnalyticsPipelineFinalizeGivesUpAfterRepairFails(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM reviews WHERE status = 'completed' AND org_id = %d`, orgID)
	agent.provider = &scriptedProvider{replies: []string{
		`not json at all`,
		`still not json`,
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "reviews per month", CountSQL: countSQL}}

	text, _, _, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "reviews per month", "", clog)
	if err != nil {
		t.Fatalf("a failing report must not fail the turn: %v", err)
	}
	if !strings.Contains(text, "could not build the result") {
		t.Fatalf("expected a graceful degradation message, got: %s", text)
	}
}

// A CSV request produces a downloadable artifact rather than a chart.
func TestAnalyticsPipelineProducesCSV(t *testing.T) {
	agent, _, db := testAgent(t, 0)
	orgID := orgWithCompletedReviews(t, db)
	agent.mcpSession.OrgID = orgID

	countSQL := fmt.Sprintf(`SELECT count(*) AS n FROM reviews WHERE status = 'completed' AND org_id = %d`, orgID)
	dataSQL := fmt.Sprintf(`SELECT id, repository, status FROM reviews WHERE status = 'completed' AND org_id = %d ORDER BY id LIMIT 5`, orgID)

	agent.provider = &scriptedProvider{replies: []string{
		fmt.Sprintf(`{"response_type":"csv","title":"Completed reviews","description":"d","query":"q","data_sql":%q,"csv_filename":"completed.csv"}`, dataSQL),
	}}

	clog := logging.NewChatTurnLogger("test", "livi")
	plan := []PlanEntry{{ID: "r1", Question: "export completed reviews", CountSQL: countSQL}}

	text, _, artifacts, err := agent.runAnalyticsPlan(context.Background(), plan, nil, "export them", "", clog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 CSV artifact, got %d", len(artifacts))
	}
	// A CSV-only turn (no chart, no notes) must not also claim nothing was
	// found - the file itself, delivered separately, IS the answer. Regression
	// guard for the exact contradiction seen in production: a correctly
	// attached CSV alongside the chat text "I could not find anything to
	// answer that."
	if text != "" {
		t.Fatalf("expected empty response text alongside the CSV artifact, got %q", text)
	}
	art := artifacts[0]
	if art.Filename != "completed.csv" {
		t.Fatalf("unexpected filename %q", art.Filename)
	}
	header := strings.SplitN(string(art.Data), "\n", 2)[0]
	if header != "id,repository,status" {
		t.Fatalf("CSV header does not match the SELECT list: %q", header)
	}
	if art.Rows == 0 {
		t.Fatal("CSV reported zero rows")
	}
}
