# Chatbot (Livi)

**Route(s):** `/chat`, `/chat/:conversationId`; debug variant `/chat-debug/*`
(dev/`LIVI_DEBUG_LOG`-gated, adds a debug-artifacts inspector)
**Component:** `ui/src/pages/Chatbot/Chatbot.tsx` and
`ChatDebugPage.tsx`, both thin wrappers over the shared
`ui/src/pages/Chatbot/ChatConversation.tsx`

## Purpose

In-app AI chat assistant ("Livi") for asking natural-language questions
about the org's data (reviews, repos, PRs, spend, onboarding) and about the
product itself (how-to / navigation questions). Answers can render as text,
tables, or charts (`InteractiveChart`). Reachable from anywhere via the
floating chat button or Ctrl+I.

## Who can access it

Any authenticated org member.

## Key actions

- Ask data questions, e.g. "how many reviews have triggered this week?",
  "which repo has the most issues?", "how much is spent on average per
  review?"
- Ask product/how-to questions, e.g. "how to onboard a team member?",
  "how to add a custom rule for a repository?" (answered from the docs
  in `ui/docs/training_data/lr_routes/` — see root `AGENTS.md`).
- Continue a previous conversation via `/chat/:conversationId`; starting a
  new one remounts the component so state starts clean.
- (`/chat-debug` only) inspect debug artifacts behind the model's answer.

## Related pages

All other pages are effectively "related" — the chatbot answers questions
about any of them by drawing on this `ui/docs/training_data/lr_routes/`
documentation set.
