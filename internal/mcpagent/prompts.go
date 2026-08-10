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

// analyticsSchema documents the tables generated SQL may read. It is written by
// hand rather than introspected from information_schema on purpose: it has to
// mirror the guard's allowlist exactly (internal/livisql/catalog.go), and it
// carries meaning no catalog exposes - that author_username is the reviewer,
// what the status values imply, which timestamp answers which question.
//
//go:embed prompts/analytics_schema.md
var analyticsSchema string

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
