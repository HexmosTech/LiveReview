package riskscore

import (
	"context"
	"database/sql"
	"sort"
)

// Percentile calibration turns a per-org risk_threshold_percentile into an
// actual score cutoff: the org's own recent score distribution defines "P20",
// not a global magic number. The curve is org-wide (one model per org), while
// the percentile read off it is repo-overridable.

// defaultCutoff is the cold-start threshold (no scores yet): 0.5, i.e. block
// reviews whose risk score is at or above the neutral midpoint. With the seed
// prior (bias -0.5), a clean review scores ~0.38 (approves) and one critical
// finding scores ~0.6 (blocks).
const defaultCutoff = 0.5

// minScoresForPercentile is the smallest historical sample PercentileCutoff
// will rank against. Below it, the nearest-rank index (p*n+99)/100-1 can land
// at or near the top of a tiny sorted array for any p, including a "strict"
// low percentile like 20 — producing a cutoff that collapses onto the
// distribution's max (often the very review being scored), which reads as a
// contradiction (e.g. "P100" score tying a "P20" cutoff). Too few points to
// rank meaningfully falls back to the same cold-start default as no history.
const minScoresForPercentile = 10

// HasEnoughHistory reports whether scores is large enough for PercentileCutoff
// to rank against meaningfully, rather than falling back to defaultCutoff. The
// UI should hide P-number labels when this is false — a tiny sample can't
// support a "P20" claim that means anything.
func HasEnoughHistory(scores []float64) bool {
	return len(scores) >= minScoresForPercentile
}

// LoadRecentScores returns the org's most recent risk scores (newest first),
// from risk_feature_snapshots, limited to limit rows.
func LoadRecentScores(ctx context.Context, db *sql.DB, orgID int64, limit int) ([]float64, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx,
		`SELECT score FROM risk_feature_snapshots WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scores := make([]float64, 0, limit)
	for rows.Next() {
		var s float64
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// PercentileCutoff returns the score value at the p-th percentile of the
// distribution (nearest-rank). A review blocks when its score >= this value.
// Empty distribution returns defaultCutoff.
func PercentileCutoff(scores []float64, p int) float64 {
	if len(scores) < minScoresForPercentile {
		return defaultCutoff
	}
	if p <= 0 {
		p = 1
	}
	if p > 100 {
		p = 100
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)

	// Nearest-rank method: index = ceil(p/100 * n) - 1, clamped.
	idx := (p*len(sorted)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// PercentileOf returns the percentile rank (0-100) of score within the
// distribution (the percentage of scores <= score), for the UI's
// "risk score 34 (P45)" display.
func PercentileOf(score float64, scores []float64) int {
	if len(scores) == 0 {
		return 0
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)

	le := 0
	for _, s := range sorted {
		if s <= score {
			le++
		}
	}
	return le * 100 / len(sorted)
}
