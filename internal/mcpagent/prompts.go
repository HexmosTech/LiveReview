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
