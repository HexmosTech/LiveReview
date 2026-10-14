package api

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/policyengine"
	"github.com/livereview/internal/riskscore"
)

// AutoApproveInsightsHandler answers the three questions nothing else in the
// product answers today: "where do my org's real review scores land relative
// to the threshold," "how many real PRs is the policy approving/blocking/
// getting overridden," and "when a decision's outcome was reported, how
// often was it wrong." See the "Auto Approve" Reports tab
// (ui/src/pages/Reports/AutoApproveInsightsReport.tsx) for the consumer.
type AutoApproveInsightsHandler struct {
	db *sql.DB
}

func NewAutoApproveInsightsHandler(db *sql.DB) *AutoApproveInsightsHandler {
	return &AutoApproveInsightsHandler{db: db}
}

type scoreHistogramBucket struct {
	RangeStart float64 `json:"range_start"`
	RangeEnd   float64 `json:"range_end"`
	Count      int     `json:"count"`
}

type scoreDistributionDTO struct {
	Buckets     []scoreHistogramBucket `json:"buckets"`
	Cutoff      float64                `json:"cutoff"`
	Percentile  int                    `json:"percentile"`
	TotalScored int                    `json:"total_scored"`
}

type decisionTrendPoint struct {
	Day      string `json:"day"`
	Approved int    `json:"approved"`
	Blocked  int    `json:"blocked"`
}

type decisionsDTO struct {
	Approved             int                  `json:"approved"`
	BlockedAtEligibility int                  `json:"blocked_at_eligibility"`
	BlockedAtStatic      int                  `json:"blocked_at_static"`
	BlockedAtRisk        int                  `json:"blocked_at_risk"`
	BlockedAtLLM         int                  `json:"blocked_at_llm"`
	Overridden           int                  `json:"overridden"`
	TotalDecided         int                  `json:"total_decided"`
	OverrideRate         float64              `json:"override_rate"`
	Trend                []decisionTrendPoint `json:"trend"`
}

type calibrationCell struct {
	Decision    string `json:"decision"`     // "approved" | "blocked"
	OutcomeType string `json:"outcome_type"` // "revert" | "incident" | "confirmed_safe"
	Count       int    `json:"count"`
}

type calibrationDTO struct {
	Cells            []calibrationCell `json:"cells"`
	TotalOutcomes    int               `json:"total_outcomes"`
	FalseApproveRate *float64          `json:"false_approve_rate,omitempty"`
	LowVolumeWarning bool              `json:"low_volume_warning"`
}

type autoApproveInsightsDTO struct {
	WindowDays        int                  `json:"window_days"`
	ScoreDistribution scoreDistributionDTO `json:"score_distribution"`
	Decisions         decisionsDTO         `json:"decisions"`
	Calibration       calibrationDTO       `json:"calibration"`
}

// lowVolumeThreshold is the minimum number of reported outcomes before the
// calibration rates are shown as meaningful rather than a low-volume warning.
const lowVolumeThreshold = 10

// Insights aggregates score distribution, decision counts, and outcome
// calibration for the org over a selectable window (default 30 days).
func (h *AutoApproveInsightsHandler) Insights(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}

	windowDays := 30
	switch c.QueryParam("window") {
	case "7":
		windowDays = 7
	case "90":
		windowDays = 90
	}
	since := time.Now().AddDate(0, 0, -windowDays)
	ctx := c.Request().Context()

	scoreDist, err := h.scoreDistribution(ctx, orgID, since)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "score distribution failed: "+err.Error())
	}
	decisions, err := h.decisions(ctx, orgID, since)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "decisions failed: "+err.Error())
	}
	calibration, err := h.calibration(ctx, orgID, since)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "calibration failed: "+err.Error())
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"insights": autoApproveInsightsDTO{
			WindowDays:        windowDays,
			ScoreDistribution: *scoreDist,
			Decisions:         *decisions,
			Calibration:       *calibration,
		},
	})
}

// scoreDistribution buckets every review scored in the window into 20 fixed
// [0,1] bins (the model's score is always sigmoid output, so the range is
// fixed regardless of org) and resolves the org's current cutoff so the UI
// can draw one reference line without recomputing it.
func (h *AutoApproveInsightsHandler) scoreDistribution(ctx context.Context, orgID int64, since time.Time) (*scoreDistributionDTO, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT score FROM risk_feature_snapshots WHERE org_id = $1 AND created_at >= $2`, orgID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	const numBuckets = 20
	buckets := make([]scoreHistogramBucket, numBuckets)
	for i := range buckets {
		buckets[i] = scoreHistogramBucket{RangeStart: float64(i) / numBuckets, RangeEnd: float64(i+1) / numBuckets}
	}

	var scores []float64
	for rows.Next() {
		var s float64
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		scores = append(scores, s)
		idx := int(s * numBuckets)
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		if idx < 0 {
			idx = 0
		}
		buckets[idx].Count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	percentile := 20
	p, found, err := policyengine.ResolvePolicy(ctx, h.db, orgID, 0)
	if err == nil && found {
		percentile = p.RiskThresholdPercentile
	}
	cutoff := riskscore.PercentileCutoff(scores, percentile)

	return &scoreDistributionDTO{Buckets: buckets, Cutoff: cutoff, Percentile: percentile, TotalScored: len(scores)}, nil
}

// decisions counts the current final verdict (review_policy_log is
// overwritten per review on every re-evaluation, so "current" is also
// "latest decided in the window") plus which earlier stage blocked it, and
// the override rate from review_policy_overrides.
func (h *AutoApproveInsightsHandler) decisions(ctx context.Context, orgID int64, since time.Time) (*decisionsDTO, error) {
	out := &decisionsDTO{}

	finalRows, err := h.db.QueryContext(ctx, `
		SELECT review_id, verdict, created_at FROM review_policy_log
		WHERE org_id = $1 AND stage = $2 AND created_at >= $3`, orgID, policyengine.StageFinal, since)
	if err != nil {
		return nil, err
	}
	defer finalRows.Close()

	type dayCounts struct{ approved, blocked int }
	byDay := map[string]*dayCounts{}
	var reviewIDs []int64
	blockedIDs := map[int64]bool{}

	for finalRows.Next() {
		var reviewID int64
		var verdict string
		var createdAt time.Time
		if err := finalRows.Scan(&reviewID, &verdict, &createdAt); err != nil {
			return nil, err
		}
		reviewIDs = append(reviewIDs, reviewID)
		day := createdAt.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = &dayCounts{}
		}
		if verdict == policyengine.FinalApproved {
			out.Approved++
			byDay[day].approved++
		} else {
			out.TotalDecided++
			byDay[day].blocked++
			blockedIDs[reviewID] = true
		}
	}
	if err := finalRows.Err(); err != nil {
		return nil, err
	}
	out.TotalDecided += out.Approved

	// Per-stage block breakdown: a review can fail risk AND llm
	// independently (neither gates the other), so these can overlap and are
	// not forced mutually exclusive.
	if len(blockedIDs) > 0 {
		ids := make([]int64, 0, len(blockedIDs))
		for id := range blockedIDs {
			ids = append(ids, id)
		}
		stageRows, err := h.db.QueryContext(ctx, `
			SELECT stage, count(*) FROM review_policy_log
			WHERE org_id = $1 AND verdict = $2 AND stage != $3 AND review_id = ANY($4)
			GROUP BY stage`, orgID, policyengine.VerdictFail, policyengine.StageFinal, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		defer stageRows.Close()
		for stageRows.Next() {
			var stage string
			var count int
			if err := stageRows.Scan(&stage, &count); err != nil {
				return nil, err
			}
			switch stage {
			case policyengine.StageEligibility:
				out.BlockedAtEligibility = count
			case policyengine.StageStatic:
				out.BlockedAtStatic = count
			case policyengine.StageRisk:
				out.BlockedAtRisk = count
			case policyengine.StageLLM:
				out.BlockedAtLLM = count
			}
		}
		if err := stageRows.Err(); err != nil {
			return nil, err
		}
	}

	var overridden int
	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*) FROM review_policy_overrides WHERE org_id = $1 AND created_at >= $2`, orgID, since).Scan(&overridden); err != nil {
		return nil, err
	}
	out.Overridden = overridden
	if out.TotalDecided > 0 {
		out.OverrideRate = float64(overridden) / float64(out.TotalDecided)
	}

	for day, c := range byDay {
		out.Trend = append(out.Trend, decisionTrendPoint{Day: day, Approved: c.approved, Blocked: c.blocked})
	}
	sort.Slice(out.Trend, func(i, j int) bool { return out.Trend[i].Day < out.Trend[j].Day })

	return out, nil
}

// calibration cross-tabs reported real-world outcomes (excluding
// source='test', the same exclusion the training path already applies)
// against what the policy actually decided for that review.
func (h *AutoApproveInsightsHandler) calibration(ctx context.Context, orgID int64, since time.Time) (*calibrationDTO, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT rpl.verdict, ro.outcome_type, count(*)
		FROM review_outcomes ro
		JOIN review_policy_log rpl ON rpl.review_id = ro.review_id AND rpl.org_id = ro.org_id AND rpl.stage = $1
		WHERE ro.org_id = $2 AND ro.source != 'test' AND ro.created_at >= $3
		GROUP BY rpl.verdict, ro.outcome_type`, policyengine.StageFinal, orgID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &calibrationDTO{}
	var approvedWithOutcome, approvedBad int
	for rows.Next() {
		var verdict, outcomeType string
		var count int
		if err := rows.Scan(&verdict, &outcomeType, &count); err != nil {
			return nil, err
		}
		decision := "blocked"
		if verdict == policyengine.FinalApproved {
			decision = "approved"
		}
		out.Cells = append(out.Cells, calibrationCell{Decision: decision, OutcomeType: outcomeType, Count: count})
		out.TotalOutcomes += count
		if decision == "approved" {
			approvedWithOutcome += count
			if outcomeType == "revert" || outcomeType == "incident" {
				approvedBad += count
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if approvedWithOutcome > 0 {
		rate := float64(approvedBad) / float64(approvedWithOutcome)
		out.FalseApproveRate = &rate
	}
	out.LowVolumeWarning = out.TotalOutcomes < lowVolumeThreshold
	return out, nil
}
