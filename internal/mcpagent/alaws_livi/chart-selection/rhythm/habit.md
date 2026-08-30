---
title: "Habit Across the Working Week"
id: livi.charts.rhythm.habit
---

<!-- alaws:commentary -->

**Applies when** the question asks whether an activity has become part of
the daily routine — whether it is a habit, whether people use it
consistently, whether there are gaps.

**Seen as:** "Are engineers actually incorporating reviews into their
daily workflow?"

<!-- alaws:laws -->

1. Apply this section where a question asks whether an activity has become part of the daily routine. {#apply-this-section-where-question}

2. Count per calendar day, or sum lines of code where the question concerns volume rather than frequency. {#count-per-calendar-day-or}

3. Zero-fill every day in the window. {#zero-fill-every-day-in}

4. Use a window long enough to reveal a rhythm, since a two-week window cannot show a habit. {#use-window-long-enough-to}

5. Set both axes to an ordinal scale rather than a temporal one, because a temporal scale places weeks on a continuous axis whose band width collapses adjacent columns into each other. {#set-both-axes-to-an}

6. Lay the grid out as weeks across and days of the week down, so that weekday gaps read as horizontal bands and quiet stretches as vertical ones. {#lay-the-grid-out-as}

7. Name the pattern in words in the description — which days are dead, whether the streak is unbroken, when it began or stopped — so the reader need not count cells. {#name-the-pattern-in-words}

8. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:
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
{#the-specification-below-is-an}
