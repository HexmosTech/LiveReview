---
title: "Prompt Assembly"
id: livi.intro.assembly
---

<!-- alaws:commentary -->

This is the single source of truth for which chapters go into which model
call, and why. Nothing else in this book or in the code should restate
this table — `internal/mcpagent/laws.go`'s `buildLawbookPrompts` points
here in its doc comment instead of repeating it, so that this section is
the only place that can go stale.

A single question passes through up to four model calls. Each call is a
separate conversation with its own instructions, assembled from a
different combination of chapters:

**[Classification](#/books/internal%2Fmcpagent%2Falaws_livi/livi.classify)** — is this a data question, an action, or conversation?
*Sent: [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general) minus Data Handling + [Classification](#/books/internal%2Fmcpagent%2Falaws_livi/livi.classify).*
Cheap on purpose: it never sees the chart laws or the database schema,
because paying for those before knowing whether the turn even needs them
is waste. Data Handling is excluded here specifically — its 23 SQL-dialect
rules (real table/column names, join gotchas, the allowed function list)
are dead weight for a call that never writes a query, and were observed
outweighing the much smaller classification instructions behind them,
pulling the model toward writing SQL instead of picking a shape.

**[Planning](#/books/internal%2Fmcpagent%2Falaws_livi/livi.planning)** — what to count, and along which dimension.
*Sent: [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general) + [Planning](#/books/internal%2Fmcpagent%2Falaws_livi/livi.planning) + [Chart Selection](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts) (laws only — the Vega-Lite
specification examples are stripped, since Planning never draws
anything).*
Chart Selection is included here, not just at Finalizing, because the
grouping that makes a chart possible is decided here. A rhythm question
has to be grouped by day before anything downstream can draw a calendar —
see "Why Chart Selection is sent twice" below.

**[Finalizing](#/books/internal%2Fmcpagent%2Falaws_livi/livi.finalizing)** — the query that produces the answer, and the shape it
takes.
*Sent: [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general) + [Chart Selection](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts) (laws and specification examples both) +
[Finalizing](#/books/internal%2Fmcpagent%2Falaws_livi/livi.finalizing).*

**[Degraded paths](#/books/internal%2Fmcpagent%2Falaws_livi/livi.degraded)** — a rejected query, or a result with no rows.
*Sent: [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general) + [Degraded Paths](#/books/internal%2Fmcpagent%2Falaws_livi/livi.degraded).*

## Why [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general) is sent to every call

General holds the obligations that hold regardless of what is being
asked — never state a figure you cannot point at, scope every query to
the organization, ask when the subject is ambiguous. There is no call
these don't apply to, so excluding General from any of them would let a
turn slip through ungoverned rather than trim anything meaningful.

The one exception is Data Handling within General, which is skipped for
Classification alone — see the note above. Every call that can actually
produce or repair a query still receives it in full.

## Why [Chart Selection](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts) is sent twice

The choice of chart is finalized at the Finalizing stage, but the
grouping that makes that chart *possible* is fixed earlier, at Planning.
Sending Chart Selection only to Finalizing produces a correct chart of
the wrong data — the plan already grouped by month, so no amount of
insight at Finalizing can turn that into a calendar heatmap. This was the
single most common failure this book exists to prevent; see the
[Chart Selection](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts) chapter's individual sections for the specific
plan-vs-finalize traps each one guards against.

Planning receives the chart *laws* without their worked Vega-Lite
specification examples — those examples exist to help Finalizing draw the
chart correctly, and Planning never draws anything, so they would only
add tokens without changing a planning decision.

<!-- alaws:laws -->

