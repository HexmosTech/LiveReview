package riskscore

import (
	"encoding/json"
	"io"
	"math"

	"github.com/cdipaolo/goml/base"
	"github.com/cdipaolo/goml/linear"
)

// Weights is one model's weight vector, laid out exactly as goml's
// Logistic.Parameters: index 0 is the bias, index i+1 is the weight for
// FeatureSpecs[i]. A row in risk_model_versions.weights_json is a []float64 of
// length FeatureCount()+1.
type Weights []float64

// AnchorLambda is the prior-anchor strength constant. The effective anchor on a
// batch refit is AnchorStrength(n) = lambda / (lambda + n), so with no labeled
// outcomes the fitted model stays at the common-sense prior, and by a few
// hundred outcomes the prior's pull is negligible.
const AnchorLambda = 1.0

// Prior returns the hand-set common-sense seed weights (docs/risk-policy-engine.md
// §3 "Cold start"). Blast-radius magnitude and critical-finding count dominate,
// security findings are weighted up, info-level findings and small diffs are
// weighted near zero. This is the starting point every org's model begins from
// and is anchored back to.
func Prior() Weights {
	w := make(Weights, FeatureCount()+1)
	w[0] = -0.5  // bias: slight default toward "safe"
	w[1] = 0.02  // br_sum (0-100)
	w[2] = 0.01  // br_max
	w[3] = 0.01  // br_weighted_avg
	w[4] = 0.20  // br_high_count
	w[5] = 0.90  // critical_count  (dominates)
	w[6] = 0.18  // warning_count
	w[7] = 0.55  // security_count  (weighted up)
	w[8] = 0.10  // files_changed (log1p)
	w[9] = 0.05  // lines_added   (log1p)
	w[10] = 0.30 // llm_class_risk
	// Per-taxonomy-category counts: correctness/reliability findings matter
	// almost as much as security, performance next, the rest start modest —
	// the same ladder the UI's default category multipliers (below) express.
	w[11] = 0.30 // reliability_count
	w[12] = 0.30 // correctness_count
	w[13] = 0.20 // performance_count
	w[14] = 0.12 // scalability_count
	w[15] = 0.12 // compliance_governance_count
	w[16] = 0.10 // maintainability_count
	w[17] = 0.10 // architecture_count
	w[18] = 0.10 // developer_experience_count
	w[19] = 0.10 // cost_count
	return w
}

// AnchorStrength returns the blend factor toward the prior for n labeled
// outcomes: 1.0 with no data (fully prior), decaying as n grows.
func AnchorStrength(n int) float64 {
	return AnchorLambda / (AnchorLambda + float64(n))
}

func sigmoid(z float64) float64 {
	if z >= 0 {
		zExp := math.Exp(-z)
		return 1 / (1 + zExp)
	}
	zExp := math.Exp(z)
	return zExp / (1 + zExp)
}

// DefaultCategoryMultiplier is the starting weight-multiplier for a finding
// category before an org has set its own override: blast radius is weighted
// highest of all, since how far a change's effects reach is the strongest
// single risk signal the model has — it applies regardless of finding
// category and often catches what per-category counts miss. Security is
// weighted up next since teams tend to take it most seriously, correctness
// and reliability next, performance next, and everything else neutral (1x).
// An explicit risk_factor_overrides row always takes priority over this.
func DefaultCategoryMultiplier(category string) float64 {
	switch category {
	case "blast_radius":
		return 2.0
	case "security":
		return 1.5
	case "correctness", "reliability":
		return 1.25
	case "performance":
		return 1.1
	default:
		return 1
	}
}

// categoryOverride returns the multiplier for a weight index (0 = bias, no
// category), falling back to DefaultCategoryMultiplier when no explicit
// override is set.
func categoryOverride(overrides map[string]float64, weightIndex int) float64 {
	if weightIndex == 0 {
		return 1
	}
	spec := FeatureSpecs[weightIndex-1]
	if spec.Category == "" {
		return 1
	}
	if overrides != nil {
		if m, ok := overrides[spec.Category]; ok && m > 0 {
			return m
		}
	}
	return DefaultCategoryMultiplier(spec.Category)
}

// Score computes sigmoid(bias + Σ effective_w_i * x_i), where
// effective_w_i = w_i * override_multiplier[category(i)]. A single dot product.
func Score(w Weights, x []float64, overrides map[string]float64) float64 {
	var z float64
	if len(w) > 0 {
		z = w[0] // bias has no category override
	}
	for i := 0; i < len(x) && i+1 < len(w); i++ {
		z += w[i+1] * categoryOverride(overrides, i+1) * x[i]
	}
	return sigmoid(z)
}

// scoreRaw is the raw dot product (no overrides), used during training and AUC.
func scoreRaw(w Weights, x []float64) float64 {
	var z float64
	if len(w) > 0 {
		z = w[0]
	}
	for i := 0; i < len(x) && i+1 < len(w); i++ {
		z += w[i+1] * x[i]
	}
	return sigmoid(z)
}

// BatchFit fits a fresh logistic model over (X, Y) using goml's batch gradient
// ascent (regularization disabled inside goml; the prior anchor is applied as a
// post-fit blend toward the common-sense prior). With no training data it
// returns the prior unchanged.
func BatchFit(X [][]float64, Y []float64, prior Weights, sampleCount int, maxIter int) Weights {
	if len(X) == 0 || len(X) != len(Y) {
		return cloneWeights(prior)
	}
	if maxIter <= 0 {
		maxIter = 200
	}

	model := linear.NewLogistic(base.BatchGA, 1e-3, 0, maxIter, X, Y, FeatureCount())
	model.Output = io.Discard
	if err := model.Learn(); err != nil {
		// goml failed (diverged / bad input): keep the prior rather than a broken model.
		return cloneWeights(prior)
	}

	fitted := Weights(model.Parameters)
	a := AnchorStrength(sampleCount)
	out := make(Weights, len(fitted))
	for i := range fitted {
		out[i] = (1-a)*fitted[i] + a*prior[i]
	}
	return out
}

// OnlineStep applies one stochastic gradient ascent step for a single labeled
// example (the real-time nudge fired when a review_outcomes row lands),
// shrinking toward the prior so a single noisy example can't swing the weights
// away from common sense. Returns a new weight vector (the old one is not
// mutated).
func OnlineStep(w Weights, x []float64, y float64, prior Weights, sampleCount int, alpha float64) Weights {
	if alpha <= 0 {
		alpha = 0.01
	}

	p := scoreRaw(w, x)
	err := y - p

	// Per-step anchor: strong when data is scarce, negligible once calibrated.
	shrink := AnchorLambda / (1 + float64(sampleCount))

	out := cloneWeights(w)
	// bias term (x_0 = 1)
	if len(out) > 0 {
		out[0] += alpha * (err*1 - shrink*(out[0]-prior[0]))
	}
	for i := 0; i < len(x) && i+1 < len(out); i++ {
		out[i+1] += alpha * (err*x[i] - shrink*(out[i+1]-prior[i+1]))
	}
	return out
}

func cloneWeights(w Weights) Weights {
	out := make(Weights, len(w))
	copy(out, w)
	return out
}

// MarshalWeights encodes a weight vector for risk_model_versions.weights_json.
func MarshalWeights(w Weights) ([]byte, error) {
	return json.Marshal([]float64(w))
}

// UnmarshalWeights decodes a weights_json value.
func UnmarshalWeights(raw []byte) (Weights, error) {
	var w Weights
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	return w, nil
}
