package mcpagent

import _ "embed"

// orgPromptGuidance is the static "how to mention the org name" guidance
// appended to the system prompt whenever an org/user context is present.
//
//go:embed prompts/org_prompt.md
var orgPromptGuidance string

// agentInstructions is the static tool-calling/domain/chart-format rules
// appended to the action branch's system prompt.
//
//go:embed prompts/agent_instructions.md
var agentInstructions string

// chatOnlyInstructions is appended to the chat branch's prompt — the model
// has no tools and no database access, so it must not fabricate structured
// answers.
//
//go:embed prompts/chat_only.md
var chatOnlyInstructions string
