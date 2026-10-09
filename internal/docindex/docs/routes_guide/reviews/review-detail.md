# Review Detail

**Route:** `/reviews/:id`
**Component:** `ui/src/pages/Reviews/ReviewDetail.tsx`

## Purpose

Full detail view of a single code review: status, timeline of review events,
AI-generated findings/summary, diff viewer, commit list, and accounting
(cost/token usage per stage, refreshed every 15s while a review is
in-flight).

## Who can access it

Any authenticated org member; the review must belong to their org
(org-scoped, enforced server-side).

## Key actions

- View the diff being reviewed (`DiffViewerPanel`) alongside AI findings.
- View the event/log timeline of the review run (`ReviewEventsPage`).
- View accounting: which stages ran, tokens/cost per stage
  (`getReviewAccounting`).
- View the commits included in this review (up to a preview limit, expandable).
- Navigate back to the reviews list.

## Blast-radius scoring and ordering

Reviews run through the git-lrc CLI upload their own blast-radius report. PR
reviews started from LiveReview (UI, API, MCP, the PR list) get one from a
server-side job that runs alongside the AI review, so the scores can appear a
little after the findings. When either exists, the diff viewer
(`DiffViewerPanel`) orders hunks by a risk score instead of plain diff order.
The score blends three dimensions:

- **Blast Radius** — how far the change can reach (callers, entry points,
  repository hotspots, architectural layers).
- **Review Priority** — how much scrutiny it needs (complexity, duplication,
  missing test coverage).
- **Finding Severity** — the most severe AI finding on the hunk
  (`critical`/`warning`/`info`), blended in at ~10% weight so a critical
  security bug outranks a trivial one.

The "Score: Whole" / "Score: Per file" / "Diff order" control switches between
the whole-diff ranking, per-file ranking, and original diff order. Each hunk's
breakdown panel (Summary and Math Mode tabs) shows the exact math behind its
score, including the finding-severity contribution.

If the server couldn't compute blast radius, the page shows a "No blast radius
for this review" notice saying why:

- **Repo too large for the cache** — the repo (shallow clone + code graph) is
  bigger than the Blast Radius Repo Cache size. Owners/super admins get an
  "Increase cache size" link to Settings → Storage; members are told to ask an
  admin. Reviews after the size is raised get blast radius again.
- **Low disk space** — the server had under 1 GB free when the review ran.

## Related pages

- [Reviews list](reviews-list.md)
- [New Review](new-review.md)

## Learn more (public docs)

- [Turn findings into tickets and follow-up tasks](https://hexmos.com/livereview/docs/livereview/mcp/usecases/turn-findings-into-tickets)
