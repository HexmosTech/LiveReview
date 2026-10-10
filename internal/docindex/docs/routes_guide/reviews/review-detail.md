# Review Detail

**Route:** `/reviews/:id`
**Component:** `ui/src/pages/Reviews/ReviewDetail.tsx`

## Purpose

Full detail view of a single code review: status, timeline of review events,
AI-generated findings/summary, diff viewer, commit list, and accounting
(cost/token usage per stage). While a review is in-flight, accounting and the
review status badge refresh every 15s, so the badge flips to Completed without
a page reload, and the Findings tab then loads the comments on its own.

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

### Risk assessment

The grey info bar under the title has a **Risk assessment** stat, coloured by
level like the per-change risk badges in Findings: **High risk** / **Moderate
risk** / **Low risk** / **Minimal risk** (from the highest risk score across the
changed code), **Calculating...** while the server is still scoring (it
updates on its own), **Failed** (red), **Skipped** (amber), or **Not
available** (grey).

Click the bar (or rest the pointer on it for 2 seconds) to expand it. The
**Risk assessment** column then shows:

- **When ready** — the highest score, how many changes and files were scored,
  the count of changes per level (High / Moderate / Low / Minimal), and the
  riskiest files (when more than one file was scored).
- **When failed or skipped** — the reason in plain words and how to fix it:
  - Repo too large for the cache, or risk assessment turned off → owners/super
    admins get an "Open Repo Cache settings" link (Settings → Storage → Repo
    Cache).
  - Couldn't access the repository (e.g. expired connector token) → a "Check
    connector" link.
  - Scoring engine not installed, scoring failed, diff failed, or took longer
    than 15 minutes → admins check the worker logs.
  - Low disk space on the server.
  Members are told to ask an admin.
- **When not available** — why: not a PR/MR review, the PR's base is older
  than the 13 months of history kept, the PR has no changes, or no result
  arrived within 15 minutes.

## Related pages

- [Reviews list](reviews-list.md)
- [New Review](new-review.md)

## Learn more (public docs)

- [Turn findings into tickets and follow-up tasks](https://hexmos.com/livereview/docs/livereview/mcp/usecases/turn-findings-into-tickets)
