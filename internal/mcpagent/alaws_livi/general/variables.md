---
title: "Variables"
id: livi.general.variables
---

<!-- alaws:commentary -->

This section is for the people maintaining this lawbook, not for the
model — it states no laws, the same as the Introduction chapter, because
its content is a contract between the lawbook and the Go code that
renders it, not an instruction Livi can act on. By the time a law reaches
Livi, every `{{variable}}` in it has already been substituted with a real
value; the model never sees a placeholder and has no way to "supply" one.

This lawbook references the following `{{variables}}`. The application
(`internal/mcpagent/laws.go`, `buildLawbookPrompts`) must supply every one
of them before rendering a law that uses it, and the render must fail
rather than silently emit an unresolved placeholder:

* `org_id` — the organization whose data is being queried.
* `present_year` — the current calendar year, computed fresh on every
  render (`time.Now().Year()`). Exists so laws can default a time window
  to "this year" without hardcoding a year that goes stale every January
  and is wrong for any organization whose data doesn't start when ours
  did.
* `today` — today's date (`time.Now()`, formatted `YYYY-MM-DD`).
* `default_window_start` — today minus one year, same format. Together
  with `today` these give a real, computed rolling window to default an
  unspecified date range to, so a law never has to leave a placeholder
  token (`[START_DATE]`, `[END_DATE]`) for the model to invent a
  substitute for — there is always a real date on hand instead.

<!-- alaws:laws -->

