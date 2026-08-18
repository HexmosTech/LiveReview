# §2 — Calendar / activity-rhythm heatmap

> §0 applies in full. Only deviations from it are stated here.

## §2.0 General rule

**When the question is about a pattern across calendar days — habit,
rhythm, consistency, gaps, bursts — render a calendar grid, not a
time-series line.** Group by day, never by author: this is a question
about the calendar, and who did it is a different question (§4).

Zero-filling is not a detail here, it is the whole chart. The grid exists
to show the weekends, the abandoned fortnight, the day someone stopped. A
missing row draws nothing, so an empty day would appear as blank paper
instead of a dark cell and the signal disappears.

## §2.1 "Are engineers actually incorporating reviews into their daily workflow?" (query #2)

Data: `reviews` counted per day — or LOC from `loc_usage_ledger` if the
question is about volume rather than frequency. Window long enough to see
a rhythm; a two-week window cannot show a habit.

**Both axes must be ordinal, not temporal.** A temporal x-scale puts weeks
on a continuous scale whose band-width maths collapses adjacent columns
into each other.

Vega-Lite spec — x = week, y = day-of-week, GitHub-green threshold scale:
```json
{
  "width": {"step": "<cell_step>"}, "height": {"step": "<cell_step>"},
  "mark": {"type": "rect", "cornerRadius": 2},
  "encoding": {
    "x": {"field": "day", "type": "ordinal", "timeUnit": "yearweek",
          "scale": {"paddingInner": 0.15},
          "axis": {"format": "%b", "labelExpr": "date(datum.value) <= 7 ? timeFormat(datum.value, '%b') : ''", "labelAngle": 0}},
    "y": {"field": "day", "type": "ordinal", "timeUnit": "day",
          "sort": ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"],
          "scale": {"paddingInner": 0.15},
          "axis": {"values": ["Mon","Wed","Fri"]}},
    "color": {"field": "value", "type": "quantitative",
              "scale": {"type": "threshold", "domain": [1, 3, 6, 10], "range": "<github_greens>"},
              "legend": null}
  }
}
```

## §2.2 "What does engineering activity look like across repositories and days?" (query #9)

Same `rect` + `color` mechanism, but y is **repository** and x stays a
plain temporal day axis — this compares repos over a continuous window
rather than showing a Mon–Sun rhythm.

Data: `loc_usage_ledger` joined to `reviews`, grouped by repository and
day. Sort repositories by their total so the busiest sit together; let the
chart sort from the data rather than hardcoding a list that goes stale.
Zero-fill matters less here — with dozens of repos the grid is mostly
empty by nature, and filling every pair would balloon the result.

Vega-Lite spec:
```json
{
  "width": {"step": 14}, "height": {"step": 26},
  "mark": {"type": "rect"},
  "encoding": {
    "x": {"field": "day", "type": "temporal", "axis": {"format": "%b %d", "labelAngle": -40}},
    "y": {"field": "repository", "type": "nominal", "sort": "<repos-by-loc-desc>"},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```
