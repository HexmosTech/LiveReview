package blastradius

// blastCategories/priorityCategories mirror BlastRadiusPanel.tsx's
// BLAST_CATEGORIES/PRIORITY_CATEGORIES exactly - this is the one rule that
// decides which dimension a signal feeds. A signal in neither set (e.g.
// "diff-shape") contributes to neither subtotal.
var blastCategories = map[string]bool{"architecture": true, "graph": true}
var priorityCategories = map[string]bool{"duplication": true, "code-metrics": true}

// SubtotalLine is one symbol's (or the hunk's own file-level) contribution
// to a dimension - the "Name: sig + sig + sig = subtotal" line in Math Mode.
type SubtotalLine struct {
	Name     string   `json:"name"`
	Signals  []Signal `json:"signals"`
	Subtotal float64  `json:"subtotal"`
}

// DimensionBreakdown is one dimension's (Blast Radius or Review Priority)
// full derivation for a hunk, mirroring renderMathDimension's Steps.
// StepSum is 0 when that step is omitted (only one part contributed - see
// the package doc comment's "dynamic step numbering" note).
type DimensionBreakdown struct {
	Title               string         `json:"title"`
	PerSymbol           []SubtotalLine `json:"perSymbol"`
	HunkSignals         []Signal       `json:"hunkSignals"`
	HunkSignalsSubtotal float64        `json:"hunkSignalsSubtotal"`
	Total               float64        `json:"total"` // == the hunk's own *Raw field; what StepScale actually divides
	Max                 float64        `json:"max"`
	Norm                float64        `json:"norm"`
	MaxSourceFile       string         `json:"maxSourceFile"`
	MaxSourceHeader     string         `json:"maxSourceHeader"`
	IsSelf              bool           `json:"isSelf"`
	StepAdd             int            `json:"stepAdd"`
	StepSum             int            `json:"stepSum"`
	StepScale           int            `json:"stepScale"`
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

// dimensionBreakdown computes one dimension's Steps, mirroring
// renderMathDimension in BlastRadiusPanel.tsx exactly - including the
// dynamic step numbering (the "add every subtotal together" step only
// exists when more than one part contributed).
func dimensionBreakdown(
	startStep int,
	title string,
	h HunkReport,
	categories map[string]bool,
	max, norm float64,
	maxSourceFile, maxSourceHeader string,
) (DimensionBreakdown, int) {
	var perSymbol []SubtotalLine
	for _, sym := range h.Symbols {
		sigs := filterByCategory(sym.Signals, categories)
		if len(sigs) == 0 {
			continue
		}
		name := sym.Name
		if name == "" {
			name = shortName(sym.QualifiedName)
		}
		perSymbol = append(perSymbol, SubtotalLine{Name: name, Signals: sigs, Subtotal: sumPoints(sigs)})
	}
	hunkSigs := filterByCategory(h.Signals, categories)
	hunkSubtotal := sumPoints(hunkSigs)

	partCount := len(perSymbol)
	if len(hunkSigs) > 0 {
		partCount++
	}
	hasMultipleParts := partCount > 1

	n := startStep
	stepAdd := n
	n++
	stepSum := 0
	if hasMultipleParts {
		stepSum = n
		n++
	}
	stepScale := n
	n++

	var total float64
	if title == "Blast Radius" {
		total = h.BlastRadiusRaw
	} else {
		total = h.ReviewPriorityRaw
	}

	return DimensionBreakdown{
		Title:               title,
		PerSymbol:           perSymbol,
		HunkSignals:         hunkSigs,
		HunkSignalsSubtotal: hunkSubtotal,
		Total:               total,
		Max:                 max,
		Norm:                norm,
		MaxSourceFile:       maxSourceFile,
		MaxSourceHeader:     maxSourceHeader,
		IsSelf:              maxSourceFile == h.FilePath && maxSourceHeader == h.Header,
		StepAdd:             stepAdd,
		StepSum:             stepSum,
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
		h.MaxBlastRadiusRaw, h.BlastRadiusNorm,
		h.MaxBlastRadiusHunkFile, h.MaxBlastRadiusHunkHeader)
	priority, afterPriority := dimensionBreakdown(afterBlast, "Review Priority", h, priorityCategories,
		h.MaxReviewPriorityRaw, h.ReviewPriorityNorm,
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
