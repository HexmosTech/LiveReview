# Reports → Taxonomy Reports

**Route:** `/reports/*` (the default Reports view; `/reports/onboarding` is a
separate page)
**Component:** `ui/src/pages/Reports/TaxonomyReports.tsx`

## Purpose

The analytics dashboard over **review findings** — every issue LiveReview's
AI has raised, sliced by severity, confidence, category, repository, and time.
Answers questions like "how many critical issues did we have last month?",
"which repo produces the most security findings?", and "are findings trending
down?"

## Who can access it

- **In the navigation:** org **owner** or **super_admin** only — the Reports
  mega-menu section carries `requiresOwnerOrAdmin: true`
  (`ui/src/components/Navbar/megaMenuData.ts`).
- **The route and its API:** any authenticated org member. `/api/v1/reports/taxonomy/*`
  requires only `RequireAuth` + org context (`internal/api/server.go`), so a
  member with the URL can load the page; they just won't find it in the menu.
- **super_admin** additionally reads from `/api/v1/admin/reports/taxonomy/*`
  (`baseEndpoint` switches on `isSuperAdmin`), which spans organizations
  rather than just the current one.

## The three modes

The page has one URL with a `mode` query parameter; anything unrecognized
falls back to `overview`.

| Mode | URL | What it shows |
|---|---|---|
| Overview | `/reports` | Headline totals and distribution/trend charts |
| Explore | `/reports?mode=explore` | The filterable findings table (drill into individual findings) |
| Custom | `/reports?mode=custom` | Build a custom export by picking datasets |

## Filters (all are URL parameters, so any view is shareable)

`since`, `until`, `severity`, `confidence`, `type`, `category`,
`subcategory`, `repository`, `provider`, `org_id`, and `grain` (trend bucket
size, default `day`). `severity`, `category` and `subcategory` accept multiple
values.

Because filters live in the URL, the mega menu links to pre-filtered views —
e.g. Last 7 Days, Last 30 Days, Critical Issues
(`?mode=explore&severity=Critical`), Security Findings
(`?mode=explore&category=Security`).

## What the overview reports

From `/summary`: `total_findings`, `total_reviews`, counts per severity
(`critical`/`high`/`medium`/`low`/`info`) and per confidence
(`high`/`medium`/`low`). Plus distribution by severity, category and
subcategory, a bucketed trend over time (`TrendAreaChart.tsx`), a
per-repository/provider breakdown, and category↔subcategory relations.

## Exports

- **PDF impact report** — `generateImpactReportPdf` in
  `ui/src/pages/Reports/pdfExport.ts`. Deep-linkable: `/reports?export=pdf`
  opens the export dialog straight on the PDF tab.
- **Raw data as CSV or XLSX** — `/api/v1/reports/taxonomy/export` and
  `/export/xlsx`. Choose among the datasets `findings`,
  `severity_distribution`, `category_distribution`, `trend`, `breakdown`. An
  `/export/preview` call reports the row count per dataset before you commit.

## Key actions

- Switch between Overview / Explore / Custom modes.
- Filter findings by time range, severity, confidence, category, repository,
  or provider — and share the resulting URL.
- Sort the findings table (by created_at, severity, confidence, type,
  category, subcategory, repository, provider, file path, or line number).
- Export a PDF impact report, or raw CSV/XLSX per dataset.

## Related pages

[Reports → Onboarding Report](onboarding-report.md),
[Reviews list](../reviews/reviews-list.md),
[Review detail](../reviews/review-detail.md)

## Learn more (public docs)

- [Generate engineering reports](https://hexmos.com/livereview/docs/livereview/mcp/usecases/generate-engineering-reports)
- [Understand engineering decisions — query past reviews and spot recurring quality issues](https://hexmos.com/livereview/docs/livereview/mcp/usecases/understand-engineering-decisions)
