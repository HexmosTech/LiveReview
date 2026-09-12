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

## Blocking the merge/approve button, not just the CI job

A CI job returning a non-zero exit code only marks that one check as failed —
it does **not**, by itself, grey out the platform's merge button. Each Git
provider needs its own separate "require this check to pass before merging"
setting enabled, or a developer can merge right past a failing/still-running
gate:

- **GitHub**: add a branch protection rule on the target branch (Settings ->
  Branches -> Branch protection rules) with **Require status checks to pass
  before merging**, and add the gate job's name (the `jobs.<id>` key in the
  workflow YAML, e.g. `livereview-gate`, unless a `name:` overrides it) to the
  required-checks list. Can be set via `gh api` too:
  ```
  gh api -X PUT repos/:owner/:repo/branches/:branch/protection \
    --input - <<'EOF'
  {
    "required_status_checks": { "strict": false, "contexts": ["livereview-gate"] },
    "enforce_admins": false,
    "required_pull_request_reviews": null,
    "restrictions": null
  }
  EOF
  ```
  Verify with `gh pr view <n> --json mergeStateStatus` — it reads `BLOCKED`
  once a required check is failing/pending.
- **GitLab**: by default GitLab lets you merge regardless of pipeline status
  (it only shows the pipeline badge) — there is a real "Merge immediately"
  button that bypasses a failing/running pipeline entirely. The setting that
  actually gates it is under **Settings -> Merge requests -> Merge checks ->
  "Pipelines must succeed"** on the project. Via API:
  ```
  curl --request PUT --header "PRIVATE-TOKEN: <token>" \
    "https://gitlab.com/api/v4/projects/<id-or-url-encoded-path>" \
    --data "only_allow_merge_if_pipeline_succeeds=true"
  ```
- **Bitbucket/Azure**: equivalent concepts exist (Bitbucket "Merge checks" /
  branch restrictions; Azure "Branch policies" -> "Build validation") but
  aren't yet exercised/documented here from a real setup — treat as
  needing the same kind of explicit opt-in before assuming the gate blocks
  merges on those platforms.

## Related pages

- [Reviews (list)](reviews-list.md)
- [Review Detail](review-detail.md)
- [Dashboard](../dashboard.md)

## Learn more (public docs)

- [CI/CD gates and merge enforcement — write a jq rule against findings and call one URL from any pipeline](https://hexmos.com/livereview/docs/livereview/mcp/usecases/cicd-gates-merge-enforcement)
- [Prevent production issues — automated reviews in your CI/CD pipeline](https://hexmos.com/livereview/docs/livereview/mcp/usecases/prevent-production-issues)
