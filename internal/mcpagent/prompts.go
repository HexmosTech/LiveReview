package mcpagent

import _ "embed"

// orgPromptGuidance is the static "how to mention the org name" guidance
// appended to the system prompt whenever an org/user context is present.
// Kept in a standalone .md file (rather than a Go string literal) so the
// wording can be reviewed/edited without touching agent.go.
//
//go:embed prompts/org_prompt.md
var orgPromptGuidance string

// agentInstructions is the static tool-calling/domain/chart-format rules
// appended to every system prompt. Kept in a standalone .md file (rather
// than a wall of Go string literals) so the prompt wording can be
// reviewed/edited without touching agent.go.
//
//go:embed prompts/agent_instructions.md
var agentInstructions string

// The analytics prompts drive the SQL path. They are only appended to the
// system prompt when the analytics engine is wired up, so an agent without it
// behaves exactly as before.

// analyticsSchemaIntro is the hand-written half of the schema section: SQL
// dialect rules, worked examples, and the enum/join facts dbctx cannot supply
// (representative values are deliberately never rendered from the live index
// - see dbctx_schema_plan.md - so the fixed, safe-to-share ones like
// reviews.status's four values stay hand-written here). The table/column
// listing itself used to live here too; it is now generated per-turn by
// schema_render.go's dbctxTableText from the live database schema, appended
// after this file's content rather than embedded in it.
//
//go:embed prompts/analytics_schema_intro.md
var analyticsSchemaIntro string

// analyticsSchemaExamples is the second hand-written half: the enum/join
// facts dbctx cannot supply, dialect rules, and worked examples. Sits after
// the generated table text in the prompt (see buildCountQueryPromptHalves),
// which is why it's a separate embed rather than one file with
// analyticsSchemaIntro - the generated text has to be spliced between them.
//
//go:embed prompts/analytics_schema_examples.md
var analyticsSchemaExamples string

// analyticsPlanInstructions teaches the first call to answer a data question
// with a count plan instead of a tool call.
//
//go:embed prompts/analytics_plan.md
var analyticsPlanInstructions string

// analyticsFinalizeInstructions is the system prompt for the second, per-report
// call, made once the row count is known.
//
//go:embed prompts/analytics_finalize.md
var analyticsFinalizeInstructions string

// analyticsRepairInstructions is the system prompt for the single retry given
// to a rejected query.
//
//go:embed prompts/analytics_repair.md
var analyticsRepairInstructions string

// analyticsNoDataInstructions is the system prompt used when a report matched
// zero rows, so the answer is a sentence rather than an empty chart.
//
//go:embed prompts/analytics_nodata.md
var analyticsNoDataInstructions string

// chatOnlyInstructions is appended to the chat branch's prompt (call #0's
// "chat" shape) - see analytics.go's WithAnalytics. Without it the model has
// no tools, no schema, and no format contract for this turn, and nothing
// stops it from fabricating a chart-shaped JSON blob out of its own
// initiative (observed: a misclassified data question landed on this branch
// and the model invented an ad hoc {"chart": {...}, "learning": {...}}
// object that matched none of the response parsers, so it rendered as raw
// JSON text in the UI instead of a chart or a sentence).
//
//go:embed prompts/chat_only.md
var chatOnlyInstructions string

// classifyInstructions is call #0's system prompt: the three-way shape
// decision (action | count_query | chat) that routes a turn before any
// schema or full tool definitions are built. See classify.go and
// livi_analytics_plan.md's "Call #0" section.
//
//go:embed prompts/analytics_classify.md
var classifyInstructions string
