package blastradius

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func loadTestReport(t *testing.T) Report {
	t.Helper()
	data, err := os.ReadFile("testdata/review_11632_report.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal testdata: %v", err)
	}
	return r
}

func findHunk(t *testing.T, r Report, filePath string, newStart int) HunkReport {
	t.Helper()
	for _, f := range r.Files {
		for _, h := range f.Hunks {
			if h.FilePath == filePath && h.NewStart == newStart {
				return h
			}
		}
	}
	t.Fatalf("hunk %s:%d not found in testdata", filePath, newStart)
	return HunkReport{}
}

func approxEqual(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.05 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// TestComputeMathMode_Review11632MultiSymbolMax is the golden case: a hunk
// that is its own max for both dimensions, with 6 contributing symbols.
// Every number here was diffed character-for-character against the live UI
// (see docs/blast-radius-backend-port-plan.md for the verification numbers).
func TestComputeMathMode_Review11632MultiSymbolMax(t *testing.T) {
	r := loadTestReport(t)
	h := findHunk(t, r, "ui/src/pages/Chatbot/rebucketChart.ts", 557)

	mm := ComputeMathMode(h)

	// Blast Radius: Steps 1-3, 6 symbols + hunk signals -> sum step present.
	approxEqual(t, "BlastRadius.Total", mm.BlastRadius.Total, 30.7)
	approxEqual(t, "BlastRadius.Max", mm.BlastRadius.Max, 30.7)
	approxEqual(t, "BlastRadius.Norm", mm.BlastRadius.Norm, 100)
	if !mm.BlastRadius.IsSelf {
		t.Error("BlastRadius.IsSelf = false, want true (this hunk is its own max)")
	}
	if mm.BlastRadius.StepAdd != 1 || mm.BlastRadius.StepSum != 2 || mm.BlastRadius.StepScale != 3 {
		t.Errorf("BlastRadius steps = %d/%d/%d, want 1/2/3",
			mm.BlastRadius.StepAdd, mm.BlastRadius.StepSum, mm.BlastRadius.StepScale)
	}
	if len(mm.BlastRadius.PerSymbol) != 6 {
		t.Fatalf("BlastRadius.PerSymbol has %d entries, want 6", len(mm.BlastRadius.PerSymbol))
	}
	wantBlastSubtotals := map[string]float64{
		"SlopeEncodingParts":  1.0,
		"SlopeStats":          1.7,
		"computeHeatmapStats": 6.9,
		"computeSlopeStats":   6.9,
		"getSlopeEncoding":    7.1,
		"isSlopeGraph":        6.9,
	}
	for _, line := range mm.BlastRadius.PerSymbol {
		want, ok := wantBlastSubtotals[line.Name]
		if !ok {
			t.Errorf("unexpected symbol %q in BlastRadius.PerSymbol", line.Name)
			continue
		}
		approxEqual(t, "BlastRadius subtotal for "+line.Name, line.Subtotal, want)
	}
	approxEqual(t, "BlastRadius.HunkSignalsSubtotal", mm.BlastRadius.HunkSignalsSubtotal, 0.3)

	// Review Priority: Steps 4-6, same shape.
	approxEqual(t, "ReviewPriority.Total", mm.ReviewPriority.Total, 69.7)
	approxEqual(t, "ReviewPriority.Max", mm.ReviewPriority.Max, 69.7)
	approxEqual(t, "ReviewPriority.Norm", mm.ReviewPriority.Norm, 100)
	if !mm.ReviewPriority.IsSelf {
		t.Error("ReviewPriority.IsSelf = false, want true")
	}
	if mm.ReviewPriority.StepAdd != 4 || mm.ReviewPriority.StepSum != 5 || mm.ReviewPriority.StepScale != 6 {
		t.Errorf("ReviewPriority steps = %d/%d/%d, want 4/5/6",
			mm.ReviewPriority.StepAdd, mm.ReviewPriority.StepSum, mm.ReviewPriority.StepScale)
	}
	wantPrioritySubtotals := map[string]float64{
		"SlopeEncodingParts":  3.0,
		"SlopeStats":          3.0,
		"computeHeatmapStats": 16.3,
		"computeSlopeStats":   29.8,
		"getSlopeEncoding":    10.8,
		"isSlopeGraph":        6.8,
	}
	for _, line := range mm.ReviewPriority.PerSymbol {
		want, ok := wantPrioritySubtotals[line.Name]
		if !ok {
			t.Errorf("unexpected symbol %q in ReviewPriority.PerSymbol", line.Name)
			continue
		}
		approxEqual(t, "ReviewPriority subtotal for "+line.Name, line.Subtotal, want)
	}

	// Combine: Steps 7-8.
	if mm.StepBlend != 7 || mm.StepHygiene != 8 {
		t.Errorf("combine steps = %d/%d, want 7/8", mm.StepBlend, mm.StepHygiene)
	}
	approxEqual(t, "BlastShare", mm.BlastShare, 60.0)
	approxEqual(t, "PriorityShare", mm.PriorityShare, 40.0)
	approxEqual(t, "Blended", mm.Blended, 100.0)
	approxEqual(t, "HygieneMultiplier", mm.HygieneMultiplier, 1.0)
	approxEqual(t, "Final", mm.Final, 100.0)

	// The load-bearing invariant: the derivation must land on the same
	// number git-lrc already computed and stored.
	approxEqual(t, "Final vs Combined", mm.Final, h.Combined)
}

// TestComputeMathMode_Review11632SingleSymbolSkipsSumStep covers the
// "dynamic step numbering" hazard: a hunk with exactly one contributing
// symbol and no hunk-level signals for a dimension omits the "add every
// subtotal together" step entirely, so step numbers run 1,2 not 1,2,3.
func TestComputeMathMode_Review11632SingleSymbolSkipsSumStep(t *testing.T) {
	r := loadTestReport(t)
	h := findHunk(t, r, "ui/src/pages/Chatbot/ChatConversation.tsx", 841)

	mm := ComputeMathMode(h)

	if mm.BlastRadius.StepSum != 0 {
		t.Errorf("BlastRadius.StepSum = %d, want 0 (step omitted - only one part contributed)",
			mm.BlastRadius.StepSum)
	}
	if mm.BlastRadius.StepAdd != 1 || mm.BlastRadius.StepScale != 2 {
		t.Errorf("BlastRadius steps = %d/../%d, want 1/../2", mm.BlastRadius.StepAdd, mm.BlastRadius.StepScale)
	}
	if mm.ReviewPriority.StepSum != 0 {
		t.Errorf("ReviewPriority.StepSum = %d, want 0", mm.ReviewPriority.StepSum)
	}
	if mm.ReviewPriority.StepAdd != 3 || mm.ReviewPriority.StepScale != 4 {
		t.Errorf("ReviewPriority steps = %d/../%d, want 3/../4", mm.ReviewPriority.StepAdd, mm.ReviewPriority.StepScale)
	}
	if mm.StepBlend != 5 || mm.StepHygiene != 6 {
		t.Errorf("combine steps = %d/%d, want 5/6", mm.StepBlend, mm.StepHygiene)
	}

	// This hunk is not its own max for either dimension - the "self" narration
	// branch must not fire.
	if mm.BlastRadius.IsSelf {
		t.Error("BlastRadius.IsSelf = true, want false (rebucketChart.ts hunk is the max, not this one)")
	}
	if mm.BlastRadius.MaxSourceFile != "ui/src/pages/Chatbot/rebucketChart.ts" {
		t.Errorf("BlastRadius.MaxSourceFile = %q, want rebucketChart.ts", mm.BlastRadius.MaxSourceFile)
	}

	approxEqual(t, "BlastRadius.Norm", mm.BlastRadius.Norm, 22.7)
	approxEqual(t, "ReviewPriority.Norm", mm.ReviewPriority.Norm, 54.7)
	approxEqual(t, "Final vs Combined", mm.Final, h.Combined)
	approxEqual(t, "Final", mm.Final, 35.5)
}

// TestComputeMathMode_Review11632ZeroSignals covers a hunk with no
// contributing signals for either dimension at all ("No signals fired").
func TestComputeMathMode_Review11632ZeroSignals(t *testing.T) {
	r := loadTestReport(t)
	h := findHunk(t, r, "ui/src/pages/Chatbot/ChatConversation.tsx", 21)

	mm := ComputeMathMode(h)

	if len(mm.BlastRadius.PerSymbol) != 0 {
		t.Errorf("BlastRadius.PerSymbol has %d entries, want 0", len(mm.BlastRadius.PerSymbol))
	}
	if len(mm.BlastRadius.HunkSignals) != 0 {
		t.Errorf("BlastRadius.HunkSignals has %d entries, want 0", len(mm.BlastRadius.HunkSignals))
	}
	approxEqual(t, "BlastRadius.Norm", mm.BlastRadius.Norm, 0)
	approxEqual(t, "ReviewPriority.Norm", mm.ReviewPriority.Norm, 0)
	approxEqual(t, "Final", mm.Final, 0)
	approxEqual(t, "Final vs Combined", mm.Final, h.Combined)
}

// TestEffectiveHygiene_ZeroIsKeptNotDefaulted is the regression case for a
// real divergence from the frontend: a *float64 of 0 (fully suppressed, a
// real value git-lrc can emit) must stay 0, not fall back to 1.0 the way an
// absent field does. Caught in code review - a plain float64 field
// couldn't tell "absent" from "present as zero" apart.
func TestEffectiveHygiene_ZeroIsKeptNotDefaulted(t *testing.T) {
	zero := 0.0
	if got := effectiveHygiene(&zero); got != 0 {
		t.Errorf("effectiveHygiene(&0) = %v, want 0 (a real zero must not be defaulted)", got)
	}
	if got := effectiveHygiene(nil); got != 1.0 {
		t.Errorf("effectiveHygiene(nil) = %v, want 1.0 (absent field defaults)", got)
	}
	half := 0.5
	if got := effectiveHygiene(&half); got != 0.5 {
		t.Errorf("effectiveHygiene(&0.5) = %v, want 0.5", got)
	}
}

func TestTier(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "blast-radius-none"},
		{0.1, "blast-radius-low"},
		{32.9, "blast-radius-low"},
		{33, "blast-radius-medium"},
		{65.9, "blast-radius-medium"},
		{66, "blast-radius-high"},
		{100, "blast-radius-high"},
	}
	for _, c := range cases {
		if got := Tier(c.score); got != c.want {
			t.Errorf("Tier(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}
