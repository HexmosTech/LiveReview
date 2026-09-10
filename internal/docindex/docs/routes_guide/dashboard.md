# Dashboard

**Route(s):** `/` (`HomeWithOAuthCheck`, redirects to Dashboard after handling
any pending OAuth callback params), `/dashboard` (`Dashboard`)
**Component:** `ui/src/pages/Home/HomeWithOAuthCheck.tsx`,
`ui/src/components/Dashboard/Dashboard.tsx`
**Widgets:** `ui/src/components/Dashboard/widgets/` (registry in `registry.ts`)

## Purpose

The landing page after login, and the org's main visual summary of LiveReview
usage. Three things stack here:

1. **Onboarding + quota banners** — the setup checklist and any billing/quota
   warnings.
2. **A customizable widget grid** — 13 drag-and-drop charts and stat panels
   (this is where all the graphs live).
3. **Recent Activity** — a fixed, API-backed activity feed at the bottom.

## Who can access it

Any authenticated user of the org. All widget data is scoped to the caller's
organization.

## The period selector

One control at the top of the grid sets the window for **every** widget:
**Today**, **This Week**, **This Month** (default), or **All Time**. Each
widget receives values already scoped to that period by the backend, so
changing it rewrites the whole grid at once.

## Customizing the grid

- **Add Widget** — pick from any widget not currently placed.
- **Edit mode** — drag to rearrange and resize widgets.
- **Remove** — drop a widget from your layout.
- **Reset Layout** — restore the defaults.

Layout is per user, saved in `localStorage` under an `lr_<feature>_<user id>`
key. It's a personal view: your arrangement does not change anyone else's, and
it doesn't follow you to another browser.

## Reading the graphs

Widgets are grouped into three categories, shown as coloured badges: **Review
Layers** (blue), **System Overview** (purple), **People** (green).

Two vocabularies recur across the charts:

- **Review layer / stage** — where a review was triggered from:
  **Pre-commit** (the `lrc` CLI, before a commit lands), **MR / PR** (on a
  merge/pull request), **API / MCP** (programmatic), and **Scheduled**.
  These come from each review's `trigger_type`.
- **Issue category** — what kind of problem a finding is: Security,
  Reliability, Correctness, Performance, Cost, Scalability, Maintainability,
  Architecture, Developer Experience, Compliance & Governance. Colours are
  consistent for a given category across every chart.

Nearly every widget is clickable and drills through to the matching filtered
view, so a chart is also a navigation shortcut.

### Review Layers

| Widget | Chart type | How to read it | Clicking through |
|---|---|---|---|
| **Review Pipeline** | Sankey | Left column = review layers, right column = issue categories; each ribbon's thickness is how many findings of that category came from that layer. Wide ribbons show which trigger source is finding which kind of problem. A layer with no reviews gets no node at all. | A category → findings filtered to it |
| **Issue Distribution** | Treemap | Nested rectangles, sized by finding count: outer tiles are categories, inner tiles subcategories. The biggest tile is your most common problem type. | Drills into that category/subcategory in Reports |
| **Issue Mix by Stage** | Radar | One spoke per issue category, one coloured ring per review stage. A ring stretched toward a spoke means that stage disproportionately catches that kind of issue — useful for seeing that, say, pre-commit catches Correctness while scheduled runs surface Security. | — |
| **Reviews by Stage** | Stat cards | Plain volume and issue counts per stage; the numeric companion to the charts above. | Findings explorer |
| **Stage Volume Comparison** | Bar | Side-by-side review volume per stage — the quickest read of which trigger source your team actually uses. | Findings explorer |

### System Overview

| Widget | Chart type | How to read it | Clicking through |
|---|---|---|---|
| **System at a Glance** | KPI tiles | Four counts: Git Hosts, AI Connectors, Repositories, PRs/MRs tracked. | Each tile → its own page (`/git`, `/ai`, repositories, merge requests) |
| **PR Count by Repo / Host** | Sunburst | Concentric rings: inner ring = git host, outer ring = repositories, arc size = PR volume. Shows at a glance which host and which repos dominate activity. | Host → `/git`; repo → repositories |
| **Connected Providers** | List | Every git host and AI provider currently connected. | The relevant provider page |
| **Review Coverage** | Gauge | Percentage of PRs/MRs in the period that received **at least one** AI review. This is the "are we actually reviewing everything?" number — a low gauge with high review counts means reviews are concentrated on a few PRs. | Reports overview |
| **Review Averages** | Stat pair | Average reviews per PR/MR (1 decimal) and per commit (2 decimals). Depth rather than breadth: how many times a typical change gets looked at. | Findings explorer |

### People

| Widget | Chart type | How to read it | Clicking through |
|---|---|---|---|
| **Top Reviewers** | Leaderboard | Contributors ranked by reviews given in the period. | User management |
| **Usage Share** | Donut | Share of total review volume per contributor — top 5 plus an "others" slice. A single dominant slice means adoption is concentrated in one person rather than spread across the team. | User management |
| **Contribution Activity** | Calendar heatmap | GitHub-style grid over the last ~7 months; each cell is a day, darker = more reviews. Good for spotting streaks, gaps, and whether usage is sustained or bursty. | A day → findings for exactly that date |

Each contributor keeps the same colour across Top Reviewers and Usage Share
(it's hashed from their email), so the same person is recognisable in both.

### When a widget is empty

Widgets render an empty state rather than a broken chart when there's no data
yet — e.g. Review Coverage shows "Review coverage will appear here once
PRs/MRs are tracked." An empty grid usually means the org hasn't run reviews
yet, not that something is wrong.

## Onboarding and banners

- **`OnboardingSteps` / `FloatingOnboardingNudge`** — the setup checklist:
  install the CLI, connect an AI provider, run a first review. The floating
  nudge stays available until dismissed. **This is where per-person setup
  progress lives** — not in the Onboarding *Report*, which is an org-wide
  analytics report despite the similar name.
- **`QuotaWarningBanner` / `QuotaExhaustedBanner`** — appear as usage limits
  are approached or exceeded, with links to upgrade.
- **`PlanBadge`** — current plan/tier.

## Key actions

- Switch the dashboard period (Today / This Week / This Month / All Time).
- Add, remove, drag, resize, or reset widgets.
- Click any chart to drill into the matching filtered view.
- Work through the onboarding checklist.
- Read the Recent Activity feed.

## Related pages

- [Reports → Taxonomy Reports](reports/taxonomy-reports.md) — where most
  widget click-throughs land, with far deeper filtering.
- [Reports → Onboarding Report](reports/onboarding-report.md) — the org-wide
  adoption/cost/quality analytics report.
- [Create Review via CLI](reviews/create-review-cli.md), [Create Review via
  MCP](reviews/create-review-mcp.md) — onboarding entry points sharing this
  page's onboarding data.
- [Reviews](reviews/reviews-list.md)

## Learn more (public docs)

- [Generate engineering reports — ask for the report you need in plain English](https://hexmos.com/livereview/docs/livereview/mcp/usecases/generate-engineering-reports)
