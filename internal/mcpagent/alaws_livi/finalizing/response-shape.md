---
title: "Response Shape"
id: livi.finalizing.response_shape
---

<!-- alaws:commentary -->

From the plan state we have the query count so our job here is just to
figure out how to visualise the data either as a chart or as a downloadable
file like csv.

The allowed marks and the flat/layered/faceted shapes a spec can take are
`livi.charts.basics` (Chart Grammar), not restated here — this section is
the response envelope those pieces sit inside, not the vocabulary itself.

<!-- alaws:laws -->

1. Reply to a finalizing request with exactly one JSON object and nothing else — no prose, no explanation, no markdown fence. {#reply-to-finalizing-request-with}

2. Set `response_type` to either `chart` or `csv`, and include `data_sql`, the query that produces the answer's rows. {#set-response-type-to-either}

3. Include `title`, `description`, `query`, `time_range` and `granularity` on every response. {#include-title-description-query-time}

4. Answer with `csv` and a `csv_filename` where the user asked for a table, a list, an export or raw data, or where the result is too large to read as a chart, and with `chart` otherwise. {#answer-with-csv-and-csv}

5. Write a query that returns the shape the plan described. {#write-query-that-returns-the}

6. Cite the laws you relied on where asked, and cite only laws you were actually given. {#cite-the-laws-you-relied}

7. Follow this worked example exactly in shape, substituting your own values — note that `data_sql` and the chart fields sit side by side in one flat object, and that there is no prose before it and no markdown fence around it:
```json
{
  "response_type": "chart",
  "title": "Reviews Completed by Month",
  "description": "<state the real total and the real busiest period, read from the rows you fetched - never copy these placeholder numbers>",
  "query": "review completions by month",
  "time_range": "<the real calendar window your data_sql actually covers, e.g. 2026-02-08 to 2026-08-13 - never copy this placeholder>",
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
{#follow-this-worked-example-exactly}

8. Begin your reply with the `{` character and end it with the matching `}`, with no text of any kind before or after. {#begin-your-reply-with-the}

9. Compute `time_range` from the actual `min`/`max` of the rows `data_sql` returns, never from the calendar window named in the question or from the worked example above — a question phrased "last 6 months" can be answered by data spanning a different window than that phrase names, and the reply must report the window it actually got. {#compute-time-range-from-actual}

9. Present the result as a chart where the question is chart-shaped — a trend, a comparison, a ranking, or anything else answered by a visual read of the data. A predicted row count above the chart limit is the only reason to export such a question to a file instead, and where that count looks wrong for the grain of the answer, treat the question's shape as the stronger signal. {#present-the-result-as-chart}
