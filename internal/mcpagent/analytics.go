package mcpagent

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/livisql"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/vlrender"
	storageanalytics "github.com/livereview/storage/analytics"
	"github.com/rs/zerolog/log"
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
	a.classifyPrompt = buildClassifyPrompt(tools)
	a.countQueryHead, a.countQueryTail = buildCountQueryPromptHalves(orgName, userName, a.mcpSession.OrgID)
	a.finalizeHead, a.finalizeTail = buildFinalizePromptHalves(orgName, userName, a.mcpSession.OrgID)

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
		done := a.runOneReport(ctx, entry, userText, clog)
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

	final := a.runFinalizePhase(ctx, entry, userText, count, clog)
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
	clog *logging.ChatTurnLogger,
) *FinalizePlan {
	base := fmt.Sprintf("Original question: %s\n\nThis report answers: %s\n\nThe result will contain %d rows.\n\nThe counting query used was:\n%s",
		userText, entry.Question, count, entry.CountSQL)
	user := base
	system := a.finalizePrompt(clog, entry.Question)

	for attempt := 1; attempt <= maxSQLAttempts; attempt++ {
		raw, err := a.completeOnce(ctx, clog, 3, "finalize", entry.ID, attempt, system, user)
		if err != nil {
			log.Error().Err(err).Str("report", entry.ID).Msg("analytics finalize call failed")
			return nil
		}
		plan, perr := parseFinalizePlan(raw)
		if perr == nil {
			// The model does not get to decide that "too much data for a chart" is fine.
			if plan.ResponseType == ResponseTypeChart && count > maxChartRows {
				plan.ResponseType = ResponseTypeCSV
			}
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

	maxRows := maxCSVRows
	if plan.ResponseType == ResponseTypeChart {
		maxRows = maxChartRows
	}

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

		if plan.ResponseType == ResponseTypeCSV || rs.Truncated {
			return a.buildCSVReport(entry, plan, rs, clog)
		}
		return a.buildChartReport(ctx, entry, plan, rs, clog)
	}

	return finishedReport{text: fmt.Sprintf("I could not fetch the data for %q.", entry.Question)}
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
		return a.buildCSVReport(entry, plan, rs, clog)
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
		spec["mark"] = mark
		spec["encoding"] = json.RawMessage(plan.Encoding)
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		log.Error().Err(err).Str("report", entry.ID).Msg("failed to marshal chart spec")
		return a.buildCSVReport(entry, plan, rs, clog)
	}
	normalized, err := vlrender.NormalizeVegaLiteSpec(specJSON)
	if err != nil {
		log.Warn().Err(err).Str("report", entry.ID).Msg("spec normalization failed, using raw spec")
		normalized = specJSON
	}

	clog.ReportFinalized(entry.ID, ResponseTypeChart, plan.Title, len(rs.Rows))
	return finishedReport{report: &vlrender.VegaLiteReport{
		Title:       plan.Title,
		Description: plan.Description,
		Query:       plan.Query,
		Spec:        normalized,
	}}
}

// buildCSVReport writes the result set to CSV. Column order comes from the
// result set rather than from map iteration, so the header matches the data.
func (a *Agent) buildCSVReport(
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

	description := plan.Description
	if rs.Truncated {
		description = strings.TrimSpace(description + fmt.Sprintf("\n\nThis export stops at %d rows. Narrow the question for a complete set.", len(rs.Rows)))
	}

	clog.ReportFinalized(entry.ID, ResponseTypeCSV, plan.Title, len(rs.Rows))
	return finishedReport{artifact: &Artifact{
		Kind:        "csv",
		Filename:    safeCSVFilename(firstNonEmpty(plan.CSVFilename, plan.Title, entry.Question)),
		Title:       plan.Title,
		Description: description,
		Query:       plan.Query,
		Data:        []byte(buf.String()),
		Rows:        len(rs.Rows),
	}}
}

// noDataText asks the model for one clean sentence. If that call fails the
// fallback is still a sentence, never an empty chart or a generic error.
func (a *Agent) noDataText(ctx context.Context, entry PlanEntry, userText string, clog *logging.ChatTurnLogger) string {
	raw, err := a.completeOnce(ctx, clog, 3, "no_data", entry.ID, 1, analyticsNoDataInstructions,
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
		analyticsRepairInstructions+orgIDFilterInstruction(a.mcpSession.OrgID),
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

	start := time.Now()
	text, usage, err := a.provider.Complete(ctx, []HistoryEntry{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}, nil)
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
				return ""
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
	out := string(payload)
	if len(notes) > 0 {
		// Notes precede the JSON: the render path strips the JSON block and
		// keeps surrounding prose as the message text.
		out = strings.Join(notes, "\n\n") + "\n" + out
	}
	return out
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
