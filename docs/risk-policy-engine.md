# Risk-Based Auto-Approve Policy Engine

## Context

A colleague already shipped the plumbing for this feature on `feat/auto-approve`:
a PR comment with a "Start a review" link (`internal/jobqueue/repo_sync_worker.go:265-321`),
a trigger endpoint that launches a review from that link
(`internal/api/pull_request_review_trigger.go`), and a worker that posts an
approve/request-changes decision back to GitHub/GitLab
(`internal/jobqueue/pr_policy_worker.go`). The decision logic itself is a
placeholder — `result.Approved = result.Critical == 0` — explicitly marked
`// ponytail: one static policy; swap for org-defined policies later`.

We're replacing that placeholder with a real, configurable, risk-based policy
engine, modeled on Meta's RADAR system (arxiv 2605.30208): a layered funnel of
**eligibility → static/deterministic gates → ML risk score → LLM semantic
classification → final decision**, rather than one opaque model. The paper's
core lessons we're deliberately carrying over:
- Risk thresholds are a per-org *dial*, not a fixed cutoff (their P5→P50 experiment).
- Safety signals (revert rate, incident rate) must feed back into calibration — this
  doesn't exist in LiveReview today and needs new plumbing.
- The system must stay legible: every decision should show *why*, stage by stage, not just a verdict.

LiveReview already has three of the five RADAR ingredients half-built, which
shapes this plan heavily:
- **Static gates** → the CI/CD gate jq-ruleset system (`ci_rulesets` table,
  `internal/api/ci_rulesets_handler.go`'s `runJQBool`/`buildCanonicalReviewDoc`)
  is *exactly* RADAR's deterministic-validation stage, just not yet reusable
  outside CI. We reuse it as-is rather than building a second rule engine.
- **LLM semantic signal** → findings already carry `Severity`/`Confidence`/`Category`
  (`pkg/models.ReviewComment`, stored in `ai_comments` JSONB). What's missing is an
  explicit *diff-level* classification (safe-refactor vs logic-risk vs security-risk),
  not per-finding severity.
- **Risk score** → blast radius already scores *hunks* (`internal/blastradius`,
  `blast_radius_hunks` table) but only aggregates via `MAX(combined)` per review
  (`blast_radius_reviews` view) — a deliberately crude placeholder with
  "no production caller today". We build the real diff-level score on top of this.
- **Eligibility** → doesn't exist at all. No seniority/level field on users, no
  org policy config table, no settings UI tab for any of this.

## Non-goals for this plan

No full automatic revert-detection (e.g. auto-recognizing a revert commit by
pattern) beyond a manual/webhook ingestion point, and no attempt to make the
online-learning update path literally streaming/sub-second — "near real
time" here means an immediate single-example gradient nudge on each incoming
labeled outcome, plus a daily stabilizing batch refit and an on-demand
button, which is what was asked for.

## Architecture

### 1. Data model

**`users.review_level`** (new column, `INTEGER NOT NULL DEFAULT 1`) — the
"seniority" dial the user asked for. No roles/tiers table; just a plain
integer owners can bump per user from the existing Users settings tab
(`ui/src/pages/Settings/UsersSettingsTab.tsx` — add one numeric field +
`PUT /api/v1/orgs/:org_id/users/:user_id` already exists, extend its payload).

**One table, with one deliberate exception**: the thing that must stay
strictly org-wide is *the trained classifier itself* — `risk_model_versions`'
learned weights and `risk_factor_overrides`' category multipliers (§3) —
because fitting/retraining a separate model per repo is the genuinely
expensive thing hundreds of repos can't each support. The *threshold*
compared against that one shared score is a different, cheap thing: just a
cutoff number, no model behind it, so it's fine — and per the user's
preference, correct — to let it vary per repo. `auto_approve_policies`
therefore carries `risk_threshold_percentile` alongside everything else,
resolved the exact same way as every other field on this table:

```sql
CREATE TABLE auto_approve_policies (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    repository_id BIGINT REFERENCES repositories(id), -- NULL = org default
    mode TEXT NOT NULL DEFAULT 'custom'
        CHECK (mode IN ('inherit','custom','always_approve','never_approve')),
    -- 'inherit' only valid on repo-scoped rows (explicit "use the org default" marker,
    -- distinct from simply having no row, so the repo settings UI in §9 can show it
    -- as a selected choice rather than an absence); org-default rows are never 'inherit'.
    min_author_level INTEGER NOT NULL DEFAULT 1,
    eligibility_whitelist_user_ids BIGINT[] NOT NULL DEFAULT '{}', -- bypass min_author_level
    risk_threshold_percentile INTEGER NOT NULL DEFAULT 20, -- P20 default; cheap to override per repo, see below
    require_llm_confidence INTEGER NOT NULL DEFAULT 8,     -- out of 10, mirrors RADAR's ACR gate
    blocking_ruleset_ids BIGINT[] NOT NULL DEFAULT '{}',   -- reuses ci_rulesets.id
    landing_delay_seconds INTEGER NOT NULL DEFAULT 0,      -- post-approve human-rejection window
    daily_approve_cap INTEGER,                             -- NULL = unlimited
    restrict_outcome_reporting_to_admins BOOLEAN NOT NULL DEFAULT false, -- see §10
    force_approve_min_level INTEGER NOT NULL DEFAULT 3,    -- see §7
    created_by, created_at, updated_at
);
```
Resolution order for a given PR: repo-scoped row if present and `mode !=
'inherit'`, else org-default row (`repository_id IS NULL`), else feature is
off — every field on this table, including the threshold, resolves per-repo
this way, pre-filled from the org default when a repo admin opens
"Customize" (§9) so they start from the global calibration and only change
what they want. The risk *score* itself always comes from the one shared
model in `risk_model_versions`
regardless, looked up by `org_id` alone. `mode = 'always_approve'`/`'never_approve'`
on either level are the "people may already know the rules for this repo"
shortcuts from the user's ask — skipping the risk-score/eligibility/LLM
stages entirely for `always_approve` (clearly a low-stakes repo, no need to
run the model against it every time), but the static jq gate (§4) still
runs even under `always_approve` — a repo-wide "trust this repo" setting is
not the same thing as "ignore a secrets-leak detector," and should never be
able to silently become that by accident. Every decision made under either
mode still writes a `review_policy_log` row ("mode override:
always_approve"), so the Auto Approve tab (§7) shows *why* just as clearly
as a normal funnel run, not a silent skip. This mirrors `ci_rulesets`'
existing org-scoping pattern — reuse its storage/handler conventions
directly (`internal/api/ci_rulesets_handler.go` is the template).

A new **`review_policy_log`** table (append-only) replaces the single
`metadata.policy_result` JSONB blob: one row per policy evaluation with
`review_id, stage, verdict, detail JSONB, created_at`, so the UI can render
the full funnel trace (eligibility → static → risk → LLM → final), not just
the final approve/reject. (`risk_feature_snapshots` and `risk_model_versions`
— the tables backing the trained risk model — are described in §3, since
they're part of that stage's design, not generic schema.)

**`review_outcomes`** (new table) for the feedback loop: `review_id, org_id,
reported_by_user_id, outcome_type ('revert'|'incident'|'confirmed_safe'),
source ('webhook'|'manual'|'ci'), reason TEXT NOT NULL, detail JSONB,
created_at`. `reason` is mandatory, not optional free text — a one-line
"why" (e.g. "rolled back due to prod 500s") required on every manual mark
and every webhook payload. This is the deliberate friction point: a bare
toggle with no permission check (per §10) would be too easy to mis-click or
spam, but a required reason keeps the manual log readable/auditable and
gives the risk model's audit trail (§7, §11) something a human reviewing it
later can actually make sense of, without needing an approval workflow or
role restriction to prevent noise. Populated by a manual "Mark as
reverted" action in the review detail UI and a documented webhook endpoint
(`POST /api/v1/reviews/:id/outcomes`, `reason` required in the payload too)
for CI/CD or incident-tooling integrations to report rollbacks/incidents
against a known review id — real auto-detection (e.g. watching for
revert-commit patterns) is future work. Critically, every insert into this
table is also the trigger for the real-time online model update described
in §3 (path 1) — this table isn't just a read-only log, it's the live
feedback signal the risk model reacts to.

**`review_policy_overrides`** (new table): `review_id, org_id,
actor_user_id, original_verdict, forced_verdict, reason TEXT NOT NULL,
created_at`. This backs **Force Approve** (§7) — a human explicitly
approving (or rejecting) a specific PR *despite* the policy engine's
verdict, with a mandatory reason, fully audited. This is distinct from
`review_outcomes`: an override is about one specific review's decision
being overridden by a person, while an outcome is a production-truth signal
(revert/incident) the model learns from — they can both exist for the same
review without conflating "a human overrode the gate" with "this PR turned
out to be risky in prod."

### 2. Eligibility stage

New `internal/policyengine/eligibility.go`: given a review + its PR, resolve
`users.review_level` for the PR author (join via `pull_requests.author_email`
or equivalent existing field). If the author's id is in
`eligibility_whitelist_user_ids`, pass immediately regardless of level —
this is the "if &lt;L3, always requires a review, unless whitelisted" rule
from the user's ask, read literally: level is the default gate, the
whitelist is the explicit per-user exception to it. Otherwise require
`review_level >= min_author_level`. Bots (reviews with no human author /
from a known bot login) get `review_level = 0` unless explicitly
whitelisted — this is the human/bot split RADAR treats as the very first
fork in the funnel.

### 3. Risk score stage — an online-learning model that updates itself

**The hard requirement here is adaptivity, not model complexity**: new
incident/rollback data must move the risk score within the day it arrives
(ideally immediately), and an admin tweaking "weight security findings
higher" must take effect immediately too. That requirement rules out the
ONNX-file approach from the earlier draft of this plan — ONNX is a *serving*
format for a model trained elsewhere; re-running an offline training script
and re-exporting a file on every incoming webhook is the wrong shape for
"updates in near-real-time." So this stage is a genuine online-learning
model, not a periodically-refreshed static artifact.

**Library**: `github.com/cdipaolo/goml`'s `linear.Logistic`, specifically its
**`StochasticGA` online-learning mode** — pure Go, no cgo, and built for
exactly this: each call to `OnlineLearn` takes one new labeled example and
nudges the existing weight vector by one gradient step, in milliseconds, no
retraining pass over the full dataset required. (`goml` also supports a full
batch fit over a dataset in one call, used below for the periodic
stabilizing pass.) This is a legitimate, citable ML library doing real
gradient-based learning — not hand-picked constants — while still being
light enough to update on every single incoming data point, which is the
actual thing the user asked for.

**Feature vector** (model inputs, shared with the simulation/backtest panel in §8):
- Blast-radius diff-level aggregates computed fresh from `blast_radius_hunks`
  rows for the review — `sum(combined)`, `count(tier >= high)`,
  `max(combined)`, `weighted_avg(combined, by hunk LOC)` — replacing the
  `MAX(combined)` placeholder's reasoning with real feature engineering.
- Finding-severity counts (critical/warning/info) from the same
  `canonicalReviewDoc` the jq rulesets already build
  (`buildCanonicalReviewDoc` in `ci_rulesets_handler.go` — reused directly,
  not re-queried).
- Diff-shape: files changed, lines added/removed (already on the review/PR row).
- LLM semantic-classification confidence + class, once §5 lands (feature
  vector is append-only across phases — new features don't break old model
  versions, they just start at 0 until the next retrain includes them).

**Cold start — common sense first, data-driven second.** Most orgs will
have zero or a handful of `review_outcomes` rows for a long time — "collect
enough data to train a model" cannot be the starting state, or the feature
is useless for exactly the orgs that most need good defaults. So the seed
model's weights aren't zero-initialized or randomly initialized before the
first fit — they're hand-set to encode the same domain reasoning already
embedded elsewhere in this codebase (blast-radius tiers, the current
critical-findings-block rule, RADAR's own safe/risk taxonomy): blast-radius
sum and critical-finding count dominate, security-category findings are
weighted up, info-level findings and small diffs are weighted near zero.
This prior is the actual fallback behavior with no data — not a "cold start
model" distinct from the "real model," but the starting *point* every org's
model begins from and is anchored back to.

That anchor is enforced structurally, not just at initialization, via a
**prior-anchored regularization term** on every batch refit (§ path 2/3):
instead of plain L2-toward-zero, penalize weights for drifting from the
common-sense prior, scaled by `λ / (1 + training_sample_count)` — so with 3
labeled outcomes the fitted model stays close to common sense (data can't
yet overrule domain knowledge), and by the time an org has a few hundred
outcomes the anchor's pull is negligible and the weights are essentially
data-driven. This is one extra term in `goml`'s existing loss, not a second
model or a blending step at inference time — inference is still a single
dot product against one weight vector, just one that was fit with this
anchor. The real-time online nudge (path 1) uses the same anchored loss per
step, so even online updates can't swing wildly away from the prior on a
single noisy example.

The UI should say this plainly rather than imply a hidden black box: the
risk-model section of §8/§11 shows a **"model confidence"** indicator derived
from `training_sample_count` (e.g. "Low — mostly common-sense defaults (3
outcomes recorded)" vs "Calibrated to your repo (180 outcomes recorded)"),
so an org with no data sees an honest "this is sensible defaults, not yet
learned from your history" rather than a number that looks authoritative
before it's earned that.

**`risk_feature_snapshots`** (new table): `review_id, org_id, model_version,
features JSONB, score, created_at` — written at every policy evaluation.
Makes the model auditable (UI can show "this review's risk score came from
these feature values") and is the training set once `review_outcomes` rows
attach to the same `review_id`s — recomputing features after the fact from
mutable tables would drift, so they're snapshotted at evaluation time instead.

**`risk_model_versions`** (new table): `id, org_id (NULL = shared default),
version, weights_json, trained_at, training_sample_count, holdout_auc,
update_kind ('seed'|'online_update'|'batch_refit'), is_active`. Weights are
a few dozen floats, so unlike the ONNX design there's no blob store involved
at all — the row itself is the model. Cold start ships one seed model
(`org_id IS NULL`, `update_kind = 'seed'`, `training_sample_count = 0`) whose
weights are exactly the hand-set common-sense prior described below, not a
fit against any data — every org starts here and only moves away from it as
real `review_outcomes` accumulate.

**Three update paths, all ending in a new `risk_model_versions` row (append-only, so any version can be reactivated as an instant rollback)**:
1. **Real-time online nudge** — the moment a `review_outcomes` row lands
   (webhook or manual "mark as reverted"), the policy engine loads that
   review's `risk_feature_snapshots` row as the labeled example and calls
   `OnlineLearn` on the org's current weights, immediately persisting the
   nudged weights as a new version and activating it. This is the actual
   "learns in real time" behavior — no batch job, no delay, one gradient step.
2. **Daily batch refit** — a River periodic job (same registration pattern
   as `blast_radius_worker.go`) re-fits a fresh `linear.Logistic` from
   scratch over all of an org's `risk_feature_snapshots ⋈ review_outcomes`,
   corrects for drift that a long sequence of noisy one-example online steps
   can accumulate, evaluates holdout AUC, and only promotes if it's at least
   as good as the current online-nudged version — the stabilizing
   counterpart to (1), not a replacement for it.
3. **On-demand refit** — the same batch refit as (2), triggered by a
   "Retrain now" button in the policy settings UI (§8) / `POST
   /api/v1/risk-model/retrain`, for when an admin just changed a weight
   override (below) and doesn't want to wait for the nightly job.

**Admin weight overrides** (`risk_factor_overrides` table: `org_id,
feature_category, multiplier, updated_at`) are how "people may update the
weights for certain factors (security, etc.)" takes effect *instantly*,
decoupled from training cadence. These are deliberately **coarse,
category-level** sliders, not one per raw model feature — `blast_radius`,
`security_findings`, `critical_findings`, `diff_size` map to small groups
of underlying features (e.g. `blast_radius` covers `sum(combined)`,
`max(combined)`, `weighted_avg(combined)` together), so an admin adjusts a
handful of legible dials instead of a dozen raw coefficients that could
easily be set into a contradictory configuration. Scoring computes
`effective_weight_i = learned_weight_i * override_multiplier[category(i)]`,
so dragging the "security" slider changes live scoring the moment it's
saved, with no retrain needed. The next batch refit (path 2/3) treats each
category override as an L2-regularization anchor (penalize that category's
learned weights for drifting far from `override_multiplier * initial_weight`)
so the model doesn't fight or silently erase the admin's stated preference
on its next pass — the override is both an instant hard effect and a soft
prior for future learning.

**Inference**: `internal/riskscore.Scorer` keeps the org's active
`risk_model_versions` weights cached in memory (invalidated whenever a new
version activates via any of the three paths above), computes
`sigmoid(Σ effective_weight_i * feature_i)` directly — a dot product, no
runtime/session overhead, which is also why this design tolerates being
updated on every single webhook event without a performance concern.

**Percentile calibration**: unchanged — per-org, a rolling window of recent
scores turns a given `risk_threshold_percentile` value into "this org's
actual P20," not a global magic number. Independent of which model version
produced the scores, so recalibration is cheap even right after an online
nudge changes the score distribution. The *calibration curve itself* is a
single org-wide thing for v1 (one model per org, per the per-org-only scope
decision) — but the `risk_threshold_percentile` cutoff read off that curve
is a plain field on `auto_approve_policies` (§1) and so *is*
repo-overridable, exactly like the rest of that table: a repo can decide
"P50 is fine for us" against the same shared score distribution without
needing its own curve.

**ONNX note**: `github.com/yalue/onnxruntime_go` is already vendored
(serving the BGE embedder in `internal/docindex/embedder.go`) and remains a
good fit if a *future* heavier model — gradient-boosted trees, a small
MLP — trained offline on a slower cadence is ever wanted; `riskscore.Scorer`
should be a small interface (`Score(features) float64`) specifically so an
ONNX-backed implementation could be swapped in later for one org without
touching the policy engine. It's just not what satisfies "updates in near
real time" today — that's what `goml`'s online mode is for.

### 4. Static/deterministic gate stage — reuse jq rulesets as-is

No new rule engine. `auto_approve_policies.blocking_ruleset_ids` references
existing `ci_rulesets` rows. The policy worker calls the *same*
`buildCanonicalReviewDoc` + `runJQBool` already used by
`CIRulesetsHandler.Evaluate` — this is the literal reuse of "the existing jq
based blockers" the user asked for. The only new code is a thin loop: for
each configured ruleset id, evaluate, block on first truthy. The existing
`ui/src/pages/CiRulesets/` builder UI becomes dual-purpose automatically (a
ruleset can be attached to CI, to auto-approve, or both) — just needs a
checkbox/multi-select in the new policy editor listing existing rulesets by
name, no changes to the ruleset UI itself.

### 5. LLM semantic classification stage — folded into the existing review call

Rather than a second LLM round-trip, this rides inside the review-generation
call that already exists: `internal/prompts/templates.go`'s
`JSONStructureExample` (the schema the model is told to return, currently
`fileSummaries` + `comments`) gets one more top-level object,
`diffClassification`:
```json
"diffClassification": {
  "class": "refactor-only|logic-change|security-sensitive|...",
  "confidence": 7
}
```
alongside a short addition to `CommentRequirements`/`ReviewGuidelines`
describing RADAR's safe/risk taxonomy (refactor-only, dead-code-removal,
logic-change, security-sensitive, etc.) and how to pick `confidence` (0–10).
Same taxonomy-enforcement pattern already used for `category`/`subcategory`
(`TaxonomyClassificationRules`, templates.go:100) — a fixed, closed list,
not free text. Whatever code already parses the model's JSON response into
`fileSummaries`/`comments` (one extra field on the same struct) picks up
`diffClassification` too and stores it on the review alongside the rest of
`review_result` (same `metadata.review_result` JSONB it already writes to —
no new storage column needed, `buildCanonicalReviewDoc` already reads from
this blob and gains one more field to surface for jq rules). Trade-off
accepted deliberately: this couples the classification's prompt/schema to
the main review prompt, so a future change to one should check the other —
worth it here to avoid a second LLM call's cost/latency per review, since
findings generation already has full context on the diff. Policy requires
`confidence >= require_llm_confidence` AND no disqualifying class, mirroring
RADAR's "any risk signal disqualifies" rule.

### 6. Rewiring `pr_policy_worker.go`

Replace the body of `Work`/the fake `policyResult` with calls into
`internal/policyengine`: resolve policy → eligibility → static gates → risk
score vs. threshold → LLM classification gate → final verdict. Each stage
writes a `review_policy_log` row. `buildPolicyComment` is extended to render
the funnel (stage-by-stage ✅/❌ table) instead of just severity counts —
this is the "show why" requirement. `landing_delay_seconds` becomes an actual
delay before `publish()` runs (queue a follow-up River job instead of
publishing immediately), giving the "human rejection window" RADAR relies on
for safety.

### 7. Review detail — "Auto Approve" tab

Every review triggered through the PR-link flow should show its policy
funnel on its own detail page, not just in the GitHub/GitLab comment. Add
`'auto-approve'` as a fourth value to `ReviewDetail.tsx`'s existing
`activeTab` union (currently `'findings' | 'accounting' | 'events'`,
`ReviewDetail.tsx:84`) and a fourth entry in the `tabs={[...]}` array passed
to the existing `Tabs` component (`ReviewDetail.tsx:679-686`) — the tab only
appears when the review has `metadata.pr_link_policy = true` (same flag the
backend already uses to decide whether to run the policy at all), so plain
ad-hoc reviews don't show an empty tab.

Content, backed entirely by `GET /api/v1/reviews/:id/policy-log` (new
endpoint, reading `review_policy_log` ordered by stage):
- A vertical stage list — Eligibility, Static Rules, Risk Score, LLM
  Classification, Final Decision — each row showing ✅/❌/⏭️ (skipped, e.g.
  static rules skipped because eligibility already blocked) and a one-line
  reason ("Author level 2 < required 3" / "Blocked by ruleset `no-secrets`"
  / "Risk score 34 (P45) ≥ threshold P20" / "approved").
- Expanding the Risk Score row shows the actual feature breakdown from that
  evaluation's `risk_feature_snapshots` row as a small table (feature name,
  value, effective weight, contribution) — directly answers "what parts of
  the decision are what" for the one stage that isn't already self-explanatory
  jq/eligibility logic, and doubles as the explainability view for the
  model-confidence indicator from §3.
- A final summary banner matching the comment already posted to
  GitHub/GitLab (reuse `buildPolicyComment`'s data, rendered as UI instead
  of markdown, so the two never drift apart in wording).
- If `mode` was `always_approve`/`never_approve` (§1), the tab shows that
  as the single reason, with a link to the policy settings — no funnel to
  render since none ran.
- **Force Approve**: when the final verdict is "blocked," a visible "Force
  approve anyway" button, gated by `users.review_level >=
  force_approve_min_level` (a configurable floor, default 3, in §8's
  Eligibility sub-tab — not tied to org role, consistent with seniority
  being the dial that already governs eligibility in §2) rather than a
  fixed owner/admin check, opens a small form requiring a reason, then
  calls `POST /api/v1/reviews/:id/force-approve`. Only the blocked→approved
  direction exists — there's no symmetric "force request-changes" on a PR
  the engine already approved, since a human can already just leave an
  ordinary review comment or request-changes on any PR regardless of what
  the bot posted; the override only needs to exist for the direction that's
  actually dangerous (bypassing a safety gate), not the direction that's
  already unrestricted. This posts the real GitHub/GitLab
  approval via the same `publish()` path as a normal approve (§6), but the
  comment is explicitly marked "⚠️ Force-approved by {user} despite policy
  block — {reason}" so the PR history is honest about what happened, and
  writes a `review_policy_overrides` row (§1) that the metrics dashboard
  (§11) can later report on ("N force-approvals this month, M of which were
  later marked as incidents" — the sharpest possible signal that a policy
  threshold is miscalibrated).

### 8. Settings UI — policy editor (sub-tabbed) + "what would've happened" simulation

New Settings tab `ui/src/pages/Settings/AutoApprovePolicyTab.tsx` (outer
registration pattern: `StorageSettingsTab.tsx`, gated `requiresOwnerOrAdmin`
like the other admin tabs in `Settings.tsx`'s `tabs` array, ~line 292). The
page itself is **five horizontal sub-tabs** (Risk Model, Static Rules,
Eligibility, General, and Production Feedback — the last described in
§10), using the same `Tabs` component/pattern already used inside
`ReviewDetail.tsx` (§7) rather than a long single-scroll form — this
directly answers the "each thing probably can be a tab within that main
tab" ask. **The trained classifier itself is org-only** (`risk_model_versions`'
weights, `risk_factor_overrides`' category multipliers, retraining — all
with no `repository_id`, never per-repo, since that's the expensive part);
everything else, *including the threshold read off that model's score*, is
backed by `auto_approve_policies` and reappears, repo-scoped, in §9's
repository settings — same components, different `repository_id` on the
save call, per the "I'd prefer to have those as repository level overrides
as well" ask.

**Sub-tab A — Risk Model (the classifier itself, org-wide only, not
repeated in §9)**: enabled toggle, the four coarse category weight sliders
(Blast Radius / Security Findings / Critical Findings / Diff Size, backed
by `risk_factor_overrides`, instant effect per §3), the model-confidence
indicator ("Low — 3 outcomes recorded" / "Calibrated — 180 outcomes
recorded"), a "Retrain now" button (`POST /api/v1/risk-model/retrain`), and
a read-only calibration chart (score distribution across the org's recent
reviews) that both this sub-tab's own org-default threshold field and every
repo's threshold override (Sub-tab D / §9) are plotted against, so an admin
tuning either always sees the same curve. The threshold field itself lives
on Sub-tab D below (org-default row included), not here — it's a plain
policy field, not part of the model.

**Simulation/backtest panel** (shown on Sub-tab A, reused everywhere a
threshold is being edited): a read-only `POST
/api/v1/auto-approve-policies/simulate` that takes the draft policy (not
yet saved — optionally for a specific repo, passing a `repository_id` so
"what would this repo's *effective* policy have done" is answerable even
though the underlying score comes from the shared model) and replays it
against the org's (or that repo's) last N completed reviews — same
evaluation path as production, result discarded — showing a table of
{review, would-approve?, which stage blocked it, risk score}. Reuses 100%
of real evaluation code (§2-§6), so there's no separate preview logic to
keep in sync — the same pattern `CiRulesetEditor.tsx` already uses to
preview a jq expression against a past review.

**Sub-tab B — Static Rules** *(repo-overridable, reused in §9)*: "Write and
test static jq rules" per the user's ask, reusing `ui/src/pages/CiRulesets/`
wholesale rather than building a second editor — this sub-tab is a thin
wrapper embedding `CiRulesetEditor.tsx`'s existing builder (expression
input, live preview against a past review, jq quick-help sidebar from
`jqQuickHelp.tsx`) plus a multi-select of which existing/newly-created
rulesets are attached to this policy as blockers (`blocking_ruleset_ids`).
A ruleset created or edited here is the exact same `ci_rulesets` row usable
for CI/CD gates — no fork, per §4. A repo override can attach a completely
different set of rulesets than the org default (e.g. a stricter ruleset
only on the payments repo).

**Sub-tab C — Eligibility** *(repo-overridable, reused in §9)*:
`min_author_level` input, explanatory copy for the "below this level,
always requires human review unless whitelisted" rule, a user picker
(searching the org's members, same component pattern as
`UsersSettingsTab.tsx`'s member list) to manage
`eligibility_whitelist_user_ids` — add/remove with each user's current
`review_level` shown inline so it's clear *why* they'd otherwise be
blocked — and a separate `force_approve_min_level` input (§7) with its own
explanatory copy, since it's a distinct dial from `min_author_level`
(one governs whose *PRs* skip the funnel, the other governs who can
*override* the funnel's verdict on anyone's PR). A repo can reasonably want
a higher bar here than the org default (e.g. the payments repo requires
L4+ where the org default is L2+).

**Sub-tab D — General** *(repo-overridable, reused in §9)*:
`risk_threshold_percentile` (shown as a percentile slider against the same
calibration chart from Sub-tab A, pre-filled from the org default on a
repo's first "Customize"), `require_llm_confidence` slider (the LLM
classification gate from §5), `landing_delay_seconds`, `daily_approve_cap`,
and `restrict_outcome_reporting_to_admins` — the threshold sits here rather
than on its own sub-tab specifically because it's a plain field on
`auto_approve_policies`, not model configuration, and belongs next to the
other per-policy knobs that need to be overridable per repo the same way.

Mega menu: add an entry under the existing "Reviews" or "Settings" section
in `ui/src/components/Navbar/megaMenuData.ts` per this repo's standing rule
that every new settings tab needs one — mirror the `requiresOwnerOrAdmin`
gating from the tab itself.

### 9. Repository-level overrides (Explore → Repositories → repository settings)

`ui/src/pages/Explore/RepositorySettings.tsx` already has exactly the
layout this needs: a `SECTIONS` array (currently just `repo-cache`,
`RepositorySettings.tsx:10`) rendering a left-hand section list and the
selected section's component on the right, hash-routed
(`#repo-cache`/`#auto-approve`). Add one entry, `{ id: 'auto-approve', name:
'Auto-approve', icon: ... }`, and a new `RepoAutoApproveSection.tsx`
component (same shape as the existing `RepoCacheSection.tsx` it sits next
to) that:
- Shows the resolved org-default policy as read-only context ("Org default:
  Custom · min level 2 · P20 threshold · 2 blocking rulesets") plus a line
  making clear which part of that *can't* be changed per-repo: the trained
  model behind the score itself (weights, retraining, category overrides —
  §3) is always the one org-wide classifier, no matter what this repo's
  policy says. The threshold compared against that score, though, is just
  another field below.
- Offers the same four choices as `auto_approve_policies.mode`:
  **Use org default** (`mode = 'inherit'`, the common case — most repos
  need nothing here), **Always auto-approve this repo**
  (`mode = 'always_approve'`, for the low-stakes repos the user mentioned —
  docs sites, internal tools), **Never auto-approve this repo**
  (`mode = 'never_approve'`, for the sensitive repos), or **Customize**
  (`mode = 'custom'`, which expands into **Static Rules / Eligibility /
  General** — §8's sub-tabs B, C, D, reusing those exact components with
  `repository_id` set instead of building repo-scoped equivalents, General
  including the `risk_threshold_percentile` field pre-filled from the org
  default — but *not* a Risk Model sub-tab, since the classifier itself
  stays global per the user's "keep the risk model simple and global"
  decision, while the threshold and everything else becomes repo-overridable).
- This is the same `auto_approve_policies` table and
  `internal/policyengine` resolution logic from §1 — a repo-scoped row is
  not a different feature, just a different `repository_id` on the same
  CRUD endpoints, so this section is mostly UI wiring, not new backend work.
  The risk score itself is always computed once per review from the org's
  single model (§3) regardless of which repo it's in — only the threshold
  compared against it, and everything else on `auto_approve_policies`, can
  differ per repo.

### 10. Outcome ingestion UX — setup, test, and soft guidance

This is the single most fragile link in the feedback loop: `review_outcomes`
is the only thing that makes the risk model real ML instead of a frozen seed
model, and most orgs have no existing process for tagging "this PR caused an
incident" — this is a brand-new habit, not a webhook they're already
sending. So the UI work here matters as much as the API/schema from §1.

**Setup panel** (a fourth sub-tab on `AutoApprovePolicyTab.tsx`, §8 —
"Production Feedback" alongside Risk Model / Static Rules / Eligibility): a dedicated
"Production feedback" subsection, modeled directly on
`SMTPSettingsTab.tsx`'s `handleTest`/"Test Connection" pattern (same
loading/success/error state shape) and `StorageSettingsTab.tsx`'s connection
test, rather than inventing a new pattern:
- Shows the org's webhook URL (`POST /api/v1/orgs/:org_id/review-outcomes`,
  authenticated the same way other inbound integration webhooks are —
  reuse whatever bearer/signature scheme `ci_rulesets`' CI integration
  already documents in `CiRulesetIntegration.tsx`/`shared.tsx` rather than
  inventing a new auth story) plus a one-click "regenerate secret."
- A **"Send test event"** button that POSTs a synthetic outcome against a
  real, recent review in that org (picked from a dropdown) marked
  `source: 'test'` so it never pollutes the training set (`risk_feature_snapshots
  ⋈ review_outcomes` queries used for refitting explicitly exclude
  `source = 'test'`), then immediately shows the actual stored row back —
  "✅ received, this review is now tagged as reverted" — so the admin gets
  the same end-to-end confidence as `handleTest` gives for SMTP: a round
  trip against the real endpoint, not a dry validation.
- Copy-paste integration snippets for the common cases, same generator
  pattern as `buildCurlSnippet`/`buildGithubActionsWorkflow` in
  `CiRulesetIntegration.tsx`'s `shared.tsx`: a `curl` one-liner for CI/CD
  rollback jobs, a GitHub Actions snippet for a `revert` label or a
  `git revert` commit message pattern, and a generic incident-tool webhook
  shape (for PagerDuty/Opsgenie-style outbound webhooks) — reusing that file's
  existing snippet-builder functions rather than writing new ones.

**Soft guidance, not a hard requirement**: most orgs reading this panel will
have *no* existing revert/incident tracking at all — this isn't "connect
your existing system," it's "here's a new habit worth starting," and the
copy should say so plainly rather than assume a mature practice exists.
Concretely:
- An inline help callout: *"Don't have incident tracking yet? Start with
  the easiest signal: mark a PR as reverted whenever you `git revert` it or
  roll back a deploy because of it. Even a few marks a month are enough for
  the risk model to start learning your repo's real risk patterns — it
  doesn't need to be exhaustive."*
- The **manual "Mark as reverted/incident" button** in the review detail
  page (already in scope from §1) is positioned as the primary, always-available
  path — not a fallback for when the webhook isn't wired up. The webhook is
  the "once you're ready to automate this" upgrade, not a prerequisite for
  the feature doing anything useful.
- **Permissions default to open, with a lockdown switch**: by default, any
  org member can click "Mark as reverted/incident" (reporting an incident
  is exactly the kind of thing you want zero friction for, and restricting
  it by default would undercut "start the habit easily"). A toggle in the
  Production Feedback sub-tab — `restrict_outcome_reporting_to_admins`
  (§1) — lets an org tighten this to owner/admin-only once they're worried
  about noisy or mistaken reports skewing the model; the webhook endpoint
  is always authenticated by its own secret regardless of this setting, so
  the toggle only affects the in-app button.
- A small "last 30 days" counter on the setup panel — "3 outcomes recorded
  (2 manual, 1 webhook)" — so an org can see at a glance whether this
  signal is flowing at all, since a silently-empty `review_outcomes` table
  is the single biggest way this whole feedback loop quietly does nothing.
- Explicitly do *not* promise automatic revert detection in this UI copy —
  the non-goal from earlier in this plan — so expectations match what's
  actually built (manual + webhook, not "we'll figure it out for you").

### 11. Metrics dashboard (wait time, revert rate, incident rate)

New read-only panel (either its own page or a card on the existing analytics
page) computing the paper's three headline metrics from data this plan
already creates:
- **Wait time**: `policy_result published_at - review started_at` (or PR-open
  time), from `review_policy_log` timestamps — no new instrumentation needed.
- **Revert rate / incident rate**: `count(review_outcomes) / count(auto-approved
  reviews)` over a trailing window, compared against the same ratio for
  human-reviewed PRs in the same repo — directly the paper's "1/3 revert
  rate" style comparison, but will read near-zero/meaningless until
  `review_outcomes` has real data (expected — the ingestion point existing is
  what matters now, not the chart being populated day one).
- **Force-approval audit log**: a simple table view over
  `review_policy_overrides` (§1/§7) — who, when, which review, what reason,
  and whether that review later got a `review_outcomes` row — the most
  direct "is this policy too strict" signal available, since every force
  approval is a human explicitly disagreeing with the engine.

## Delivery order

1. **Schema + eligibility + policy CRUD** (reuse `ci_rulesets_handler.go`
   patterns): `users.review_level`, `auto_approve_policies`,
   `review_policy_log`, `review_outcomes`, `review_policy_overrides`,
   `risk_feature_snapshots`, `risk_model_versions` migrations;
   `internal/policyengine` package; new API handler + routes (including
   `POST /api/v1/reviews/:id/force-approve`). No UI yet — policy rows
   settable via SQL/API for internal testing.
2. **`internal/riskscore`**: feature extraction, the `goml`-based
   `linear.Logistic` scorer (online + batch modes), `risk_model_versions`/
   `risk_feature_snapshots`/`risk_factor_overrides` migrations, the
   seed/cold-start model (batch-fit once against weak labels, since
   `review_outcomes` starts empty), the online-update hook fired from
   `review_outcomes` inserts, and the daily River batch-refit job + its
   on-demand API trigger.
3. **Rewire `pr_policy_worker.go`** onto the real engine (eligibility +
   reused jq-ruleset gates + the trained risk scorer) — ships a materially
   better auto-approve than the current placeholder even before the LLM
   classification stage lands.
4. **LLM semantic classification stage** (§5) — additive gate, doesn't block
   shipping step 3 first; extends the feature vector for future retrains.
5. **Review detail "Auto Approve" tab** (§7) + **Settings UI** (§8):
   policy editor sub-tabs and simulation panel.
6. **Repository-level overrides** (§9): `RepoAutoApproveSection.tsx` and
   the repo-settings `mode` picker, reusing §8's sub-tab components.
7. **Outcome ingestion UX** (§10): manual mark-as-reverted action, the
   webhook setup/test/guidance panel, and excluding `source = 'test'` rows
   from training queries.
8. **Metrics dashboard** (§11).

## Verification

- Unit tests for `internal/riskscore` (feature extraction + scorer math) and
  `internal/policyengine` (each stage's pass/fail logic), following existing
  test patterns in `internal/api/ci_gate_script_e2e_test.go`.
- End-to-end: trigger a review via the existing PR-link flow against a real
  test PR, confirm `review_policy_log` captures all stages and the posted
  GitHub/GitLab comment renders the funnel correctly for both an approve and
  a blocked case.
- Settings UI: start dev server, exercise the policy editor and simulation
  panel against real historical reviews in a test org, verify `npx tsc
  --noEmit` passes (no full build without separate approval, per standing
  project rule).
