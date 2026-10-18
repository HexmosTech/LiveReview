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
		{"no history, score equals default cutoff", nil, 0.5, 20, "Risk score 0.50 is above the cutoff of 0.50"},
		{"one past score, higher score blocked", []float64{0.4}, 0.9, 20, "Risk score 0.90 (P100) is above the cutoff of 0.40 (P20)"},
		{"one past score, lower score passes", []float64{0.4}, 0.2, 20, "Risk score 0.20 (P0) is below the cutoff of 0.40 (P20)"},
		{"full history, blocked", ten, 0.9, 20, "Risk score 0.90 (P90) is above the cutoff of 0.20 (P20)"},
		{"full history, passes", ten, 0.15, 20, "Risk score 0.15 (P10) is below the cutoff of 0.20 (P20)"},
		{"full history, score equals cutoff", ten, 0.2, 20, "Risk score 0.20 (P20) is above the cutoff of 0.20 (P20)"},
		{"full history, balanced passes", ten, 0.45, 50, "Risk score 0.45 (P40) is below the cutoff of 0.50 (P50)"},
		{"all past scores the same", []float64{1, 1, 1, 1, 1}, 1.0, 20, "Risk score 1.00 (P100) is above the cutoff of 1.00 (P20)"},
	}
	for _, c := range cases {
		cutoff := riskscore.PercentileCutoff(c.history, c.threshold)
		pct := riskscore.PercentileOf(c.score, c.history)
		if got := riskReason(c.score, cutoff, pct, c.threshold, len(c.history) > 0); got != c.want {
			t.Errorf("[%s]\n got: %s\nwant: %s", c.name, got, c.want)
		}
	}
}

// With past scores, a blocked review's P number is never below the threshold
// and a passing one's is never at or above it, so the message can't contradict itself.
func TestRiskReasonPercentilesNeverContradict(t *testing.T) {
	histories := [][]float64{
		{0.4},
		{0.6, 0.7},
		{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		{0.42, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		{0.05, 0.05, 0.3, 0.3, 0.3, 0.9},
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
