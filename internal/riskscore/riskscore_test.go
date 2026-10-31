package riskscore

import (
	"math"
	"math/rand"
	"testing"
)

func TestPriorLength(t *testing.T) {
	if got := len(Prior()); got != FeatureCount()+1 {
		t.Fatalf("Prior length = %d, want %d", got, FeatureCount()+1)
	}
}

func TestScoreInRange(t *testing.T) {
	prior := Prior()
	x := make([]float64, FeatureCount())
	for i := range x {
		x[i] = 1
	}
	for _, overrides := range []map[string]float64{nil, {"blast_radius": 3, "security_findings": 0.5}} {
		s := Score(prior, x, overrides)
		if s < 0 || s > 1 {
			t.Fatalf("score %v out of [0,1]", s)
		}
	}
}

func TestScoreOverrideEffect(t *testing.T) {
	prior := Prior()
	x := make([]float64, FeatureCount())
	// Realistic log1p'd-scale values, not raw counts (features are
	// log1p-compressed by ExtractFeatures before Score ever sees them -- a
	// log1p'd br_sum realistically tops out around 8-9, so a raw value like
	// 50 here would already be past where a real feature vector could land,
	// and would clamp both branches to the same z ceiling, hiding the
	// override's effect entirely -- exactly what this test is supposed to
	// catch).
	x[0] = 2.0              // br_sum, log1p(~6.4) -- a sizeable but plausible blast radius
	x[4] = math.Log1p(1)    // critical_count: one critical finding

	base := Score(prior, x, nil)
	boosted := Score(prior, x, map[string]float64{"blast_radius": 10})
	if boosted <= base {
		t.Fatalf("boosting blast_radius should raise risk score: base %v, boosted %v", base, boosted)
	}
}

func TestBatchFitEmptyReturnsPrior(t *testing.T) {
	prior := Prior()
	got := BatchFit(nil, nil, prior, 0, 100)
	if len(got) != len(prior) {
		t.Fatalf("length mismatch")
	}
	for i := range got {
		if math.Abs(got[i]-prior[i]) > 1e-9 {
			t.Fatalf("BatchFit(empty) drifted from prior at %d: %v vs %v", i, got[i], prior[i])
		}
	}
}

func TestBatchFitWithData(t *testing.T) {
	prior := Prior()
	// A dataset strongly predictive of risk: critical_count high -> risky.
	X := [][]float64{}
	Y := []float64{}
	for i := 0; i < 40; i++ {
		x := make([]float64, FeatureCount())
		x[4] = float64(i % 5) // critical_count varies 0..4
		X = append(X, x)
		Y = append(Y, []float64{0, 0, 1, 1, 1}[i%5])
	}
	fitted := BatchFit(X, Y, prior, len(X), 200)
	if len(fitted) != len(prior) {
		t.Fatalf("length mismatch")
	}
	// A high-critical example should score above a low-critical one.
	low := make([]float64, FeatureCount())
	high := make([]float64, FeatureCount())
	high[4] = 4
	if Score(fitted, low, nil) >= Score(fitted, high, nil) {
		t.Fatalf("high-critical example should score higher")
	}
}

func TestOnlineStepShrinksTowardPrior(t *testing.T) {
	prior := Prior()
	w := cloneWeights(prior)
	w[5] = prior[5] + 5 // perturb critical_count weight far from prior
	x := make([]float64, FeatureCount())
	next := OnlineStep(w, x, 0, prior, 0, 0.1)
	if math.Abs(next[5]-prior[5]) >= math.Abs(w[5]-prior[5]) {
		t.Fatalf("online step should shrink toward prior: before %v -> %v, prior %v", w[5], next[5], prior[5])
	}
}

func TestAnchorStrength(t *testing.T) {
	if AnchorStrength(0) != 1.0 {
		t.Fatalf("AnchorStrength(0) = %v, want 1.0", AnchorStrength(0))
	}
	if AnchorStrength(1000) >= 0.01 {
		t.Fatalf("AnchorStrength(1000) should be negligible, got %v", AnchorStrength(1000))
	}
}

func TestPercentileCutoff(t *testing.T) {
	if got := PercentileCutoff(nil, 20); got != defaultCutoff {
		t.Fatalf("empty distribution cutoff = %v, want %v", got, defaultCutoff)
	}
	scores := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	if got := PercentileCutoff(scores, 20); math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("P20 of 0.1..1.0 = %v, want 0.2", got)
	}
	if got := PercentileCutoff(scores, 50); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("P50 = %v, want 0.5", got)
	}
}

func TestPercentileOf(t *testing.T) {
	scores := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	if got := PercentileOf(0.5, scores); got != 50 {
		t.Fatalf("PercentileOf(0.5) = %d, want 50", got)
	}
	if got := PercentileOf(1.0, scores); got != 100 {
		t.Fatalf("PercentileOf(1.0) = %d, want 100", got)
	}
}

// --- categoryOverride: explicit override vs default, and the two indices
// (bias, empty-category) that must never be scaled by either. ---

func TestCategoryOverrideExplicitBeatsDefault(t *testing.T) {
	// weightIndex 1 -> FeatureSpecs[0] = br_sum, category "blast_radius".
	if got := categoryOverride(map[string]float64{"blast_radius": 5}, 1); got != 5 {
		t.Fatalf("explicit override should win over the default multiplier: got %v, want 5", got)
	}
}

func TestCategoryOverrideFallsBackToDefaultWhenUnset(t *testing.T) {
	// weightIndex 7 -> FeatureSpecs[6] = security_count, category "security".
	want := DefaultCategoryMultiplier("security")
	if got := categoryOverride(nil, 7); got != want {
		t.Fatalf("got %v, want default multiplier %v", got, want)
	}
	if got := categoryOverride(map[string]float64{}, 7); got != want {
		t.Fatalf("empty (not nil) overrides map should also fall back to default: got %v, want %v", got, want)
	}
}

func TestCategoryOverrideBiasNeverScaled(t *testing.T) {
	if got := categoryOverride(map[string]float64{"blast_radius": 99}, 0); got != 1 {
		t.Fatalf("the bias term (weightIndex 0) must never be scaled by any override, got %v", got)
	}
}

func TestCategoryOverrideEmptyCategoryNeverScaled(t *testing.T) {
	// weightIndex 6 -> FeatureSpecs[5] = warning_count, category "" (no override applies).
	if got := categoryOverride(map[string]float64{"security": 99}, 6); got != 1 {
		t.Fatalf("a feature with no category must never be scaled, got %v", got)
	}
}

// --- Formal-ish property tests over the seed prior: every weight the prior
// ships with is non-negative, so the common-sense model must never consider
// "more of any single risk signal" to be safer. A random, fixed-seed sweep
// catches a future hand-edit to Prior() that introduces a wrong-signed
// weight, which a handful of hand-picked example points would not. ---

func TestPriorAllFeatureWeightsNonNegative(t *testing.T) {
	prior := Prior()
	for i := 1; i < len(prior); i++ {
		if prior[i] < 0 {
			t.Fatalf("prior weight %d (%s) is negative (%v): this would make the seed model non-monotonic in that feature",
				i, FeatureSpecs[i-1].Name, prior[i])
		}
	}
}

func TestPriorScoreMonotonicInEachFeature(t *testing.T) {
	prior := Prior()
	rng := rand.New(rand.NewSource(42))
	// Sampled up to 60, not 10: a log1p'd feature realistically tops out
	// around 8-9 even for an extreme real-world value (log1p(4840) ≈ 8.5,
	// the actual br_sum behind the production bug this suite guards
	// against), but this test exercises Score as a pure function over any
	// input, including well past that -- including past the zClampAbs
	// clamp boundary (12) -- so a future unbounded feature added without
	// log1p compression, or a future clamp/weight regression, gets caught
	// here instead of only showing up in production.
	const sampleMax = 60.0
	for trial := 0; trial < 20; trial++ {
		base := make([]float64, FeatureCount())
		for i := range base {
			base[i] = rng.Float64() * sampleMax
		}
		baseScore := Score(prior, base, nil)
		for i := range base {
			bumped := append([]float64{}, base...)
			bumped[i] += rng.Float64()*sampleMax + 0.1 // strictly positive delta
			bumpedScore := Score(prior, bumped, nil)
			if bumpedScore < baseScore-1e-12 {
				t.Fatalf("trial %d, feature %d (%s): increasing this feature (holding others fixed) decreased the seed model's score (%v -> %v)",
					trial, i, FeatureSpecs[i].Name, baseScore, bumpedScore)
			}
		}
	}
}

func TestAnchorStrengthMonotonicallyDecreasing(t *testing.T) {
	prev := AnchorStrength(0)
	if prev != 1.0 {
		t.Fatalf("AnchorStrength(0) = %v, want 1.0", prev)
	}
	for _, n := range []int{1, 5, 10, 50, 100, 500, 1000, 5000} {
		cur := AnchorStrength(n)
		if cur >= prev {
			t.Fatalf("AnchorStrength(%d) = %v should be strictly less than the previous sample count's %v", n, cur, prev)
		}
		prev = cur
	}
}

// TestOnlineStepRegularizationWeakensAsSampleCountGrows isolates the
// prior-anchor term from the gradient term (by choosing y so err=0: y is set
// to exactly what the model already predicts, so the only thing OnlineStep
// does is pull w[5] back toward the prior) and sweeps sampleCount, confirming
// that pull shrinks as an org accumulates more labeled outcomes -- ties
// model.go's documented shrink = AnchorLambda/(1+sampleCount) formula to an
// actual behavioral assertion, not just a comment.
func TestOnlineStepRegularizationWeakensAsSampleCountGrows(t *testing.T) {
	prior := Prior()
	w := cloneWeights(prior)
	w[5] = prior[5] + 5 // perturb one weight far from the prior
	x := make([]float64, FeatureCount())
	y := sigmoid(w[0]) // makes err = y - scoreRaw(w, x) == 0

	prevMove := math.Inf(1)
	for _, n := range []int{0, 10, 100, 1000} {
		next := OnlineStep(w, x, y, prior, n, 0.1)
		move := math.Abs(next[5] - w[5])
		if move >= prevMove {
			t.Fatalf("sampleCount %d: regularization pull %v should be smaller than at a lower sample count (%v)", n, move, prevMove)
		}
		prevMove = move
	}
}

func TestRankAUC(t *testing.T) {
	scores := []float64{0.9, 0.8, 0.7, 0.3, 0.2, 0.1}
	labels := []float64{1, 1, 1, 0, 0, 0}
	if got := rankAUC(scores, labels); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("perfect separation AUC = %v, want 1.0", got)
	}
}
