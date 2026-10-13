-- migrate:up

-- Risk-based auto-approve policy engine. See docs/risk-policy-engine.md for the
-- full design. Replaces the placeholder "no critical findings" policy in
-- internal/jobqueue/pr_policy_worker.go with a layered funnel
-- (eligibility -> static jq gates -> trained risk score -> LLM classification -> decision).

-- 1. The seniority dial that gates eligibility: a plain integer owners bump per
--    user (no roles/tiers table). Default 1 (lowest).
ALTER TABLE users ADD COLUMN review_level INTEGER NOT NULL DEFAULT 1;

-- 2. Pull-request author email, captured opportunistically during PR sync so the
--    identity map (below) can match an author to a LiveReview user by email as
--    well as by provider username. Often absent on GitHub (commit author email
--    may be used); nullable.
ALTER TABLE pull_requests ADD COLUMN author_email TEXT;

-- 3. User identities: the "alternative emails / handles" each org member can hold,
--    used to map a PR author (provider username or email) back to a LiveReview
--    user so eligibility uses their real review_level instead of the default.
--    Org-scoped; a member manages their own, owner/admin manage others'.
CREATE TABLE user_identities (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL REFERENCES orgs(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    kind       TEXT NOT NULL CHECK (kind IN ('email', 'username')),
    value      TEXT NOT NULL,             -- normalized (lowercased) email or handle
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, kind, value)
);

CREATE INDEX idx_user_identities_org_user ON user_identities (org_id, user_id);
CREATE INDEX idx_user_identities_lookup ON user_identities (org_id, kind, value);

-- 4. Per-org / per-repo auto-approve policy. repository_id NULL = org default.
--    'inherit' is only valid on repo-scoped rows (explicit "use the org default").
CREATE TABLE auto_approve_policies (
    id                                BIGSERIAL PRIMARY KEY,
    org_id                            BIGINT NOT NULL REFERENCES orgs(id),
    repository_id                     BIGINT REFERENCES repositories(id),
    mode                              TEXT NOT NULL DEFAULT 'custom'
        CHECK (mode IN ('inherit', 'custom', 'always_approve', 'never_approve')),
    min_author_level                  INTEGER NOT NULL DEFAULT 1,
    eligibility_whitelist_user_ids    BIGINT[] NOT NULL DEFAULT '{}',
    risk_threshold_percentile         INTEGER NOT NULL DEFAULT 20,
    require_llm_confidence            INTEGER NOT NULL DEFAULT 8,
    blocking_ruleset_ids              BIGINT[] NOT NULL DEFAULT '{}',
    landing_delay_seconds             INTEGER NOT NULL DEFAULT 0,
    daily_approve_cap                 INTEGER,
    restrict_outcome_reporting_to_admins BOOLEAN NOT NULL DEFAULT false,
    force_approve_min_level           INTEGER NOT NULL DEFAULT 3,
    created_by                        BIGINT,
    created_at                        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_auto_approve_policies_org_default ON auto_approve_policies (org_id) WHERE repository_id IS NULL;
CREATE UNIQUE INDEX idx_auto_approve_policies_org_repo ON auto_approve_policies (org_id, repository_id) WHERE repository_id IS NOT NULL;
CREATE INDEX idx_auto_approve_policies_org ON auto_approve_policies (org_id);

-- 5. Append-only policy evaluation trace: one row per stage per review, so the
--    UI can render the full funnel (eligibility -> static -> risk -> llm -> final),
--    not just the final approve/reject.
CREATE TABLE review_policy_log (
    id         BIGSERIAL PRIMARY KEY,
    review_id  BIGINT NOT NULL REFERENCES reviews(id),
    org_id     BIGINT NOT NULL REFERENCES orgs(id),
    stage      TEXT NOT NULL,
    verdict    TEXT NOT NULL,
    detail     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_policy_log_review ON review_policy_log (review_id);
CREATE INDEX idx_review_policy_log_org ON review_policy_log (org_id, created_at DESC);

-- 6. Production-truth outcomes (revert / incident / confirmed-safe) that feed the
--    risk model's feedback loop. reason is mandatory. 'test' source is excluded
--    from training.
CREATE TABLE review_outcomes (
    id                  BIGSERIAL PRIMARY KEY,
    review_id           BIGINT NOT NULL REFERENCES reviews(id),
    org_id              BIGINT NOT NULL REFERENCES orgs(id),
    reported_by_user_id BIGINT,
    outcome_type        TEXT NOT NULL CHECK (outcome_type IN ('revert', 'incident', 'confirmed_safe')),
    source              TEXT NOT NULL CHECK (source IN ('webhook', 'manual', 'ci', 'test')),
    reason              TEXT NOT NULL,
    detail              JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_outcomes_review ON review_outcomes (review_id);
CREATE INDEX idx_review_outcomes_org_created ON review_outcomes (org_id, created_at DESC);

-- 7. Human force-approve/override of a specific review's verdict, fully audited.
CREATE TABLE review_policy_overrides (
    id               BIGSERIAL PRIMARY KEY,
    review_id        BIGINT NOT NULL REFERENCES reviews(id),
    org_id           BIGINT NOT NULL REFERENCES orgs(id),
    actor_user_id    BIGINT NOT NULL REFERENCES users(id),
    original_verdict TEXT NOT NULL,
    forced_verdict   TEXT NOT NULL,
    reason           TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_policy_overrides_review ON review_policy_overrides (review_id);
CREATE INDEX idx_review_policy_overrides_org ON review_policy_overrides (org_id, created_at DESC);

-- 8. Feature snapshots written at every policy evaluation: the training set once
--    review_outcomes rows attach to the same review_ids. Recomputed-after-the-fact
--    features would drift, so they're snapshotted at evaluation time.
CREATE TABLE risk_feature_snapshots (
    id            BIGSERIAL PRIMARY KEY,
    review_id     BIGINT NOT NULL REFERENCES reviews(id),
    org_id        BIGINT NOT NULL REFERENCES orgs(id),
    model_version INTEGER NOT NULL,
    features      JSONB NOT NULL,
    score         DOUBLE PRECISION NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_risk_feature_snapshots_review ON risk_feature_snapshots (review_id);
CREATE INDEX idx_risk_feature_snapshots_org ON risk_feature_snapshots (org_id, created_at DESC);

-- 9. The trained risk model. Weights are a few dozen floats, so the row IS the
--    model (no blob store). org_id NULL = the shared seed/default every org
--    starts from. Append-only: any version can be reactivated as an instant rollback.
CREATE TABLE risk_model_versions (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT REFERENCES orgs(id),
    version               INTEGER NOT NULL,
    weights_json          JSONB NOT NULL,
    trained_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    training_sample_count INTEGER NOT NULL DEFAULT 0,
    holdout_auc           DOUBLE PRECISION,
    update_kind           TEXT NOT NULL CHECK (update_kind IN ('seed', 'online_update', 'batch_refit')),
    is_active             BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_risk_model_versions_org_active ON risk_model_versions (org_id) WHERE is_active;

-- 10. Coarse, category-level admin weight overrides (blast_radius / security_findings /
--     critical_findings / diff_size): an instant multiplier on live scoring,
--     decoupled from the nightly retrain cadence.
CREATE TABLE risk_factor_overrides (
    id               BIGSERIAL PRIMARY KEY,
    org_id           BIGINT NOT NULL REFERENCES orgs(id),
    feature_category TEXT NOT NULL,
    multiplier       DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, feature_category)
);

-- migrate:down

DROP TABLE IF EXISTS risk_factor_overrides;
DROP TABLE IF EXISTS risk_model_versions;
DROP TABLE IF EXISTS risk_feature_snapshots;
DROP TABLE IF EXISTS review_policy_overrides;
DROP TABLE IF EXISTS review_outcomes;
DROP TABLE IF EXISTS review_policy_log;
DROP TABLE IF EXISTS auto_approve_policies;
DROP TABLE IF EXISTS user_identities;
ALTER TABLE pull_requests DROP COLUMN IF EXISTS author_email;
ALTER TABLE users DROP COLUMN IF EXISTS review_level;
