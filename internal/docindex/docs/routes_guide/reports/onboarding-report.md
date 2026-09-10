# Reports → Onboarding Report

**Route:** `/reports/onboarding`
**Component:** `ui/src/pages/Reports/OnboardingReport.tsx`
**Chart catalog:** `internal/onboardingreport/templates.json`

## Purpose

A broad, pre-built analytics report on **how the org is actually using
LiveReview** — adoption, per-repository and per-engineer activity, finding
quality, cost, and engagement. Despite the name it is not a per-person
setup checklist; it is the "is LiveReview landing in this org, and what is it
costing/finding?" report, built from 57 predefined charts.

Use it for questions like "is adoption growing or flat?", "which engineers and
repos actually use it?", "what is this costing per line of code?", and "do
people find the findings useful?"

## Sections

Charts are grouped into seven sections, each fetched on demand:

| Section id | Label | Covers |
|---|---|---|
| `adoption` | Adoption & Growth | Daily review counts, cumulative LOC reviewed, adoption levels/breadth, top adopters, active engineers over time |
| `repos` | Repository Analysis | Per-repo velocity change, LOC reviewed by repo, Pareto of top repos |
| `engineers` | Engineer Analysis | Top engineers by reviews, LOC Pareto, trigger types per engineer |
| `quality` | Review Quality & Findings | Trigger-type mix and trends, severity distribution, top problem categories/subcategories, top files with issues |
| `cost` | Cost & Efficiency | Daily cost, cost per line of code, cost by AI provider, median review duration |
| `engagement` | Engagement & Trust | Completed reviews, comments per review, upvote/downvote trends, feedback by category |
| `summary` | Summary & Comparison | Cross-cutting activity trend, severity distribution, per-engineer change comparisons |

Each chart carries a title, description, `query_summary`, chart type,
granularity, time range, row count, and optional stats. Charts are rendered
from **Vega-Lite** specs (`vega_spec`) produced server-side, with a shared
theme injected at render time so they look identical everywhere.

## Who can access it

- **The route and its API:** any authenticated org member — the
  `/api/v1/reports/onboarding` group applies only `RequireAuth` plus org
  context, and `internal/api/server.go` labels it "org-scoped: any member".
  Data is scoped to the caller's org (`{{.OrgID}}` is substituted into each
  chart's SQL by `PrepareSQL`).
- **In the navigation:** the Reports section is owner/super_admin-gated, so
  in practice members won't discover it in the menu.

## How to get here

There is **no mega-menu entry** for this report. Reach it by opening
**Reports** (`/reports`) and following the Onboarding Report link there
(`ui/src/pages/Reports/TaxonomyReports.tsx`), or by going to
`/reports/onboarding` directly.

## Exports

Export runs as an **async job**, not an instant download, because rendering 57
charts takes a while:

1. `POST /api/v1/reports/onboarding/export?format=pdf|html` starts the job and
   returns a `job_id` plus a total.
2. The page polls `GET /export/:jobId/status` and shows a progress bar with
   the current chart being rendered.
3. On completion it fetches `GET /export/:jobId/file` and saves it.

Formats are **PDF** and **HTML** (`internal/onboardingreport/generate_pdf.go`,
`generate_html.go`). A failed export can be retried from the modal.

## Key actions

- Browse charts by section.
- Read a chart's description and query summary to understand what it measures.
- Export the whole report as PDF or HTML, watching job progress.

## Related pages

[Reports → Taxonomy Reports](taxonomy-reports.md), [Dashboard](../dashboard.md)
