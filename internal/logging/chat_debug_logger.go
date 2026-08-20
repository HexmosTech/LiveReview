package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Debug log for the Livi/Slack/Discord/Teams chat agent (see
// internal/mcpagent.Agent.RunTurn). Unlike ReviewLogger (one file per review),
// this is a single shared file so every session can be tailed/grepped
// together - lines are tagged [session_id][HH:MM:SS][source] for filtering.
//
// Enabled only when LIVI_DEBUG_LOG=true (off by default: full chat/tool
// content, including review comments, would otherwise sit in plaintext on
// disk for every prod conversation). The file is wiped at process boot via
// InitChatDebugLog, which must be called exactly once - only the process
// that owns the chat/bot code (the "livereview-api" pm2 app) should call it,
// since other pm2 processes (workers) never touch this file.

const chatDebugLogPath = "chat_debug_logs/chat_debug.log"

// dbctxDebugLogPath holds the full dbctx schema text that DBCtxResponse used
// to inline into chatDebugLogPath - one schema dump is tens of KB, and it
// drowned out everything else when grepping/tailing the main log for a
// specific turn. chat_debug.log keeps just the call/elapsed/len summary;
// the full table text lives here instead, correlated by the same
// [session_id][time][source] prefix.
const dbctxDebugLogPath = "chat_debug_logs/dbctx_debug.log"

var (
	chatLogEnabled bool
	chatLogFile    *os.File
	chatLogMu      sync.Mutex
	chatLogOnce    sync.Once

	dbctxLogFile *os.File
	dbctxLogMu   sync.Mutex
)

// InitChatDebugLog enables and wipes the shared chat debug log if
// LIVI_DEBUG_LOG is truthy. Safe to call multiple times; only the first call
// takes effect. Call once at process startup, before any chat traffic.
func InitChatDebugLog() {
	chatLogOnce.Do(func() {
		val := strings.ToLower(strings.TrimSpace(os.Getenv("LIVI_DEBUG_LOG")))
		if val != "true" && val != "1" {
			return
		}
		dir := filepath.Dir(chatDebugLogPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("[ChatDebugLog] failed to create log dir %s: %v\n", dir, err)
			return
		}
		// os.Create truncates an existing file - this is the boot-wipe.
		f, err := os.Create(chatDebugLogPath)
		if err != nil {
			fmt.Printf("[ChatDebugLog] failed to create log file %s: %v\n", chatDebugLogPath, err)
			return
		}
		chatLogFile = f
		chatLogEnabled = true
		fmt.Printf("[ChatDebugLog] enabled, writing to %s\n", chatDebugLogPath)

		// Same directory, same boot-wipe policy as chatLogFile - only the
		// destination differs. A failure here degrades DBCtxRequest/Response
		// to no-ops (see their nil check) rather than taking down the main log.
		if f, err := os.Create(dbctxDebugLogPath); err != nil {
			fmt.Printf("[ChatDebugLog] failed to create log file %s: %v\n", dbctxDebugLogPath, err)
		} else {
			dbctxLogFile = f
			fmt.Printf("[ChatDebugLog] enabled, writing to %s\n", dbctxDebugLogPath)
		}
	})
}

// dbctxFileWriter adapts dbctxLogFile to io.Writer for DBCtxDebugWriter,
// sharing dbctxLogMu with writeDBCtx so a concurrent per-turn DBCtx line and
// dbctx's own boot-time progress output ("4/4 Building search index...")
// never interleave mid-line.
type dbctxFileWriter struct{}

func (dbctxFileWriter) Write(p []byte) (int, error) {
	dbctxLogMu.Lock()
	defer dbctxLogMu.Unlock()
	if dbctxLogFile == nil {
		return len(p), nil
	}
	n, err := dbctxLogFile.Write(p)
	dbctxLogFile.Sync()
	return n, err
}

// DBCtxDebugWriter returns the writer for dbctx's own progress output (its
// Options.Logger) - the "N/N Building ... index" and terminology-import
// lines it prints itself, as opposed to the DBCtxRequest/DBCtxResponse lines
// this package writes around each call. Both end up in dbctxDebugLogPath so
// the whole dbctx lifecycle - boot-time index build through every per-turn
// query - lives in one file. Safe to use even when LIVI_DEBUG_LOG is off or
// InitChatDebugLog hasn't run yet: writes are silently discarded until the
// log file exists.
func DBCtxDebugWriter() io.Writer {
	return dbctxFileWriter{}
}

// ChatTurnLogger writes correlated debug lines for one chat session. Agents
// are reused across many sessions/conversations (cached per org by the
// bots), so the logger - not the agent - carries the session identity.
type ChatTurnLogger struct {
	sessionID string
	source    string
}

// NewChatTurnLogger returns a logger for one session. source should be one
// of "livi", "slack", "discord", "teams". Safe to construct even when
// LIVI_DEBUG_LOG is off - all methods become no-ops.
func NewChatTurnLogger(sessionID, source string) *ChatTurnLogger {
	return &ChatTurnLogger{sessionID: sessionID, source: source}
}

func (l *ChatTurnLogger) Enabled() bool {
	return l != nil && chatLogEnabled
}

func (l *ChatTurnLogger) write(label, format string, args ...interface{}) {
	if !l.Enabled() {
		return
	}
	chatLogMu.Lock()
	defer chatLogMu.Unlock()
	line := fmt.Sprintf("[%s][%s][%s] %s: %s\n",
		l.sessionID, time.Now().Format("15:04:05"), l.source, label, fmt.Sprintf(format, args...))
	chatLogFile.WriteString(line)
	chatLogFile.Sync()
}

// writeDBCtx mirrors write but targets dbctxLogFile instead of chatLogFile -
// see dbctxDebugLogPath's doc comment for why DBCtx lines split out here.
func (l *ChatTurnLogger) writeDBCtx(label, format string, args ...interface{}) {
	if !l.Enabled() || dbctxLogFile == nil {
		return
	}
	dbctxLogMu.Lock()
	defer dbctxLogMu.Unlock()
	line := fmt.Sprintf("[%s][%s][%s] %s: %s\n",
		l.sessionID, time.Now().Format("15:04:05"), l.source, label, fmt.Sprintf(format, args...))
	dbctxLogFile.WriteString(line)
	dbctxLogFile.Sync()
}

func (l *ChatTurnLogger) Context(orgName, userEmail, providerModel string) {
	l.write("Context", "org=%q user=%q ai=%q", orgName, userEmail, providerModel)
}

func (l *ChatTurnLogger) UserInput(text string) {
	l.write("User Input", "%s", text)
}

// call is the LLM call number from livi_analytics_plan.md's "Call #0"
// diagram (0=classify, 1=action, 2=count-query proposal, 3=finalize), so a
// log line can be matched straight to a box in the diagram without having
// to infer it from the preceding Branch Selected line. 0 on these
// step-numbered lines means "not part of the numbered pipeline" (the chat
// branch, or a plain tool-only agent with analytics disabled) rather than
// classify - classify never goes through AIRequest/AIResponse, it has its
// own LLMCallRequest/LLMCallResponse lines below, always tagged call=0.
func (l *ChatTurnLogger) AIRequest(call, step int, historyJSON string) {
	l.write("AI Request", "call=%d step=%d payload=%s", call, step, redactSecrets(historyJSON))
}

func (l *ChatTurnLogger) AIResponse(call, step int, elapsed time.Duration, tokensIn, tokensOut int, response string) {
	l.write("AI Response", "call=%d step=%d elapsed=%s tokens_in=%d tokens_out=%d %s",
		call, step, elapsed.Round(time.Millisecond), tokensIn, tokensOut, response)
}

func (l *ChatTurnLogger) AIError(call, step int, elapsed time.Duration, err error) {
	l.write("AI Error", "call=%d step=%d elapsed=%s error=%v", call, step, elapsed.Round(time.Millisecond), err)
}

// LLMCallRequest/LLMCallResponse/LLMCallError log the one-shot calls that run
// outside the main step loop (classify, analytics finalize, no-data, and SQL
// repair - see mcpagent.Agent.completeOnce and classify.go). Each one is
// otherwise invisible: it does not carry a step number, so call/kind/report/
// attempt together are what let you tell which of a turn's several off-loop
// calls produced a given line. call is the diagram call number (see
// AIRequest's doc comment) - repair and no-data reuse the number of the call
// whose SQL/report they are patching (2 for a count-phase repair, 3 for a
// data-phase repair or a no-data substitute for finalize), since neither is
// a distinct box in the diagram.
func (l *ChatTurnLogger) LLMCallRequest(call int, kind, reportID string, attempt int, payload string) {
	l.write("AI Request", "call=%d kind=%s report=%s attempt=%d payload=%s", call, kind, reportID, attempt, redactSecrets(payload))
}

func (l *ChatTurnLogger) LLMCallResponse(call int, kind, reportID string, attempt int, elapsed time.Duration, tokensIn, tokensOut int, response string) {
	l.write("AI Response", "call=%d kind=%s report=%s attempt=%d elapsed=%s tokens_in=%d tokens_out=%d %s",
		call, kind, reportID, attempt, elapsed.Round(time.Millisecond), tokensIn, tokensOut, response)
}

func (l *ChatTurnLogger) LLMCallError(call int, kind, reportID string, attempt int, elapsed time.Duration, err error) {
	l.write("AI Error", "call=%d kind=%s report=%s attempt=%d elapsed=%s error=%v", call, kind, reportID, attempt, elapsed.Round(time.Millisecond), err)
}

// DBCtxRequest/DBCtxResponse log dbctx's schema lookup - the Go-side call
// that builds the "### Tables" section of call #2/#3's prompt (see
// mcpagent/schema_render.go's dbctxTableText). This is NOT an LLM call and
// the model never invokes it directly; it runs before the LLM call to
// assemble that call's system prompt, so these lines appear just before the
// AIRequest/LLMCallRequest line for the call whose prompt they fed.
//
// The full schema text goes to dbctxDebugLogPath only (see writeDBCtx) -
// chat_debug.log gets just the call/elapsed/len summary, so tailing/grepping
// a turn there isn't drowned out by a multi-KB table dump on every call.
func (l *ChatTurnLogger) DBCtxRequest(call int, role, queryText string) {
	l.write("DBCtx Request", "call=%d role=%s query=%q", call, role, queryText)
	l.writeDBCtx("DBCtx Request", "call=%d role=%s query=%q", call, role, queryText)
}

func (l *ChatTurnLogger) DBCtxResponse(call int, elapsed time.Duration, tableText string, err error) {
	if err != nil {
		l.write("DBCtx Response", "call=%d elapsed=%s degraded=true error=%v", call, elapsed.Round(time.Millisecond), err)
		l.writeDBCtx("DBCtx Response", "call=%d elapsed=%s degraded=true error=%v", call, elapsed.Round(time.Millisecond), err)
		return
	}
	l.write("DBCtx Response", "call=%d elapsed=%s len=%d", call, elapsed.Round(time.Millisecond), len(tableText))
	l.writeDBCtx("DBCtx Response", "call=%d elapsed=%s len=%d %s", call, elapsed.Round(time.Millisecond), len(tableText), tableText)
}

// BranchSelected records call #0's classify decision and which (prompt,
// tools) pair got swapped into the turn's system message, at the point the
// decision is made - not left to be inferred later from which shape of
// prompt/response shows up downstream. This is the one line that answers
// "why did this turn go down the wrong path" for the call #0/dispatch split
// (see livi_analytics_plan.md's "Call #0" section).
func (l *ChatTurnLogger) BranchSelected(shape string, promptLen, toolCount int) {
	l.write("Branch Selected", "shape=%s prompt_len=%d tools=%d", shape, promptLen, toolCount)
}

// SchemaSourceDegraded records that a count_query-branch turn rendered the
// static fallback table list instead of the live dbctx-derived schema,
// because the index wasn't ready or failed to build (see
// dbctx_schema_plan.md's Rollout/risk section). The index failure itself is
// logged once, at process startup, via zerolog - this is the per-turn
// counterpart: it answers "was this specific bad SQL caused by a stale
// schema" for a turn that otherwise looks identical in the log to one that
// ran against the real schema.
func (l *ChatTurnLogger) SchemaSourceDegraded(reason string) {
	l.write("Schema Source Degraded", "reason=%s", reason)
}

func (l *ChatTurnLogger) ToolCall(step int, name, argsJSON string) {
	l.write("Tool Call", "step=%d %s %s", step, name, redactSecrets(argsJSON))
}

func (l *ChatTurnLogger) ToolResponse(step int, name string, elapsed time.Duration, rawLen int, truncated bool, content string) {
	l.write("Tool Response", "step=%d %s elapsed=%s len=%d truncated=%v %s",
		step, name, elapsed.Round(time.Millisecond), rawLen, truncated, content)
}

func (l *ChatTurnLogger) ToolError(step int, name string, elapsed time.Duration, err error) {
	l.write("Tool Error", "step=%d %s elapsed=%s error=%v", step, name, elapsed.Round(time.Millisecond), err)
}

func (l *ChatTurnLogger) FinalResponse(text string) {
	l.write("Final Response To User", "%s", text)
}

func (l *ChatTurnLogger) StepLimitHit(maxSteps int) {
	l.write("Error", "hit step limit (%d steps) without a final answer", maxSteps)
}

// SQL analytics phases. The whole reason Livi's wrong numbers were hard to
// diagnose is that nothing recorded what was actually computed - so every
// generated statement, every rejection and every row count is logged verbatim
// here. reportID identifies one entry of a multi-report turn; phase is "count"
// or "data".

// SQLPlan records the report array the model proposed for this turn.
func (l *ChatTurnLogger) SQLPlan(step int, planJSON string) {
	l.write("SQL Plan", "step=%d %s", step, planJSON)
}

// SQLGenerated records the model's raw SQL before the guard touches it, so a
// rejection can be traced back to exactly what was written. attempt
// distinguishes the original statement (1) from repair retries (2, ...) -
// without it, a rewritten repair line and the original are indistinguishable
// except by reading the SQL text itself.
func (l *ChatTurnLogger) SQLGenerated(reportID, phase string, attempt int, rawSQL string) {
	l.write("SQL Generated", "report=%s phase=%s attempt=%d %s", reportID, phase, attempt, collapseSQL(rawSQL))
}

// SQLRewritten records the org-scoped statement that will actually execute.
// This is the ground truth for "where did this number come from".
func (l *ChatTurnLogger) SQLRewritten(reportID, phase string, attempt int, rewritten string) {
	l.write("SQL Rewritten", "report=%s phase=%s attempt=%d %s", reportID, phase, attempt, collapseSQL(rewritten))
}

// SQLRejected records a guard refusal and the attempt number, so a retry loop
// that is not converging is visible rather than silent.
func (l *ChatTurnLogger) SQLRejected(reportID, phase string, attempt int, reason string) {
	l.write("SQL Rejected", "report=%s phase=%s attempt=%d reason=%s", reportID, phase, attempt, reason)
}

// SQLResult records what the database returned.
func (l *ChatTurnLogger) SQLResult(reportID, phase string, attempt int, elapsed time.Duration, rows int, truncated bool) {
	l.write("SQL Result", "report=%s phase=%s attempt=%d elapsed=%s rows=%d truncated=%v",
		reportID, phase, attempt, elapsed.Round(time.Millisecond), rows, truncated)
}

// SQLError records an execution failure (timeout, unresolved relation, ...).
func (l *ChatTurnLogger) SQLError(reportID, phase string, attempt int, elapsed time.Duration, err error) {
	l.write("SQL Error", "report=%s phase=%s attempt=%d elapsed=%s error=%v",
		reportID, phase, attempt, elapsed.Round(time.Millisecond), err)
}

// ReportFinalized records how one report ended up being presented.
func (l *ChatTurnLogger) ReportFinalized(reportID, responseType, title string, rows int) {
	l.write("Report Finalized", "report=%s type=%s rows=%d title=%q", reportID, responseType, rows, title)
}

// collapseSQL folds a multi-line statement onto one line so the log stays
// greppable - a multi-line query would otherwise swamp the file it shares
// with every other session.
func collapseSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var secretPattern = regexp.MustCompile(`(?i)"(authorization|bearer|api[_-]?key|token|password|secret)"\s*:\s*"([^"]{0,4})[^"]*"`)

// redactSecrets masks likely-sensitive values in JSON-ish text before it hits
// disk in plaintext. Best-effort, not a substitute for not logging secrets.
func redactSecrets(s string) string {
	return secretPattern.ReplaceAllString(s, `"$1":"$2***"`)
}
