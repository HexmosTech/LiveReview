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
  Granting permission can also happen inline, right after triggering a review, via
  an "Enable" button on the toast that appears — no need to visit this page first.
- Toggle "First finding is ready" — notified as soon as the first batch of results
  streams in for a review you triggered. On by default.
- Toggle "Review completed or failed" — notified when the whole review finishes.
  On by default.

## Related pages

[Settings overview](settings-overview.md), [Account](account.md)
