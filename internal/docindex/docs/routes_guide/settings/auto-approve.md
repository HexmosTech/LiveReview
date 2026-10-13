# Settings → Auto Approve

**Route:** `/settings#auto-approve`
**Who sees it:** org **owner** (or super_admin)
(`canManageCurrentOrg` in `ui/src/pages/Settings/Settings.tsx`).

## Purpose

Configure the risk-based auto-approve policy that decides, after every review
started from a PR/MR "Start a review" link, whether the review is automatically
approved or changes are requested — without a human clicking approve. See
`docs/risk-policy-engine.md` for the full design, and
`internal/policyengine` for the engine.

The decision is a funnel, evaluated in order, each stage recorded in
`review_policy_log` and rendered in the review's **Auto Approve** tab:

1. **Eligibility** — the PR author must map to a LiveReview user (via their
   Git username/email, matched through `user_identities`) with a
   `review_level` at or above the policy's `min_author_level`. Explicitly
   whitelisted users pass regardless of level; bots default to level 0.
2. **Static rules** — the configured CI/CD rulesets (`ci_rulesets`) run as
   hard blocking gates (they also run under "always approve").
3. **Risk score** — a trained logistic model scores the review and blocks it
   if the score is at or above the configured percentile of the org's score
   distribution.
4. **LLM classification** — the review LLM's diff-level `diffClassification`
   must have confidence at/above the required level and no disqualifying
   class (security-sensitive).

## Layout

Five sub-tabs:

- **Risk Model** — the active model version + training-sample count (model
  confidence), category weight overrides (blast radius / security / critical /
  diff size, instant effect), a manual **Refit model now** button, and a
  **Simulate on recent reviews** replay of the draft policy.
- **Static Rules** — pick which CI rulesets are blocking gates.
- **Eligibility** — `min_author_level` and `force_approve_min_level`.
- **General** — `risk_threshold_percentile`, required LLM confidence, landing
  delay (human-rejection window), daily approve cap, and the
  "restrict outcome reporting to admins" switch.
- **Production Feedback** — the outcome webhook URL and the last-30-days
  outcome counter.

## Key actions

- **Save policy** upserts the organization-default policy (a single row in
  `auto_approve_policies` with `repository_id IS NULL`). Individual repos can
  override it from [repository settings](../explore/repository-settings.md).
- **Refit model now** runs a batch refit over all labeled outcomes and
  promotes it only if its AUC is at least as good as the current model.
- **Simulate** replays the draft against recent completed reviews (dry-run,
  nothing persisted).
- Report production outcomes (revert / incident / confirmed-safe) manually
  from a review's **Auto Approve** tab, or POST them to the webhook. Each
  non-test outcome nudges the model in real time.

## Related pages

[Settings overview](settings-overview.md) ·
[CI Rulesets](../reviews/ci-cd-gates.md) ·
[Repository settings](../explore/repository-settings.md) ·
[Review detail](../reviews/review-detail.md)
