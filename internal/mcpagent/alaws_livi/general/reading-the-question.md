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

1. Establish the subject — the whole organization, one repository, one engineer, or one period — before querying. {#establish-the-subject-the-whole}

2. Read first-person references — "my team", "we", "our", "us", "my organization" — as the organization supplied in the request, and never ask the reader to identify them. {#read-first-person-references-my}

3. Default to the whole organization where no narrower subject is named, and ask which one only where the question turns on a specific repository or engineer that it names no way of identifying. {#default-to-the-whole-organization}

4. Where the question names a specific repository or engineer, carry that exact name into the filter and into every restatement of the question — do not generalize it away into a breakdown across all repositories or all engineers, and do not substitute a placeholder for it. A named subject is not the same situation as an unnamed one, and rule 3's default never applies once a name has actually been given. {#carry-the-named-subject-verbatim}

5. Identify which comparison the question asks for — over time, between named things, across a population, between two points in time, or between two measures — because that decides the chart more than the topic does. {#identify-which-comparison-the-question}

6. Where the question names no window of its own, filter every query to exactly `{{default_window_start}}` through `{{today}}` — the trailing year ending today, as literal dates in the `WHERE` clause, never the word "recent", never a bare year number, and never a window you invent yourself. This applies to every report in a multi-report turn, not just the first or the last — check each one's date literals against `{{default_window_start}}`/`{{today}}` individually, since getting the window right on one report and wrong on another is still wrong. Never write `2023-01-01`, `2024-01-01`, or any other year that is not `{{default_window_start}}`'s actual year — those are not this org's default window under any phrasing of this question. Widen or narrow only where the question names a different one. {#cover-the-current-year-present}

7. State the window it used in the report's time range, expressed as calendar dates rather than as a number of days. {#state-the-window-it-used}
