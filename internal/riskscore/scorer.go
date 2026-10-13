package riskscore

import (
	"context"
	"database/sql"
	"sync"
)

// The scorer keeps each org's active weights (and category overrides) cached in
// memory; a scoring call is a single dot product with no DB round-trip. The
// cache is invalidated whenever a new version activates (online nudge, batch
// refit) or an override is saved, via Invalidate.

type cachedOrg struct {
	version     int
	sampleCount int
	weights     Weights
	overrides   map[string]float64
}

var (
	cacheMu sync.Mutex
	cache   = map[int64]cachedOrg{}
)

// Invalidate drops the cached model for orgID so the next ScoreFor reloads it.
func Invalidate(orgID int64) {
	cacheMu.Lock()
	delete(cache, orgID)
	cacheMu.Unlock()
}

// Scorer scores feature vectors against the org's active model. A thin
// interface wrapper around ScoreFor keeps the door open for an ONNX-backed
// implementation later (see docs/risk-policy-engine.md §3 ONNX note).
type Scorer struct {
	db    *sql.DB
	store *Store
}

// NewScorer returns a Scorer backed by db.
func NewScorer(db *sql.DB) *Scorer { return &Scorer{db: db, store: NewStore(db)} }

// ModelInfo is the "model confidence" signal surfaced in the UI: which version
// is active and how many labeled outcomes trained it (0 = common-sense seed).
type ModelInfo struct {
	Version         int  `json:"version"`
	TrainingSamples int  `json:"training_samples"`
	IsSeed          bool `json:"is_seed"`
}

// Info returns the active model's version and training-sample count without
// scoring, for the settings UI's model-confidence indicator.
func (s *Scorer) Info(ctx context.Context, orgID int64) (ModelInfo, error) {
	cacheMu.Lock()
	c, ok := cache[orgID]
	cacheMu.Unlock()
	if ok {
		return ModelInfo{Version: c.version, TrainingSamples: c.sampleCount, IsSeed: c.sampleCount == 0}, nil
	}
	// ActiveWeights lazily creates the shared seed model if it doesn't exist yet.
	version, _, sampleCount, err := s.store.ActiveWeights(ctx, orgID)
	if err != nil {
		return ModelInfo{}, err
	}
	return ModelInfo{Version: version, TrainingSamples: sampleCount, IsSeed: sampleCount == 0}, nil
}

// ScoreFor computes sigmoid(Σ effective_w · x) against the org's active model,
// loading and caching it on first use.
func (s *Scorer) ScoreFor(ctx context.Context, orgID int64, x []float64) (float64, error) {
	cacheMu.Lock()
	c, ok := cache[orgID]
	cacheMu.Unlock()
	if ok {
		return Score(c.weights, x, c.overrides), nil
	}

	if err := s.store.EnsureSeedModel(ctx); err != nil {
		return 0, err
	}
	version, weights, sampleCount, err := s.store.ActiveWeights(ctx, orgID)
	if err != nil {
		return 0, err
	}
	overrides, err := s.store.LoadOverrides(ctx, orgID)
	if err != nil {
		return 0, err
	}

	cacheMu.Lock()
	cache[orgID] = cachedOrg{version: version, sampleCount: sampleCount, weights: weights, overrides: overrides}
	cacheMu.Unlock()

	return Score(weights, x, overrides), nil
}
