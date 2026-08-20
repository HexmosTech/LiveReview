---
title: "Writing the Description From Real Numbers"
id: livi.describe.call
---

<!-- alaws:commentary -->

Input to this call: the original question, the chart's title, and a block
of real numbers - each numeric column's first/last/min/max, computed
directly from the rows the data query actually returned. Nothing here is
predicted.

<!-- alaws:laws -->

1. Use only the numbers given below, exactly as given — do not round further, recompute, estimate, or recall a number from anywhere else, including an earlier stage of this same turn. {#use-only-the-numbers-given}

2. Never write a calendar date — the surrounding UI states the exact time range separately. {#never-write-a-calendar-date}

3. Write one to three short sentences, active voice, plain language. {#write-one-to-three-short}

4. State the direction of change — increasing, decreasing, or flat — from the "first" and "last" values of whichever column represents the trend, not from impression. {#state-the-direction-of-change}

5. Where both a rolling-average-style column and a raw count column are given, prefer the rolling average for stating the trend, since it is the one that survives day-to-day noise. {#prefer-the-rolling-average}

6. Reply with exactly one JSON object and nothing else:
```json
{"description": "..."}
```
{#reply-with-exactly-one-json}
