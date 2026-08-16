Team instuction: fill 3 inputs before giving the prompt to llm for each query rca.

0. Clear the chat_debug.log file before running the query in Livi.
1. RCA doc:'s name
2. RCA doc:'s section number
3. Expected: the detailed graph.html path

---

Verify and rewrite one RCA section.

Context files:

- RCA doc: scripts/adoption_chart/rca/<name>/rca.md — section "# N."
- Expected output: file:///home/lovestaco/hex/lr/LiveReview/scripts/adoption_chart/<expected-demo>.html
- Evidence: chat_debug_logs/chat_debug.log (one analytics request; lines are
  huge — don't read whole lines). Anchor on these log lines, not "AI Response":
  - `User Input:` — grep for the query text from "### Query:" in this
    section to find the request; the bracketed hash on that line is the
    request id.
  - `SQL Plan: step=0` — the call-#2 plan JSON (one entry per report).
  - `Report Finalized:` — title + mark + row count per report.
  - `Final Response To User:` — the actual rendered payload: per report,
    `title`, `description`, `time_range`, `granularity`, `query`, and the
    full `data.values`. This is what "### Query:" / "### Result from
    livi:" and the fact-check both quote from — not the raw per-step `AI
Response` lines (there can be several, including retries).
    If the log doesn't contain this request (rotated/cleared since), say so
    and stop — don't reconstruct from memory or guess.
- Query catalogue: scripts/adoption_chart/cto_chart_ideas.html — find the
  matching row by grepping the query text itself, not by row number (row
  numbers drift as the table grows).

Task 0 — Fill in the section's header fields from the log, if not already
filled in:

- `### Query:` — verbatim from the `User Input:` line.
- `### Result from livi:` — one block per report, in the order rendered:
  the title as a heading, then `description` verbatim, then `Time range:`,
  `Granularity:`, `Query:` read from that report's own JSON fields.
  Leave the `[](...)` image line exactly as `[](<screenshot-filename>)` —
  an empty placeholder. Don't invent a filename, don't claim to have seen
  the rendered image; the actual screenshot gets pasted in separately.

Task 1 — Fact-check the claims already in the section against the debug
log. Check: chart titles, what each chart actually shows (mark, fields,
row count, sort behavior), any numbers quoted in the description text
against the chart's own `data.values`, and whether the charts are actually
"unrelated" to the query or just missing some specific thing (banding,
target rule, zero-count rows, etc — be precise, not a blanket "unrelated").
If `### What is missing from the demo:` is empty, fact-check the claims
in `### Result from livi:` instead — those are the claims on record for
this section. For every claim: right / wrong / imprecise, and why. Cite
the log for each correction (request id + line/call number).

Task 2 — Rewrite `### What is missing from the demo:` in place as a
fix-ready brief for another LLM, with exactly three labeled parts and
nothing else:

- **Symptom**: what Livi actually planned and rendered for this query —
  chart titles, marks, row counts, and the call-#2 plan JSON entries from
  the log.
- **Expected**: the chart spec from the file listed under "Expected
  output" above — SQL grouping and window, exact bands/encodings, mark,
  and the KPI numbers shown on that page.
- **Root cause**: which pipeline stage made the wrong decision (classify /
  plan / finalize), which prompt file under internal/mcpagent/prompts/
  drives that stage, and why that prompt produced this outcome (e.g. an
  existing routing rule that covers a similar question type but not this
  one — quote the actual rule).

Do NOT add a fix recommendation or acceptance test.
Do NOT touch any other section of the file.
