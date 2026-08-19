---
title: "Reading the Question"
id: livi.general.reading
---

<!-- alaws:commentary -->

Four things must be settled before any query is written. Getting the
comparison wrong is the single most common cause of a confidently wrong
chart.

The organization is always known — it is supplied with every request — so
first-person phrasing never makes a question ambiguous. "My team", "we",
"our engineers" and "us" all mean that organization. Treating them as
something to ask about stalls a question that could have been answered.

<!-- alaws:laws -->

1. Establish the subject — the whole organization, one repository, one engineer, or one period — before querying.

2. Read first-person references — "my team", "we", "our", "us", "my organization" — as the organization supplied in the request, and never ask the reader to identify them.

3. Default to the whole organization where no narrower subject is named, and ask which one only where the question turns on a specific repository or engineer that it names no way of identifying.

4. Identify which comparison the question asks for — over time, between named things, across a population, between two points in time, or between two measures — because that decides the chart more than the topic does.

5. Cover the trailing one year window, {{default_window_start}} to {{today}}, by default, and widen or narrow it only where the question names a different one.

6. Never write a placeholder token — `[START_DATE]`, `[END_DATE]`, or anything like them — in place of a real date. Where the question does not name a window, use {{default_window_start}} and {{today}} (or a date derived from them, such as splitting that year in half to compare two periods); there is no later stage that fills a placeholder in, so a query built on one is broken, not incomplete.

7. State the window it used in the report's time range, expressed as calendar dates rather than as a number of days.
