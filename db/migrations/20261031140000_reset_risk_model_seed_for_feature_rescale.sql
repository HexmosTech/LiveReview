-- migrate:up

-- riskscore.Prior() changed: every structurally-unbounded count/sum feature
-- (br_sum, br_high_count, critical_count, warning_count, security_count, and
-- the 9 taxonomy counts) is now log1p-compressed before scoring, with weights
-- retuned to match. The old br_sum weight assumed a 0-100 range, but br_sum is
-- actually a raw, unbounded SUM(combined) across every blast-radius hunk in a
-- diff (real production values ran 1700-4840), which saturated the sigmoid
-- past the point float64 can represent 1+exp(-z) as anything but exactly 1.0
-- - confirmed in production, where an org's entire risk-score history was
-- pinned to exactly 1.0 for every review. See internal/riskscore/model.go's
-- Prior() doc comment and internal/riskscore/saturation_test.go for the full
-- investigation and regression coverage.
--
-- EnsureSeedModel (internal/riskscore/store.go) only inserts the shared seed
-- row (org_id IS NULL, update_kind = 'seed') if one doesn't already exist - it
-- never updates one in place. At the time this migration was written, that
-- table held exactly one row in production, and no org had ever trained a
-- custom model (training_sample_count was 0 everywhere) - so there is no
-- fitted-model data to preserve or migrate. Deleting the stale row lets
-- EnsureSeedModel's existing idempotent bootstrap logic reinsert it from the
-- corrected Prior() the next time any review is scored, with no new code path
-- needed.
DELETE FROM risk_model_versions WHERE org_id IS NULL AND update_kind = 'seed';

-- migrate:down

-- Data-only reset; the prior seed weights are not reconstructed here (see git
-- history for the exact values this migration replaced). EnsureSeedModel will
-- reinsert a seed row from whatever Prior() the running code defines, same as
-- after any fresh deploy - there is no way to "undo" this short of also
-- reverting the Prior()/ExtractFeatures code changes.
DELETE FROM risk_model_versions WHERE org_id IS NULL AND update_kind = 'seed';
