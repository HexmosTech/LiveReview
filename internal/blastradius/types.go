// Package blastradius ports the scoring/signals side of git-lrc's blast
// radius report - tiering, category-to-dimension splitting, and the full
// Math Mode step-by-step derivation - to Go, computed at artifact-upload
// time and stored in Postgres (storage/blastradius, blast_radius_hunks) so
// it's queryable via plain SQL (Livi chat included) instead of living only
// in an opaque S3 blob. The diff viewer
// (ui/src/components/reviews/diffviewer/BlastRadiusPanel.tsx) still
// recomputes everything itself client-side, unchanged - this package's
// output isn't wired into that read path. See
// docs/blast-radius-backend-port-plan.md for the full design and the
// numbers this package's tests were verified against.
//
// Deliberately NOT ported: Sunburst/Flamegraph's call-graph tree
// (buildHierarchy/groupCallers) - it still renders as SVG client-side
// either way and needs the full Callers/Path graph regardless, so porting
// the tree-building buys nothing. Types below still carry Callers/Path so
// the raw artifact round-trips losslessly through this package, but nothing
// here reads them.
package blastradius

import "encoding/json"

// Signal is one scored input ("Caller reach", "Test coverage", ...). Category
// is the field that determines which dimension (Blast Radius vs Review
// Priority) it contributes to - see blastCategories/priorityCategories in
// mathmode.go. Nothing else in the artifact encodes that split.
type Signal struct {
	Name     string  `json:"Name"`
	Detail   string  `json:"Detail,omitempty"`
	Points   float64 `json:"Points"`
	Category string  `json:"Category"`
}

// CallerRef is one caller of a symbol, up to 3 hops away. Round-tripped for
// fidelity but not read by this package (see the package doc comment).
type CallerRef struct {
	QualifiedName string   `json:"QualifiedName"`
	Depth         int      `json:"Depth"`
	Weight        float64  `json:"Weight"`
	Path          []string `json:"Path,omitempty"`
	PreRename     bool     `json:"PreRename,omitempty"`
}

// CrossRepoImpact is one cross-repo call linking a symbol this hunk touches
// to a route/handler in a related repo (see docs/multi-repo-blast-radius-plan.md).
// Populated by the cross_repo_link job, never by git-lrc - it's additive to
// the report git-lrc already uploads, not part of its own scoring.
type CrossRepoImpact struct {
	// RelatedRepo is the engine project id the edge points into. It predates
	// RelatedRepoName and is still emitted by the engine, so it stays for
	// backward compatibility (older artifacts and the cross-repo tests).
	RelatedRepo string `json:"RelatedRepo"`
	// RelatedRepoID/RelatedRepoName/RelatedRepoWebURL are LiveReview's own
	// repositories row for the target. The UI prefers these over the engine
	// project id so a chip can show "org/service" and link to the repository
	// instead of rendering an index-cache path.
	RelatedRepoID     int64   `json:"RelatedRepoID,omitempty"`
	RelatedRepoName   string  `json:"RelatedRepoName,omitempty"`
	RelatedRepoWebURL string  `json:"RelatedRepoWebURL,omitempty"`
	CallSite          string  `json:"CallSite"` // the matched route/handler qualified name
	EdgeType          string  `json:"EdgeType"` // CROSS_HTTP_CALLS, CROSS_ASYNC_CALLS, ...
	Confidence        float64 `json:"Confidence,omitempty"`
	// MatchKind records how the edge was found: "exact" for the engine's own
	// cross-repo pass, "version-alias" when LiveReview matched a versioned
	// call site (/v1/orders) to an unversioned route (/orders) because the
	// parser drops NestJS/Spring URI version segments.
	MatchKind string `json:"MatchKind,omitempty"`
}

// CrossRepoHop is one link in a multi-repo impact chain. Hops[0] is the first
// related repo reached from the reviewed repo; Hops[1] is the repo that repo
// calls, and so on.
type CrossRepoHop struct {
	RelatedRepo       string `json:"RelatedRepo"`
	RelatedRepoID     int64  `json:"RelatedRepoID,omitempty"`
	RelatedRepoName   string `json:"RelatedRepoName,omitempty"`
	RelatedRepoWebURL string `json:"RelatedRepoWebURL,omitempty"`
	CallSite          string `json:"CallSite"` // route qualified name in this hop's repo
	Handler           string `json:"Handler,omitempty"`
	EdgeType          string `json:"EdgeType"`
}

// CrossRepoChain is a path of cross-repo calls reached from one touched
// symbol, e.g. reviewed repo -> service B -> service C. It is informational
// (the direct hop already contributes the main Cross-repo impact signal) and
// rendered behind a collapsed "Extended impact chain" affordance.
type CrossRepoChain struct {
	Hops []CrossRepoHop `json:"Hops"`
}

// SymbolContribution is one symbol (function/type/...) touched by a hunk,
// with its own signal list and pre-summed subtotals.
type SymbolContribution struct {
	QualifiedName        string            `json:"QualifiedName"`
	Name                 string            `json:"Name"`
	Label                string            `json:"Label,omitempty"`
	Method               string            `json:"Method,omitempty"`
	Signals              []Signal          `json:"Signals,omitempty"`
	BlastRadiusRaw       float64           `json:"BlastRadiusRaw"`
	ReviewPriorityRaw    float64           `json:"ReviewPriorityRaw"`
	DirectCount          int               `json:"DirectCount,omitempty"`
	TransitiveCount      int               `json:"TransitiveCount,omitempty"`
	Callers              []CallerRef       `json:"Callers,omitempty"`
	RenamedFrom          string            `json:"RenamedFrom,omitempty"`
	ImpactedPackages     []string          `json:"ImpactedPackages,omitempty"`
	ImpactedRepoChains   []CrossRepoChain  `json:"ImpactedRepoChains,omitempty"`
	MethodBlastRadius    float64           `json:"MethodBlastRadius,omitempty"`
	IsEntryPoint         bool              `json:"IsEntryPoint,omitempty"`
	Complexity           float64           `json:"Complexity,omitempty"`
	Cognitive            float64           `json:"Cognitive,omitempty"`
	LoopDepth            int               `json:"LoopDepth,omitempty"`
	OutDegree            int               `json:"OutDegree,omitempty"`
	TestCount            int               `json:"TestCount,omitempty"`
	ImpactedRelatedRepos []CrossRepoImpact `json:"ImpactedRelatedRepos,omitempty"`
}

// Weights blends the two normalized dimension scores into Combined - see
// HunkReport.Weights (default {0.6, 0.4}, but each hunk carries its own).
type Weights struct {
	BlastRadius    float64 `json:"BlastRadius"`
	ReviewPriority float64 `json:"ReviewPriority"`
}

// HunkReport is one hunk's full scoring report - the unit ComputeMathMode
// and Tier operate on. Field names/shape match ui/src/types/reviews.ts's
// BlastRadiusHunkReport exactly, so the raw JSON git-lrc uploads unmarshals
// directly with no field renaming.
type HunkReport struct {
	FilePath                    string   `json:"FilePath"`
	Header                      string   `json:"Header"`
	NewStart                    int      `json:"NewStart"`
	NewLines                    int      `json:"NewLines"`
	Content                     string   `json:"Content,omitempty"`
	Signals                     []Signal `json:"Signals,omitempty"`
	BlastRadiusRaw              float64  `json:"BlastRadiusRaw"`
	BlastRadiusNorm             float64  `json:"BlastRadiusNorm"`
	MaxBlastRadiusRaw           float64  `json:"MaxBlastRadiusRaw"`
	MaxBlastRadiusHunkFile      string   `json:"MaxBlastRadiusHunkFile,omitempty"`
	MaxBlastRadiusHunkHeader    string   `json:"MaxBlastRadiusHunkHeader,omitempty"`
	ReviewPriorityRaw           float64  `json:"ReviewPriorityRaw"`
	ReviewPriorityNorm          float64  `json:"ReviewPriorityNorm"`
	MaxReviewPriorityRaw        float64  `json:"MaxReviewPriorityRaw"`
	MaxReviewPriorityHunkFile   string   `json:"MaxReviewPriorityHunkFile,omitempty"`
	MaxReviewPriorityHunkHeader string   `json:"MaxReviewPriorityHunkHeader,omitempty"`
	Combined                    float64  `json:"Combined"`
	// Pointer, not float64: a real HygieneMultiplier of 0 (fully suppressed)
	// must be distinguishable from the field being absent. JS's fallback is
	// `typeof detail.HygieneMultiplier === 'number' ? ... : 1.0`, which keeps
	// a real 0 - a plain float64 here couldn't tell "absent" from "present as
	// zero" and would wrongly default absent-vs-zero the same way.
	HygieneMultiplier *float64             `json:"HygieneMultiplier"`
	Weights           Weights              `json:"Weights"`
	Symbols           []SymbolContribution `json:"Symbols,omitempty"`
	// SymbolCombine is the roll-up formula the generator used:
	// "sqrt" (git-lrc since 2026-10-01) or "linear" (older artifacts, field
	// absent). ComputeMathMode uses it to render the same arithmetic; when
	// absent, it infers the mode from the recorded raw score.
	SymbolCombine     string   `json:"SymbolCombine,omitempty"`
	ImpactedPackages  []string `json:"ImpactedPackages,omitempty"`
	FileCouplingBonus float64  `json:"FileCouplingBonus,omitempty"`
	// ImpactedRelatedRepos rolls up every Symbols[].ImpactedRelatedRepos entry
	// for this hunk (set by AddCrossRepoImpact), so the diff-viewer panel can
	// render a hunk-level chip list the same way it already does for
	// ImpactedPackages, without reading into Symbols itself.
	ImpactedRelatedRepos []CrossRepoImpact `json:"ImpactedRelatedRepos,omitempty"`
	// ImpactedRepoChains rolls up every Symbols[].ImpactedRepoChains entry
	// (deduped by hop signature) so the hunk-level panel can offer the same
	// extended-chain expansion without reading into Symbols.
	ImpactedRepoChains []CrossRepoChain `json:"ImpactedRepoChains,omitempty"`
}

// FileReport is one file's hunks.
type FileReport struct {
	Path  string       `json:"Path"`
	Hunks []HunkReport `json:"Hunks"`
}

// Report is the full artifact git-lrc uploads via PutDiffReviewArtifact -
// unmarshal the raw blob directly into this.
type Report struct {
	Project          string          `json:"Project"`
	GeneratedAt      string          `json:"GeneratedAt"`
	Files            []FileReport    `json:"Files"`
	ImpactedPackages json.RawMessage `json:"ImpactedPackages,omitempty"`
	// CrossRepoDiagnostics records best-effort enrichment problems (engine,
	// graph or target failures). It is diagnostic only; the UI ignores it.
	// Keeping it in the artifact makes a silent no-op visible without server
	// log access.
	CrossRepoDiagnostics []string `json:"CrossRepoDiagnostics,omitempty"`
}
