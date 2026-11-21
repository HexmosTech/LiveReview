package blastradius

import (
	"fmt"
	"math"
)

// CrossRepoSignalName is the Signal.Name the cross-repo impact bonus is
// recorded under, so the frontend's allSignals() breakdown and the Math Mode
// panel both label it the same way a normal structural signal would be.
const CrossRepoSignalName = "Cross-repo impact"

// CrossRepoChainSignalName labels the small, capped bonus for reaching a repo
// beyond the direct neighbours of the reviewed repo. It is deliberately a
// separate signal so Math Mode can show "direct" vs "extended reach" and so a
// re-run can replace one without disturbing the other.
const CrossRepoChainSignalName = "Cross-repo chain"

// crossRepoChainBonusPerHop/crossRepoChainMaxBonus keep multi-hop reach from
// dominating the score. Depth is informational first: a two-hop chain adds a
// nudge, not a tier flip on its own.
const (
	crossRepoChainBonusPerHop = 4.0
	crossRepoChainMaxBonus    = 8.0
)

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

// RecordCrossRepoChains attaches multi-hop chains to the symbol named
// symbolQN inside (filePath, newStart, newLines), adds a small capped
// "graph"-category Signal for reach beyond direct neighbours, and returns
// whether the symbol was found. It is idempotent: a re-run replaces both the
// chains and the chain signal instead of stacking a second one. The caller
// must call Renormalize afterward.
func RecordCrossRepoChains(report *Report, filePath string, newStart, newLines int, symbolQN string, chains []CrossRepoChain) bool {
	if len(chains) == 0 {
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
				if prior := removeSignalByName(sym, CrossRepoChainSignalName); prior != 0 {
					sym.BlastRadiusRaw -= prior
					hunk.BlastRadiusRaw -= prior
				}
				maxExtraHops := 0
				for _, c := range chains {
					if extra := len(c.Hops) - 1; extra > maxExtraHops {
						maxExtraHops = extra
					}
				}
				sym.ImpactedRepoChains = chains
				if maxExtraHops > 0 {
					bonus := math.Min(float64(maxExtraHops)*crossRepoChainBonusPerHop, crossRepoChainMaxBonus)
					sym.Signals = append(sym.Signals, Signal{
						Name:     CrossRepoChainSignalName,
						Detail:   crossRepoChainDetail(chains),
						Points:   bonus,
						Category: "graph",
					})
					sym.BlastRadiusRaw += bonus
					hunk.BlastRadiusRaw += bonus
				}
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

// removeSignalByName is removeCrossRepoSignal generalized for the chain
// signal. Kept separate so the direct-impact path continues to match exactly
// one name and cannot accidentally strip the chain bonus.
func removeSignalByName(sym *SymbolContribution, name string) float64 {
	var prior float64
	kept := sym.Signals[:0]
	for _, s := range sym.Signals {
		if s.Name == name {
			prior += s.Points
			continue
		}
		kept = append(kept, s)
	}
	sym.Signals = kept
	return prior
}

func crossRepoChainDetail(chains []CrossRepoChain) string {
	longest := 0
	repos := map[string]bool{}
	for _, c := range chains {
		if len(c.Hops) > longest {
			longest = len(c.Hops)
		}
		for _, h := range c.Hops {
			repos[crossRepoDisplayName(CrossRepoImpact{RelatedRepo: h.RelatedRepo, RelatedRepoName: h.RelatedRepoName})] = true
		}
	}
	if longest <= 1 {
		return "Reached one related repo"
	}
	if len(repos) == 1 {
		for repo := range repos {
			return fmt.Sprintf("Reached %s through %d hops", repo, longest)
		}
	}
	return fmt.Sprintf("Reached %d related repos through %d hops", len(repos), longest)
}

func crossRepoDetail(impacts []CrossRepoImpact) string {
	if len(impacts) == 1 {
		return "Called from " + crossRepoDisplayName(impacts[0])
	}
	repos := map[string]bool{}
	for _, im := range impacts {
		repos[crossRepoDisplayName(im)] = true
	}
	if len(repos) == 1 {
		for repo := range repos {
			return "Called from " + repo
		}
	}
	return "Called from related repos"
}

// crossRepoDisplayName prefers the LiveReview repository name over the
// engine's project id, so older artifacts (which only have RelatedRepo) still
// render something and new ones show "org/service".
func crossRepoDisplayName(im CrossRepoImpact) string {
	if im.RelatedRepoName != "" {
		return im.RelatedRepoName
	}
	return im.RelatedRepo
}

// ChainSignature is a stable key for deduping chains at the hunk level.
func ChainSignature(c CrossRepoChain) string {
	out := ""
	for _, h := range c.Hops {
		out += h.RelatedRepo + "|" + h.CallSite + "|" + h.Handler + ";"
	}
	return out
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
			hunk.ImpactedRepoChains = nil
			seenChains := map[string]bool{}
			for _, sym := range hunk.Symbols {
				hunk.ImpactedRelatedRepos = append(hunk.ImpactedRelatedRepos, sym.ImpactedRelatedRepos...)
				for _, chain := range sym.ImpactedRepoChains {
					sig := ChainSignature(chain)
					if seenChains[sig] {
						continue
					}
					seenChains[sig] = true
					hunk.ImpactedRepoChains = append(hunk.ImpactedRepoChains, chain)
				}
			}
		}
	}
}
