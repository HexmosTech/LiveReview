package blastradius

import (
	"math"
	"strings"
)

// blastCategories/priorityCategories mirror BlastRadiusPanel.tsx's
// BLAST_CATEGORIES/PRIORITY_CATEGORIES exactly - this is the one rule that
// decides which dimension a signal feeds. A signal in neither set (e.g.
// "diff-shape") contributes to neither subtotal.
var blastCategories = map[string]bool{"architecture": true, "graph": true}
var priorityCategories = map[string]bool{"duplication": true, "code-metrics": true}

// SymbolCombine names the roll-up a report's generator used to turn
// per-symbol contributions into the hunk's raw score:
//
//   - "linear": sum(symbols) + hunk signals (reports before 2026-10-01)
//   - "sqrt":   sqrt(sum(symbols)) + hunk signals (git-lrc's current formula)
//
// Older artifacts carry no marker; ComputeMathMode infers the mode from the
// recorded raw score so historical reviews keep showing the arithmetic they
// were generated with. New reports set the field explicitly.
const (
	SymbolCombineLinear = "linear"
	SymbolCombineSqrt   = "sqrt"
)

// SubtotalLine is one symbol's (or the hunk's own file-level) contribution
// to a dimension - the "Name: sig + sig + sig = subtotal" line in Math Mode.
type SubtotalLine struct {
	Name     string   `json:"name"`
	Signals  []Signal `json:"signals"`
	Subtotal float64  `json:"subtotal"`
}

// DimensionBreakdown is one dimension's (Blast Radius or Review Priority)
// full derivation for a hunk, mirroring renderMathDimension's Steps.
//
// StepSum/StepCombine/StepHunk/StepAdjust are 0 when that step is omitted -
// the "dynamic step numbering" rule. Linear artifacts sum every subtotal in
// one step; saturating artifacts show sum -> sqrt -> add file-level signals,
// which is what the backend actually computes.
type DimensionBreakdown struct {
	Title string `json:"title"`
	// Mode is "linear" or "sqrt" - the formula the steps below render.
	Mode string `json:"mode"`
	// DeclaredCombine is the report's SymbolCombine value (empty on old
	// artifacts, where Mode was inferred).
	DeclaredCombine     string         `json:"declaredCombine,omitempty"`
	PerSymbol           []SubtotalLine `json:"perSymbol"`
	HunkSignals         []Signal       `json:"hunkSignals"`
	HunkSignalsSubtotal float64        `json:"hunkSignalsSubtotal"`
	SymbolSubtotal      float64        `json:"symbolSubtotal"`
	CombinedSymbols     float64        `json:"combinedSymbols"`
	// Total is the formula's result before any post-scoring adjustment.
	Total float64 `json:"total"`
	// Raw is the hunk's recorded *Raw field - what StepScale divides. They
	// differ only when a post-scoring bonus (cross-repo impact) landed after
	// the roll-up; StepAdjust explains the difference.
	Raw           float64 `json:"raw"`
	ArtifactDelta float64 `json:"artifactDelta"`
	Max           float64 `json:"max"`
	// MaxInferred is true for older artifacts that recorded Norm but not the
	// raw maximum it was divided by. The panel then says the divisor was
	// recovered from the rank instead of claiming no hunk scored above zero.
	MaxInferred     bool    `json:"maxInferred,omitempty"`
	Norm            float64 `json:"norm"`
	MaxSourceFile   string  `json:"maxSourceFile"`
	MaxSourceHeader string  `json:"maxSourceHeader"`
	IsSelf          bool    `json:"isSelf"`
	StepAdd         int     `json:"stepAdd"`
	StepSum         int     `json:"stepSum"`
	StepCombine     int     `json:"stepCombine"`
	StepHunk        int     `json:"stepHunk"`
	StepAdjust      int     `json:"stepAdjust"`
	StepScale       int     `json:"stepScale"`
}

// MathMode is a hunk's complete Math Mode derivation - both dimensions plus
// the final blend/hygiene combine section. Final must always equal the
// hunk's own Combined field; a test asserts this invariant.
type MathMode struct {
	BlastRadius       DimensionBreakdown `json:"blastRadius"`
	ReviewPriority    DimensionBreakdown `json:"reviewPriority"`
	Weights           Weights            `json:"weights"`
	BlastShare        float64            `json:"blastShare"`
	PriorityShare     float64            `json:"priorityShare"`
	Blended           float64            `json:"blended"`
	HygieneMultiplier float64            `json:"hygieneMultiplier"`
	Final             float64            `json:"final"`
	StepBlend         int                `json:"stepBlend"`
	StepHygiene       int                `json:"stepHygiene"`
}

// effectiveWeights mirrors MathModeView's fallback: `detail.Weights &&
// typeof detail.Weights.BlastRadius === 'number' ? detail.Weights : {0.6,
// 0.4}`. Go's JSON unmarshal can't distinguish "field absent" from "present
// as zero" for a plain float64, so this treats an all-zero Weights (which
// every real artifact avoids - a blend of nothing/nothing is meaningless
// anyway) as "absent" and substitutes the default.
func effectiveWeights(w Weights) Weights {
	if w.BlastRadius == 0 && w.ReviewPriority == 0 {
		return Weights{BlastRadius: 0.6, ReviewPriority: 0.4}
	}
	return w
}

// effectiveHygiene mirrors the frontend's `typeof detail.HygieneMultiplier
// === 'number' ? detail.HygieneMultiplier : 1.0` exactly: nil (field absent
// from the JSON) defaults to 1.0, but a real 0 (fully suppressed) is kept -
// HunkReport.HygieneMultiplier is a *float64 specifically so this
// distinction is possible (see its doc comment).
func effectiveHygiene(h *float64) float64 {
	if h == nil {
		return 1.0
	}
	return *h
}

func sumPoints(signals []Signal) float64 {
	total := 0.0
	for _, s := range signals {
		total += s.Points
	}
	return total
}

func filterByCategory(signals []Signal, categories map[string]bool) []Signal {
	var out []Signal
	for _, s := range signals {
		if categories[s.Category] {
			out = append(out, s)
		}
	}
	return out
}

// detectCombineMode returns the roll-up formula and the total it produces,
// plus how far that total is from the artifact's recorded raw score.
// `declared` wins when present; otherwise the recorded score is the ground
// truth and the formula that reproduces it is chosen. The two formulas
// differ by far more than rounding on every real multi-point hunk, so the
// fit is unambiguous.
func detectCombineMode(declared string, symbolSubtotal, crossRepoBonus, hunkSubtotal, raw float64) (mode string, total, delta float64) {
	linear := symbolSubtotal + hunkSubtotal
	saturating := math.Sqrt(symbolSubtotal) + hunkSubtotal
	switch strings.ToLower(strings.TrimSpace(declared)) {
	case SymbolCombineSqrt:
		return SymbolCombineSqrt, saturating, raw - saturating
	case SymbolCombineLinear:
		return SymbolCombineLinear, linear, raw - linear
	}
	// Cross-repo enrichment (RecordCrossRepoImpact/Chains) adds its bonus
	// after the roll-up, to both the symbol sum and the recorded raw score.
	// Reconstruct what the sqrt formula would have produced before that
	// bonus so a historical cross-repo artifact is not mistaken for linear.
	base := symbolSubtotal - crossRepoBonus
	if base < 0 {
		base = 0
	}
	saturatingWithBonus := math.Sqrt(base) + hunkSubtotal + crossRepoBonus
	saturatingDelta := math.Abs(raw - saturating)
	if adjusted := math.Abs(raw - saturatingWithBonus); adjusted < saturatingDelta {
		saturatingDelta = adjusted
	}
	if saturatingDelta+1e-9 < math.Abs(raw-linear) {
		return SymbolCombineSqrt, saturating, raw - saturating
	}
	return SymbolCombineLinear, linear, raw - linear
}

// effectiveMax recovers the normalization divisor for older artifacts that
// recorded Norm but not Max.
func effectiveMax(raw, max, norm float64) (value float64, inferred bool) {
	if max > 0 {
		return max, false
	}
	if raw > 0 && norm > 0 {
		return raw / (norm / 100), true
	}
	return 0, false
}

// dimensionBreakdown computes one dimension's Steps, mirroring
// renderMathDimension in BlastRadiusPanel.tsx exactly - including the
// dynamic step numbering (the "add every subtotal together" step only
// exists when more than one part contributed).
func dimensionBreakdown(
	startStep int,
	title string,
	h HunkReport,
	categories map[string]bool,
	raw, max, norm float64,
	maxSourceFile, maxSourceHeader string,
) (DimensionBreakdown, int) {
	var perSymbol []SubtotalLine
	symbolSubtotal := 0.0
	for _, sym := range h.Symbols {
		sigs := filterByCategory(sym.Signals, categories)
		if len(sigs) == 0 {
			continue
		}
		name := sym.Name
		if name == "" {
			name = shortName(sym.QualifiedName)
		}
		subtotal := sumPoints(sigs)
		symbolSubtotal += subtotal
		perSymbol = append(perSymbol, SubtotalLine{Name: name, Signals: sigs, Subtotal: subtotal})
	}
	hunkSigs := filterByCategory(h.Signals, categories)
	hunkSubtotal := sumPoints(hunkSigs)

	crossRepoBonus := 0.0
	if categories["graph"] {
		for _, sym := range h.Symbols {
			for _, sig := range sym.Signals {
				if sig.Name == CrossRepoSignalName || sig.Name == CrossRepoChainSignalName {
					crossRepoBonus += sig.Points
				}
			}
		}
	}

	mode, total, delta := detectCombineMode(h.SymbolCombine, symbolSubtotal, crossRepoBonus, hunkSubtotal, raw)
	combinedSymbols := math.Sqrt(symbolSubtotal)

	n := startStep
	stepAdd := n
	n++
	stepSum := 0
	switch mode {
	case SymbolCombineLinear:
		parts := len(perSymbol)
		if len(hunkSigs) > 0 {
			parts++
		}
		if parts > 1 {
			stepSum = n
			n++
		}
	default:
		if len(perSymbol) > 1 {
			stepSum = n
			n++
		}
	}
	stepCombine := 0
	if mode == SymbolCombineSqrt && symbolSubtotal > 0 {
		stepCombine = n
		n++
	}
	stepHunk := 0
	if mode == SymbolCombineSqrt && len(hunkSigs) > 0 {
		stepHunk = n
		n++
	}
	stepAdjust := 0
	if math.Abs(delta) > 0.05 {
		stepAdjust = n
		n++
	}
	stepScale := n
	n++

	effectiveMaxValue, maxInferred := effectiveMax(raw, max, norm)
	isSelf := max > 0 && maxSourceFile == h.FilePath && maxSourceHeader == h.Header

	return DimensionBreakdown{
		Title:               title,
		Mode:                mode,
		DeclaredCombine:     h.SymbolCombine,
		PerSymbol:           perSymbol,
		HunkSignals:         hunkSigs,
		HunkSignalsSubtotal: hunkSubtotal,
		SymbolSubtotal:      symbolSubtotal,
		CombinedSymbols:     combinedSymbols,
		Total:               total,
		Raw:                 raw,
		ArtifactDelta:       delta,
		Max:                 effectiveMaxValue,
		MaxInferred:         maxInferred,
		Norm:                norm,
		MaxSourceFile:       maxSourceFile,
		MaxSourceHeader:     maxSourceHeader,
		IsSelf:              isSelf,
		StepAdd:             stepAdd,
		StepSum:             stepSum,
		StepCombine:         stepCombine,
		StepHunk:            stepHunk,
		StepAdjust:          stepAdjust,
		StepScale:           stepScale,
	}, n
}

// ComputeMathMode ports MathModeView + renderMathDimension exactly: the
// full step-by-step derivation of Combined from h's raw signal data. Emits
// raw float64s, never pre-formatted strings: Go's %.1f and JS's toFixed(1)
// round ties differently (banker's rounding vs half-away-from-zero), and
// this package has no frontend consumer to format for anyway.
func ComputeMathMode(h HunkReport) MathMode {
	weights := effectiveWeights(h.Weights)
	hygiene := effectiveHygiene(h.HygieneMultiplier)

	blast, afterBlast := dimensionBreakdown(1, "Blast Radius", h, blastCategories,
		h.BlastRadiusRaw, h.MaxBlastRadiusRaw, h.BlastRadiusNorm,
		h.MaxBlastRadiusHunkFile, h.MaxBlastRadiusHunkHeader)
	priority, afterPriority := dimensionBreakdown(afterBlast, "Review Priority", h, priorityCategories,
		h.ReviewPriorityRaw, h.MaxReviewPriorityRaw, h.ReviewPriorityNorm,
		h.MaxReviewPriorityHunkFile, h.MaxReviewPriorityHunkHeader)

	blastShare := weights.BlastRadius * h.BlastRadiusNorm
	priorityShare := weights.ReviewPriority * h.ReviewPriorityNorm
	blended := blastShare + priorityShare
	final := blended * hygiene

	return MathMode{
		BlastRadius:       blast,
		ReviewPriority:    priority,
		Weights:           weights,
		BlastShare:        blastShare,
		PriorityShare:     priorityShare,
		Blended:           blended,
		HygieneMultiplier: hygiene,
		Final:             final,
		StepBlend:         afterPriority,
		StepHygiene:       afterPriority + 1,
	}
}

// shortName mirrors ui/src/lib/blastRadius.ts's shortName: the last
// dot-separated segment of a qualified name, or the input unchanged if it
// has none.
func shortName(qualifiedName string) string {
	for i := len(qualifiedName) - 1; i >= 0; i-- {
		if qualifiedName[i] == '.' {
			return qualifiedName[i+1:]
		}
	}
	return qualifiedName
}
