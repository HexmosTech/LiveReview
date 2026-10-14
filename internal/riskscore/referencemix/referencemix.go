// Package referencemix is test-support code only: it builds one fixed,
// reproducible set of 60 synthetic reviews, shaped like a believable repo's
// recent history (mostly small/clean, a smaller medium-risk group, a few
// clearly risky ones). It exists so more than one test package can score
// the exact same reference distribution and report consistent percentile
// numbers -- internal/riskscore's threshold experiment and
// internal/policyengine's scenario corpus both import this, instead of each
// keeping a private copy that could silently drift apart.
//
// Nothing in internal/riskscore or internal/policyengine's production code
// imports this package. It is only ever used from _test.go files.
package referencemix

import (
	"math"
	"math/rand"
)

// Example is one synthetic review: an optional human-readable label (empty
// for filler rows) and its full feature vector.
type Example struct {
	Label    string
	Features []float64
}

// Build returns the fixed 60-review mix, given featureCount (the caller's
// riskscore.FeatureCount(), passed in rather than imported, so this package
// has no dependency on riskscore and can be imported from any package that
// scores features the same way).
func Build(featureCount int) []Example {
	rng := rand.New(rand.NewSource(7))
	var out []Example

	f := func(brSum, brMax, brWeighted, brHigh, critical, warning, security, files, lines, llmRisk float64) []float64 {
		v := make([]float64, featureCount)
		v[0] = brSum
		v[1] = brMax
		v[2] = brWeighted
		v[3] = brHigh
		v[4] = critical
		v[5] = warning
		v[6] = security
		v[7] = math.Log1p(files)
		v[8] = math.Log1p(lines)
		v[9] = llmRisk
		return v
	}

	// Named, representative examples (the ones the report quotes by name).
	out = append(out,
		Example{"docs-only edit", f(0, 0, 0, 0, 0, 0, 0, 1, 10, 0)},
		Example{"typical two-file bugfix", f(0, 0, 0, 0, 0, 1, 0, 2, 25, 0)},
		Example{"small refactor, one warning", f(5, 5, 5, 0, 0, 1, 0, 4, 60, 0)},
		Example{"medium feature, no findings", f(15, 15, 15, 0, 0, 0, 0, 8, 150, 0)},
		Example{"medium feature, one security warning", f(10, 10, 10, 0, 0, 0, 1, 6, 120, 0)},
		Example{"large refactor, high blast radius", f(70, 70, 70, 1, 0, 0, 0, 20, 500, 0)},
		Example{"one critical security finding", f(0, 0, 0, 0, 1, 0, 1, 2, 30, 1)},
	)

	// Filler: a realistic long tail around the named examples, fixed seed so
	// this is reproducible. ~70% small/clean, ~22% medium, ~8% risky.
	for range 53 {
		r := rng.Float64()
		switch {
		case r < 0.70: // small/clean
			out = append(out, Example{"", f(
				rng.Float64()*5, rng.Float64()*5, rng.Float64()*5, 0,
				0, float64(rng.Intn(2)), 0,
				float64(1+rng.Intn(5)), float64(5+rng.Intn(80)), 0)})
		case r < 0.92: // medium
			out = append(out, Example{"", f(
				10+rng.Float64()*25, 10+rng.Float64()*25, 10+rng.Float64()*25, 0,
				0, float64(rng.Intn(3)), float64(rng.Intn(2)),
				float64(3+rng.Intn(10)), float64(50+rng.Intn(200)), 0)})
		default: // risky
			out = append(out, Example{"", f(
				50+rng.Float64()*40, 50+rng.Float64()*40, 50+rng.Float64()*40, 1,
				float64(rng.Intn(2)), float64(rng.Intn(3)), 1,
				float64(10+rng.Intn(20)), float64(200+rng.Intn(400)), float64(rng.Intn(2)))})
		}
	}
	return out
}
