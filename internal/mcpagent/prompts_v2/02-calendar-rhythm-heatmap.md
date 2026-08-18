# §2 — Calendar / activity rhythm

> §0 applies in full. Only deviations from it are stated here.

## §2.0 Governing rule

**When a question is about a pattern across calendar days — habit, rhythm,
consistency, gaps, bursts — render a calendar grid, not a time-series
line.** Group by day, never by author: this asks about the calendar, and
who did it is a different question (§4).

Zero-filling is not a detail here, it is the whole chart. The grid exists
to show the weekends, the abandoned fortnight, the day someone stopped. A
missing row draws nothing, so an empty day appears as blank paper instead
of a dark cell and the signal disappears.

---

## §2.1 — Habit across the working week

**Applies when** the question asks whether an activity has become part of
the daily routine — whether it is a habit, whether people use it
consistently, whether there are gaps.

1. Count per calendar day from `reviews`, or sum LOC from
   `loc_usage_ledger` if the question is about volume rather than
   frequency.
2. Zero-fill every day in the window.
3. Use a window long enough to show a rhythm. A two-week window cannot
   show a habit.
4. **Both axes must be ordinal, not temporal.** A temporal x-scale puts
   weeks on a continuous scale whose band-width maths collapses adjacent
   columns into each other.
5. Lay it out as weeks across and day-of-week down, so weekday gaps read
   as horizontal bands and quiet stretches as vertical ones.
6. In the description, name the pattern in words — which days are dead,
   whether the streak is unbroken, when it started or stopped. The reader
   should not have to count cells.

**Seen as:** query #2 — "Are engineers actually incorporating reviews into
their daily workflow?"

Vega-Lite spec:
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

---

## §2.2 — Activity of many entities across days

**Applies when** the question asks how activity is distributed across both
a set of entities and time at once — which repositories were busy when,
where the bursts and the dead stretches are.

1. Group by the entity and the day together, one row per cell.
2. Keep the x-axis a plain temporal day axis, not §2.1's ordinal
   year-week banding. This compares entities over a continuous window
   rather than showing a Mon–Sun rhythm.
3. Sort entities by their total so the busiest sit together. Let the chart
   sort from the data rather than hardcoding a list, which goes stale the
   moment a new entity appears.
4. Zero-filling matters less than in §2.1: with dozens of entities the
   grid is mostly empty by nature, and filling every pair balloons the
   result for little gain.
5. In the description, call out the specific bursts and the entities that
   went quiet. A dense grid without that is a picture, not an answer.

**Seen as:** query #9 — "What does engineering activity look like across
repositories and days?"

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
