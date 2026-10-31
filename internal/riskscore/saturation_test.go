package riskscore

import (
	"math"
	"math/rand"
	"testing"
)

// prodFeatureVector reconstructs a feature vector the way the new
// ExtractFeatures would produce it, from the RAW values actually stored in
// production's risk_feature_snapshots.features column (pulled read-only from
// .env.prod's DATABASE_URL during the investigation that found this bug).
// Those rows were captured under the OLD ExtractFeatures, where only
// files/lines (args 7/8 here) were already log1p'd -- everything else was
// raw. This helper applies the log1p transform ExtractFeatures now applies,
// to the same raw numbers, so these are honest real-world regression
// fixtures rather than synthetic guesses.
func prodFeatureVector(brSum, brMax, brWeighted, brHigh, critical, warning, security, filesLog1p, linesLog1p, llmRisk, reliability, correctness, performance, scalability, complianceGov, maintainability, architecture, devExp, cost float64) []float64 {
	return []float64{
		math.Log1p(brSum), brMax, brWeighted, math.Log1p(brHigh),
		math.Log1p(critical), math.Log1p(warning), math.Log1p(security),
		filesLog1p, linesLog1p, llmRisk,
		math.Log1p(reliability), math.Log1p(correctness), math.Log1p(performance),
		math.Log1p(scalability), math.Log1p(complianceGov), math.Log1p(maintainability),
		math.Log1p(architecture), math.Log1p(devExp), math.Log1p(cost),
	}
}

// TestRealProductionScoresNoLongerPinned is the regression test that would
// have caught the original bug before it shipped: org 3's entire score
// history was exactly 1.0 for every one of its last 10 reviews, despite
// materially different underlying feature vectors (different finding
// counts, different blast-radius magnitudes) -- because br_sum (a raw,
// unbounded SUM(combined) across every blast-radius hunk, real values
// 1700-4840) fed a weight calibrated for an assumed 0-100 range, saturating
// the sigmoid past the point float64 can represent anything but exactly
// 1.0. Org 677 showed the same mechanism directly: score behaved normally
// when br_sum=0, pinned to 1.0 the instant br_sum crossed ~150.
func TestRealProductionScoresNoLongerPinned(t *testing.T) {
	prior := Prior()

	// Org 3's 10 most recent production reviews, newest first (review IDs
	// and raw values exactly as queried from risk_feature_snapshots).
	org3 := map[string][]float64{
		"13543": prodFeatureVector(4839.98, 82.22, 26.14934136243884, 2, 0, 7, 0, 4.624972813284271, 8.983690681332686, 1, 1, 2, 0, 0, 0, 1, 0, 3, 0),
		"13542": prodFeatureVector(4839.98, 82.22, 26.14934136243884, 2, 0, 0, 0, 4.624972813284271, 8.983690681332686, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		"13539": prodFeatureVector(3586.34, 82.27, 26.178802709802294, 2, 1, 6, 0, 4.477336814478207, 8.886547412512042, 1, 3, 1, 0, 0, 0, 1, 0, 2, 0),
		"13523": prodFeatureVector(1708.27, 96.42, 37.37333796457103, 2, 5, 25, 3, 3.6635616461296463, 7.965545573129992, 1, 12, 12, 1, 0, 1, 8, 2, 4, 1),
		"13528": prodFeatureVector(2365.97, 97.87, 33.26371175908222, 2, 1, 25, 4, 3.970291913552122, 8.339261982923576, 1, 6, 11, 0, 0, 0, 2, 1, 2, 0),
		"13527": prodFeatureVector(1902, 97.87, 36.93545110410095, 2, 2, 28, 6, 3.784189633918261, 8.061802274538348, 1, 8, 7, 1, 0, 0, 5, 1, 2, 0),
		"13526": prodFeatureVector(1775.62, 98.06, 37.01939345314506, 2, 1, 19, 4, 3.784189633918261, 8.044626279767337, 1, 4, 7, 0, 0, 0, 4, 0, 1, 0),
		"13522": prodFeatureVector(1824.93, 96.46, 38.080896277556924, 2, 9, 7, 2, 3.7376696182833684, 7.92588031673756, 1, 4, 5, 1, 0, 0, 8, 0, 0, 0),
		"13521": prodFeatureVector(1805.47, 96.46, 38.10692504570384, 2, 8, 24, 4, 3.6635616461296463, 7.914252278742441, 1, 8, 11, 3, 0, 0, 9, 4, 4, 0),
		"13484": prodFeatureVector(2417.72, 80.24, 37.482306049822064, 2, 7, 37, 12, 3.7376696182833684, 7.941295570906532, 1, 14, 15, 3, 1, 0, 8, 1, 1, 0),
	}

	scores := map[string]float64{}
	for id, x := range org3 {
		scores[id] = Score(prior, x, nil)
	}

	// None of these real, materially-different reviews should be bit-identical.
	seen := map[float64]string{}
	for id, s := range scores {
		if prev, ok := seen[s]; ok {
			t.Errorf("review %s and review %s scored bit-identical (%v) despite different features -- the exact artifact this test guards against", id, prev, s)
		}
		seen[s] = id
	}

	// None should be pinned at the literal ceiling the clamp exists to prevent.
	for id, s := range scores {
		if s >= 1.0 {
			t.Errorf("review %s score = %v, want strictly < 1.0", id, s)
		}
	}

	// 13542 and 13543 share identical blast-radius features (same diff) but
	// 13543 has real findings (7 warnings, 3 taxonomy findings) and 13542 has
	// none -- 13543 must score higher.
	if scores["13543"] <= scores["13542"] {
		t.Errorf("13543 (has findings) should score above 13542 (identical blast radius, zero findings): %v vs %v", scores["13543"], scores["13542"])
	}
}

// TestOrg677BeforeAfterBlastRadius replays the exact before/after pair found
// in production: two reviews with br_sum=0 score normally and distinctly;
// two reviews with real blast-radius data (br_sum 150-230, crossing the
// threshold that used to saturate the old model) must no longer be
// bit-identical to each other or pinned at the ceiling.
func TestOrg677BeforeAfterBlastRadius(t *testing.T) {
	prior := Prior()

	noBlastA := prodFeatureVector(0, 0, 0, 0, 0, 0, 0, 0.6931471805599453, 2.0794415416798357, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	noBlastB := prodFeatureVector(0, 0, 0, 0, 0, 0, 0, 0.6931471805599453, 2.1972245773362196, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	withBlastA := prodFeatureVector(230.26, 100, 60.22118343195266, 1, 3, 1, 1, 1.6094379124341003, 5.135798437050262, 1, 1, 2, 0, 0, 0, 0, 0, 0, 0)
	withBlastB := prodFeatureVector(153.66, 100, 51.22, 1, 0, 0, 0, 1.0986122886681096, 3.091042453358316, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)

	sNoBlastA := Score(prior, noBlastA, nil)
	sNoBlastB := Score(prior, noBlastB, nil)
	sWithBlastA := Score(prior, withBlastA, nil)
	sWithBlastB := Score(prior, withBlastB, nil)

	if sWithBlastA >= 1.0 || sWithBlastB >= 1.0 {
		t.Errorf("real blast-radius reviews pinned at ceiling: %v, %v", sWithBlastA, sWithBlastB)
	}
	if sWithBlastA == sWithBlastB {
		t.Errorf("the two with-blast-radius reviews scored bit-identical (%v) despite different features (230.26/3 critical/1 warning vs 153.66/0 findings)", sWithBlastA)
	}
	if sNoBlastA >= sWithBlastA || sNoBlastB >= sWithBlastB {
		t.Errorf("a review with real blast-radius data should score higher than one with none: noBlastA=%v withBlastA=%v, noBlastB=%v withBlastB=%v",
			sNoBlastA, sWithBlastA, sNoBlastB, sWithBlastB)
	}
}

// TestCategoryOverrideSweepNeverInstantlySaturatesRealisticInputs sweeps every
// category override from 0 to 10 against a battery of realistic feature
// vectors (including the real production ones above) and checks the score
// moves smoothly rather than jumping straight to the clamp ceiling/floor for
// any single step -- a crude proxy for "no default+override combination
// alone collapses all differentiation," answering "how do I know this works
// when a customer tweaks the sliders" with a sweep instead of a handful of
// hand-picked examples.
func TestCategoryOverrideSweepNeverInstantlySaturatesRealisticInputs(t *testing.T) {
	prior := Prior()
	battery := [][]float64{
		prodFeatureVector(4839.98, 82.22, 26.14934136243884, 2, 0, 7, 0, 4.624972813284271, 8.983690681332686, 1, 1, 2, 0, 0, 0, 1, 0, 3, 0),
		prodFeatureVector(230.26, 100, 60.22118343195266, 1, 3, 1, 1, 1.6094379124341003, 5.135798437050262, 1, 1, 2, 0, 0, 0, 0, 0, 0, 0),
		prodFeatureVector(0, 0, 0, 0, 0, 0, 0, 0.6931471805599453, 2.0794415416798357, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
	}
	categories := OverrideCategories()
	// All explicit, strictly increasing, and never 0/negative: categoryOverride
	// treats any override <= 0 as "fall through to the category's own default
	// multiplier" (DefaultCategoryMultiplier, which varies per category -- 2.0
	// for blast_radius, 1.5 for security, 1.0 for most others). An earlier
	// version of this sweep used 0 as its first step expecting it to mean "no
	// effect", but since 0 actually resolves to that category's default, and
	// the swept category's next explicit step (1.0) is *below* blast_radius's
	// own default (2.0), the score legitimately dropped between those two
	// steps -- not a model bug, a test bug (comparing "default 2.0x" against
	// "explicit 1.0x" isn't a monotonic increase). Starting low and explicit
	// avoids that trap.
	overrideSteps := []float64{0.1, 0.5, 1, 2, 5, 10}

	for _, cat := range categories {
		var prevScores []float64
		for _, step := range overrideSteps {
			overrides := map[string]float64{cat: step}
			var stepScores []float64
			for _, x := range battery {
				s := Score(prior, x, overrides)
				if math.IsNaN(s) || math.IsInf(s, 0) {
					t.Fatalf("category %s override %v: score is NaN/Inf for feature vector %v", cat, step, x)
				}
				if s <= 0 || s >= 1 {
					t.Fatalf("category %s override %v: score %v out of open range (0,1)", cat, step, s)
				}
				stepScores = append(stepScores, s)
			}
			if prevScores != nil {
				for i := range stepScores {
					// Monotonic in override strength for a category whose
					// weights are all non-negative (already asserted
					// separately by TestPriorAllFeatureWeightsNonNegative).
					if stepScores[i] < prevScores[i]-1e-9 {
						t.Errorf("category %s: increasing override to %v decreased score for battery item %d (%v -> %v)", cat, step, i, prevScores[i], stepScores[i])
					}
				}
			}
			prevScores = stepScores
		}
	}
}

// TestZClampNeverExactlyReached fuzzes large/adversarial feature vectors and
// confirms the score is always strictly inside (0,1) -- i.e. the clamp is
// actually doing its job of keeping sigmoid's input away from the ~37
// threshold where float64 can no longer represent 1+exp(-z) as anything but
// exactly 1.0 (or exp(z)/(1+exp(z)) as anything but exactly 0.0).
func TestZClampNeverExactlyReached(t *testing.T) {
	prior := Prior()
	rng := rand.New(rand.NewSource(99))
	for trial := 0; trial < 200; trial++ {
		x := make([]float64, FeatureCount())
		for i := range x {
			x[i] = rng.Float64() * 1000 // deliberately adversarial, well past any realistic log1p'd value
		}
		overrides := map[string]float64{}
		for _, cat := range OverrideCategories() {
			overrides[cat] = rng.Float64() * 10
		}
		s := Score(prior, x, overrides)
		if s <= 0 || s >= 1 {
			t.Fatalf("trial %d: score %v not strictly inside (0,1) despite zClampAbs=%v", trial, s, zClampAbs)
		}
	}
}
