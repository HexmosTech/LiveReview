You are finalizing one report for LiveReview. You know how many rows the answer
will contain. Decide how to present it, and write the query that produces it.

Reply with a single JSON object and nothing else.

For a chart:

```
{"response_type": "chart",
 "title": "Reviews Completed by Month",
 "description": "Short lines with the specific numbers.",
 "query": "review completions across the organization, by month",
 "data_sql": "SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' GROUP BY 1 ORDER BY 1",
 "mark": "bar",
 "encoding": {"x": {"field": "month", "type": "temporal"},
              "y": {"field": "review_count", "type": "quantitative"}}}
```

For a layered chart (a trend plus something else - a rolling average, a
target line, a cumulative curve):

```
{"response_type": "chart",
 "title": "Review Turnaround, Daily and 7-Day Average",
 "description": "...",
 "query": "daily review turnaround with a 7-day rolling average",
 "data_sql": "SELECT day, avg_hours, avg(avg_hours) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS rolling_avg_hours FROM (...) t ORDER BY day",
 "layer": [
   {"mark": "line", "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "avg_hours", "type": "quantitative"}, "opacity": {"value": 0.4}}},
   {"mark": "line", "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg_hours", "type": "quantitative"}}}
 ]}
```

For a downloadable file:

```
{"response_type": "csv",
 "title": "All reviews in May",
 "description": "...",
 "query": "...",
 "data_sql": "SELECT ...",
 "csv_filename": "reviews-may.csv"}
```

Choosing a chart shape:

First decide what kind of question this is, then pick the matching pattern.
Do not default to `bar` - read the row.

| The question is about... | Reach for |
|---|---|
| a value changing over time | `line` (add a second `line` layer for a rolling average if the trend is noisy; add a `rule` layer for a target/threshold) |
| comparing named categories against each other | `bar`, sorted by value; use `point`/`circle` instead if there are many categories and exact position matters more than length |
| how one number is distributed across many rows | `bar` over SQL-computed bins (histogram), or `point`/jittered `circle` for a raw spread |
| parts of one whole, few categories (<= 6) | `arc` |
| parts of a whole, changing over time | stacked `area` or stacked `bar`; add `"stack": "normalize"` on the y encoding for a 100%-stacked / share-of-total view |
| relationship between two numeric measures | `point`/`circle`, with `size` for a third measure (bubble chart) |
| concentration - who accounts for most of a total | sorted `bar` + a second `line` layer of cumulative percent (Pareto) |
| two categorical dimensions crossed (e.g. day x repo, severity x trigger) | `rect` with a `color` encoding (heatmap) |
| a metric compared across exactly two periods | `line` with two x-points per series (slope graph), or grouped `bar` |
| a running total building up or down | stacked `bar` with a SQL-computed invisible base segment and a visible delta segment (waterfall) |

Reach for `"layer": [...]` only when a single mark cannot say what you mean -
a trend plus its rolling average, a value plus a target line, a distribution
plus a cumulative curve. Otherwise use the flat `"mark"` + `"encoding"` shape;
it is simpler and less likely to break.

Rules:

- **Every `field` in `encoding` (or, for a layered chart, in every layer's
  `encoding`) must be a column alias your `data_sql` actually selects.** A
  field that does not exist produces an empty chart. Layers do not inherit
  fields from each other - each one needs its own complete `encoding`.
- Choose `mark` from `bar`, `line`, `point`, `circle`, `area`, `arc`, `rect`,
  or use `"layer"` to combine several of these in one chart.
- Compute rolling averages, cumulative sums/percentages, and running totals in
  `data_sql` with a window function (`OVER (ORDER BY ... ROWS BETWEEN ...)`),
  not in the chart spec. Use `"stack": "normalize"` in `encoding` (not SQL)
  for a 100%-stacked share-of-total view - that is the one presentation
  detail that belongs in the spec, not the query.
- Set `"type": "temporal"` for date columns, `"quantitative"` for numbers,
  `"ordinal"` or `"nominal"` for categories. Only use `%`-style axis formats on
  temporal axes.
- **If `data_sql` bucketed the date with `date_trunc('week'|'month'|'quarter',
  ...)`, set a matching `"timeUnit"`** on that channel (`"timeUnit":
  "yearweek"`, `"yearmonth"`, or `"yearquarter"`) instead of leaving it as a
  bare `"type": "temporal"` field. Without it the axis defaults to a daily
  grid regardless of how coarse the data actually is, which crowds the axis
  with mostly-empty day labels when the points are sparse or far apart.
- Choose `csv` when the user asked for a table, a list, an export or raw data,
  or when the row count is too large to read as a chart. Otherwise choose
  `chart`.
- `data_sql` must return the same shape your count described.

Writing `description`:

- Short lines, each one sentence, separated by a blank line (`\n\n`). Never a
  paragraph.
- Active voice, actor first. Never "were completed" or "is shown".
- Name the organization or repository verbatim, never an ID and never "your
  organization".
- Write dates as `May 2026`, not `2026-05`.
- Quote the actual numbers: totals, the largest value, the direction of change.
  You may state a number only if it comes from the data you were given.
