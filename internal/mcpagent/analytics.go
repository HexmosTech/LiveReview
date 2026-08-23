package mcpagent

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/livereview/internal/livisql"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/vlrender"
	storageanalytics "github.com/livereview/storage/analytics"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

// Bounds on one analytics turn. Every one of these exists because the
// alternative is unbounded: this codebase has already shipped a retry storm
// (see isAuthError), and a loop driven by model output is the easiest place to
// ship another.
const (
	// maxReportsPerTurn caps the fan-out. Each report costs an LLM call and two
	// database round trips.
	maxReportsPerTurn = 4

	// maxSQLAttempts is the budget per SQL slot: one original plus one repair.
	// Count and data SQL get separate budgets.
	maxSQLAttempts = 2

	// maxChartRows is the point past which a chart stops communicating. Beyond
	// it the model is asked to aggregate coarser or switch to CSV - never
	// silently truncated, because a clipped chart looks complete.
	maxChartRows = 500

	// maxCSVRows bounds an export.
	maxCSVRows = 5000

	// analyticsTurnTimeout bounds the whole fan-out regardless of per-query
	// timeouts, so a slow model cannot hold a request open indefinitely.
	analyticsTurnTimeout = 90 * time.Second
)

// AnalyticsEngine executes guard-rewritten SQL. Declared here rather than
// imported as a concrete type so the orchestration can be tested with a fake.
type AnalyticsEngine interface {
	Count(ctx context.Context, rewritten string) (int64, error)
	Query(ctx context.Context, rewritten string, maxRows int) (*storageanalytics.ResultSet, error)
}

// WithAnalytics enables the SQL analytics path. With a nil engine, or a session
// carrying no org id, behaviour is byte-identical to the tool-only agent.
//
// Enabling it precomputes the three branch-specific (prompt, tools) pairs
// call #0 dispatches between - see livi_analytics_plan.md's "Call #0"
// section and agent.go's Agent struct doc comment. The raw-row tools are
// withdrawn from the action branch's tool list at the same moment the
// count_query branch is taught to write SQL instead: leaving both available
// would let the model keep counting rows in its head, which is the bug this
// path exists to fix.
func (a *Agent) WithAnalytics(engine AnalyticsEngine) *Agent {
	a.analytics = engine
	if !a.analyticsEnabled() {
		return a
	}
	orgName, userName := a.mcpSession.OrgName, a.mcpSession.UserName
	tools := withoutRawRowTools(a.mcpSession.Tools)

	a.actionTools = a.provider.FormatTools(tools)
	a.actionPrompt = buildSystemPrompt(tools, orgName, userName)
	a.chatPrompt = buildPromptHeader(orgName, userName) + "\n\n" + chatOnlyInstructions

	// Load the alaws lawbook and render the analytics-specific prompts
	// from it. Each pipeline branch sees only the law sections relevant
	// to its stage.
	lb, err := buildLawbookPrompts(orgName, userName, a.mcpSession.OrgID)
	if err != nil {
		log.Fatal().Err(err).Msg("alaws lawbook failed to load")
	}
	a.classifyPrompt = lb.classify
	// Append tool names so the model can distinguish action (has tools)
	// from count_query/chat (no tools). The lawbook's classify chapter
	// governs the routing decision itself, but the model still needs to
	// see which tools exist to know when an action is possible.
	if len(tools) > 0 {
		var b strings.Builder
		b.WriteString(a.classifyPrompt)
		b.WriteString("\n\nAvailable tool names (arguments are not shown here):\n")
		for _, t := range tools {
			b.WriteString(fmt.Sprintf("- %s\n", t.Name))
		}
		a.classifyPrompt = b.String()
	}
	a.countQueryHead = lb.planHead
	a.countQueryTail = lb.planTail
	a.finalizeHead = lb.finalizeHead
	a.finalizeTail = lb.finalizeTail
	a.repairPrompt = lb.repair
	a.noDataPrompt = lb.noData
	a.describePrompt = lb.describe
	a.interpretSystem = lb.interpretSystem
	a.chartTypes = lb.chartTypes

	// Kept in sync for any caller still reading systemPrompt/providerTools
	// directly (there is none in this codebase today, but they remain
	// public-ish Agent state) - the action branch is the closest equivalent
	// to what those fields meant before this split.
	a.providerTools = a.actionTools
	a.systemPrompt = a.actionPrompt
	return a
}

// rawRowTools return bulk rows for the model to aggregate itself. They are the
// direct cause of the miscounting the SQL path replaces, so they are withheld
// once analytics is on. Single-record lookups stay: "tell me about review 42"
// is answered better by the REST tool than by SQL.
var rawRowTools = map[string]bool{
	"GET_api_v1_reviews":                  true,
	"GET_api_v1_billing_usage_members":    true,
	"GET_api_v1_billing_usage_operations": true,
	"GET_api_v1_billing_usage_summary":    true,
}

func withoutRawRowTools(tools []MCPToolDef) []MCPToolDef {
	out := make([]MCPToolDef, 0, len(tools))
	present := make(map[string]bool, len(tools))
	for _, t := range tools {
		present[t.Name] = true
		if rawRowTools[t.Name] {
			continue
		}
		out = append(out, t)
	}
	// A name that never appeared means an MCP route was renamed and this filter
	// has quietly stopped doing anything - exactly the kind of silent regression
	// that would let raw-row aggregation creep back in.
	for name := range rawRowTools {
		if !present[name] {
			log.Warn().Str("tool", name).
				Msg("raw-row tool filter references a tool the MCP server no longer exposes; the filter may be stale")
		}
	}
	return out
}

func (a *Agent) analyticsEnabled() bool {
	return a.analytics != nil && a.mcpSession != nil && a.mcpSession.OrgID != 0
}

// analyticsRole normalizes this session's role for logging only - it has no
// bearing on which tables/columns are visible (see livisql.CatalogFor).
// Unknown or unrecognized roles normalize to "member" so debug logs never
// show a raw, unnormalized value; this matters for the bots, which
// authenticate an organization rather than a user and so never carry a
// real role.
func (a *Agent) analyticsRole() livisql.Role {
	role := livisql.Role(strings.ToLower(strings.TrimSpace(a.mcpSession.UserRole)))
	switch role {
	case livisql.RoleOwner, livisql.RoleSuperAdmin:
	default:
		role = livisql.RoleMember
	}
	return role
}

// guard builds the SQL guard for this session's org.
func (a *Agent) guard() *livisql.Guard {
	return livisql.New(livisql.CatalogFor(allTableNames()), a.mcpSession.OrgID)
}

// finishedReport is one completed entry of a multi-report answer.
type finishedReport struct {
	report   *vlrender.VegaLiteReport // chart output, nil otherwise
	artifact *Artifact                // csv output, nil otherwise
	text     string                   // no_data / degraded output, empty otherwise
}

// runAnalyticsPlan executes every entry of the plan and assembles one answer.
//
// The returned string keeps the existing {"reports":[...]} contract so the web,
// Slack, Discord and Teams render paths need no changes; CSV files come back
// separately as artifacts because a string cannot carry them.
func (a *Agent) runAnalyticsPlan(
	ctx context.Context,
	plan []PlanEntry,
	history []HistoryEntry,
	userText string,
	schemaTableText string,
	clog *logging.ChatTurnLogger,
) (string, []HistoryEntry, []Artifact, error) {
	ctx, cancel := context.WithTimeout(ctx, analyticsTurnTimeout)
	defer cancel()

	truncatedPlan := false
	if len(plan) > maxReportsPerTurn {
		plan = plan[:maxReportsPerTurn]
		truncatedPlan = true
	}

	var (
		reports   []vlrender.VegaLiteReport
		artifacts []Artifact
		notes     []string
	)

	// Sequential on purpose: each report holds a pool connection and makes an
	// LLM call, and sequential failure semantics are far easier to reason about.
	// Bounded concurrency is a follow-up once latency is measured.
	for _, entry := range plan {
		done := a.runOneReport(ctx, entry, userText, schemaTableText, clog)
		switch {
		case done.report != nil:
			reports = append(reports, *done.report)
		case done.artifact != nil:
			artifacts = append(artifacts, *done.artifact)
		case done.text != "":
			notes = append(notes, done.text)
		}
	}

	if truncatedPlan {
		notes = append(notes, fmt.Sprintf("I answered the first %d parts of that question. Ask again for the rest.", maxReportsPerTurn))
	}

	responseText := assembleAnalyticsResponse(reports, notes, len(artifacts) > 0)
	clog.FinalResponse(responseText)

	history = append(history, HistoryEntry{"role": "assistant", "text": responseText})
	return responseText, history, artifacts, nil
}

// runOneReport takes a single plan entry from count through to rendered output.
// It never returns an error: one report failing must not fail the turn, so
// failures degrade to a sentence and the other reports still render.
func (a *Agent) runOneReport(
	ctx context.Context,
	entry PlanEntry,
	userText string,
	schemaTableText string,
	clog *logging.ChatTurnLogger,
) finishedReport {
	count, ok := a.runCountPhase(ctx, entry, clog)
	if !ok {
		return finishedReport{text: fmt.Sprintf("I could not work out how to answer %q. Try narrowing it to a date range.", entry.Question)}
	}

	// Zero rows is an answer, not a failure. Short-circuiting here also avoids
	// rendering an empty chart, which is what the old path did.
	if count == 0 {
		text := a.noDataText(ctx, entry, userText, clog)
		clog.ReportFinalized(entry.ID, ResponseTypeNoData, entry.Question, 0)
		return finishedReport{text: text}
	}

	final := a.runFinalizePhase(ctx, entry, userText, count, schemaTableText, clog)
	if final == nil {
		return finishedReport{text: fmt.Sprintf("I could not build the result for %q.", entry.Question)}
	}
	return a.materializeReport(ctx, entry, final, clog)
}

// runCountPhase validates and runs the count query, repairing once if the guard
// or the database rejects it.
func (a *Agent) runCountPhase(ctx context.Context, entry PlanEntry, clog *logging.ChatTurnLogger) (int64, bool) {
	sqlText := entry.CountSQL
	for attempt := 1; attempt <= maxSQLAttempts; attempt++ {
		clog.SQLGenerated(entry.ID, "count", attempt, sqlText)

		rewritten, err := a.guard().Rewrite(sqlText)
		if err != nil {
			clog.SQLRejected(entry.ID, "count", attempt, err.Error())
			if attempt == maxSQLAttempts {
				return 0, false
			}
			sqlText, err = a.repairSQL(ctx, 2, entry.ID, attempt, entry.Question, sqlText, hintFor(err), clog)
			if err != nil {
				return 0, false
			}
			continue
		}
		clog.SQLRewritten(entry.ID, "count", attempt, rewritten)

		start := time.Now()
		count, err := a.analytics.Count(ctx, rewritten)
		if err != nil {
			clog.SQLError(entry.ID, "count", attempt, time.Since(start), err)
			if attempt == maxSQLAttempts {
				return 0, false
			}
			sqlText, err = a.repairSQL(ctx, 2, entry.ID, attempt, entry.Question, sqlText,
				"The query failed to execute: "+err.Error()+" Rewrite it.", clog)
			if err != nil {
				return 0, false
			}
			continue
		}
		clog.SQLResult(entry.ID, "count", attempt, time.Since(start), 1, false)
		return count, true
	}
	return 0, false
}

// runFinalizePhase asks the model how to present the report now that the row
// count is known, then validates and runs the data query. An unparseable
// reply (invalid JSON, or - as seen with Gemini's reasoning eating into its
// output budget on a richer chart decision - a response cut off mid-object)
// gets one repair attempt, the same budget the SQL phases already give a
// rejected statement, instead of failing the report outright.
func (a *Agent) runFinalizePhase(
	ctx context.Context,
	entry PlanEntry,
	userText string,
	count int64,
	schemaTableText string,
	clog *logging.ChatTurnLogger,
) *FinalizePlan {
	base := fmt.Sprintf("Original question: %s\n\nThis report answers: %s\n\nA separate, earlier estimate query predicted roughly %d rows - this number is stale the moment your data_sql differs from the counting query below in any way (an added filter, a different grouping), and it is a row count, not a metric of anything. Never state it, or any number derived from it, in the description; only the rows your own data_sql actually returns are real.\n\nThe counting query used was:\n%s",
		userText, entry.Question, count, entry.CountSQL)
	user := base
	system := a.finalizePrompt(schemaTableText)

	for attempt := 1; attempt <= maxSQLAttempts; attempt++ {
		raw, err := a.completeOnce(ctx, clog, 3, "finalize", entry.ID, attempt, system, user)
		if err != nil {
			log.Error().Err(err).Str("report", entry.ID).Msg("analytics finalize call failed")
			return nil
		}
		plan, perr := parseFinalizePlan(raw)
		if perr == nil {
			// Deliberately NOT downgrading chart->csv here based on `count`.
			// `count` is a prediction from a separate, earlier LLM call
			// (the count phase) and is not reliable - it can undercount
			// (grouping wrong) or wildly overcount (an ungrouped total)
			// independently of how many rows the real answer has. Forcing
			// the decision here, before the actual data query has even
			// run, downgrades good charts to CSV based on bad guesses.
			// materializeReport makes this call for real, off the actual
			// fetched row count.
			return plan
		}
		clog.SQLRejected(entry.ID, "finalize", attempt, perr.Error())
		if attempt == maxSQLAttempts {
			return nil
		}
		user = fmt.Sprintf("%s\n\nYour previous reply was rejected: %s\n\nReply again with a single complete JSON object and nothing else - no prose, no markdown fence, no partial or truncated output.", base, perr.Error())
	}
	return nil
}

// materializeReport runs the data query and turns rows into a chart or a CSV.
func (a *Agent) materializeReport(
	ctx context.Context,
	entry PlanEntry,
	plan *FinalizePlan,
	clog *logging.ChatTurnLogger,
) finishedReport {
	if plan.ResponseType == ResponseTypeNoData {
		return finishedReport{text: strings.TrimSpace(plan.Text)}
	}

	// Always fetch with the generous CSV ceiling. The chart/CSV decision
	// happens below, after the real row count is known - not here, based
	// on a limit chosen from the (unreliable) count-phase prediction. A
	// chart-shaped answer that's actually small must not get truncated
	// down to maxChartRows before we've even seen it.
	maxRows := maxCSVRows

	sqlText := plan.DataSQL
	for attempt := 1; attempt <= maxSQLAttempts; attempt++ {
		clog.SQLGenerated(entry.ID, "data", attempt, sqlText)

		rewritten, err := a.guard().Rewrite(sqlText)
		if err != nil {
			clog.SQLRejected(entry.ID, "data", attempt, err.Error())
			if attempt == maxSQLAttempts {
				break
			}
			if sqlText, err = a.repairSQL(ctx, 3, entry.ID, attempt, entry.Question, sqlText, hintFor(err), clog); err != nil {
				break
			}
			continue
		}
		clog.SQLRewritten(entry.ID, "data", attempt, rewritten)

		start := time.Now()
		rs, err := a.analytics.Query(ctx, rewritten, maxRows)
		if err != nil {
			clog.SQLError(entry.ID, "data", attempt, time.Since(start), err)
			if attempt == maxSQLAttempts {
				break
			}
			if sqlText, err = a.repairSQL(ctx, 3, entry.ID, attempt, entry.Question, sqlText,
				"The query failed to execute: "+err.Error()+" Rewrite it.", clog); err != nil {
				break
			}
			continue
		}
		clog.SQLResult(entry.ID, "data", attempt, time.Since(start), len(rs.Rows), rs.Truncated)

		if len(rs.Rows) == 0 {
			clog.ReportFinalized(entry.ID, ResponseTypeNoData, plan.Title, 0)
			return finishedReport{text: fmt.Sprintf("I found no data for %q.", entry.Question)}
		}

		relabelTriggerTypeValues(rs.Rows)

		// The model does not get to decide that "too much data for a
		// chart" is fine, but the decision is made off the real,
		// just-fetched row count - not the count phase's prediction,
		// which can be wrong in either direction (see the comment in
		// runFinalizePhase).
		if plan.ResponseType == ResponseTypeChart && len(rs.Rows) > maxChartRows {
			plan.ResponseType = ResponseTypeCSV
		}
		if plan.ResponseType == ResponseTypeCSV || rs.Truncated {
			return a.buildCSVReport(ctx, entry, plan, rs, clog)
		}
		return a.buildChartReport(ctx, entry, plan, rs, clog)
	}

	return finishedReport{text: fmt.Sprintf("I could not fetch the data for %q.", entry.Question)}
}

// computeTimeRange derives the calendar window a result set actually covers,
// straight from the rows themselves, rather than trusting the model's own
// `time_range` text. The model has repeatedly copied a worked example's
// placeholder date range verbatim instead of stating the real one (see
// finalizing/response-shape.md and finalizing/output.md) - this makes that
// class of error structurally impossible for any report with a temporal
// column, by computing the answer instead of asking for it.
//
// coerce (storage/analytics/coerce.go) already turns every timestamp column
// into an RFC3339 string, so detection is just: find the column where every
// non-nil value across every row parses as RFC3339, preferring whichever
// such column has the most non-nil values (the real time axis, as opposed to
// an incidental text column that happens to parse). Returns "" if no column
// qualifies, in which case the caller falls back to the model's own text -
// some charts (e.g. a ranking with no date axis) have no time column at all.
func computeTimeRange(rs *storageanalytics.ResultSet) string {
	if rs == nil || len(rs.Rows) == 0 {
		return ""
	}

	bestCol := ""
	bestCount := 0
	var bestMin, bestMax time.Time

	for _, col := range rs.Columns {
		var colMin, colMax time.Time
		count := 0
		ok := true
		for _, row := range rs.Rows {
			v := row[col]
			if v == nil {
				continue
			}
			s, isStr := v.(string)
			if !isStr {
				ok = false
				break
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				ok = false
				break
			}
			if count == 0 || t.Before(colMin) {
				colMin = t
			}
			if count == 0 || t.After(colMax) {
				colMax = t
			}
			count++
		}
		if !ok || count == 0 {
			continue
		}
		if count > bestCount {
			bestCol, bestCount, bestMin, bestMax = col, count, colMin, colMax
		}
	}

	if bestCol == "" {
		return ""
	}
	// Humanized ("January 1, 2023"), matching the same date style the rest
	// of the response is already required to use (finalizing/describing.md,
	// action/response-format.md) - a bare "2023-01-01 to 2024-07-24" reads
	// like debug output, not something written for the person asking.
	const humanDate = "January 2, 2006"
	if bestMin.Format(humanDate) == bestMax.Format(humanDate) {
		return bestMin.Format(humanDate)
	}
	return fmt.Sprintf("%s to %s", bestMin.Format(humanDate), bestMax.Format(humanDate))
}

// describeFacts is the real, already-computed numeric summary of a query
// result, handed to the post-data description call so it has nothing left
// to guess. Never built from anything the model wrote - only from rs itself.
type describeFacts struct {
	Description string `json:"description"`
}

// columnStats is one numeric column's first/last/min/max across the actual
// rows fetched. "First"/"last" follow row order, which is already the
// chart's own ORDER BY - meaningful as "earliest"/"most recent" without
// needing to know which column is the time axis.
type columnStats struct {
	First, Last, Min, Max float64
}

// computeNumericFacts extracts every numeric column's first/last/min/max
// from the real result set, formatted as text for the describe prompt. Skips
// columns whose values are all identical (a constant like period_avg still
// gets through since first==last==min==max there, which is exactly the
// correct thing to state about a period average). Returns "" if the result
// has no numeric columns at all (e.g. every column is text/temporal).
func computeNumericFacts(rs *storageanalytics.ResultSet) string {
	if rs == nil || len(rs.Rows) == 0 {
		return ""
	}
	var lines []string
	for _, col := range rs.Columns {
		var stats columnStats
		count := 0
		ok := true
		for _, row := range rs.Rows {
			v := row[col]
			if v == nil {
				continue
			}
			// coerce (storage/analytics/coerce.go) hands back different Go
			// types depending on the Postgres type: count(*)/bigint columns
			// arrive as int64 (its default case), while avg()/NUMERIC
			// columns arrive as float64 (NUMERIC goes through coerceBytes,
			// which parses to float64). A version of this that only handled
			// float64 silently dropped every raw count column from the
			// facts block, leaving the description call with rolling
			// averages but not the counts they're averaging.
			var f float64
			switch n := v.(type) {
			case float64:
				f = n
			case int64:
				f = float64(n)
			case int32:
				f = float64(n)
			case int:
				f = float64(n)
			default:
				ok = false
			}
			if !ok {
				break
			}
			if count == 0 {
				stats.First = f
			}
			stats.Last = f
			if count == 0 || f < stats.Min {
				stats.Min = f
			}
			if count == 0 || f > stats.Max {
				stats.Max = f
			}
			count++
		}
		if !ok || count == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: first=%.2f, last=%.2f, min=%.2f, max=%.2f", col, stats.First, stats.Last, stats.Min, stats.Max))
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("Rows returned: %d\n%s", len(rs.Rows), strings.Join(lines, "\n"))
}

// regenerateDescription rewrites plan.Description from the real query
// result. runFinalizePhase's description is written before materializeReport
// ever runs plan.DataSQL - the model was stating numbers for a query it had
// not yet seen the result of, which is forecasting, not reading, no matter
// how the lawbook's wording is tightened. This call runs after the real rows
// are in hand and is given nothing but those rows' own first/last/min/max,
// so there is nothing left for it to invent. Falls back to the original
// (unreliable) description on any failure - a stale-but-present description
// beats no description.
func (a *Agent) regenerateDescription(ctx context.Context, entry PlanEntry, plan *FinalizePlan, rs *storageanalytics.ResultSet, clog *logging.ChatTurnLogger) string {
	facts := computeNumericFacts(rs)
	if facts == "" {
		return plan.Description
	}
	user := fmt.Sprintf("Original question: %s\n\nChart title: %s\n\nReal numbers from the query result:\n%s", entry.Question, plan.Title, facts)

	// No timeout of its own: this call shares the parent turn's ctx and
	// analyticsTurnTimeout (90s) bounds it same as every other call in the
	// pipeline. A short timeout here (previously 6s) was added when a slow
	// describe call could make the whole turn vanish - the client would
	// disconnect before persistTurn ever ran, losing the user's question
	// too. That's fixed now: AppendUserMessage (webchat_handler.go) saves
	// the question the moment it arrives, before the agent even runs, so a
	// slow describe call only delays the reply, it can't lose the turn.
	// Observed latency on this call has been 17-20s on some AI connectors -
	// a short timeout meant it almost never actually succeeded, defeating
	// the point of adding it.
	raw, err := a.completeOnce(ctx, clog, 4, "describe", entry.ID, 1, a.describePrompt, user)
	if err != nil {
		return plan.Description
	}
	var out describeFacts
	if err := json.Unmarshal([]byte(vlrender.ExtractJSONBlock(raw)), &out); err != nil {
		return plan.Description
	}
	desc := strings.TrimSpace(out.Description)
	if desc == "" {
		return plan.Description
	}
	return desc
}

// buildChartReport assembles the Vega-Lite spec in Go. The model supplies only
// presentation - mark, encoding, prose - and never touches the numbers, which
// go into data.values exactly as Postgres returned them.
func (a *Agent) buildChartReport(
	ctx context.Context,
	entry PlanEntry,
	plan *FinalizePlan,
	rs *storageanalytics.ResultSet,
	clog *logging.ChatTurnLogger,
) finishedReport {
	// A field the query did not actually return produces a silently empty
	// chart, which is worse than an error, so check before building.
	available := make(map[string]bool, len(rs.Columns))
	for _, c := range rs.Columns {
		available[c] = true
	}
	var missing []string
	for _, f := range plan.encodingFields() {
		if !available[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		clog.SQLRejected(entry.ID, "encoding", 1,
			fmt.Sprintf("encoding references missing columns %v; available %v", missing, rs.Columns))
		// The data is sound even though the presentation is not, so fall back
		// to CSV rather than discarding a correct result.
		return a.buildCSVReport(ctx, entry, plan, rs, clog)
	}

	spec := map[string]any{
		"$schema": "https://vega.github.io/schema/vega-lite/v5.json",
		"width":   600,
		"height":  340,
		// The vl-convert theme (see internal/vlrender, VL_CONVERT_THEME) renders
		// with a transparent background and medium-gray axis text, meant for
		// embedding on a light dashboard page. Rendered standalone into chat,
		// that transparency picks up whatever is behind the PNG - here the
		// app's dark background - leaving low-contrast gray text on dark blue.
		// Forcing a real white background keeps every theme legible regardless
		// of where the PNG ends up.
		"background": "#ffffff",
		"data":       map[string]any{"values": rs.Rows},
	}
	switch {
	case len(strings.TrimSpace(string(plan.Facet))) > 0:
		// A faceted/trellis chart (one small panel per contributor, per
		// repository, ...) carries its facet channel plus a single-panel
		// spec repeated per facet value - a top-level mark/encoding pair
		// would be redundant and Vega-Lite rejects having both.
		spec["facet"] = json.RawMessage(plan.Facet)
		spec["spec"] = json.RawMessage(plan.Spec)
	case len(strings.TrimSpace(string(plan.Layer))) > 0:
		// A layered chart (trend + rolling average, value + target line, ...)
		// carries its own mark/encoding per layer; a top-level mark/encoding
		// pair would be redundant and Vega-Lite rejects having both.
		spec["layer"] = json.RawMessage(plan.Layer)
	default:
		mark := plan.Mark
		if strings.TrimSpace(mark) == "" {
			mark = "bar"
		}
		isRhythmQuestion := looksLikeRhythmQuestion(entry.Question, plan.Title, plan.Query, plan.Description)
		rollingLayer, hasRollingLayer := maybeAddRollingAverageLayer(mark, plan.Encoding, len(rs.Rows))
		switch {
		case isRhythmQuestion && maybeCalendarHeatmap(plan.Encoding, len(rs.Rows), spec):
			// maybeCalendarHeatmap already wrote mark/encoding/width/height
			// directly into spec when it returns true - see its doc comment.
		case hasRollingLayer:
			spec["layer"] = rollingLayer
		default:
			spec["mark"] = mark
			spec["encoding"] = json.RawMessage(plan.Encoding)
		}
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		log.Error().Err(err).Str("report", entry.ID).Msg("failed to marshal chart spec")
		return a.buildCSVReport(ctx, entry, plan, rs, clog)
	}
	specJSON = sanitizeChartSpec(specJSON)
	normalized, err := vlrender.NormalizeVegaLiteSpec(specJSON)
	if err != nil {
		log.Warn().Err(err).Str("report", entry.ID).Msg("spec normalization failed, using raw spec")
		normalized = specJSON
	}

	timeRange := plan.TimeRange
	if computed := computeTimeRange(rs); computed != "" {
		timeRange = computed
	}
	description := a.regenerateDescription(ctx, entry, plan, rs, clog)

	clog.ReportFinalized(entry.ID, ResponseTypeChart, plan.Title, len(rs.Rows))
	return finishedReport{report: &vlrender.VegaLiteReport{
		Title:       plan.Title,
		Description: description,
		Query:       plan.Query,
		TimeRange:   timeRange,
		Granularity: plan.Granularity,
		Spec:        normalized,
	}}
}

// rhythmQuestionKeywords are hardcoded, case-insensitive substrings that mark
// a question as asking about usage *rhythm* - a daily-habit/consistency
// question ("are engineers actually incorporating reviews into their daily
// workflow", "is this a habit yet") - not just a plain trend. Matched
// against the report's own question plus everything the model itself wrote
// about the chart (title/query/description), since the exact phrasing that
// triggered this pattern can show up in either.
var rhythmQuestionKeywords = []string{
	"daily workflow", "rhythm", "habit", "consisten", // consistency/consistently
	"calendar heatmap", "day of week", "day-of-week", "weekday pattern",
	"incorporat", // incorporate/incorporating
}

func looksLikeRhythmQuestion(texts ...string) bool {
	for _, t := range texts {
		lower := strings.ToLower(t)
		for _, kw := range rhythmQuestionKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// calendarHeatmapMinRows is the floor on how many dated rows the model's own
// query must have returned before this is worth treating as calendar data at
// all - not a floor on how much of the grid ends up populated (that's always
// calendarHeatmapDays regardless, since the grid is zero-filled to a full
// year - see maybeCalendarHeatmap). This just guards against misfiring on a
// degenerate 1-2-row result that happened to pass the shape checks above.
const calendarHeatmapMinRows = 3

// calendarHeatmapDays is always a full year (365 days, matching GitHub's own
// contribution graph), ending today - regardless of what window the model's
// data_sql actually queried. See maybeCalendarHeatmap's doc comment for why
// the grid's date range is no longer trusted to the model.
const calendarHeatmapDays = 365

// isDayGranularity reports whether a Vega-Lite timeUnit is day-level or
// finer (or absent, which SQL almost always means date_trunc('day', ...)).
// Duplicated from rollingWindowFor's day-branch check rather than shared: it
// tests a different, narrower question (day-or-finer only, no week/month
// acceptance) and the two callers' meaning of "close enough" would drift out
// of sync if forced through one function.
func isDayGranularity(timeUnit string) bool {
	tu := strings.ToLower(timeUnit)
	if tu == "" {
		return true
	}
	for _, fine := range []string{"date", "day", "hours", "minutes", "seconds"} {
		if strings.Contains(tu, fine) {
			return true
		}
	}
	return false
}

// maybeCalendarHeatmap replaces spec's mark/width/height and writes a
// top-level "encoding" with a GitHub-contribution-graph-style calendar grid,
// when the chart is a single-series daily count/sum over a long enough
// window - see analytics_finalize.md's calendar-heatmap chart-shape row and
// looksLikeRhythmQuestion's doc comment for why this is triggered by
// keyword-matching the question rather than asked of the model directly:
// getting an LLM to hand-write a correct band-scale Vega-Lite calendar
// heatmap (ordinal, not temporal, x/y scales; paddingInner for the gap
// between cells; a threshold color scale; deduped month-only-once axis
// labels) proved unreliable across several iterations building this exact
// chart by hand in scripts/adoption_chart/generate_heatmap.py - so instead
// the model only has to supply daily-granularity data (x/y field names),
// and Go builds the entire presentation deterministically, the same
// division of labor as maybeAddRollingAverageLayer.
//
// Colors are GitHub's own *light*-mode green scale (not the dark-mode
// palette generate_heatmap.py uses for its standalone dark HTML) because
// buildChartReport always renders against a forced white background (see
// its "background": "#ffffff" comment) - vl-convert produces a PNG embedded
// in the chat UI, not a themeable page, so there is no dark background here
// to design against.
//
// Mutates spec in place (rather than returning a value the caller
// assembles) because a calendar heatmap needs band-scale width/height
// ({"step": N}, not the plain-number 600x340 buildChartReport sets
// unconditionally before this runs) in addition to mark/encoding - three
// keys to overwrite together is more naturally a mutation than a 3-tuple
// return.
// parseCalendarRow extracts a "YYYY-MM-DD" date key and a float value from
// one result row, for maybeCalendarHeatmap's zero-fill pass. dateField's
// value is whatever coerce() in storage/analytics/coerce.go produced for a
// timestamp column - an RFC3339 string, formatted in the value's own zone
// rather than UTC (see that function's doc comment on why: converting to
// UTC can shift date_trunc('day', ...) buckets to the wrong calendar date).
// Parsing with the same RFC3339 layout and then formatting the date portion
// preserves that same wall-clock date here.
func parseCalendarRow(row map[string]any, dateField, valueField string) (string, float64, bool) {
	dateRaw, ok := row[dateField]
	if !ok {
		return "", 0, false
	}
	dateStr, ok := dateRaw.(string)
	if !ok {
		return "", 0, false
	}
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		// Some data_sql aliases already produce a bare date (no time
		// component) rather than a timestamp - accept that shape too.
		t, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return "", 0, false
		}
	}

	var val float64
	switch v := row[valueField].(type) {
	case int64:
		val = float64(v)
	case float64:
		val = v
	case nil:
		val = 0
	default:
		return "", 0, false
	}
	return t.Format("2006-01-02"), val, true
}

func maybeCalendarHeatmap(encoding json.RawMessage, rowCount int, spec map[string]any) bool {
	var channels map[string]json.RawMessage
	if err := json.Unmarshal(encoding, &channels); err != nil {
		return false
	}
	for _, extra := range []string{"color", "detail", "size", "column", "row"} {
		if _, ok := channels[extra]; ok {
			return false
		}
	}

	type channelSpec struct {
		Field    string `json:"field"`
		Type     string `json:"type"`
		TimeUnit string `json:"timeUnit"`
		Title    string `json:"title"`
	}
	var x, y channelSpec
	xRaw, ok := channels["x"]
	if !ok || json.Unmarshal(xRaw, &x) != nil {
		return false
	}
	yRaw, ok := channels["y"]
	if !ok || json.Unmarshal(yRaw, &y) != nil {
		return false
	}
	if x.Type != "temporal" || y.Type != "quantitative" || x.Field == "" || y.Field == "" {
		return false
	}
	if !isDayGranularity(x.TimeUnit) || rowCount < calendarHeatmapMinRows {
		return false
	}

	// The model's data_sql picks whatever WHERE-clause window and grouping
	// it wants - it has already proven unreliable at both remembering to
	// span a full year and remembering to zero-fill missing days via
	// generate_series (see the "shows only 90 days" and "empty boxes
	// missing" bugs this replaced). Rather than keep asking the model to
	// get a SQL detail right that a prompt tweak has already failed to fix
	// twice, Go now completely rebuilds data.values itself: read whatever
	// dated values the query DID return, keyed by calendar date, and re-emit
	// exactly calendarHeatmapDays consecutive days ending today, filling
	// anything the model's rows didn't cover with 0. This makes the grid's
	// shape (a full year, gaps included) independent of what the model's
	// SQL actually queried - the same "Go controls the parts an LLM has
	// proven unreliable at" split as maybeAddRollingAverageLayer.
	byDate := map[string]float64{}
	if data, ok := spec["data"].(map[string]any); ok {
		if values, ok := data["values"].([]map[string]any); ok {
			for _, row := range values {
				dateStr, val, ok := parseCalendarRow(row, x.Field, y.Field)
				if !ok {
					continue
				}
				byDate[dateStr] += val
			}
		}
	}
	today := time.Now()
	filled := make([]map[string]any, 0, calendarHeatmapDays+1)
	for i := calendarHeatmapDays; i >= 0; i-- {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		filled = append(filled, map[string]any{x.Field: d, y.Field: byDate[d]})
	}
	spec["data"] = map[string]any{"values": filled}

	valueLabel := firstNonEmpty(strings.TrimSpace(y.Title), "Value")

	// buildChartReport forces a white background/light-theme colors for
	// every OTHER chart type (see its own "background": "#ffffff" comment -
	// that default exists so an unpredictable embed context never leaves
	// gray-on-gray text), but a GitHub-style contribution graph specifically
	// needs the opposite: it's only recognizable, and its "empty" cells only
	// read as *empty* rather than as stray white boxes, against a dark card
	// with GitHub's actual dark-mode green ramp - confirmed against
	// github.com's own graph and this repo's generate_heatmap.py, which
	// already renders this exact chart correctly with these exact colors.
	// So this is the one chart type that overrides those chat-wide defaults
	// rather than inheriting them.
	spec["background"] = "#0d1117"
	spec["config"] = map[string]any{
		"axis":   map[string]any{"labelColor": "#8b949e", "titleColor": "#e6ebf5", "labelFontSize": 10},
		"title":  map[string]any{"color": "#e6ebf5", "fontSize": 16, "anchor": "start"},
		"legend": map[string]any{"labelColor": "#8b949e", "titleColor": "#e6ebf5"},
		"view":   map[string]any{"stroke": "transparent"},
	}
	// GitHub's own dark-mode contribution-graph greens (empty -> darkest).
	githubGreensDark := []string{"#161b22", "#0e4429", "#006d32", "#26a641", "#39d353"}

	// Plain numbers, not {"step": N} - the chat frontend
	// (ui/src/pages/Chatbot/InteractiveChart.tsx) resizes every chart to
	// fill its container client-side, but it only derives the aspect ratio
	// to resize by (specAspectRatio = spec.width / spec.height) when both
	// are plain numbers; a step/band sizing object made it silently skip
	// that entirely, so the grid just rendered at its own small intrinsic
	// size and left the rest of the card empty - exactly the "empty space
	// on the right and bottom" bug. These numbers only need to set a
	// believable aspect ratio (~53 weeks wide by 7 days tall) - the actual
	// on-screen pixel size is whatever the frontend stretches it to.
	spec["width"] = 900
	spec["height"] = 130
	spec["mark"] = map[string]any{"type": "rect", "cornerRadius": 2}
	spec["encoding"] = map[string]any{
		// type "ordinal", not "temporal": a temporal x/y here puts week/day
		// columns on a continuous scale, whose band-width math for a rect
		// mark collapses/overlaps adjacent cells into each other instead of
		// giving each one a fixed-width cell - confirmed the hard way
		// building generate_heatmap.py's standalone version of this exact
		// chart, see that script's comment on the same bug.
		"x": map[string]any{
			"field": x.Field, "type": "ordinal", "timeUnit": "yearweek", "title": nil,
			"scale": map[string]any{"paddingInner": 0.15},
			"axis": map[string]any{
				"format": "%b",
				// Only label a week column if it's the first week whose
				// start falls in a new month - otherwise every ~4-5 weeks
				// in a month would repeat that month's label.
				"labelExpr":  "date(datum.value) <= 7 ? timeFormat(datum.value, '%b') : ''",
				"labelAngle": 0, "ticks": false, "domain": false, "grid": false,
			},
		},
		"y": map[string]any{
			"field": x.Field, "type": "ordinal", "timeUnit": "day", "title": nil,
			"sort":  []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
			"scale": map[string]any{"paddingInner": 0.15},
			// GitHub only labels every other row (Mon/Wed/Fri) to keep the
			// axis readable at this cell size.
			"axis": map[string]any{"values": []string{"Mon", "Wed", "Fri"}, "ticks": false, "domain": false, "grid": false},
		},
		"color": map[string]any{
			"field": y.Field, "type": "quantitative", "title": nil,
			"scale": map[string]any{"type": "threshold", "domain": []int{1, 3, 6, 10}, "range": githubGreensDark},
			// No legend at all - the grid + hover tooltip is enough; every
			// legend placement/orientation tried here left leftover chrome
			// the grid didn't need.
			"legend": nil,
		},
		"tooltip": []any{
			map[string]any{"field": x.Field, "type": "temporal", "title": "Date", "format": "%A, %b %d, %Y"},
			map[string]any{"field": y.Field, "type": "quantitative", "title": valueLabel},
		},
	}
	return true
}

// rollingWindow describes the smoothing window to use for one x-axis bucket
// granularity: how many trailing rows the window transform averages over,
// and what that window is called in the UI. The caller requires at least
// 2x rows before using one, so at least one full window's worth of history
// exists on both sides of "recent" - otherwise the "average" is really just
// the whole series restated.
type rollingWindow struct {
	rows  int
	label string
}

// rollingWindowFor maps an x-channel's Vega-Lite timeUnit to the smoothing
// window appropriate for that bucket size, and the minimum row count before
// it's worth showing at all. One row in the data IS one bucket (one day, one
// week, one month, ...), so the window size must scale with the bucket or
// the label lies about what it's averaging: a fixed 7-row window is a real
// 7-day average on daily data, but a mislabeled ~7-week average on weekly
// data. Chosen windows: daily -> 7-day (a week), weekly -> 4-week (about a
// month), monthly -> 6-month (half a year) - each roughly "the next unit up"
// from the bucket, which is what a viewer skimming the chart would expect
// "the average" to mean at that zoom level. Quarter/year buckets return
// !ok - a handful of quarterly/yearly points is usually already readable
// without smoothing, and there's no obviously-right "next unit up" to
// average them into.
func rollingWindowFor(timeUnit string) (rollingWindow, bool) {
	tu := strings.ToLower(timeUnit)
	switch {
	case tu == "" || strings.Contains(tu, "date") || strings.Contains(tu, "day") ||
		strings.Contains(tu, "hours") || strings.Contains(tu, "minutes") || strings.Contains(tu, "seconds"):
		// Empty timeUnit means a raw temporal field with no explicit
		// bucketing in the encoding - assume day granularity, matching
		// what SQL almost always produces in that case (date_trunc('day', ...)).
		return rollingWindow{rows: 7, label: "7-day"}, true
	case strings.Contains(tu, "week"):
		return rollingWindow{rows: 4, label: "4-week"}, true
	case strings.Contains(tu, "month"):
		// Checked after "week": no Vega-Lite timeUnit contains both
		// substrings, so order doesn't matter for correctness, but month is
		// checked before the day-granularity branch's "date" substring
		// match already resolved above - "yearmonthdate" is caught by the
		// "date" check first, so by the time we get here tu is a pure
		// month-or-coarser bucket.
		return rollingWindow{rows: 6, label: "6-month"}, true
	default:
		return rollingWindow{}, false
	}
}

// maybeAddRollingAverageLayer augments a flat mark+encoding chart into a
// two-layer spec (the original mark, plus a rolling-average line sized to
// the chart's own bucket granularity) when the chart is a single-series
// time trend with enough points for bucket-to-bucket noise to obscure
// whether it is actually trending. This runs unconditionally in Go rather
// than relying on the model to ask for it: analytics_finalize.md's
// chart-shape table already documents "add a second line layer for a
// rolling average if the trend is noisy" as one row among many, and in
// practice the model does not reliably choose it (the "is adoption
// increasing" report that motivated this returned a plain bar with no
// rolling average at all). Vega-Lite's own window transform computes the
// average client-side from the same data.values the query already
// returned, so this needs no new SQL and no LLM cooperation - it cannot be
// skipped the way a prompt instruction can.
//
// Deliberately conservative: returns false (unchanged spec) for anything
// the model already made a deliberate richer choice about. A color, detail,
// size, column, or row channel means grouped or multi-series data, where
// one rolling-average line drawn across all of it would be presentationally
// wrong, not helpful - and x must be temporal with y the sole quantitative
// channel, i.e. a plain "count over bucket" shape, not something this
// heuristic should guess about.
func maybeAddRollingAverageLayer(mark string, encoding json.RawMessage, rowCount int) (json.RawMessage, bool) {
	switch mark {
	case "", "bar", "line", "area", "point", "circle":
	default:
		return nil, false
	}

	var channels map[string]json.RawMessage
	if err := json.Unmarshal(encoding, &channels); err != nil {
		return nil, false
	}
	for _, extra := range []string{"color", "detail", "size", "column", "row"} {
		if _, ok := channels[extra]; ok {
			return nil, false
		}
	}

	type channelSpec struct {
		Field    string `json:"field"`
		Type     string `json:"type"`
		TimeUnit string `json:"timeUnit"`
		Title    string `json:"title"`
	}
	var x, y channelSpec
	xRaw, ok := channels["x"]
	if !ok || json.Unmarshal(xRaw, &x) != nil {
		return nil, false
	}
	yRaw, ok := channels["y"]
	if !ok || json.Unmarshal(yRaw, &y) != nil {
		return nil, false
	}
	if x.Type != "temporal" || y.Type != "quantitative" || x.Field == "" || y.Field == "" {
		return nil, false
	}

	// The baseline (period average) rule is a flat mean of whatever rows
	// exist - meaningful regardless of how many there are or how they're
	// bucketed, so it's added whenever the basic single-series time-trend
	// shape checks above pass. The rolling-average line is different: it
	// only means what its label says (win.rows *rows*, e.g. "6-month") when
	// there are enough rows for that window to be more signal than noise
	// (rollingWindowFor's !ok / rowCount check below) - so it's the only
	// one gated on granularity/row count, added on top of the baseline
	// when that gate passes.
	win, winOK := rollingWindowFor(x.TimeUnit)
	includeRolling := winOK && rowCount >= win.rows*2

	// A named, identical color scale (same domain/range) on every layer's
	// "datum" color channel is what makes Vega-Lite draw one shared legend
	// for layers that otherwise have no data-driven color channel at all (a
	// plain bar + synthetic line(s)): each layer's color is a literal
	// constant, not a field lookup, but literal-vs-literal still
	// participates in a legend the same way a real categorical field would.
	baseLabel := firstNonEmpty(strings.TrimSpace(y.Title), "Value")
	baselineLabel := "Period average (baseline)"
	domain := []string{baseLabel, baselineLabel}
	colorRange := []string{"#7c9cff", "#ff5c7c"}
	var rollingLabel string
	if includeRolling {
		rollingLabel = win.label + " rolling average"
		// Insert rolling before baseline so bar/line/rule keep their
		// established color order (blue/orange/pink) regardless of which
		// optional layers are present.
		domain = []string{baseLabel, rollingLabel, baselineLabel}
		colorRange = []string{"#7c9cff", "#ffb454", "#ff5c7c"}
	}
	colorFor := func(label string) map[string]any {
		return map[string]any{
			"datum":  label,
			"type":   "nominal",
			"scale":  map[string]any{"domain": domain, "range": colorRange},
			"legend": map[string]any{"title": nil, "orient": "top"},
		}
	}

	// Every layer's y channel gets the exact same explicit title
	// (baseLabel), instead of each layer defaulting to a title derived from
	// its own field name ("Reviews Completed" vs "rolling_avg" vs
	// "period_avg"). All layers share one y-axis/scale (Vega-Lite's
	// default resolve), and when layers sharing an axis carry different
	// titles, Vega-Lite concatenates them into one garbled label instead of
	// picking one - identical titles on every layer sidesteps that instead
	// of fighting it. An earlier version of this used "axis": null on the
	// second/third layers to suppress the title collision, which turned out
	// to suppress the ENTIRE shared axis (no tick labels at all) rather
	// than just that layer's title - do not reintroduce that.
	channels["y"] = mustMarshalJSON(map[string]any{"field": y.Field, "type": "quantitative", "title": baseLabel})
	channels["color"] = mustMarshalJSON(colorFor(baseLabel))
	baseEncoding, err := json.Marshal(channels)
	if err != nil {
		return nil, false
	}

	layer := []any{
		map[string]any{"mark": firstNonEmpty(mark, "bar"), "encoding": json.RawMessage(baseEncoding)},
	}

	if includeRolling {
		layer = append(layer, map[string]any{
			"transform": []any{map[string]any{
				"window": []any{map[string]any{"op": "mean", "field": y.Field, "as": "rolling_avg"}},
				"frame":  []any{-(win.rows - 1), 0},
				"sort":   []any{map[string]any{"field": x.Field}},
			}},
			"mark": map[string]any{"type": "line", "strokeWidth": 2.5},
			"encoding": map[string]any{
				"x":     xRaw,
				"y":     map[string]any{"field": "rolling_avg", "type": "quantitative", "title": baseLabel},
				"color": colorFor(rollingLabel),
				"tooltip": []any{
					map[string]any{"field": x.Field, "type": "temporal", "title": "Period"},
					// format: the mean/window aggregates below are raw
					// floats (e.g. 4.333333...) with no rounding of their
					// own - .2f keeps the tooltip readable.
					map[string]any{"field": "rolling_avg", "type": "quantitative", "title": rollingLabel, "format": ".2f"},
				},
			},
		})
	}

	// The baseline rule needs no Go-side computation of the actual
	// average: an "aggregate" transform collapses the same data.values
	// every other layer sees into a single {period_avg} row, entirely
	// client-side in the browser, the same way the rolling-average
	// layer's "window" transform above needs no precomputed numbers.
	layer = append(layer, map[string]any{
		"transform": []any{map[string]any{
			"aggregate": []any{map[string]any{"op": "mean", "field": y.Field, "as": "period_avg"}},
		}},
		"mark": map[string]any{"type": "rule", "strokeDash": []any{6, 4}, "strokeWidth": 1.5},
		"encoding": map[string]any{
			"y":     map[string]any{"field": "period_avg", "type": "quantitative", "title": baseLabel},
			"color": colorFor(baselineLabel),
			"tooltip": []any{
				map[string]any{"field": "period_avg", "type": "quantitative", "title": baselineLabel, "format": ".2f"},
			},
		},
	})

	b, err := json.Marshal(layer)
	if err != nil {
		return nil, false
	}
	return b, true
}

// mustMarshalJSON marshals a value this package built itself (plain
// map[string]any/string/[]string literals, never user/model input), so a
// marshal error here would mean a programming mistake, not bad data -
// exactly the case where a panic-on-can't-happen helper is appropriate
// instead of threading an error return through colorFor's every caller.
func mustMarshalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshalJSON: %v", err))
	}
	return b
}

// buildCSVReport writes the result set to CSV. Column order comes from the
// result set rather than from map iteration, so the header matches the data.
func (a *Agent) buildCSVReport(
	ctx context.Context,
	entry PlanEntry,
	plan *FinalizePlan,
	rs *storageanalytics.ResultSet,
	clog *logging.ChatTurnLogger,
) finishedReport {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	if err := w.Write(rs.Columns); err != nil {
		return finishedReport{text: fmt.Sprintf("I could not export %q.", entry.Question)}
	}
	record := make([]string, len(rs.Columns))
	for _, row := range rs.Rows {
		for i, col := range rs.Columns {
			switch v := row[col].(type) {
			case nil:
				record[i] = ""
			case string:
				record[i] = v
			default:
				record[i] = fmt.Sprint(v)
			}
		}
		if err := w.Write(record); err != nil {
			return finishedReport{text: fmt.Sprintf("I could not export %q.", entry.Question)}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return finishedReport{text: fmt.Sprintf("I could not export %q.", entry.Question)}
	}

	description := a.regenerateDescription(ctx, entry, plan, rs, clog)
	if rs.Truncated {
		description = strings.TrimSpace(description + fmt.Sprintf("\n\nThis export stops at %d rows. Narrow the question for a complete set.", len(rs.Rows)))
	}

	timeRange := plan.TimeRange
	if computed := computeTimeRange(rs); computed != "" {
		timeRange = computed
	}

	clog.ReportFinalized(entry.ID, ResponseTypeCSV, plan.Title, len(rs.Rows))
	return finishedReport{artifact: &Artifact{
		Kind:        "csv",
		Filename:    safeCSVFilename(firstNonEmpty(plan.CSVFilename, plan.Title, entry.Question)),
		Title:       plan.Title,
		Description: description,
		Query:       plan.Query,
		TimeRange:   timeRange,
		Granularity: plan.Granularity,
		Data:        []byte(buf.String()),
		Rows:        len(rs.Rows),
	}}
}

// noDataText asks the model for one clean sentence. If that call fails the
// fallback is still a sentence, never an empty chart or a generic error.
func (a *Agent) noDataText(ctx context.Context, entry PlanEntry, userText string, clog *logging.ChatTurnLogger) string {
	raw, err := a.completeOnce(ctx, clog, 3, "no_data", entry.ID, 1, a.noDataPrompt,
		fmt.Sprintf("Original question: %s\n\nThis report answers: %s\n\nThere are zero matching rows.", userText, entry.Question))
	if err == nil {
		if plan, perr := parseFinalizePlan(raw); perr == nil && strings.TrimSpace(plan.Text) != "" {
			return strings.TrimSpace(plan.Text)
		}
		if text := strings.TrimSpace(vlrender.StripVegaJSON(raw)); text != "" && len(text) < 400 {
			return text
		}
	}
	return fmt.Sprintf("I found no data for %q.", entry.Question)
}

// repairSQL gives the model one chance to fix a rejected statement. Only the
// hint is fed back, never internal guard detail. failedAttempt is the attempt
// number of the statement being repaired, so the resulting log line ties the
// repair call back to the rejection that triggered it. call is the diagram
// number of the phase being repaired: 2 for a count-phase statement, 3 for a
// data-phase one - repair itself is not a distinct box in the diagram.
func (a *Agent) repairSQL(ctx context.Context, call int, reportID string, failedAttempt int, question, badSQL, hint string, clog *logging.ChatTurnLogger) (string, error) {
	// This is a fresh, isolated exchange (see completeOnce's doc comment) - it
	// never sees the count/finalize prompt's org context header, so the
	// org_id value has to be repeated here too, or a missing-org-filter
	// rejection could never actually be fixed.
	raw, err := a.completeOnce(ctx, clog, call, "repair", reportID, failedAttempt,
		a.repairPrompt,
		fmt.Sprintf("Question: %s\n\nThis query was rejected:\n%s\n\nReason: %s\n\nReturn only the corrected SQL.", question, badSQL, hint))
	if err != nil {
		return "", err
	}
	fixed := extractSQL(raw)
	if strings.TrimSpace(fixed) == "" {
		return "", fmt.Errorf("repair produced no SQL")
	}
	return fixed, nil
}

// completeOnce makes a one-shot LLM call outside the agent loop: a fresh
// two-message conversation, no tools, no accumulated history. Keeping it
// isolated is what stops one report's retries from polluting another's context.
//
// Unlike the main RunTurn step loop, these calls carry no step number, so
// they were previously invisible in the debug log - no request, no response,
// no latency, no token count. kind/reportID/attempt exist purely to make
// these calls traceable: kind distinguishes finalize/no_data/repair, reportID
// ties the call to one entry of a multi-report turn, attempt ties a repair
// call back to the SQL attempt it is fixing.
func (a *Agent) completeOnce(ctx context.Context, clog *logging.ChatTurnLogger, call int, kind, reportID string, attempt int, system, user string) (string, error) {
	if clog.Enabled() {
		if b, err := json.Marshal([]HistoryEntry{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		}); err == nil {
			clog.LLMCallRequest(call, kind, reportID, attempt, string(b))
		}
	}

	// finalize and no_data expect a JSON object; repair expects bare SQL
	// (extractSQL pulls it straight from the text, no JSON wrapper) - forcing
	// JSON mode there would fight the format it's actually asked to return.
	var opts []llms.CallOption
	if kind != "repair" {
		opts = append(opts, llms.WithJSONMode())
	}

	start := time.Now()
	text, usage, err := a.provider.Complete(ctx, []HistoryEntry{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}, nil, opts...)
	elapsed := time.Since(start)
	if err != nil {
		clog.LLMCallError(call, kind, reportID, attempt, elapsed, err)
		return "", err
	}
	clog.LLMCallResponse(call, kind, reportID, attempt, elapsed, usage.InputTokens, usage.OutputTokens, text)
	return text, nil
}

func hintFor(err error) string {
	var re *livisql.RejectionError
	if errors.As(err, &re) && re.LLMHint != "" {
		return re.LLMHint
	}
	return "The query was rejected. Rewrite it as a simple SELECT over the documented tables."
}

// assembleAnalyticsResponse produces the string RunTurn returns. Charts go into
// the existing {"reports":[...]} envelope that every surface already renders;
// plain notes are returned as prose when there is nothing to draw. hasArtifacts
// must be true whenever the turn produced at least one CSV (or other file)
// artifact - those are returned separately (see RunTurnWithArtifacts) and
// never appear in reports/notes, so without this a successful CSV-only turn
// (reports and notes both empty, but a real file was produced and attached)
// would otherwise fall through to the "could not find anything" message
// right next to the correctly-delivered file - a contradiction a user would
// rightly read as a bug, because it is one.
func assembleAnalyticsResponse(reports []vlrender.VegaLiteReport, notes []string, hasArtifacts bool) string {
	if len(reports) == 0 {
		if len(notes) == 0 {
			if hasArtifacts {
				return "I've put the results in a file you can download."
			}
			return "I could not find anything to answer that."
		}
		return strings.Join(notes, "\n\n")
	}
	payload, err := json.Marshal(map[string]any{"reports": reports})
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal analytics reports")
		return strings.Join(notes, "\n\n")
	}
	// Notes (skip reasons) are only in debug artifacts, not in chat response.
	return string(payload)
}

// rowsToPreviewCSV converts rows to a CSV string for debug preview.
// Truncates at ~2KB, always at a row boundary to keep CSV parseable.
func rowsToPreviewCSV(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}

	// Collect all unique column names across all rows.
	colSet := make(map[string]struct{})
	for _, row := range rows {
		for k := range row {
			colSet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	// Use byte slice so we can truncate at row boundaries.
	var buf []byte
	// Header.
	for i, c := range cols {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, c...)
	}
	buf = append(buf, '\n')

	// Rows — truncate at row boundary to keep CSV parseable.
	const maxPreview = 2048
	for _, row := range rows {
		rowStart := len(buf)
		for i, c := range cols {
			if i > 0 {
				buf = append(buf, ',')
			}
			v := row[c]
			var s string
			switch val := v.(type) {
			case nil:
				s = ""
			case string:
				s = val
			case float64:
				s = fmt.Sprintf("%g", val)
			case int64:
				s = fmt.Sprintf("%d", val)
			case bool:
				if val {
					s = "true"
				} else {
					s = "false"
				}
			default:
				s = fmt.Sprintf("%v", val)
			}
			// Quote fields containing comma, newline, carriage return, or double quote.
			if strings.ContainsAny(s, ",\n\r\"") {
				s = `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
		if len(buf) > maxPreview {
			// Roll back incomplete row to keep CSV valid.
			buf = buf[:rowStart]
			buf = append(buf, "... (truncated)\n"...)
			break
		}
	}
	return string(buf)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// extractSQL pulls a statement out of a model reply that may be fenced, or may
// be JSON carrying a sql field, or may be bare SQL.
func extractSQL(raw string) string {
	trimmed := strings.TrimSpace(raw)

	var obj struct {
		SQL      string `json:"sql"`
		CountSQL string `json:"count_sql"`
		DataSQL  string `json:"data_sql"`
	}
	if json.Unmarshal([]byte(vlrender.ExtractJSONBlock(trimmed)), &obj) == nil {
		if s := firstNonEmpty(obj.SQL, obj.CountSQL, obj.DataSQL); s != "" {
			return strings.TrimSpace(s)
		}
	}

	if idx := strings.Index(trimmed, "```"); idx >= 0 {
		rest := trimmed[idx+3:]
		rest = strings.TrimPrefix(rest, "sql")
		if end := strings.Index(rest, "```"); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
	}
	return trimmed
}

// ---------------------------------------------------------------------------
// Multi-interpret pipeline (replaces plan+finalize for count_query)
// ---------------------------------------------------------------------------

const maxInterpretations = 5

// interpretEnvelope is the JSON shape the interpreting prompt asks for.
type interpretEnvelope struct {
	Query         string           `json:"query"`
	Interpretation string          `json:"interpretation"`
	Interpretations []Interpretation `json:"interpretations"`
}

// parseInterpretations extracts up to maxInterpretations entries from the
// LLM's JSON response. Accepts both wrapped {"interpretations":[...]} and
// bare [...] shapes.
func parseInterpretations(text string) ([]Interpretation, bool) {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(text))
	if body == "" {
		return nil, false
	}

	var env interpretEnvelope
	if err := json.Unmarshal([]byte(body), &env); err == nil && len(env.Interpretations) > 0 {
		return normalizeInterpretations(env.Interpretations)
	}

	// Bare array fallback.
	if strings.HasPrefix(body, "[") {
		var interps []Interpretation
		if err := json.Unmarshal([]byte(body), &interps); err == nil && len(interps) > 0 {
			return normalizeInterpretations(interps)
		}
	}

	return nil, false
}

func normalizeInterpretations(in []Interpretation) ([]Interpretation, bool) {
	out := make([]Interpretation, 0, len(in))
	for _, interp := range in {
		if strings.TrimSpace(interp.SQL) == "" {
			continue
		}
		if strings.TrimSpace(interp.ChartType) == "" {
			interp.ChartType = "bar"
		}
		// Use Name as fallback title if Title is empty.
		if strings.TrimSpace(interp.Title) == "" {
			interp.Title = interp.Name
		}
		if strings.TrimSpace(interp.Title) == "" {
			interp.Title = interp.ChartType + " chart"
		}
		out = append(out, interp)
		if len(out) >= maxInterpretations {
			break
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// runMultiInterpret executes the multi-interpret analytics pipeline. It
// makes a single LLM call to get up to 5 SQL+chart interpretations, then
// executes each through the SQL guard, builds charts, and assembles the
// response. Returns (responseText, artifacts, debugArtifacts, error).
func (a *Agent) runMultiInterpret(
	ctx context.Context,
	userText string,
	clog *logging.ChatTurnLogger,
	orgID int64,
	orgName string,
) (string, []Artifact, *DebugArtifacts, error) {
	ctx, cancel := context.WithTimeout(ctx, analyticsTurnTimeout)
	defer cancel()

	debug := &DebugArtifacts{
		Query: userText,
	}

	// System prompt: self-contained from PromptBook template (includes
	// all rules inline, no schema splice).
	system := a.interpretPrompt()
	debug.SystemPrompt = system

	// User message: query + org context + dbctx schema + chart types
	// (matching the Python script's format in interpretation.py).
	userMsg, err := a.interpretUserMessage(clog, userText, orgID, orgName)
	if err != nil {
		return "Livi is in preparing mode, please come back after 60s.", nil, debug, nil
	}
	debug.SchemaContext = userMsg

	// Compose the full request text sent to the LLM for debug visibility.
	debug.FullRequest = system + "\n---\n" + userMsg

	raw, err := a.completeOnce(ctx, clog, 2, "interpret", "multi", 1, system, userMsg)
	if err != nil {
		log.Error().Err(err).Msg("multi-interpret LLM call failed")
		return "I had trouble understanding that question. Please try rephrasing.", nil, debug, err
	}
	debug.LLMRawResponse = raw

	interps, ok := parseInterpretations(raw)
	if !ok {
		log.Warn().Str("raw_preview", truncateContent(raw, 200)).Msg("could not parse interpretations from LLM response")
		return "I could not produce a valid analysis plan. Please try rephrasing.", nil, debug, nil
	}
	debug.Interpretations = interps

	var (
		reports   []vlrender.VegaLiteReport
		artifacts []Artifact
		notes     []string
	)

	for i, interp := range interps {
		entry := DebugResultEntry{
			Index:     i,
			Title:     interp.Title,
			ChartType: interp.ChartType,
			SQL:       interp.SQL,
		}

		result := a.executeInterpretation(ctx, interp, userText, clog)
		entry.Status = result.Status
		entry.SkipReason = result.SkipReason
		entry.RowCount = result.RowCount
		entry.Stats = result.Stats

		// Include row data as CSV for debug preview (from any outcome).
		if len(result.Rows) > 0 {
			entry.CSVData = rowsToPreviewCSV(result.Rows)
		}

		switch {
		case result.Chart != nil:
			reports = append(reports, *result.Chart)
			// Include Vega-Lite spec in debug for inspection.
			if specBytes, err := json.Marshal(result.Chart.Spec); err == nil {
				entry.VegaSpec = string(specBytes)
			}
		case result.Artifact != nil:
			artifacts = append(artifacts, *result.Artifact)
			// For rendered CSV artifacts, include data if not already set
			// and the data looks like CSV (starts with a header row).
			if entry.CSVData == "" && result.Artifact.Kind == "csv" && len(result.Artifact.Data) > 0 {
				entry.CSVData = string(result.Artifact.Data)
			}
		case result.SkipReason != "":
			notes = append(notes, fmt.Sprintf("Skipped %q: %s", interp.Title, result.SkipReason))
		}

		debug.Results = append(debug.Results, entry)
	}

	responseText := assembleAnalyticsResponse(reports, notes, len(artifacts) > 0)
	clog.FinalResponse(responseText)
	return responseText, artifacts, debug, nil
}

// executeInterpretation runs one interpretation through the SQL guard,
// executes it, and builds a chart or CSV from the results.
func (a *Agent) executeInterpretation(
	ctx context.Context,
	interp Interpretation,
	userText string,
	clog *logging.ChatTurnLogger,
) InterpretationResult {
	sqlText := interp.SQL

	// Validate with the guard.
	rewritten, err := a.guard().Rewrite(sqlText)
	if err != nil {
		clog.SQLRejected(interp.Title, "data", 1, err.Error())
		// Try repairing once.
		fixed, rerr := a.repairSQL(ctx, 2, interp.Title, 1, userText, sqlText, hintFor(err), clog)
		if rerr != nil {
			return InterpretationResult{Status: "failed", SkipReason: "SQL rejected: " + err.Error()}
		}
		sqlText = fixed
		rewritten, err = a.guard().Rewrite(sqlText)
		if err != nil {
			return InterpretationResult{Status: "failed", SkipReason: "SQL still rejected after repair: " + err.Error()}
		}
	}
	clog.SQLRewritten(interp.Title, "data", 1, rewritten)

	// Execute.
	start := time.Now()
	rs, err := a.analytics.Query(ctx, rewritten, maxCSVRows)
	if err != nil {
		clog.SQLError(interp.Title, "data", 1, time.Since(start), err)
		return InterpretationResult{Status: "failed", SkipReason: "Query failed: " + err.Error()}
	}
	clog.SQLResult(interp.Title, "data", 1, time.Since(start), len(rs.Rows), rs.Truncated)

	relabelTriggerTypeValues(rs.Rows)

	// Weak result filter.
	if len(rs.Rows) <= 1 {
		reason := "single-row result (weak)"
		if len(rs.Rows) == 0 {
			// Distinct from "weak" - zero rows usually means a filter
			// eliminated everything (often a hallucinated enum value the
			// WHERE clause matched literally, silently, e.g. status =
			// 'processed' instead of 'accounted'), not that the shape was
			// merely thin.
			reason = "query returned no rows - check for an invalid filter or enum value"
		}
		return InterpretationResult{
			Status:     "skipped",
			SkipReason: reason,
			RowCount:   len(rs.Rows),
			Rows:       rs.Rows, // preserve for debug preview
		}
	}

	// Smart time aggregation for day-level time series.
	aggregated, timeUnit, groupFields := smartAggregateTime(rs)
	if aggregated != nil {
		rs = aggregated
	}

	// Build chart.
	chart, art := a.buildChartFromInterp(ctx, interp, rs, timeUnit, groupFields, clog)

	stats := computeStats(rs.Rows, interp.Encoding)

	if chart != nil {
		return InterpretationResult{
			Status:   "rendered",
			Chart:    chart,
			RowCount: len(rs.Rows),
			Stats:    stats,
			Rows:     rs.Rows,
		}
	}
	if art != nil {
		return InterpretationResult{
			Status:   "rendered",
			Artifact: art,
			RowCount: len(rs.Rows),
			Stats:    stats,
			Rows:     rs.Rows,
		}
	}
	return InterpretationResult{
		Status:   "skipped",
		SkipReason: "could not build chart",
		RowCount: len(rs.Rows),
	}
}

// buildChartFromInterp builds a Vega-Lite chart from an Interpretation.
// If the LLM provided a vega_lite_spec with DATA_PLACEHOLDER, replaces
// the placeholder with actual data (matching the Python script's approach
// in scripts/prelivi/interpretation.py). Otherwise falls back to building
// a minimal spec from chart_type + encoding.
func (a *Agent) buildChartFromInterp(
	ctx context.Context,
	interp Interpretation,
	rs *storageanalytics.ResultSet,
	timeUnit string,
	groupFields []string,
	clog *logging.ChatTurnLogger,
) (*vlrender.VegaLiteReport, *Artifact) {
	if len(rs.Rows) > maxChartRows {
		return nil, a.buildCSVFromRS(interp.Title, interp.Description, interp.SQL, rs)
	}

	var specJSON []byte

	// If the LLM provided a complete vega_lite_spec, use it with
	// DATA_PLACEHOLDER replacement (Python script pattern).
	if len(interp.VegaLiteSpec) > 0 {
		specBytes, err := json.Marshal(interp.VegaLiteSpec)
		if err == nil {
			specStr := string(specBytes)
			if strings.Contains(specStr, "DATA_PLACEHOLDER") {
				dataJSON, err := json.Marshal(rs.Rows)
				if err == nil {
					specStr = strings.Replace(specStr, `"DATA_PLACEHOLDER"`, string(dataJSON), 1)
				}
			}
			specJSON = []byte(specStr)
		}
	}

	// Fallback: build a minimal spec from chart_type + encoding.
	if specJSON == nil {
		mark, defaultEncoding := chartTypeToMark(interp.ChartType, timeUnit)
		encodingJSON := defaultEncoding
		if len(interp.Encoding) > 0 {
			if e, err := json.Marshal(interp.Encoding); err == nil {
				encodingJSON = json.RawMessage(e)
			}
		}
		spec := map[string]any{
			"$schema":    "https://vega.github.io/schema/vega-lite/v5.json",
			"width":      600,
			"height":     340,
			"background": "#ffffff",
			"data":       map[string]any{"values": rs.Rows},
		}
		if interp.ChartType == "trellis_bar" {
			spec["facet"] = json.RawMessage(defaultEncoding)
			spec["spec"] = map[string]any{"mark": "bar", "encoding": encodingJSON}
		} else {
			spec["mark"] = mark
			spec["encoding"] = encodingJSON
		}
		var err error
		specJSON, err = json.Marshal(spec)
		if err != nil {
			log.Error().Err(err).Str("interp", interp.Title).Msg("failed to marshal chart spec")
			return nil, a.buildCSVFromRS(interp.Title, interp.Description, interp.SQL, rs)
		}
	}

	specJSON = sanitizeChartSpec(specJSON)

	normalized, err := vlrender.NormalizeVegaLiteSpec(specJSON)
	if err != nil {
		log.Warn().Err(err).Str("interp", interp.Title).Msg("spec normalization failed, using raw")
		normalized = specJSON
	}

	timeRange := computeTimeRange(rs)

	// Use Name as title if Title is generic/missing.
	title := interp.Title
	if title == "" || title == "Query Result" {
		title = interp.Name
	}
	if title == "" {
		title = interp.ChartType + " chart"
	}

	clog.ReportFinalized(title, ResponseTypeChart, title, len(rs.Rows))
	return &vlrender.VegaLiteReport{
		Title:       title,
		Description: interp.Description,
		Query:       interp.SQL,
		TimeRange:   timeRange,
		Granularity: timeUnit,
		Spec:        normalized,
	}, nil
}

// buildCSVFromRS creates a CSV artifact from a result set.
func (a *Agent) buildCSVFromRS(title, description, query string, rs *storageanalytics.ResultSet) *Artifact {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	w.Write(rs.Columns)
	for _, row := range rs.Rows {
		vals := make([]string, len(rs.Columns))
		for i, col := range rs.Columns {
			vals[i] = fmt.Sprintf("%v", row[col])
		}
		w.Write(vals)
	}
	w.Flush()
	return &Artifact{
		Kind:        "csv",
		Filename:    safeCSVFilename(title),
		Title:       title,
		Description: description,
		Query:       query,
		Data:        []byte(buf.String()),
		Rows:        len(rs.Rows),
	}
}

// chartTypeToMark maps a chart_type string to a Vega-Lite mark and default
// encoding. Returns (mark, encodingJSON).
func chartTypeToMark(chartType, timeUnit string) (string, json.RawMessage) {
	switch chartType {
	case "bar":
		return `"bar"`, json.RawMessage(`{"x":{"type":"nominal","sort":"-y"},"y":{"type":"quantitative"}}`)
	case "grouped_bar":
		return `"bar"`, json.RawMessage(`{"x":{"type":"nominal"},"y":{"type":"quantitative"},"color":{"type":"nominal"},"xOffset":{"type":"nominal"}}`)
	case "stacked_bar":
		return `"bar"`, json.RawMessage(`{"x":{"type":"nominal"},"y":{"type":"quantitative"},"color":{"type":"nominal"}}`)
	case "line":
		return `{"type":"line","point":true}`, json.RawMessage(`{"x":{"type":"temporal","timeUnit":"` + timeUnit + `"},"y":{"type":"quantitative"}}`)
	case "multi_line":
		return `{"type":"line","point":true}`, json.RawMessage(`{"x":{"type":"temporal","timeUnit":"` + timeUnit + `"},"y":{"type":"quantitative"},"color":{"type":"nominal"}}`)
	case "area":
		return `"area"`, json.RawMessage(`{"x":{"type":"temporal","timeUnit":"` + timeUnit + `"},"y":{"type":"quantitative"}}`)
	case "stacked_area":
		return `"area"`, json.RawMessage(`{"x":{"type":"temporal","timeUnit":"` + timeUnit + `"},"y":{"type":"quantitative"},"color":{"type":"nominal"}}`)
	case "scatter":
		return `"point"`, json.RawMessage(`{"x":{"type":"quantitative"},"y":{"type":"quantitative"}}`)
	case "pie":
		return `{"type":"arc","innerRadius":50}`, json.RawMessage(`{"theta":{"type":"quantitative"},"color":{"type":"nominal"}}`)
	case "heatmap":
		return `"rect"`, json.RawMessage(`{"x":{"type":"ordinal"},"y":{"type":"ordinal"},"color":{"type":"quantitative","scale":{"scheme":"blues"}}}`)
	case "horizontal_bar":
		return `"bar"`, json.RawMessage(`{"y":{"type":"nominal","sort":"-x"},"x":{"type":"quantitative"}}`)
	case "boxplot":
		return `{"type":"boxplot","extent":1.5}`, json.RawMessage(`{"x":{"type":"nominal"},"y":{"type":"quantitative"}}`)
	case "trellis_bar":
		// For trellis, the "encoding" is actually the facet channel.
		return `"bar"`, json.RawMessage(`{"field":"group","type":"nominal","columns":3}`)
	default:
		return `"bar"`, json.RawMessage(`{"x":{"type":"nominal","sort":"-y"},"y":{"type":"quantitative"}}`)
	}
}

// smartAggregateTime detects day-level time series and re-aggregates to
// week (≤90 day span) or month (>90 day span). Returns nil if no
// aggregation was needed. Also strips timezone suffixes for Vega
// compatibility. Returns (aggregatedRS, timeUnit, groupFields).
func smartAggregateTime(rs *storageanalytics.ResultSet) (*storageanalytics.ResultSet, string, []string) {
	if len(rs.Rows) == 0 {
		return nil, "", nil
	}

	// Find temporal and group fields from the data shape.
	var timeField string
	var valueFields []string
	var groupFields []string

	for _, col := range rs.Columns {
		// Check if this column looks temporal (contains dates).
		sample := rs.Rows[0][col]
		if sample == nil {
			continue
		}
		s := fmt.Sprintf("%v", sample)
		if len(s) >= 10 && (s[4] == '-' || strings.Contains(s, "T")) {
			// Looks like a date/datetime.
			if timeField == "" {
				timeField = col
				continue
			}
		}
		// Check if numeric.
		switch sample.(type) {
		case int, int32, int64, float32, float64:
			valueFields = append(valueFields, col)
		default:
			// Try parsing as number.
			if _, err := fmt.Sscanf(s, "%f", new(float64)); err == nil {
				valueFields = append(valueFields, col)
			} else if col != timeField {
				groupFields = append(groupFields, col)
			}
		}
	}

	if timeField == "" || len(valueFields) == 0 {
		// No time series — just strip timezones.
		stripTimezones(rs)
		return nil, "yearmonthdate", nil
	}

	// Parse dates and compute span.
	type datedRow struct {
		date time.Time
		row  map[string]any
	}
	var dated []datedRow
	for _, row := range rs.Rows {
		raw := row[timeField]
		if raw == nil {
			continue
		}
		s := fmt.Sprintf("%v", raw)
		s = strings.Replace(s, "+00:00", "", 1)
		s = strings.Replace(s, "Z", "", 1)
		var t time.Time
		for _, layout := range []string{
			"2006-01-02T15:04:05",
			"2006-01-02T15:04:05.000",
			"2006-01-02T15:04:05.000000",
			"2006-01-02",
		} {
			var err error
			t, err = time.Parse(layout, s)
			if err == nil {
				break
			}
		}
		if t.IsZero() {
			continue
		}
		dated = append(dated, datedRow{date: t, row: row})
	}

	if len(dated) <= 7 {
		// Too few points to aggregate — just strip timezones.
		stripTimezones(rs)
		return nil, "yearmonthdate", nil
	}

	// This used to coarsen a long-span day-level series down to week/month
	// buckets server-side, which silently threw away the daily rows a
	// chart's own Day/Week/Month toggle needs - the toggle re-buckets
	// client-side from whatever granularity reaches it, so a chart handed a
	// pre-aggregated 7-point monthly series can never show daily points
	// again, no matter what the toggle is set to. Always keep day-level
	// data now and let the frontend do the coarsening.
	_ = groupFields
	stripTimezones(rs)
	return nil, "yearmonthdate", nil
}

// stripTimezones removes +00:00 suffixes from temporal column values so
// Vega-Lite doesn't choke on "Incompatible time units".
func stripTimezones(rs *storageanalytics.ResultSet) {
	for _, row := range rs.Rows {
		for k, v := range row {
			if s, ok := v.(string); ok {
				if strings.Contains(s, "+00:00") {
					row[k] = strings.Replace(s, "+00:00", "", 1)
				}
			}
		}
	}
}

// toFloat converts a value to float64, returning 0 on failure.
func toFloat(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	default:
		var f float64
		fmt.Sscanf(fmt.Sprintf("%v", v), "%f", &f)
		return f
	}
}

// computeStats produces human-readable summary statistics for a result
// set, similar to prelivi's compute_stats.
func computeStats(rows []map[string]any, encoding map[string]any) []string {
	if len(rows) == 0 {
		return []string{"No data returned."}
	}

	var lines []string
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}

	// Try to identify time, category, and numeric fields from encoding.
	var timeField, categoryField string
	var numericFields []string

	if encoding != nil {
		for _, enc := range encoding {
			if m, ok := enc.(map[string]any); ok {
				f, _ := m["field"].(string)
				t, _ := m["type"].(string)
				switch t {
				case "temporal":
					timeField = f
				case "nominal", "ordinal":
					categoryField = f
				case "quantitative":
					numericFields = append(numericFields, f)
				}
			}
		}
	}

	// Fallback: detect from data.
	if len(numericFields) == 0 {
		for _, k := range keys {
			if toFloat(rows[0][k]) != 0 {
				numericFields = append(numericFields, k)
				break
			}
		}
	}

	nf := ""
	if len(numericFields) > 0 {
		nf = numericFields[0]
	}

	if timeField != "" && nf != "" {
		// Time series stats.
		type tv struct {
			t string
			v float64
		}
		var vals []tv
		for _, r := range rows {
			v := toFloat(r[nf])
			t := fmt.Sprintf("%v", r[timeField])
			vals = append(vals, tv{t: t, v: v})
		}
		if len(vals) > 0 {
			total := 0.0
			maxV, minV := vals[0], vals[0]
			for _, x := range vals {
				total += x.v
				if x.v > maxV.v {
					maxV = x
				}
				if x.v < minV.v {
					minV = x
				}
			}
			avg := total / float64(len(vals))
			lines = append(lines, fmt.Sprintf("Total: %.0f", total))
			lines = append(lines, fmt.Sprintf("Avg per period: %.1f", avg))
			lines = append(lines, fmt.Sprintf("Peak: %.0f (%s)", maxV.v, maxV.t))
			lines = append(lines, fmt.Sprintf("Low: %.0f (%s)", minV.v, minV.t))
			if len(vals) >= 2 && vals[0].v > 0 {
				changePct := ((vals[len(vals)-1].v - vals[0].v) / vals[0].v) * 100
				direction := "up"
				if changePct < 0 {
					direction = "down"
				} else if changePct == 0 {
					direction = "flat"
				}
				lines = append(lines, fmt.Sprintf("Trend: %s %.0f%% (%s -> %s)", direction, abs(changePct), vals[0].t, vals[len(vals)-1].t))
			}
			lines = append(lines, fmt.Sprintf("Data points: %d", len(vals)))
		}
	} else if categoryField != "" && nf != "" {
		// Category stats.
		type cv struct {
			c string
			v float64
		}
		var vals []cv
		for _, r := range rows {
			v := toFloat(r[nf])
			c := fmt.Sprintf("%v", r[categoryField])
			vals = append(vals, cv{c: c, v: v})
		}
		if len(vals) > 0 {
			total := 0.0
			maxV, minV := vals[0], vals[0]
			for _, x := range vals {
				total += x.v
				if x.v > maxV.v {
					maxV = x
				}
				if x.v < minV.v {
					minV = x
				}
			}
			avg := total / float64(len(vals))
			lines = append(lines, fmt.Sprintf("Total: %.0f", total))
			lines = append(lines, fmt.Sprintf("Across %d categories", len(vals)))
			lines = append(lines, fmt.Sprintf("Avg per category: %.1f", avg))
			lines = append(lines, fmt.Sprintf("Highest: %s (%.0f)", maxV.c, maxV.v))
			lines = append(lines, fmt.Sprintf("Lowest: %s (%.0f)", minV.c, minV.v))
		}
	} else {
		lines = append(lines, fmt.Sprintf("%d rows returned", len(rows)))
	}

	return lines
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
