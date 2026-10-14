package riskscore

// TestThresholdExperiment answers the questions a reader of the
// verification report keeps asking: "what does the threshold setting
// actually do to the approval rate," "is the threshold a fixed number," and
// "what happens at the extreme ends (P0, P100)." Every question gets a
// real, computed answer here instead of a guess. No database is needed:
// PercentileCutoff and Score are pure functions over plain feature vectors,
// so this scores a fixed, reproducible mix of reviews
// (internal/riskscore/referencemix) with the real seed model.
//
// See docs/risk-policy-explainability.md's "Threshold" section for the
// numbers this test produces, quoted directly. internal/policyengine's
// scenario corpus test imports the same referencemix package, so every
// scenario's reported percentile rank is measured against this identical
// distribution.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/livereview/internal/riskscore/referencemix"
)

// tierDump and namedExampleDump are the machine-readable record this test
// logs (prefix "TDDATA:") -- the verification report's Thresholds tab is
// regenerated directly from this JSON by scripts/regen_risk_report.py, never
// hand-transcribed. If you change referencemix, Prior(), or this test's tier
// list, re-run `make regen-risk-report` (or the script directly) to keep the
// report in sync -- it will NOT update itself.
type tierDump struct {
	Percentile int     `json:"percentile"`
	Tag        string  `json:"tag"`
	Cutoff     float64 `json:"cutoff"`
	Approved   int     `json:"approved"`
	Total      int     `json:"total"`
}

type namedExampleDump struct {
	Label    string          `json:"label"`
	Score    float64         `json:"score"`
	Decision map[string]bool `json:"decision"` // percentile (as string) -> approved
}

type thresholdExperimentDump struct {
	Tiers         []tierDump         `json:"tiers"`
	NamedExamples []namedExampleDump `json:"named_examples"`
}

// TestThresholdExperimentApprovalRatesByTier is the "what does P10/P20/P50/
// P80 actually do" answer: compute the cutoff and resulting approval rate
// for each tier against the same realistic mix, and show which named
// examples flip from approved to blocked at the stricter tiers.
func TestThresholdExperimentApprovalRatesByTier(t *testing.T) {
	prior := Prior()
	mix := referencemix.Build(FeatureCount())

	var scores []float64
	named := map[string]float64{}
	var namedOrder []string // insertion order from referencemix.Build, not map iteration (which Go randomizes) -- keeps TDDATA reproducible byte-for-byte across runs.
	for _, ex := range mix {
		s := Score(prior, ex.Features, nil)
		scores = append(scores, s)
		if ex.Label != "" {
			named[ex.Label] = s
			namedOrder = append(namedOrder, ex.Label)
		}
	}

	tiers := []struct {
		percentile int
		tag        string
	}{
		{0, "the strictest possible setting"},
		{10, "strict"},
		{20, "balanced (today's default)"},
		{50, "relaxed"},
		{80, "very relaxed"},
		{100, "the most relaxed possible setting"},
	}

	dump := thresholdExperimentDump{}
	for _, label := range namedOrder {
		dump.NamedExamples = append(dump.NamedExamples, namedExampleDump{Label: label, Score: named[label], Decision: map[string]bool{}})
	}

	t.Logf("synthetic mix: %d reviews, mostly small/clean (matches a typical repo's recent history)", len(scores))
	for _, tier := range tiers {
		cutoff := PercentileCutoff(scores, tier.percentile)
		approved := 0
		for _, s := range scores {
			if s < cutoff {
				approved++
			}
		}
		rate := float64(approved) * 100 / float64(len(scores))
		t.Logf("P%-3d %-32s cutoff=%.3f  approved=%d/%d (%.0f%%)", tier.percentile, tier.tag, cutoff, approved, len(scores), rate)
		dump.Tiers = append(dump.Tiers, tierDump{Percentile: tier.percentile, Tag: tier.tag, Cutoff: cutoff, Approved: approved, Total: len(scores)})
		for i, label := range namedOrder {
			score := named[label]
			decision := "approved"
			approvedBool := score < cutoff
			if !approvedBool {
				decision = "BLOCKED"
			}
			t.Logf("    at P%d: %-38s score=%.3f -> %s", tier.percentile, label, score, decision)
			dump.NamedExamples[i].Decision[fmt.Sprintf("%d", tier.percentile)] = approvedBool
		}
	}

	dumpJSON, err := json.Marshal(dump)
	if err != nil {
		t.Fatalf("marshal threshold dump: %v", err)
	}
	t.Logf("TDDATA:%s", dumpJSON)

	// Sanity: a stricter percentile (lower P) must never approve more of the
	// mix than a more relaxed one (higher P) -- the dial must point the
	// right way, end to end.
	prevApproved := -1
	for _, tier := range tiers {
		cutoff := PercentileCutoff(scores, tier.percentile)
		approved := 0
		for _, s := range scores {
			if s < cutoff {
				approved++
			}
		}
		if approved < prevApproved {
			t.Fatalf("P%d approved fewer reviews (%d) than a stricter, lower percentile already approved (%d) -- the dial points the wrong way", tier.percentile, approved, prevApproved)
		}
		prevApproved = approved
	}

	// The two edges, stated precisely: P0 (and P1, since PercentileCutoff
	// clamps p<=0 to 1) must approve nothing -- its cutoff is the mix's own
	// lowest score, and nothing scores strictly below the lowest score.
	// P100 must approve everything except the single riskiest review in the
	// mix -- its cutoff is the mix's own highest score, which is the one
	// review that is never strictly below its own cutoff.
	p0Cutoff := PercentileCutoff(scores, 0)
	p0Approved := 0
	for _, s := range scores {
		if s < p0Cutoff {
			p0Approved++
		}
	}
	if p0Approved != 0 {
		t.Fatalf("P0 should approve nothing (strictest possible setting), approved %d/%d", p0Approved, len(scores))
	}

	p100Cutoff := PercentileCutoff(scores, 100)
	p100Approved := 0
	for _, s := range scores {
		if s < p100Cutoff {
			p100Approved++
		}
	}
	if p100Approved != len(scores)-1 {
		t.Fatalf("P100 should approve every review except the single riskiest one (%d/%d), got %d/%d", len(scores)-1, len(scores), p100Approved, len(scores))
	}
}

// TestThresholdExperimentSamePercentileDifferentCutoff is the literal answer
// to "is the threshold a fixed number": the same P20 *setting*, run against
// two different organizations' recent score histories, produces two
// different cutoff *numbers*. The setting (percentile) is static until an
// admin changes it; the number it resolves to moves with the org's own
// history automatically.
func TestThresholdExperimentSamePercentileDifferentCutoff(t *testing.T) {
	quietOrgHistory := make([]float64, 0, 40)
	for i := 0; i < 40; i++ {
		quietOrgHistory = append(quietOrgHistory, 0.2+float64(i%10)*0.02) // 0.2-0.38, a calm repo
	}
	riskyOrgHistory := make([]float64, 0, 40)
	for i := 0; i < 40; i++ {
		riskyOrgHistory = append(riskyOrgHistory, 0.5+float64(i%10)*0.04) // 0.5-0.86, a repo mid-incident-spike
	}

	quietCutoff := PercentileCutoff(quietOrgHistory, 20)
	riskyCutoff := PercentileCutoff(riskyOrgHistory, 20)

	t.Logf("same policy setting (P20) on two different orgs' recent history:")
	t.Logf("  quiet org  (scores mostly 0.20-0.38): P20 cutoff = %.3f", quietCutoff)
	t.Logf("  busy org   (scores mostly 0.50-0.86): P20 cutoff = %.3f", riskyCutoff)

	same := map[string]float64{"quiet_cutoff": quietCutoff, "busy_cutoff": riskyCutoff}
	sameJSON, err := json.Marshal(same)
	if err != nil {
		t.Fatalf("marshal same-percentile dump: %v", err)
	}
	t.Logf("TDSAME:%s", sameJSON)

	if quietCutoff == riskyCutoff {
		t.Fatalf("expected the two orgs' cutoffs to differ even at the same P20 setting; got the same value %.3f for both", quietCutoff)
	}
	if riskyCutoff <= quietCutoff {
		t.Fatalf("the org with a riskier recent history should calibrate to a higher cutoff: quiet=%.3f risky=%.3f", quietCutoff, riskyCutoff)
	}
}
