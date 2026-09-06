# CI/CD Gates

**Route(s):** `/ci-rulesets` (list), `/ci-rulesets/new` (create), `/ci-rulesets/:id/edit` (edit), `/ci-rulesets/:id/integration` (integration snippet)
**Component:** `ui/src/pages/CiRulesets/` (`CiRulesetsList.tsx`, `CiRulesetEditor.tsx`, `CiRulesetIntegration.tsx`)

## Purpose

Lets an org define named "rulesets" — a jq expression evaluated against the
canonical findings document of a review — that decide whether a CI/CD
pipeline should block or allow a merge. Unlike a fixed severity threshold,
the jq expression can combine severity, category, subcategory, and
confidence however the team needs (e.g. block on any security finding, or
only on high-confidence critical findings). Works identically regardless of
how the underlying review was triggered — web UI, `git-lrc`, a webhook, or
the API/MCP — because all reviews produce the same canonical findings
document.

Once saved, a ruleset exposes `GET /ci-rulesets/:id/evaluate`: a pipeline
calls that one URL and gates the job on its HTTP status code alone — no AI
model or JSON parsing needed on the CI side.

## Who can access it

Organization owners and super admins (gated in the mega menu by
`ctx.isSuperAdmin || ctx.orgRole === 'owner'`; enforced server-side on the
`/ci-rulesets` API routes as well).

## Key actions

- **List** (`/ci-rulesets`): see every saved ruleset, its description, jq
  expression, and last-updated time; open "Get code" for the integration
  snippet, "Edit", or "Delete".
- **Create/Edit** (`/ci-rulesets/new`, `/ci-rulesets/:id/edit`): name and
  describe the ruleset, write the jq expression in a live editor with:
  - Preset one-click expressions (Any Critical, Any Security, Critical OR
    Security, Critical+Warning > 2, High-confidence critical, Never block).
  - A taxonomy reference-chip picker that inserts jq comparisons for
    severity/category/subcategory/confidence.
  - A "jq quick help" cheat-sheet panel.
  - An "Ask LLM" helper that builds a copy-paste prompt (with the org's
    taxonomy and a sample document) for an external model (ChatGPT, Gemini,
    DeepSeek) to draft a complex expression — not an in-app LLM call.
  - Live results: the expression is evaluated against a synthetic sample
    document and/or real past reviews as you type, showing a `BLOCK`/`ALLOW`
    verdict before you save.
- **Integration** (`/ci-rulesets/:id/integration`): copy-paste curl snippet
  and a full GitHub Actions workflow (plus GitLab/Bitbucket/Azure/generic
  tabs) that call `GET /ci-rulesets/:id/evaluate` with the
  `LIVEREVIEW_API_KEY` CI secret; requires `jq` on the runner only if the
  pipeline wants to inspect the response body (the exit code alone is
  sufficient to gate the job).

## Related pages

- [Reviews (list)](reviews-list.md)
- [Review Detail](review-detail.md)
- [Dashboard](../dashboard.md)
