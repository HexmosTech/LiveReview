package blastradius

import "math"

// CrossRepoSignalName is the Signal.Name the cross-repo impact bonus is
// recorded under, so the frontend's allSignals() breakdown and the Math Mode
// panel both label it the same way a normal structural signal would be.
const CrossRepoSignalName = "Cross-repo impact"

// crossRepoBonusPerImpact/crossRepoMaxBonus cap how much a cross-repo match
// can move a hunk's score: one weak match should nudge ordering, not single-
// handedly flip a hunk's tier. Same order of magnitude as FileCouplingBonus.
const (
	crossRepoBonusPerImpact = 8.0
	crossRepoMaxBonus       = 20.0
)

// RecordCrossRepoImpact attaches impacts (already deduped by the caller) to
// the symbol named symbolQN inside the hunk identified by
// (filePath, newStart, newLines), adds a capped "graph"-category Signal so
// it participates in Combined/Tier via the normal scoring path, and returns
// whether a matching hunk+symbol was found. The caller must call Renormalize
// on the whole report afterward (not here, so a batch of calls only
// renormalizes once).
func RecordCrossRepoImpact(report *Report, filePath string, newStart, newLines int, symbolQN string, impacts []CrossRepoImpact) bool {
	if len(impacts) == 0 {
		return false
	}
	for fi := range report.Files {
		file := &report.Files[fi]
		if file.Path != filePath {
			continue
		}
		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			if hunk.NewStart != newStart || hunk.NewLines != newLines {
				continue
			}
			for si := range hunk.Symbols {
				sym := &hunk.Symbols[si]
				if sym.QualifiedName != symbolQN {
					continue
				}
				// Idempotent, not additive: a second call for the same symbol (e.g.
				// a re-run of the cross_repo_link job against an already-enriched
				// report) replaces its prior cross-repo signal/bonus instead of
				// stacking a second one on top.
				if prior := removeCrossRepoSignal(sym); prior != 0 {
					sym.BlastRadiusRaw -= prior
					hunk.BlastRadiusRaw -= prior
				}
				bonus := math.Min(float64(len(impacts))*crossRepoBonusPerImpact, crossRepoMaxBonus)
				sym.ImpactedRelatedRepos = impacts
				sym.Signals = append(sym.Signals, Signal{
					Name:     CrossRepoSignalName,
					Detail:   crossRepoDetail(impacts),
					Points:   bonus,
					Category: "graph",
				})
				sym.BlastRadiusRaw += bonus
				hunk.BlastRadiusRaw += bonus
				return true
			}
		}
	}
	return false
}

// removeCrossRepoSignal strips any prior cross-repo Signal from sym and
// returns its Points (0 if there was none), so a caller can subtract the
// matching amount back out of BlastRadiusRaw before re-adding a fresh one.
func removeCrossRepoSignal(sym *SymbolContribution) float64 {
	var prior float64
	kept := sym.Signals[:0]
	for _, s := range sym.Signals {
		// Matches on Name alone - CrossRepoSignalName is this package's own
		// constant and unique among the signals this file emits, so pinning
		// the match to today's Category too would only make this silently
		// stop finding its own prior signal if that category ever changed.
		if s.Name == CrossRepoSignalName {
			prior += s.Points
			continue
		}
		kept = append(kept, s)
	}
	sym.Signals = kept
	return prior
}

func crossRepoDetail(impacts []CrossRepoImpact) string {
	if len(impacts) == 1 {
		return "Called from " + impacts[0].RelatedRepo
	}
	repos := map[string]bool{}
	for _, im := range impacts {
		repos[im.RelatedRepo] = true
	}
	n := 0
	for range repos {
		n++
	}
	if n == 1 {
		for repo := range repos {
			return "Called from " + repo
		}
	}
	return "Called from related repos"
}

// Renormalize recomputes MaxBlastRadiusRaw/BlastRadiusNorm (and the derived
// Combined) across every hunk in report, after one or more hunks' raw blast
// radius changed post-scoring (see RecordCrossRepoImpact). This mirrors the
// normalization the original scorer performs once over the whole diff: the
// highest BlastRadiusRaw across every hunk becomes every hunk's
// MaxBlastRadiusRaw, and each hunk's Norm/Combined/Tier is recomputed from
// that. ReviewPriority's own Max/Norm are untouched - only blast radius can
// change here.
func Renormalize(report *Report) {
	var maxRaw float64
	var maxFile, maxHeader string
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			if hunk.BlastRadiusRaw > maxRaw {
				maxRaw, maxFile, maxHeader = hunk.BlastRadiusRaw, file.Path, hunk.Header
			}
		}
	}
	for fi := range report.Files {
		file := &report.Files[fi]
		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			hunk.MaxBlastRadiusRaw = maxRaw
			hunk.MaxBlastRadiusHunkFile = maxFile
			hunk.MaxBlastRadiusHunkHeader = maxHeader
			if maxRaw > 0 {
				hunk.BlastRadiusNorm = hunk.BlastRadiusRaw / maxRaw * 100
			} else {
				hunk.BlastRadiusNorm = 0
			}
			hunk.Combined = ComputeMathMode(*hunk).Final
			// Rebuilt from the symbols themselves rather than accumulated
			// incrementally in RecordCrossRepoImpact, so this stays correct
			// (no duplicates) no matter how many times that function ran for
			// this hunk's symbols.
			hunk.ImpactedRelatedRepos = nil
			for _, sym := range hunk.Symbols {
				hunk.ImpactedRelatedRepos = append(hunk.ImpactedRelatedRepos, sym.ImpactedRelatedRepos...)
			}
		}
	}
}
