// Package riskscore implements the trained risk model behind the auto-approve
// policy engine: feature extraction, an online-learning logistic regression
// (goml's linear.Logistic for batch gradient ascent, plus a custom
// prior-anchored regularizer and category-level weight overrides), and the
// percentile calibration that turns a per-org risk_threshold_percentile into a
// concrete cutoff. See docs/risk-policy-engine.md §3 for the design.
package riskscore

import (
	"context"
	"database/sql"
	"math"
	"strings"

	"github.com/livereview/internal/canonicaldoc"
	"github.com/livereview/pkg/models"
)

// FeatureSpec describes one model feature. The slice is append-only: never
// reorder or remove entries, because old model versions keep scoring with the
// same indices (a feature a model predates simply reads as 0).
type FeatureSpec struct {
	Name     string
	Category string // override category; "" means no category-level override applies
}

// FeatureSpecs is the canonical feature vector. goml adds its own bias term, so
// the feature vector itself carries no intercept; weights are stored as
// [bias, w0, w1, ...] where w_i corresponds to FeatureSpecs[i].
var FeatureSpecs = []FeatureSpec{
	{"br_sum", "blast_radius"},
	{"br_max", "blast_radius"},
	{"br_weighted_avg", "blast_radius"},
	{"br_high_count", "blast_radius"},
	{"critical_count", "critical_findings"},
	{"warning_count", ""},
	{"security_count", "security"},
	{"files_changed", "diff_size"},
	{"lines_added", "diff_size"},
	{"llm_class_risk", ""},
	// Per-taxonomy-category finding counts (internal/prompts/templates.go's
	// TaxonomyClassificationRules is the canonical 10-category list; "security"
	// above already covers that category, these add the other 9). Appended
	// rather than inserted so old model_versions rows keep scoring correctly
	// (Score/scoreRaw/OnlineStep all stop at len(w), reading a historical,
	// shorter weight vector as if these simply weren't there yet).
	{"reliability_count", "reliability"},
	{"correctness_count", "correctness"},
	{"performance_count", "performance"},
	{"scalability_count", "scalability"},
	{"compliance_governance_count", "compliance_governance"},
	{"maintainability_count", "maintainability"},
	{"architecture_count", "architecture"},
	{"developer_experience_count", "developer_experience"},
	{"cost_count", "cost"},
}

// OverrideCategories returns every distinct non-empty category string used by
// FeatureSpecs — i.e. every category a Settings UI slider can target — in
// FeatureSpecs order with duplicates removed. Single source of truth so the
// API/UI never drifts from what scoring actually applies overrides to.
func OverrideCategories() []string {
	seen := make(map[string]bool, len(FeatureSpecs))
	out := make([]string, 0, len(FeatureSpecs))
	for _, f := range FeatureSpecs {
		if f.Category == "" || seen[f.Category] {
			continue
		}
		seen[f.Category] = true
		out = append(out, f.Category)
	}
	return out
}

// FeatureCount returns the number of model features (excluding goml's bias).
func FeatureCount() int { return len(FeatureSpecs) }

// BlastStats is the diff-level blast-radius aggregate the feature extractor
// consumes, loaded from blast_radius_hunks. Zero values mean "no blast radius
// data" (CLI review, disabled cache, or the scoring job hasn't finished yet);
// the policy engine treats that as zeros rather than risk.
type BlastStats struct {
	Sum         float64
	Max         float64
	WeightedAvg float64
	HighCount   int
	FileCount   int
	LinesAdded  int
}

// LoadBlastStats reads the blast-radius aggregates for a review from
// blast_radius_hunks, scoped to orgID.
func LoadBlastStats(ctx context.Context, db *sql.DB, orgID, reviewID int64) (BlastStats, error) {
	var br BlastStats
	err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(combined), 0),
			COALESCE(MAX(combined), 0),
			COALESCE(SUM(combined * new_lines) / NULLIF(SUM(new_lines), 0), 0),
			COALESCE(COUNT(*) FILTER (WHERE tier = 'blast-radius-high'), 0),
			COALESCE(COUNT(DISTINCT file_path), 0),
			COALESCE(SUM(new_lines), 0)
		FROM blast_radius_hunks
		WHERE review_id = $1 AND org_id = $2`, reviewID, orgID).
		Scan(&br.Sum, &br.Max, &br.WeightedAvg, &br.HighCount, &br.FileCount, &br.LinesAdded)
	if err != nil {
		return br, err
	}
	return br, nil
}

// ExtractFeatures builds the length-FeatureCount() feature vector from blast
// radius aggregates, finding-severity counts from the canonical review doc, and
// (when present) the diff-level semantic classification. Feature values are
// lightly bounded/compressed (log1p on counts) so no single feature dominates
// the dot product.
// log1p'd features: every count/sum that is structurally unbounded (grows with
// diff size or finding volume, not capped to a known range) is compressed with
// math.Log1p before it reaches the dot product. br_sum is the feature that
// actually saturated production (a raw SUM(combined) across every blast-radius
// hunk in the diff, real values 1700-4840, against a weight calibrated for an
// assumed 0-100 range) - but every count feature here shares the identical
// structural flaw once a diff/review has enough findings (production data
// already shows warning_count up to 37, critical_count up to 12, both alone
// enough to saturate at their un-compressed weights). br_max, br_weighted_avg
// stay raw because they're genuinely bounded (a MAX or a weighted AVERAGE of a
// per-hunk score can't runaway the way a SUM or a COUNT can); llm_class_risk
// stays raw because it's already a bounded ordinal (0/0.5/1).
func ExtractFeatures(br BlastStats, doc *canonicaldoc.ReviewDoc) []float64 {
	f := make([]float64, FeatureCount())
	f[0] = math.Log1p(br.Sum)
	f[1] = br.Max
	f[2] = br.WeightedAvg
	f[3] = math.Log1p(float64(br.HighCount))
	if doc != nil {
		f[4] = math.Log1p(float64(doc.Counts.BySeverity["critical"]))
		f[5] = math.Log1p(float64(doc.Counts.BySeverity["warning"]))
		f[6] = math.Log1p(float64(doc.Counts.ByCategory["security"]))
	}
	f[7] = math.Log1p(float64(br.FileCount))
	f[8] = math.Log1p(float64(br.LinesAdded))
	if doc != nil {
		f[9] = classRisk(doc.DiffClassification)
		// doc.Counts.ByCategory keys are the taxonomy labels lower-cased as-is
		// (canonicaldoc.Build), so "Compliance & Governance"/"Developer
		// Experience" keep their space/"&" rather than becoming snake_case.
		f[10] = math.Log1p(float64(doc.Counts.ByCategory["reliability"]))
		f[11] = math.Log1p(float64(doc.Counts.ByCategory["correctness"]))
		f[12] = math.Log1p(float64(doc.Counts.ByCategory["performance"]))
		f[13] = math.Log1p(float64(doc.Counts.ByCategory["scalability"]))
		f[14] = math.Log1p(float64(doc.Counts.ByCategory["compliance & governance"]))
		f[15] = math.Log1p(float64(doc.Counts.ByCategory["maintainability"]))
		f[16] = math.Log1p(float64(doc.Counts.ByCategory["architecture"]))
		f[17] = math.Log1p(float64(doc.Counts.ByCategory["developer experience"]))
		f[18] = math.Log1p(float64(doc.Counts.ByCategory["cost"]))
	}
	return f
}

// classRisk maps the diff-level semantic classification to a coarse risk
// ordinal (0 safe, 0.5 mixed, 1 risky). Absent/unknown classification reads 0.
func classRisk(dc *models.DiffClassification) float64 {
	if dc == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(dc.Class)) {
	case "logic-change", "security-sensitive":
		return 1
	case "config-change", "dependency-change", "data-migration":
		return 0.5
	default: // refactor-only, dead-code-removal, docs-only, test-only, unknown
		return 0
	}
}
