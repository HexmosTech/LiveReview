# Livi SQL analytics — implementation plan

Companion to `livi_analytics_flow.mmd` (the flow diagram). This document is the
implementation detail behind that diagram.

## Context

Livi's analytics answers are wrong. Asked for monthly review counts it called
`GET_api_v1_reviews?per_page=200`, pulled ~35KB of raw rows into its context and
counted them in its own reasoning — reporting 51 reviews for May and 16 for June
when the database holds 55 and 18. Percentage-change questions are unverifiable for
the same reason, and "average reviews per engineer per day" degrades as rows grow.

The cause is architectural, not a prompt bug: an LLM is doing counting, grouping,
ranking and arithmetic over raw rows. No prompt or calculator tool fixes that — a
calculator only helps once you already hold the right numbers, and the error happens
upstream of any arithmetic.

The fix: move all aggregation into Postgres. The LLM writes SQL and decides
presentation; Postgres computes every number; Go injects those numbers into the
chart verbatim so the model never re-types a value.

Locked-in decisions:
- Read-only via `BeginTx(ReadOnly: true)` + `SET LOCAL statement_timeout` on the
  **existing** pool. No new Postgres role, no new DSN, zero ops change.
- SQL parsed with a **pure-Go** Postgres parser — `CGO_ENABLED=0` in all
  Dockerfiles rules out `pg_query_go`.
- CSV delivered via a new download endpoint + UI download link.
- Raw-row analytics tools are **removed** from the agent outright.

## The security model (read this first)

Three things drive the design and are easy to get wrong:

**1. Do not *prove* org scoping — *enforce* it.** "Check that an `org_id` predicate
scopes every table" is not soundly decidable from an AST: `org_id = (SELECT ...)`,
a predicate sitting in a `LEFT JOIN ... ON` clause (which does not filter), a
correlated subquery, `CASE WHEN`. Any such checker is a heuristic guarding the
tenant boundary.

Instead Go **rewrites** the query, prepending shadow CTEs that shadow the real
table names:

```sql
WITH reviews AS NOT MATERIALIZED (SELECT ... FROM public.reviews WHERE org_id = $1),
     repositories AS NOT MATERIALIZED (...),
     ...
<the LLM's SELECT, unchanged>
```

Postgres resolves the unqualified name `reviews` to the CTE, so the org filter is
applied by the planner no matter what the LLM wrote. The LLM's own predicates can
only further restrict rows *inside* an already-scoped relation — `org_id = 1 OR 1=1`
is harmless. The validator's job shrinks to rejecting anything that could escape the
shadow: schema-qualified names (`public.reviews`), `FROM ONLY`, CTE names colliding
with allowlisted tables, and non-allowlisted relations or functions.

**2. `users` holds secrets.** `users.password_hash` and `users.onboarding_api_key`
are real columns in `db/schema.sql`. The `users` shadow projects an explicit safe
column list, so `SELECT password_hash FROM users` fails with "column does not
exist". `users` also has no `org_id` — membership is via `user_roles`, so its shadow
joins through it.

**3. Raw SQL bypasses the role gating the REST tools enforced.**
`GET_api_v1_billing_usage_members` is owner-gated today. Handing every member SQL
over `loc_usage_ledger` / `org_billing_state` is privilege escalation. The table
allowlist is therefore a function of the caller's role: `livisql.CatalogFor(role)`.
Surfaces that authenticate an org rather than a user (the Slack/Discord/Teams bots)
default to the **member** catalog.

## Architecture

> **Numbering note:** call numbers below are fixed to *what a call does*, not
> to *turn order* — only one of call 1 or (call 2 → call 3) ever runs on a
> given turn, since action and count_query are alternate branches, not
> sequential stages. See [Call #0](#call-0-classify-before-paying-for-schema)
> for why the branch split exists and what each call's prompt does and
> doesn't contain.

```
LLM call #0  (question + recent history + tool NAMES only, NO schema, NO param schemas)
   -> {"shape": "action" | "count_query" | "chat"}
          │
   ┌──────┼──────────────────────────────┐
   │      │                              │
"action"  "count_query"                "chat"
   │      │                              │
   ▼      ▼                              ▼
LLM call #1                       LLM call #2                     plain-text
(tools, NO schema,                (schema + SQL rules,            call, no
NO SQL rules)                     NO general tool list)           schema/tools
   │                              emits {"analytics_plan":
   ▼                                [{id, question, count_sql}, ...]}
existing MCP action path,                │
unchanged                                └─ per entry (sequential, bounded fan-out):
                                               validate+rewrite -> COUNT in read-only txn
                                                  ├─ count == 0 -> no_data prose, no data query at all
                                                  └─ LLM call #3 (fresh 3-message context, sees the count)
                                                        -> response_type + data_sql + mark/encoding + title/description
                                                     validate+rewrite -> run -> dynamic scan -> []map[string]any
                                                        ├─ chart: encoding fields must exist in result columns;
                                                        │         Go builds the spec, rows go in data.values verbatim
                                                        └─ csv:   Go writes the file, registers a download URL
```

The model never sees a data row and never emits a number that came from data.

## Package layout

```
internal/livisql/                 pure logic, no database/sql import
  catalog.go       role-scoped table allowlist + shadow CTE bodies + function allowlist
  guard.go         Validate() and Rewrite(): parse, walk, reject, prepend CTEs, deparse
  errors.go        RejectionError{Code, Detail, LLMHint}
  guard_test.go    attack corpus — the security regression suite
  catalog_test.go  drift test: every table/column named here exists in db/schema.sql

storage/analytics/               owns *sql.DB, mirrors storage/reviews/ style
  adhoc_store.go   AdHocStore: read-only txn, timeouts, Count(), Query()
  coerce.go        driver value -> JSON-safe any

internal/mcpagent/
  analytics.go        orchestration, fan-out, report assembly, CSV writing
  analytics_types.go  plan/finalize JSON shapes + parsers
  agent.go            MODIFIED: WithAnalytics, tool filtering, RunTurn hook
  types.go            MODIFIED: MCPSession.OrgID, MCPSession.UserRole, Artifact
  prompts/analytics_schema.md, analytics_instructions.md, analytics_finalize.md,
          description_style.md (extracted, shared)
  prompts/agent_instructions.md  MODIFIED: delete the "count rows yourself" guidance

internal/api/chat_files.go       generalized TTL file registry (charts + CSV)
internal/api/webchat_handler.go  MODIFIED: wire engine, artifacts -> Files
internal/logging/chat_debug_logger.go  MODIFIED: SQL-phase methods
ui/src/api/chatbot.ts, ui/src/pages/Chatbot/Chatbot.tsx
```

Dependency direction `livisql <- storage/analytics <- mcpagent <- internal/api`; no
cycles. `livisql` needs no DB, so its security tests run without one.

## Parser

**`github.com/wasilibs/go-pgquery`** — libpg_query (the actual Postgres grammar)
compiled to WASM on wazero: pure Go, builds under `CGO_ENABLED=0`, and `wazero` is
already an indirect dependency via gitleaks. Grammar fidelity matters here: a
dialect-approximating parser means "parser sees X, server executes Y". It also
exposes `Deparse()`, so we execute the canonicalized deparse of the tree we
validated rather than the LLM's raw string, which structurally kills comment and
whitespace tricks.

Accepted costs: the module is untagged (pin the pseudo-version), it embeds a
multi-MB wasm blob (measure the binary delta), and first parse pays a wazero compile
cost (warm it in a goroutine at boot). Fallbacks if supply-chain policy rejects it:
`auxten/postgresql-parser` (tagged, pure Go, but unmaintained since 2021 and CRDB
dialect) then `cockroachdb/cockroachdb-parser`. **Spike the parser against our real
analytics SQL — `date_trunc`, `FILTER (WHERE ...)`, window functions, CTEs — before
building on it.**

## Validator rules

Pre-parse: reject `len(sql) > 8000`.

1. Exactly one statement, and it is a `SelectStmt`.
2. Reject `WITH RECURSIVE`, `intoClause`, `lockingClause` (`FOR UPDATE/SHARE`), and
   any `ParamRef` — the rewrite owns `$1`.
3. Walk the whole tree (every subquery, sublink, `LATERAL`, set-op arm):
   - `RangeVar`: `schemaname` must be empty, `inh` default (no `FROM ONLY`), and
     `relname` in the role's catalog **or** a CTE visible in scope.
   - `CommonTableExpr`: `ctename` must not collide with an allowlisted table.
   - `FuncCall`/`RangeFunction`: name must be in the function **allowlist**
     (`count, sum, avg, min, max, round, coalesce, date_trunc, extract, to_char,
     now, rank, row_number, percentile_cont, ...`). An allowlist can't be outrun by
     a function nobody thought of; a denylist can.

Rejections return a `RejectionError` whose `LLMHint` is the only thing fed back on
retry.

## Executor

```go
tx, _ := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelReadCommitted})
defer tx.Rollback()
tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeoutMS))
tx.ExecContext(ctx, "SET LOCAL idle_in_transaction_session_timeout = 15000")
tx.ExecContext(ctx, "SET LOCAL search_path = ''")
rows, _ := tx.QueryContext(ctx, rewritten, orgID)
```

`search_path = ''` means any unqualified relation that is not one of our CTEs fails
to resolve at all rather than binding to a real table. Shadow bodies are
`public.`-qualified so they still work.

Dynamic scan via `rows.Columns()` + `[]any`; reject duplicate/empty column names
with the hint "give every selected expression a unique alias". `coerce` must be
exhaustive — this is where JSON marshalling breaks:
- `time.Time` → `t.UTC().Format(time.RFC3339)` (date_trunc buckets are `timestamptz`;
  Vega-Lite `"temporal"` wants ISO-8601)
- `[]byte` → JSON/JSONB: unmarshal; NUMERIC/DECIMAL: `ParseFloat` (lib/pq returns
  numerics as `[]byte`, so `avg()` would otherwise render as base64); else string
- `float64` NaN/Inf → `nil` (`json.Marshal` errors on them; `avg()` over an empty
  group produces them)
- NULL → `nil`

## Call #0: classify before paying for schema

**Added after the fact, once tokens started to matter.** The original design
had one call decide *and* act: it always carried the full schema + SQL rules
text in its system prompt, whether or not the turn ended up being an action
("trigger a review for repo X") or a data question ("reviews per month").
Since pricing is per-token, not per-call, that schema text was paid for on
every action turn even though action turns never used it — pure waste, and
action turns are likely the majority of traffic (most chat turns are "do a
thing," not "chart a thing").

**Fix: split classification into its own small call, in front of everything
else — and give each downstream purpose its own fixed call number, so no
call's prompt is shared between two unrelated jobs.** Call numbers below are
tied to *what the call does*, never to turn order — only one of call 1 or
(call 2 → call 3) runs per turn, they are alternate branches from call 0, not
four calls in sequence:

```
call #0 (small, classify only)  -> {"shape": "action" | "count_query" | "chat"}
   ├─ action      -> call #1 (tools, NO schema, NO SQL rules)
   │                 -> emits the tool call directly -> existing MCP action path, unchanged
   ├─ count_query -> call #2 (schema + SQL rules, NO general tool list)
   │                 -> emits analytics_plan[] (as today's original call #1 did)
   │                 -> per report entry -> call #3 (finalize, sees the row count)
   └─ chat         -> plain-text call, no schema, no tools
```

Call #0's prompt is deliberately minimal: the user's question (+ recent
history for follow-ups like "now break that down by repo"), a short list of
**tool names only** (no parameter schemas — full schemas are only needed by
call #1, which is the one that actually picks a tool and fills its args), and
the three-way shape instruction. No `analyticsSchema`, no
`agentInstructions` tool-calling rules. This is the one call every turn pays
for regardless of outcome, so it has to stay small on principle, not just
today's table count.

Once call #0 returns `"action"`, the turn goes **directly** to call #1 — it
never touches `analyticsSchema`/`analyticsPlanInstructions` at all, because
those are call #2's prompt pieces exclusively now. That's the actual point of
splitting the numbering: before this, "the call that handles actions" and
"the call that handles data questions" were the same call with a merged
prompt, so an action turn always paid for schema text it would never use.
After the split, call #1's system prompt contains *only* tool descriptions +
`agentInstructions`' tool-calling rules — never the SQL/schema block.

**Where this changes the existing single-call design:** `buildSystemPrompt`
(the one that unconditionally appends `analyticsSchema` +
`analyticsPlanInstructions` when `withAnalytics`) stops being called once per
turn with a fixed shape. It's now called *after* call #0 resolves, building
call #1's prompt (tools + `agentInstructions`, no analytics block at all) or
call #2's prompt (`analyticsSchema` + `analyticsPlanInstructions`, no general
tool-list section — a data-answer turn never calls a REST tool) depending on
the branch chosen.

**Cost accounting, honestly — this is not a free win:**
- **Action turn**: before, 1 call (heavy: schema + tools + rules, all merged).
  After, 2 calls (call #0: cheap classify + call #1: tools only, no schema).
  Net token cost is very likely *lower* — the schema block was multiple
  hundred tokens paid for nothing — but it's now 2 round trips instead of 1,
  so **latency goes up** on the majority-case turn to save tokens on it. That
  trade only makes sense if the schema block is large enough, or traffic
  skewed enough toward actions, to matter more than the added round trip.
  Worth measuring against real traffic before committing, not just assuming.
- **Count-query turn**: before, 1 call (call #1, doing double duty) + 1 call
  #2-equivalent (finalize) per report. After, call #0 + call #2 (plan) before
  report fan-out even starts, so a 3-report answer that used to cost 1 + 3 =
  4 calls now costs 1 (call #0) + 1 (call #2) + 3 (call #3 × 3 reports) = 5.
  More calls, but each one is smaller — call #0 doesn't repeat the schema,
  and call #2 no longer needs to also carry the general tool-calling
  instructions it never uses on this branch.
- **Model choice for call #0**: since its whole job is a 3-way shape decision
  over a short prompt, it's the one call in this pipeline that's a candidate
  for a cheaper/smaller model than the rest of the pipeline uses, if the
  provider abstraction (`internal/llm`) supports per-call model overrides.
  Worth checking before assuming it has to run on the same model as
  everything else.
- **Risk**: a 3-way classifier can be wrong — "chat" vs "count_query" is
  usually clear, but "action" vs "count_query" has a real ambiguous zone
  ("show me my failed reviews" could be either a filtered list tool call or a
  count/chart). A misclassification into `action` when the user wanted data
  means a wasted round trip through call #1's tool-calling path before the
  user re-asks — worse than today's single merged call, which at least
  always has both options available in the same context to pick correctly
  the first time.
  This is the main argument *against* this split, not just a cost note: it
  trades some first-try accuracy for average token savings. Needs a small
  eval set of ambiguous real questions before shipping, not just the happy
  path.

### Implementation: per-turn system prompt selection (not once-per-session)

`agent.go` today bakes the system prompt in once, on the first turn only:

```go
if len(history) == 0 && a.systemPrompt != "" {
    history = append(history, HistoryEntry{"role": "system", "content": a.systemPrompt})
}
```

`a.systemPrompt` and `a.providerTools` are also fixed fields on `*Agent`, set
once at `NewAgent`/`WithAnalytics` time. That's a real conflict with the call
#0 split: which branch a turn needs is decided fresh every turn, so a session
that asks a data question, then says "now trigger a review for that repo,"
then asks another data question needs a *different* system prompt each of
those three turns — not the one baked in at turn one. Bolting the split onto
the existing single-prompt assumption would silently keep sending the wrong
(or a stale merged) prompt after the first turn. This needs to change, not
just be worked around:

1. **Precompute three (prompt, tools) pairs at construction, not one.**
   `NewAgent`/`WithAnalytics` build `actionPrompt`/`actionTools`
   (tool list + `agentInstructions`, no schema), `countQueryPrompt`/`nil`
   tools (`analyticsSchemaIntro` + plan instructions, no REST tools), and
   `chatPrompt`/`nil` tools (bare persona/org context only) once. Cheap —
   it's string concatenation, not a call.
2. **Call #0 runs on its own small, independently-bounded input — never the
   full session `history`.** Last ~2-3 exchanges + the new user message +
   tool *names* only. If it read the whole growing `history`, its cost would
   creep up over a long conversation and quietly defeat the reason it exists.
3. **`history[0]` gets replaced every turn, not appended once.** After call
   #0 resolves `shape`, pick the matching (prompt, tools) pair and
   overwrite `history[0]` (insert if absent) before entering the existing
   step loop with that branch's tool list. This is not unusual — providers
   re-apply the system message fresh on every completion call regardless, so
   this only fixes the bug where *this codebase's* loop wrote it once. The
   rest of `history` (actual conversation turns) is untouched, so switching
   branches turn-to-turn keeps full conversational context.
4. **The in-loop shape-sniffing (`parseToolCalls` → `parseAnalyticsPlan`
   fallback) simplifies.** It only existed because one call used to do both
   jobs. After the split, the `count_query` branch's call #2 can expect
   `analytics_plan` directly (falling back to plain text if the model
   doesn't comply, per the existing error handling), and the `action`
   branch's call #1 can expect a tool call directly.

Net effect: no new token/call cost beyond what the branch split already
costs — this is pure control-flow correctness, the piece that makes "call #0
decides, call #1/#2/#3 just act" actually hold across a multi-turn session
instead of only on message one. `analytics_pipeline_test.go`'s assumption
that the system prompt is injected once will need updating to match.

## Call #2 → call #3 protocol (count_query branch, after call #0 routes here)

Call #2 is the count_query branch's only job: no `tool` shape to discriminate
any more (that shape belongs entirely to call #1 now, on the action branch).
`parseToolCalls` already ignores JSON without a `tool` field, so **it needs no
change** — call #2's output is always `analytics_plan`:

```json
{"analytics_plan": [
  {"id":"r1","question":"Reviews per month in 2026",
   "count_sql":"SELECT count(*) AS n FROM (SELECT date_trunc('month', created_at) AS m FROM reviews GROUP BY 1) t"}
]}
```

`count_sql` counts the rows *the answer* would have, not rows scanned — that is what
makes the chart/csv decision meaningful. Say so in the prompt with this exact
subquery-wrapping example.

Call #3 runs per entry in a **fresh 3-message conversation** (not the main history):
cheaper, and isolates one report's retries from the others. Via
`provider.Complete(ctx, hist, nil)` — `nil` tools skips `WithTools`.

```json
{"response_type":"chart","title":"...","description":"...","query":"...",
 "data_sql":"SELECT date_trunc('month', created_at) AS month, count(*) AS review_count FROM reviews GROUP BY 1 ORDER BY 1",
 "mark":"bar",
 "encoding":{"x":{"field":"month","type":"temporal"},
             "y":{"field":"review_count","type":"quantitative"}}}
```

Go overrides the model regardless of what it asked for:
- `count == 0` → force `no_data`, never run `data_sql`
- chart: every `encoding[*].field` must exist in `rows.Columns()`, else retry
- `count > 500` and chart → reject with "aggregate coarser, or return csv"

Go builds the spec itself — `{$schema, width:600, height:340, data:{values:rows},
mark, encoding, title}` → `vlrender.NormalizeVegaLiteSpec` → existing render path.

## Integration

**Preserve the `{"reports":[...]}` string contract.** `RunTurn` keeps returning a
string; the analytics path marshals `[]vlrender.VegaLiteReport` into it. So
`renderImagesFromVega`, Slack, Discord, Teams and `mcpagent_handler` need **zero
changes**.

CSV bytes don't fit in a string, and the `Agent` is shared across concurrent
sessions by the bots (so no mutable state on it — that would cross-deliver files
between users). Hence:

```go
type Artifact struct{ Kind, Filename, Title, Description, Query string; Data []byte; Rows int }

func (a *Agent) RunTurnWithArtifacts(ctx, history, userText, sessionID, source string)
    (string, []HistoryEntry, []Artifact, error)

func (a *Agent) RunTurn(...) (string, []HistoryEntry, error)  // thin wrapper, unchanged signature
```

All five existing call sites keep compiling untouched; `HandleWebChat` switches to
the new method.

Wiring: `MCPSession` gains `OrgID` and `UserRole`. `NewAgent` is unchanged; add
`func (a *Agent) WithAnalytics(engine AnalyticsEngine) *Agent`. With `engine == nil`
or `OrgID == 0` behaviour is byte-identical to today, so web chat can ship first and
bots follow in a later commit without a flag. `AnalyticsEngine` is an interface
declared in `mcpagent` and implemented by `*analytics.AdHocStore`, keeping the
package unit-testable with a fake.

Tool filtering happens in `NewAgent` when analytics is on, before both `FormatTools`
and `buildSystemPrompt`: drop `GET_api_v1_reviews`,
`GET_api_v1_billing_usage_members`, `..._operations`, `..._summary`. Keep the
`_id_*` detail tools — "tell me about review 42" is worse in SQL. Log a warning if a
denylisted name is *absent* from the MCP tool list; that means a route was renamed
and the filter silently stopped working.

Prompt surgery in `agent_instructions.md`: delete "Aggregation: you CAN count,
group, sort yourself", the `per_page=200` pagination block, and the "Common
patterns" list — those are the instructions that produced the miscount. Keep the
side-effecting-action rules verbatim, and extract the description style rules
(short lines, `\n\n`, active voice, org name verbatim, humanized dates) into
`description_style.md` so call #3 shares them.

## CSV endpoint + UI

Generalize the chart registry (`chartFiles` map, TTL, throttled cleanup) out of
`webchat_handler.go` into `internal/api/chat_files.go`, keyed by id with
`{Path, TmpDir, Filename, ContentType, OrgID, CreatedAt}`. `ServeChartPNG` reads
from it unchanged.

`func (s *Server) ServeChatCSV(c echo.Context) error` → `c.Attachment(path, filename)`,
route `v1.GET("/chat/csv/:id", s.ServeChatCSV)`.

**The CSV route requires auth and must compare `pc.OrgID` against the registry
entry's `OrgID`.** `/chat/charts/:id` is currently reachable with only a random
8-byte id; a CSV is a bulk org-data export and must not inherit that laxity.
(Tightening the chart route is worth doing separately.)

`WebChatResponse` gains `Files []WebChatFile`. UI: add `ChatFile`/`files` to
`chatbot.ts` (plus the currently-dropped `sessionId`, which breaks debug-log
correlation across turns today), and a download card in `Chatbot.tsx` reusing the
existing `downloadImage` blob pattern. No new page/route/settings tab, so no
mega-menu entry is needed.

## Guardrails

The codebase has already shipped one unbounded-retry bug (the auth-retry storm fixed
by `isAuthError`), so every loop is a plain counted `for` with no `continue` that
can skip the increment, and no recursion.

| Limit | Value |
|---|---|
| reports per turn | 4 (truncate, and say so in the response) |
| SQL attempts | 2 per slot (count and data have separate budgets) |
| LLM calls per turn | 14 hard ceiling (`1 + 1 + 4×3`: call #0 + call #2 + up to 3 per report's call #3 retries × 4 reports); tripping it is a bug — log loudly |
| `statement_timeout` | 8000ms (`LIVI_SQL_TIMEOUT_MS`) |
| `idle_in_transaction_session_timeout` | 15000ms |
| rows (csv) | 5000 (fetch 5001 to detect truncation) |
| rows (chart) | 500, beyond which force csv |
| SQL length | 8000 chars, pre-parse |
| turn wall clock | 90s around the whole fan-out |

The diagram's `RO1 --execution error--> P1Q` back-edge must be implemented as "retry
within this report's budget", **not** a jump back to call #2 (the plan-proposing
call) — that would be an unbounded cycle. On exhaustion the report degrades to a
plain-text apology and the other reports still render; one bad report never fails
the turn.

No `LIMIT` is ever injected — truncating would render a partial chart that looks
complete. Oversized results are rejected with a "group more coarsely" hint instead.

Fan-out is **sequential** initially (each report holds a pool connection *and* makes
an LLM call); bounded concurrency is a follow-up once latency is measured.

Keep `LIVI_SQL_ANALYTICS` (default true) as an ops kill switch — not a rollout gate,
an escape hatch if the guard misbehaves in production. `WithAnalytics(nil)` is
already the identity behaviour, so flipping it off restores today's path.

## Observability

> **Corrected against the actual shipped logger.** `internal/logging/chat_debug_logger.go`
> already implements most of the SQL-phase logging this section originally
> proposed under different names — `SQLPlan`, `SQLGenerated`, `SQLRewritten`,
> `SQLRejected`, `SQLResult`, `SQLError`, `ReportFinalized`, plus
> `LLMCallRequest`/`LLMCallResponse`/`LLMCallError` for calls that run outside
> the main step loop (tagged by `kind`/`reportID`/`attempt`). This section now
> documents what's there and what the call #0/dispatch split still needs.

**Already shipped, reused as-is:** `SQLPlan`, `SQLGenerated`, `SQLRewritten`,
`SQLRejected`, `SQLResult`, `SQLError`, `ReportFinalized` cover the
call #2 → call #3 → per-report pipeline end to end — every proposed statement,
its rewritten/guarded form, every rejection, every row count, and how each
report was finalized, all logged verbatim to `chat_debug_logs/chat_debug.log`
under the session's `[sessionID][HH:MM:SS][source]` tag. This is why "why is
this number wrong" is answerable from that file alone for the existing
pipeline, and the call-split doesn't change any of it — call #3 already logs
through `LLMCallRequest`/`LLMCallResponse` with `kind="finalize"`.

**New, needed for the call #0/dispatch split** (see "Implementation: per-turn
system prompt selection" above — this is the logging half of that change,
without which a misrouted turn is invisible exactly the way the original
miscounting bug was):

- **Call #0 itself reuses the existing off-loop methods**, no new method
  needed: `clog.LLMCallRequest("classify", "", 1, payload)` /
  `clog.LLMCallResponse("classify", "", 1, elapsed, tokensIn, tokensOut, response)`
  (`reportID` empty — classify isn't per-report). `payload` here is call #0's
  own bounded input (last few turns + tool names), which is worth logging in
  full precisely because it's *not* the same thing as the main `AIRequest`
  history dump — a bug in the truncation/bounding logic would otherwise be
  invisible.
- **New `ChatTurnLogger.BranchSelected(shape string, promptLen, toolCount int)`**,
  logged right after the `SWAP` step, before entering the branch's step loop.
  This is the one line that answers "why did this turn go down the wrong
  path" without having to infer it from which system-prompt shape shows up
  later in the log — log the decision at the point it's made, not just its
  downstream effect.
- **New `ChatTurnLogger.SchemaSourceDegraded(reason string)`**, logged once,
  only on the `count_query` branch, only when `schema_render.go`'s
  `dbctxTableText(role)` had to fall back to the static minimal table list
  because the dbctx index wasn't ready or `Index.Err()` was non-nil (see
  `dbctx_schema_plan.md`'s Rollout/risk section). Without this, a turn that
  silently ran on the degraded fallback schema instead of the real one looks
  identical in the log to a turn that ran normally — exactly the kind of gap
  this whole logging system exists to close.
- **dbctx's own index build lifecycle is a process-startup event, not a
  per-turn one** — it happens once before any chat session exists, so it
  does not go through `ChatTurnLogger` (which requires a session id). Log it
  through the existing `zerolog` `log` package instead, same as other
  startup code: `log.Info()` on build start, `log.Info()` with table/column/FK
  counts and duration on success, `log.Error()` with the error on failure —
  this is what makes `SchemaSourceDegraded` above traceable back to a root
  cause instead of a mystery.

## Verification

**Tier 1 — guard unit tests, no DB.** The security suite. Each case asserts either a
specific `RejectionError` code or a rewrite containing the shadow CTEs:
`org_id = 1 OR 1=1` · `public.reviews` · `FROM ONLY reviews` · `pg_catalog.pg_class` ·
`FROM reviews r, users u` · `SELECT password_hash FROM users` · `pg_read_file(...)` ·
`dblink(...)` · `SELECT 1; DROP TABLE reviews` · a user-defined CTE named `reviews` ·
`WITH RECURSIVE` · `SELECT $1` · `INSERT`/`UPDATE`/`COPY ... TO PROGRAM` ·
`FOR UPDATE` · `loc_usage_ledger` as member (reject) and as owner (pass) ·
`LEFT JOIN ... ON reviews.org_id = 99`.

**Tier 2 — executor integration test against live PG.** Seed two orgs with distinct
known counts; run every Tier-1 query that passes validation as org A and assert zero
rows belong to org B. Make this a required CI job. Plus a coercion test: one query
returning `timestamptz`, `numeric`, `jsonb`, `NULL`, `bool`, `bigint` and `avg()`
over an empty group → `json.Marshal` succeeds, timestamps are RFC3339.

**Tier 3 — protocol parser tests** from real `chat_debug_logs/` fixtures, including
fenced / prose-wrapped / bare-array / trailing-commentary variants. Assert that a
chart-JSON response and a tool-call response both return `ok=false` from
`parseAnalyticsPlan` — the discriminator must not steal the other two paths.

**Tier 4 — end-to-end bug repro.** Ground truth first:
```sql
SELECT date_trunc('month', created_at), count(*) FROM reviews WHERE org_id = 3 GROUP BY 1 ORDER BY 1;
-- May 2026 = 55, June 2026 = 18
```
Then with `LIVI_DEBUG_LOG=1` as athreyac4@gmail.com (org_id=3):
1. "How many reviews did we do in May and June?" → `data.values` contains exactly 55
   and 18, and the description quotes them. (Today: 51 and 16.)
2. "Reviews by month and my top reviewers" → two report blocks from one turn.
3. A day with no reviews → clean prose, no chart, and **no second SQL execution**.
4. CSV export → `files[0]` downloads, row count matches the count query.
5. Cross-tenant: different `X-Org-Context` yields different numbers; a user outside
   org 3 cannot fetch org 3's CSV id.
6. Regression: the trigger-a-review action path still asks for the URL and calls the
   tool; one bot surface still renders charts.

**Tier 5 — build.** `CGO_ENABLED=0 go build ./...` for amd64 and arm64 (the whole
reason for the parser choice), and record the binary size delta from the wasm blob.

## Implementation order

1. `internal/livisql` catalog + guard + full attack corpus. **Land first** — it is
   the security boundary and needs nothing else to be testable.
2. `storage/analytics` executor + coercion + live-PG cross-tenant test.
3. `internal/logging` SQL methods.
4. `mcpagent` protocol types + parsers + fixture tests (no LLM needed).
5. `mcpagent/analytics.go` orchestration + `RunTurnWithArtifacts` + `WithAnalytics`
   + tool filtering, against a fake engine and fake provider.
6. Prompts: the three new files, extract `description_style.md`, surgery on
   `agent_instructions.md`.
7. Wire `HandleWebChat`. **Tier 4 steps 1–3 must pass here, before CSV exists.**
8. `chat_files.go` + `ServeChatCSV` + route + `WebChatResponse.Files`.
9. UI: `chatbot.ts` + `Chatbot.tsx`.
10. Wire the three bots (`OrgID`, `UserRole`, `WithAnalytics`) — charts flow through
    the unchanged string contract.

## Residual risks

- `wasilibs/go-pgquery` is untagged; pin the pseudo-version and check it against
  `osv-scanner.toml` policy. Spike it before committing to it.
- RLS is deliberately not used: it is bypassed by superusers and by the table owner
  unless `FORCE ROW LEVEL SECURITY` is set, and the local docker-compose role is
  likely the owner — it would give false confidence. If production's role is a
  non-owner non-superuser, adding FORCE RLS later is a cheap second layer.
- Role gating is new surface area. If `pc.Role` is not reliably populated on a
  surface, default it to the member catalog.
- Latency: four sequential reports is call #0 + call #2 + 4 × call #3 = 6 LLM
  calls minimum on the count_query branch (more with retries). Measure before
  adding concurrency.
