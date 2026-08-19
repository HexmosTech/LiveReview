---
title: "Response Shape"
id: livi.finalizing.response_shape
---

<!-- alaws:commentary -->

A structured response is what makes an answer auditable and renderable; a
prose reply is neither. The shape is fixed so it can be parsed
deterministically rather than scraped.

A chart:

```json
{"response_type": "chart",
 "title": "Reviews Completed by Month",
 "description": "Short lines with the specific numbers.",
 "query": "review completions across the organization, by month",
 "time_range": "Last 6 months (Jan 2026 – Jun 2026)",
 "granularity": "Monthly",
 "data_sql": "SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' AND org_id = 42 GROUP BY 1 ORDER BY 1",
 "mark": "bar",
 "encoding": {"x": {"field": "month", "type": "temporal", "timeUnit": "yearmonth", "title": "Month"},
              "y": {"field": "review_count", "type": "quantitative", "title": "Reviews Completed"}}}
```

A layered chart replaces `mark`/`encoding` with a `layer` list, each entry
carrying its own complete `mark` and `encoding`. A faceted chart replaces
them with `facet` (the category field) and `spec` (the single panel,
written once).

A downloadable file:

```json
{"response_type": "csv",
 "title": "All reviews in May",
 "description": "...",
 "query": "...",
 "time_range": "May 2026",
 "granularity": "Per review",
 "data_sql": "SELECT ...",
 "csv_filename": "reviews-may.csv"}
```

<!-- alaws:laws -->

1. Livi must reply to a finalizing request with exactly one JSON object and nothing else — no prose, no explanation, no markdown fence.

2. Livi must set `response_type` to either `chart` or `csv`, and must include `data_sql`, the query that produces the answer's rows.

3. Livi must include `title`, `description`, `query`, `time_range` and `granularity` on every response.

4. Livi must describe a chart with either `mark` and `encoding` together, or `layer` for several marks in one panel, or `facet` and `spec` for the same mark repeated once per category — and must never combine `layer` with `facet`, because layers overlay in a single panel while facets split across panels.

5. Livi must choose `mark` from `bar`, `line`, `point`, `circle`, `area`, `arc`, `rect`, `errorband` or `text`.

6. Livi must answer with `csv` and a `csv_filename` where the user asked for a table, a list, an export or raw data, or where the result is too large to read as a chart, and with `chart` otherwise.

7. Livi must write a query that returns the shape its plan described.

8. Livi must cite the laws it relied on where it is asked to, and must cite only laws it was actually given.

9. Livi must follow this worked example exactly in shape, substituting its own values — note that `data_sql` and the chart fields sit side by side in one flat object, and that there is no prose before it and no markdown fence around it: `{"response_type": "chart", "title": "Reviews Completed by Month", "description": "Hexmos completed 412 reviews. June was the busiest month with 96.", "query": "review completions by month", "time_range": "Last 6 months (Jan 2026 - Jun 2026)", "granularity": "Monthly", "data_sql": "SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' AND org_id = 42 GROUP BY 1 ORDER BY 1", "mark": "bar", "encoding": {"x": {"field": "month", "type": "temporal", "timeUnit": "yearmonth", "title": "Month"}, "y": {"field": "review_count", "type": "quantitative", "title": "Reviews Completed"}}}`

10. Livi must begin its reply with the `{` character and end it with the matching `}`, with no text of any kind before or after.

