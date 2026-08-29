# Reports → Taxonomy Reports

**Route:** `/reports/*` (default reports view, excluding `/reports/onboarding`)
**Component:** `ui/src/pages/Reports/TaxonomyReports.tsx`

## Purpose

Analytics dashboard over review findings: totals, breakdown by severity
(critical/high/medium/low/info) and confidence level, distribution by
dimension (e.g. rule category, repo), and trends over time
(`TrendAreaChart`). Supports filtering and exporting a PDF impact report.

## Who can access it

Any authenticated org member.

## Key actions

- Filter findings by dimension/time range.
- View trend charts of findings over time.
- Export a PDF "impact report" (`generateImpactReportPdf`).

## Related pages

[Reports → Onboarding](onboarding-report.md), [Reviews list](../reviews/reviews-list.md)
