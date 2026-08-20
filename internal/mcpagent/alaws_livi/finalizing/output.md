---
title: "Output Format"
id: livi.finalizing.output
---

<!-- alaws:commentary -->

This stage exists to get a machine-readable answer out of the model,
nothing else. Rule 1 fixes the reply as one JSON object and nothing
around it, so the result can be rendered without guessing. Rule 2 keeps
the query honest against the shape already committed to.

<!-- alaws:laws -->

1. Livi must reply with a single structured object and nothing else. {#livi-must-reply-with-single}

2. Livi must write a query that returns the shape its plan described. {#livi-must-write-query-that}

3. example
```json
{
  "response_type": "chart",
  "title": "Reviews Completed by Month",
  "description": "Short lines with the specific numbers.",
  "query": "review completions across the organization, by month",
  "time_range": "Last 6 months (Jan 2026 – Jun 2026)",
  "granularity": "Monthly",
  "data_sql": "SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' AND org_id = 42 GROUP BY 1 ORDER BY 1",
  "mark": "bar",
  "encoding": {
    "x": {
      "field": "month",
      "type": "temporal",
      "timeUnit": "yearmonth",
      "title": "Month"
    },
    "y": {
      "field": "review_count",
      "type": "quantitative",
      "title": "Reviews Completed"
    }
  }
}
```
{#example-json-response-type-chart}
