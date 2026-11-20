package blastradius

import "testing"

const (
	crossRepoTestFile   = "ui/src/pages/Chatbot/ChatConversation.tsx"
	crossRepoTestStart  = 841
	crossRepoTestLines  = 12
	crossRepoTestSymbol = "home-lovestaco-hex-lr-LiveReview.ui.src.pages.Chatbot.ChatConversation.ChatConversation"
)

func TestRecordCrossRepoImpact_NoMatch(t *testing.T) {
	r := loadTestReport(t)
	ok := RecordCrossRepoImpact(&r, "no/such/file.go", 1, 1, "no.such.symbol", []CrossRepoImpact{{RelatedRepo: "x"}})
	if ok {
		t.Fatal("expected no match for a nonexistent hunk/symbol")
	}
}

func TestRecordCrossRepoImpact_EmptyImpacts(t *testing.T) {
	r := loadTestReport(t)
	before := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	ok := RecordCrossRepoImpact(&r, crossRepoTestFile, crossRepoTestStart, crossRepoTestLines, crossRepoTestSymbol, nil)
	if ok {
		t.Fatal("expected false for an empty impacts slice")
	}
	after := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	if after.BlastRadiusRaw != before.BlastRadiusRaw {
		t.Fatal("empty impacts must not change the hunk")
	}
}

func TestRecordCrossRepoImpact_AddsSignalAndBumpsRaw(t *testing.T) {
	r := loadTestReport(t)
	before := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)

	impacts := []CrossRepoImpact{
		{RelatedRepo: "acme/billing-worker", CallSite: "billing.HandleChargeWebhook", EdgeType: "CROSS_HTTP_CALLS", Confidence: 0.9},
	}
	if !RecordCrossRepoImpact(&r, crossRepoTestFile, crossRepoTestStart, crossRepoTestLines, crossRepoTestSymbol, impacts) {
		t.Fatal("expected a match")
	}

	after := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	wantRaw := before.BlastRadiusRaw + crossRepoBonusPerImpact
	if after.BlastRadiusRaw != wantRaw {
		t.Fatalf("BlastRadiusRaw = %v, want %v", after.BlastRadiusRaw, wantRaw)
	}
	if len(after.ImpactedRelatedRepos) != 1 || after.ImpactedRelatedRepos[0].RelatedRepo != "acme/billing-worker" {
		t.Fatalf("hunk ImpactedRelatedRepos = %+v", after.ImpactedRelatedRepos)
	}

	var sym *SymbolContribution
	for i := range after.Symbols {
		if after.Symbols[i].QualifiedName == crossRepoTestSymbol {
			sym = &after.Symbols[i]
		}
	}
	if sym == nil {
		t.Fatal("symbol not found after RecordCrossRepoImpact")
	}
	found := false
	for _, s := range sym.Signals {
		if s.Name == CrossRepoSignalName && s.Category == "graph" {
			found = true
			if s.Points != crossRepoBonusPerImpact {
				t.Fatalf("signal Points = %v, want %v", s.Points, crossRepoBonusPerImpact)
			}
		}
	}
	if !found {
		t.Fatal("expected a Cross-repo impact signal with Category=graph")
	}
}

// TestRecordCrossRepoImpact_BonusIsCapped verifies many impacts on one
// symbol still only move BlastRadiusRaw by crossRepoMaxBonus, not
// len(impacts)*crossRepoBonusPerImpact unbounded.
func TestRecordCrossRepoImpact_BonusIsCapped(t *testing.T) {
	r := loadTestReport(t)
	before := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)

	var impacts []CrossRepoImpact
	for range 10 {
		impacts = append(impacts, CrossRepoImpact{RelatedRepo: "acme/svc", CallSite: "x", EdgeType: "CROSS_HTTP_CALLS"})
	}
	RecordCrossRepoImpact(&r, crossRepoTestFile, crossRepoTestStart, crossRepoTestLines, crossRepoTestSymbol, impacts)

	after := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	wantRaw := before.BlastRadiusRaw + crossRepoMaxBonus
	if after.BlastRadiusRaw != wantRaw {
		t.Fatalf("BlastRadiusRaw = %v, want %v (bonus must be capped at %v)", after.BlastRadiusRaw, wantRaw, crossRepoMaxBonus)
	}
}

func TestRenormalize_RecomputesCombinedAndTier(t *testing.T) {
	r := loadTestReport(t)
	before := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	beforeTier := Tier(before.Combined)

	// A large bonus should be enough to move this hunk's tier up, since it's
	// well below its own current max.
	impacts := []CrossRepoImpact{
		{RelatedRepo: "acme/billing-worker", CallSite: "billing.HandleChargeWebhook", EdgeType: "CROSS_HTTP_CALLS"},
		{RelatedRepo: "acme/other-svc", CallSite: "other.Handler", EdgeType: "CROSS_HTTP_CALLS"},
		{RelatedRepo: "acme/third-svc", CallSite: "third.Handler", EdgeType: "CROSS_HTTP_CALLS"},
	}
	RecordCrossRepoImpact(&r, crossRepoTestFile, crossRepoTestStart, crossRepoTestLines, crossRepoTestSymbol, impacts)
	Renormalize(&r)

	after := findHunk(t, r, crossRepoTestFile, crossRepoTestStart)
	if after.Combined <= before.Combined {
		t.Fatalf("Combined did not increase: before=%v after=%v", before.Combined, after.Combined)
	}
	afterTier := Tier(after.Combined)
	t.Logf("tier: %s (%.2f) -> %s (%.2f)", beforeTier, before.Combined, afterTier, after.Combined)

	// Invariant Renormalize must preserve: Combined always equals
	// ComputeMathMode(h).Final, the same invariant the rest of this package
	// already asserts for untouched reports.
	if got := ComputeMathMode(after).Final; got != after.Combined {
		t.Fatalf("Combined (%v) != ComputeMathMode(h).Final (%v)", after.Combined, got)
	}
}

// TestRenormalize_UntouchedHunksStayConsistent asserts the Combined ==
// ComputeMathMode(h).Final invariant still holds for every hunk in the
// report after Renormalize, not just the one hunk a cross-repo impact
// touched.
func TestRenormalize_UntouchedHunksStayConsistent(t *testing.T) {
	r := loadTestReport(t)
	impacts := []CrossRepoImpact{{RelatedRepo: "acme/tiny-svc", CallSite: "x", EdgeType: "CROSS_HTTP_CALLS"}}
	RecordCrossRepoImpact(&r, crossRepoTestFile, crossRepoTestStart, crossRepoTestLines, crossRepoTestSymbol, impacts)
	Renormalize(&r)

	for _, f := range r.Files {
		for _, h := range f.Hunks {
			if got := ComputeMathMode(h).Final; got != h.Combined {
				t.Fatalf("%s:%d Combined (%v) != ComputeMathMode(h).Final (%v) after Renormalize", h.FilePath, h.NewStart, h.Combined, got)
			}
		}
	}
}
