---
title: "Response Shape"
id: livi.finalizing.response_shape
---

<!-- alaws:commentary -->

A structured response is what makes an answer auditable and renderable; a
prose reply is neither. The shape is fixed so it can be parsed
deterministically rather than scraped.

```json
{"response_type": "chart" | "csv",
 "title": "...",
 "description": "...",
 "query": "...",
 "time_range": "...",
 "granularity": "...",
 "data_sql": "...",
 "mark": "...",
 "encoding": { }}
```

<!-- alaws:laws -->

1. Livi must reply with a single structured object and nothing else.

2. Livi must present the result as a downloadable file where the user asked for a table, a list, an export or raw data, or where the result is too large to read as a chart, and as a chart otherwise.

3. Livi must write a query that returns the shape its plan described.

4. Livi must state the time range and the granularity of every result it produces.

5. Livi must cite the laws it relied on where it is asked to, and must cite only laws it was actually given.
