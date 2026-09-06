-- migrate:up

-- CI/CD gate rulesets: a named, org-scoped jq boolean expression evaluated
-- against a canonical JSON document of one review's findings (severity,
-- confidence, category, subcategory, type -- the same taxonomy fields
-- storage/reviews/taxonomy_report_store.go already reads from
-- reviews.metadata->review_result->comments). A ruleset gets a stable ID;
-- CI calls GET /api/v1/ci-rulesets/:id/evaluate?review_id=... and blocks or
-- allows based on the HTTP status code alone, no client-side jq or JSON
-- parsing required. See internal/api/ci_rulesets_handler.go.

CREATE TABLE ci_rulesets (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES orgs(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    jq_expr     TEXT NOT NULL,
    created_by  BIGINT REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ci_rulesets_org_id ON ci_rulesets (org_id);

COMMENT ON TABLE ci_rulesets IS 'Org-scoped CI/CD gate rules: jq_expr is run against a canonical per-review findings document; a truthy result blocks the CI job (see evaluateCIRuleset).';
COMMENT ON COLUMN ci_rulesets.jq_expr IS 'A jq expression evaluated against {review_id, org_id, repository, provider, status, findings[], counts{by_severity,by_category,total}}. Truthy result => block.';

-- migrate:down

DROP TABLE IF EXISTS ci_rulesets;
