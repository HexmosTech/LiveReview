package riskscore

import (
	"math"
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
	x[0] = 50 // br_sum
	x[4] = 1  // critical_count

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

func TestRankAUC(t *testing.T) {
	scores := []float64{0.9, 0.8, 0.7, 0.3, 0.2, 0.1}
	labels := []float64{1, 1, 1, 0, 0, 0}
	if got := rankAUC(scores, labels); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("perfect separation AUC = %v, want 1.0", got)
	}
}
