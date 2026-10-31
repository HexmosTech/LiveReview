package policyengine

import (
	"testing"

	"github.com/livereview/internal/riskscore"
)

// Runs each risk message scenario (no, minimal and full score history) through the
// real cutoff/percentile helpers and checks the risk stage message.
func TestRiskReasonScenarios(t *testing.T) {
	ten := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	cases := []struct {
		name      string
		history   []float64
		score     float64
		threshold int
		want      string
	}{
		{"no history, blocked", nil, 1.0, 20, "Risk score 1.00 is above the cutoff of 0.50"},
		{"no history, passes", nil, 0.3, 20, "Risk score 0.30 is below the cutoff of 0.50"},
		{"no history, score equals default cutoff", nil, 0.5, 20, "Risk score 0.50 is at the cutoff of 0.50"},
		// Below minScoresForPercentile (10): too few points to rank
		// meaningfully, so PercentileCutoff falls back to the same default
		// as no history, and no P-labels are shown at all.
		{"one past score, higher score blocked", []float64{0.4}, 0.9, 20, "Risk score 0.90 is above the cutoff of 0.50"},
		{"one past score, lower score passes", []float64{0.4}, 0.2, 20, "Risk score 0.20 is below the cutoff of 0.50"},
		{"five past scores (still below min sample), all tied", []float64{1, 1, 1, 1, 1}, 1.0, 20, "Risk score 1.00 is above the cutoff of 0.50"},
		// At/above minScoresForPercentile: real percentile ranking kicks in,
		// and both the score's and cutoff's P-labels come from the same
		// PercentileOf function, so they stay directly comparable.
		{"full history, blocked", ten, 0.9, 20, "Risk score 0.90 (P90) is above the cutoff of 0.20 (P20)"},
		{"full history, passes", ten, 0.15, 20, "Risk score 0.15 (P10) is below the cutoff of 0.20 (P20)"},
		{"full history, score equals cutoff", ten, 0.2, 20, "Risk score 0.20 (P20) is at the cutoff of 0.20 (P20)"},
		{"full history, balanced passes", ten, 0.45, 50, "Risk score 0.45 (P40) is below the cutoff of 0.50 (P50)"},
	}
	for _, c := range cases {
		cutoff := riskscore.PercentileCutoff(c.history, c.threshold)
		pct := riskscore.PercentileOf(c.score, c.history)
		cutoffPct := riskscore.PercentileOf(cutoff, c.history)
		hasHistory := riskscore.HasEnoughHistory(c.history)
		if got := riskReason(c.score, cutoff, pct, cutoffPct, c.threshold, hasHistory); got != c.want {
			t.Errorf("[%s]\n got: %s\nwant: %s", c.name, got, c.want)
		}
	}
}

// With a large enough history for PercentileCutoff to rank against, a
// blocked review's P number is never below the threshold and a passing
// one's is never at or above it, so the message can't contradict itself.
// Below the minimum sample size, PercentileCutoff intentionally decouples
// from the percentile ranking (falls back to the default cutoff instead of
// a misleading rank), so the invariant only applies once there's enough
// history for it to mean something.
func TestRiskReasonPercentilesNeverContradict(t *testing.T) {
	histories := [][]float64{
		{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		{0.42, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		{0.05, 0.05, 0.3, 0.3, 0.3, 0.9, 0.9, 0.95, 0.99, 1.0},
	}
	for _, h := range histories {
		for _, threshold := range []int{10, 20, 50, 80} {
			for s := 0.0; s <= 1.0001; s += 0.01 {
				cutoff := riskscore.PercentileCutoff(h, threshold)
				pct := riskscore.PercentileOf(s, h)
				if s >= cutoff && pct < threshold {
					t.Errorf("history %v P%d: score %.2f blocked but ranks P%d", h, threshold, s, pct)
				}
				if s < cutoff && pct >= threshold {
					t.Errorf("history %v P%d: score %.2f passes but ranks P%d", h, threshold, s, pct)
				}
			}
		}
	}
}

// Below the minimum sample size, PercentileCutoff always falls back to the
// cold-start default rather than ranking off a handful of points — this is
// what prevents the degenerate case from review 13543 (a tiny/skewed history
// producing a cutoff that collapses onto the distribution's max).
func TestPercentileCutoffFallsBackBelowMinSample(t *testing.T) {
	small := [][]float64{
		nil,
		{0.4},
		{0.6, 0.7},
		{1, 1, 1, 1, 1},
		{0.1, 0.9, 1, 1, 1, 1, 1, 1, 1},
	}
	for _, h := range small {
		for _, threshold := range []int{1, 20, 50, 100} {
			if got := riskscore.PercentileCutoff(h, threshold); got != 0.5 {
				t.Errorf("history %v P%d: cutoff = %.2f, want default 0.50", h, threshold, got)
			}
		}
	}
}
