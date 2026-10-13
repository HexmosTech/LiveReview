package riskscore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// TrainingExample is one labeled (features, risky) pair for model fitting,
// built from risk_feature_snapshots ⋈ review_outcomes.
type TrainingExample struct {
	ReviewID int64
	Features []float64
	Risky    float64 // 1.0 risky (revert/incident), 0.0 safe (confirmed_safe)
}

// LoadTrainingSet returns the org's labeled training examples, excluding
// 'test'-source outcomes so the setup-panel round-trip never pollutes the model.
func (s *Store) LoadTrainingSet(ctx context.Context, orgID int64) ([]TrainingExample, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.review_id, s.features, o.outcome_type
		FROM risk_feature_snapshots s
		JOIN review_outcomes o ON o.review_id = s.review_id AND o.org_id = s.org_id
		WHERE s.org_id = $1 AND o.source <> 'test'
		ORDER BY s.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TrainingExample
	for rows.Next() {
		var reviewID int64
		var raw []byte
		var outcomeType string
		if err := rows.Scan(&reviewID, &raw, &outcomeType); err != nil {
			return nil, err
		}
		var features []float64
		if err := json.Unmarshal(raw, &features); err != nil {
			continue
		}
		out = append(out, TrainingExample{
			ReviewID: reviewID,
			Features: features,
			Risky:    outcomeLabel(outcomeType),
		})
	}
	return out, rows.Err()
}

// outcomeLabel maps an outcome_type to a training label.
func outcomeLabel(outcomeType string) float64 {
	switch outcomeType {
	case "revert", "incident":
		return 1
	default: // confirmed_safe
		return 0
	}
}

// ApplyOutcome is the real-time online nudge (path 1): when a review_outcomes
// row lands, load that review's snapshot as the labeled example and take one
// gradient step on the org's current weights, persisting a new active version.
// source 'test' outcomes are skipped by the caller.
func (s *Store) ApplyOutcome(ctx context.Context, orgID, reviewID int64, outcomeType string) error {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT features FROM risk_feature_snapshots WHERE review_id = $1 AND org_id = $2`,
		reviewID, orgID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil // no snapshot (e.g. policy never scored this review); nothing to learn
	}
	if err != nil {
		return err
	}
	var features []float64
	if err := json.Unmarshal(raw, &features); err != nil {
		return err
	}
	if len(features) != FeatureCount() {
		return fmt.Errorf("feature vector length %d != %d; model drifted", len(features), FeatureCount())
	}

	_, weights, sampleCount, err := s.ActiveWeights(ctx, orgID)
	if err != nil {
		return err
	}

	next := OnlineStep(weights, features, outcomeLabel(outcomeType), Prior(), sampleCount, 0.01)
	_, err = s.SaveVersion(ctx, orgID, next, sampleCount+1, nil, "online_update")
	return err
}

// RefitResult summarizes a batch refit.
type RefitResult struct {
	OrgID         int64   `json:"org_id"`
	TrainingCount int     `json:"training_count"`
	HoldoutAUC    float64 `json:"holdout_auc"`
	Promoted      bool    `json:"promoted"`
	Version       int     `json:"version,omitempty"`
	Reason        string  `json:"reason"`
}

// Refit is the daily/on-demand batch refit (paths 2/3): fit a fresh logistic
// model over all labeled snapshots, evaluate AUC, and promote only if it is at
// least as good as the current active version (or the current version is the
// seed). Corrects drift a long sequence of noisy online steps can accumulate.
func (s *Store) Refit(ctx context.Context, orgID int64) (*RefitResult, error) {
	examples, err := s.LoadTrainingSet(ctx, orgID)
	if err != nil {
		return nil, err
	}
	res := &RefitResult{OrgID: orgID, TrainingCount: len(examples), Promoted: false}
	if len(examples) == 0 {
		res.Reason = "no labeled outcomes yet"
		return res, nil
	}

	X := make([][]float64, 0, len(examples))
	Y := make([]float64, 0, len(examples))
	for _, ex := range examples {
		X = append(X, padFeatures(ex.Features))
		Y = append(Y, ex.Risky)
	}

	trainX, trainY, evalX, evalY := splitHoldout(X, Y)
	fitted := BatchFit(trainX, trainY, Prior(), len(trainX), 200)
	auc := rankAUC(scoreAll(evalX, fitted), evalY)
	res.HoldoutAUC = auc

	_, currentWeights, currentSamples, err := s.ActiveWeights(ctx, orgID)
	if err != nil {
		return nil, err
	}
	currentAUC := rankAUC(scoreAll(evalX, currentWeights), evalY)

	if currentSamples == 0 || auc >= currentAUC {
		version, err := s.NextVersion(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if _, err := s.SaveVersion(ctx, orgID, fitted, len(examples), &auc, "batch_refit"); err != nil {
			return nil, err
		}
		res.Promoted = true
		res.Version = version
		res.Reason = "promoted (AUC improved or replaced seed)"
	} else {
		res.Reason = fmt.Sprintf("not promoted (AUC %.3f < current %.3f)", auc, currentAUC)
	}
	return res, nil
}

// scoreAll scores each row with the given (raw) weights.
func scoreAll(X [][]float64, w Weights) []float64 {
	out := make([]float64, len(X))
	for i := range X {
		out[i] = scoreRaw(w, X[i])
	}
	return out
}

// padFeatures returns a copy of f padded/truncated to FeatureCount().
func padFeatures(f []float64) []float64 {
	out := make([]float64, FeatureCount())
	for i := range out {
		if i < len(f) {
			out[i] = f[i]
		}
	}
	return out
}

// splitHoldout splits X/Y into 80% train / 20% eval when >= 20 samples, else
// returns all data as both train and eval (in-sample AUC).
func splitHoldout(X [][]float64, Y []float64) ([][]float64, []float64, [][]float64, []float64) {
	if len(X) < 20 {
		return X, Y, X, Y
	}
	n := len(X)
	trainN := n * 4 / 5
	return X[:trainN], Y[:trainN], X[trainN:], Y[trainN:]
}

// rankAUC ranks scores against binary labels (pairwise). Returns 0.5 when
// there are no positive or no negative examples.
func rankAUC(scores, labels []float64) float64 {
	nPos, nNeg := 0, 0
	rank := 0.0
	for i, si := range scores {
		if labels[i] == 1 {
			nPos++
			for j, sj := range scores {
				if labels[j] == 0 {
					switch {
					case si > sj:
						rank++
					case si == sj:
						rank += 0.5
					}
				}
			}
		} else {
			nNeg++
		}
	}
	if nPos == 0 || nNeg == 0 {
		return 0.5
	}
	return rank / float64(nPos*nNeg)
}
