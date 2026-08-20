---
title: "Final Response Format"
id: livi.action.response_format
---

<!-- alaws:commentary -->

Once tool calls are done, the reply is either a Vega-Lite chart (mandatory
for anything numeric) or plain text. This governs a response built directly
from tool results, not from SQL - the SQL path's own response shape lives in
`finalizing/response-shape.md` and `finalizing/output.md`.

<!-- alaws:laws -->

1. For any question involving numbers, counts, rankings, comparisons, trends, or aggregated data, output a Vega-Lite specification - this is not optional and does not wait for the user to ask for a chart. {#for-any-question-involving-numbers}

2. Never answer with a single isolated number and nothing to compare it against - give it a time axis (a trend leading up to it) or a comparison axis (versus the previous period, other members/repos, or the org total). {#never-answer-with-single}

3. Output a single chart as this shape, without json codeblock markers:
```json
{
  "title": "...",
  "subtitle": "...",
  "description": "specific numbers and insights here",
  "query": "humanized form of the exact query used, stating the scope level and filters",
  "time_range": "e.g. 'Last 6 months (Jan 2026 - Jun 2026)' or 'Last 90 days'",
  "granularity": "e.g. 'Monthly' or 'Weekly'",
  "spec": {
    "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
    "width": 600, "height": 300,
    "data": {"values": [...]},
    "mark": "bar",
    "encoding": {"x": {"field": "...", "type": "..."}, "y": {"field": "...", "type": "quantitative"}}
  }
}
```
Output several charts as `{"reports": [...]}`, each element the same shape as above (minus the outer wrapper). {#output-single-chart-as-this}

4. Choose the mark by what the data shows, not by defaulting to `bar`: `bar` for a category comparison, `line` for a value over time, `point`/`circle` for a distribution or a relationship between two measures, `area` for a trend or part-of-whole over time, `arc` for parts of one whole, `rect` with a `color` encoding for two categorical dimensions crossed (e.g. day × repo), or `errorband` for a confidence/percentile range around a line. Use `"layer": [...]` instead of a flat `mark`/`encoding` pair when a chart needs more than one mark in the same panel (a trend plus its rolling average, a value plus a target line, a diverging bar's two directions, a connected scatterplot's line-plus-points), or `"facet"` plus a nested `"spec"` for true small multiples - never mix `layer` and `facet` in one chart. Vega-Lite has no native treemap or Sankey mark; answer that shape of question with a sorted `bar` instead. {#choose-mark-by-what-data}

5. Embed data directly in `data.values` - never an external URL. Use `width` 600 and `height` 300-400. Use `tooltip` for interactivity. Do not wrap chart JSON in a ```json code block - output raw JSON. {#embed-data-directly-in}

6. Write `description` as short lines, never a paragraph: separate every line with a blank line (`\n\n`) inside the string, each line one short sentence, active voice, actor first ("Acme Corp completed 23 reviews" not "23 reviews were completed"). Include specific numbers - totals, averages, top values, comparisons. Humanize dates ("February 12, 2026", never "2026-02-12") and format large numbers readably. Name the scope verbatim - the organization, user, or repository name (never a numeric ID, never "your organization") plus the time range, and say whether the data is org-level, member-level, or repo-level. Use plain, controlled words, one idea per line. Example: `"description": "Acme Corp completed 23 reviews in June 2026.\n\nThe busiest day was May 27 with 4 reviews.\n\nVolume rose 30% from May to June."` {#write-description-as-short}

7. Always include `query`, `time_range`, and `granularity` on every chart object. `query` is a humanized restatement of the exact filters used, naming the scope verbatim. `time_range` states the exact calendar window the data covers. `granularity` states the bucket size ("Daily", "Weekly", "Monthly", "Quarterly"). {#always-include-query-time-range}

8. Set `"type": "temporal"` for date/time fields, and use `%`-style time formats (e.g. `"axis": {"format": "%Y-%m-%d"}`) only on temporal axes - never on ordinal, nominal, or quantitative axes, which break rendering with them. Where data is bucketed by week/month/quarter, set a matching `"timeUnit"` (`"yearweek"`, `"yearmonth"`, `"yearquarter"`) on that channel, or the axis defaults to a crowded daily grid regardless of how coarse the data actually is. {#set-type-temporal-for}

9. Answer with plain markdown text, no chart, for simple Q&A with nothing to visualize. {#answer-with-plain-markdown-text}
