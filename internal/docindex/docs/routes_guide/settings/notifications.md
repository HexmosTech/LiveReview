# Settings → Notifications

**Route:** `/settings#notifications`
**Who sees it:** every authenticated user (self-service, no org-role gate)

## Purpose

Control browser push notifications for reviews you personally trigger. LiveReview
can notify you the moment the first finding is ready, and again when the review
finishes (completed or failed) — even if the LiveReview tab or browser is closed,
using the Web Push API and a service worker. Reviews with no identifiable triggering
user (scheduled or webhook-triggered reviews) never send these notifications; this
tab only affects reviews started from a logged-in session.

## Key actions

- Enable or disable browser push notifications for this specific browser/device.
  Granting permission can also happen inline, right on a review's detail page, via
  an "Enable notifications" banner shown there (closeable, not auto-dismissed), which
  also links back to this page — no need to visit this page first.
- If the browser has already blocked notifications for this site, this page (and the
  review-detail banner) shows guidance instead of the Enable button: click the lock or
  info icon next to the browser's address bar, allow notifications there, then reload —
  browsers never let a site re-trigger the native permission prompt once blocked.
- Toggle "First finding is ready" — notified as soon as the first batch of results
  streams in for a review you triggered. On by default.
- Toggle "Review completed or failed" — notified when the whole review finishes.
  On by default. A review that finished with errors is titled "Review completed
  with errors" rather than "Review failed", since most failures here are partial
  (one batch erroring), not a total loss of the review.
- Each notification names the review (`#<id>`, `owner/repo`, and the PR/MR title
  when known) so it's recognizable without opening it, and clicking it opens that
  review's detail page directly (focusing an already-open tab if one matches,
  otherwise opening a new one).

## Related pages

[Settings overview](settings-overview.md), [Account](account.md)
