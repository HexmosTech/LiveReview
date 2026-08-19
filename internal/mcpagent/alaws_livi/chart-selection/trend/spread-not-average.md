---
title: "Exception: When the Spread Matters"
id: livi.charts.trend.spread
---

<!-- alaws:commentary -->

**Applies when** the question is about how consistent or reliable a
measure is, not where its centre sits — durations, latencies, anything
with a tail. "Are we getting faster" is really "is the slow case getting
worse".

**This section overrides the chapter's smoothing rule.** A rolling average
would conceal precisely the signal being asked about: the median can hold
steady while the worst case doubles.

**Seen as:** "Are reviews getting faster?"

<!-- alaws:laws -->

1. Apply this section, in place of the chapter's smoothing rule, where a question concerns the consistency or reliability of a measure rather than its centre. {#apply-this-section-in-place}

2. Restrict the data to records that actually completed, requiring both a completed status and a real completion timestamp, since an unfinished record has no duration. {#restrict-the-data-to-records}

3. Derive the duration per record and convert it to a unit a person reads easily. {#derive-the-duration-per-record}

4. Return three percentiles per bucket — a median, a low and a high — using an ordered-set aggregate. {#return-three-percentiles-per-bucket}

5. Bucket weekly rather than daily, because percentiles need enough records within a bucket to be stable. {#bucket-weekly-rather-than-daily}

6. Quote both the median and the high percentile in the description, since reporting the median alone recreates the very problem this section exists to prevent. {#quote-both-the-median-and}

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:
```json
{
  "width": 700, "height": 340,
  "layer": [
    {"mark": {"type": "errorband", "color": "#7c9cff", "opacity": 0.25},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p10", "type": "quantitative"}, "y2": {"field": "p90"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "point": true},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p50", "type": "quantitative"}}}
  ]
}
```
{#the-specification-below-is-an}
