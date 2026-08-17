---
id: chart.crosstab
number: 9
title: Two Categorical Dimensions Crossed (Heatmap)
---

# §9 — Two categorical dimensions crossed

## §9.0 General rule

**When the question crosses two categorical dimensions (not a category
over continuous time — that's §2/§8) and asks where a count/rate is
concentrated in the resulting grid, render a `rect` + `color` heatmap, one
cell per combination.** The working mechanism for this is already proven
by §2.2's repository × day heatmap (`repo_day_heatmap.py`) — day there is
treated as a plain categorical/temporal axis, not the Mon–Sun rhythm axis
§2.1 uses, so the same `rect`+`color` recipe generalizes directly to other
two-category crossings.

## §9.1 Specific rule — "Are serious issues being caught before PR/MR?" (severity × trigger) (query #16)

- Would be: rows = trigger source (pre-commit / PR-MR / MCP / API),
  columns = severity (critical/high/medium/low), color = finding
  count/rate.
- **Exception, not a gap in the rule:** `ai_comments.content` stores the
  comment payload as JSON, not a normalized issue-severity column — this
  chart is blocked on schema/extraction work, not on chart design. Do not
  attempt to fake this from a text search over `ai_comments.content`; wait
  until severity is reliably extractable.

## §9.2 Specific rule — "Where are the issues concentrated?" (repository × file) (query #18)

- Would be: rows = file/path, columns = repository, color = finding
  count, `text`/size for severity, top files per repo only.
- `ai_comments.file_path` does exist and makes file-level concentration
  directly possible in principle — this one is closer to buildable than
  §9.1, but hasn't been validated against real dev-DB volume yet (most
  `ai_comments` rows in the dev DB are currently sparse/empty — see
  earlier data-availability triage for this catalogue).

**Both rules in this section are documented but unbuilt on purpose** —
per the standing rule for this whole chart catalogue: never fabricate a
demo for data that doesn't exist yet in the dev DB. §9.0's mechanism is
proven (via §2.2); what's missing is the underlying data, not the chart
recipe.
