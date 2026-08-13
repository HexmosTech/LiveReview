package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// statisticsRow mirrors DashboardData's statistics fields for one org, assembled from the
// batched queries below instead of buildDashboardData's 4 individual per-org round trips.
type statisticsRow struct {
	TotalReviews       int
	TotalComments      int
	ConnectedProviders int
	ActiveAIConnectors int
}

const statsReviewsBatchQuery = `SELECT org_id, COUNT(*) FROM reviews WHERE org_id = ANY($1) GROUP BY org_id`

const statsCommentsBatchQuery = `
	SELECT org_id, COALESCE(SUM(COALESCE(NULLIF(data->>'commentCount', '')::int, 0)), 0)
	FROM review_events
	WHERE org_id = ANY($1) AND event_type = 'completion'
	GROUP BY org_id
`

const statsProvidersBatchQuery = `SELECT org_id, COUNT(*) FROM integration_tokens WHERE org_id = ANY($1) GROUP BY org_id`

const statsConnectorsBatchQuery = `SELECT org_id, COUNT(*) FROM ai_connectors WHERE org_id = ANY($1) GROUP BY org_id`

// fetchStatisticsBatch replaces collectStatistics's 4 per-org queries with 4 batched round trips
// total, covering every org in one pass each — same "one query per table, not per org" pattern
// already used by system_overview/people.
func (dm *DashboardManager) fetchStatisticsBatch(ctx context.Context, orgIDs []int64) (map[int64]statisticsRow, error) {
	result := make(map[int64]statisticsRow, len(orgIDs))
	for _, id := range orgIDs {
		result[id] = statisticsRow{}
	}

	fill := func(query string, apply func(row *statisticsRow, value int)) error {
		rows, err := dm.db.QueryContext(ctx, query, pq.Array(orgIDs))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var orgID int64
			var value int
			if err := rows.Scan(&orgID, &value); err != nil {
				return err
			}
			row := result[orgID]
			apply(&row, value)
			result[orgID] = row
		}
		return rows.Err()
	}

	if err := fill(statsReviewsBatchQuery, func(r *statisticsRow, v int) { r.TotalReviews = v }); err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	if err := fill(statsCommentsBatchQuery, func(r *statisticsRow, v int) { r.TotalComments = v }); err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	if err := fill(statsProvidersBatchQuery, func(r *statisticsRow, v int) { r.ConnectedProviders = v }); err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	if err := fill(statsConnectorsBatchQuery, func(r *statisticsRow, v int) { r.ActiveAIConnectors = v }); err != nil {
		return nil, fmt.Errorf("query connectors: %w", err)
	}
	return result, nil
}

// connectorSetupProgressBatchQuery is identical to collectConnectorSetupProgress's query, just
// scoped to every org at once instead of WHERE it.org_id = $1 in a per-org loop.
const connectorSetupProgressBatchQuery = `
	SELECT
		it.org_id,
		it.id,
		it.connection_name,
		it.provider,
		it.projects_cache,
		it.created_at,
		COALESCE((
			SELECT COUNT(*)
			FROM webhook_registry wr
			WHERE wr.integration_token_id = it.id
			AND (wr.status = 'manual' OR wr.status = 'active' OR wr.status = 'automatic')
		), 0) as connected_count
	FROM integration_tokens it
	WHERE it.org_id = ANY($1)
	ORDER BY it.org_id, it.created_at DESC
`

func (dm *DashboardManager) fetchConnectorSetupProgressBatch(ctx context.Context, orgIDs []int64) (map[int64][]ConnectorSetupProgress, error) {
	rows, err := dm.db.QueryContext(ctx, connectorSetupProgressBatchQuery, pq.Array(orgIDs))
	if err != nil {
		return nil, fmt.Errorf("query connector setup progress: %w", err)
	}
	defer rows.Close()

	recentThreshold := time.Now().Add(-connectorSetupRecentThreshold)
	result := map[int64][]ConnectorSetupProgress{}

	for rows.Next() {
		var orgID, connectorID int64
		var connectorName, provider string
		var projectsCacheRaw []byte
		var connectedCount int
		var createdAt time.Time

		if err := rows.Scan(&orgID, &connectorID, &connectorName, &provider, &projectsCacheRaw, &createdAt, &connectedCount); err != nil {
			return nil, fmt.Errorf("scan connector setup progress: %w", err)
		}

		totalProjects := 0
		if projectsCacheRaw != nil {
			var projectsCache struct {
				Projects []interface{} `json:"projects"`
			}
			if err := json.Unmarshal(projectsCacheRaw, &projectsCache); err == nil {
				totalProjects = len(projectsCache.Projects)
			}
		}

		var phase, message string
		if totalProjects == 0 {
			phase = "discovering"
			message = "Discovering projects..."
		} else if connectedCount == 0 {
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: 0/%d", totalProjects)
		} else if connectedCount < totalProjects {
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: %d/%d", connectedCount, totalProjects)
		} else {
			phase = "ready"
			message = fmt.Sprintf("Ready: %d projects connected", totalProjects)
		}

		isRecent := createdAt.After(recentThreshold)
		if phase != "ready" || isRecent {
			result[orgID] = append(result[orgID], ConnectorSetupProgress{
				ConnectorID:       connectorID,
				ConnectorName:     connectorName,
				Provider:          provider,
				Phase:             phase,
				TotalProjects:     totalProjects,
				ConnectedProjects: connectedCount,
				Message:           message,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows connector setup progress: %w", err)
	}
	return result, nil
}

// onboardingRow is each org's earliest owner/super_admin — the same user collectOnboardingData
// would find via ORDER BY u.created_at ASC LIMIT 1, just for every org in one round trip.
type onboardingRow struct {
	UserID        int64
	OnboardingKey sql.NullString
	LastCLIUsedAt sql.NullTime
}

const onboardingBatchQuery = `
	SELECT DISTINCT ON (ur.org_id)
		ur.org_id, u.id, u.onboarding_api_key, u.last_cli_used_at
	FROM user_roles ur
	INNER JOIN users u ON u.id = ur.user_id
	INNER JOIN roles r ON ur.role_id = r.id
	WHERE ur.org_id = ANY($1) AND r.name IN ('owner', 'super_admin')
	ORDER BY ur.org_id, u.created_at ASC
`

func (dm *DashboardManager) fetchOnboardingDataBatch(ctx context.Context, orgIDs []int64) (map[int64]onboardingRow, error) {
	rows, err := dm.db.QueryContext(ctx, onboardingBatchQuery, pq.Array(orgIDs))
	if err != nil {
		return nil, fmt.Errorf("query onboarding: %w", err)
	}
	defer rows.Close()

	result := map[int64]onboardingRow{}
	for rows.Next() {
		var orgID int64
		var row onboardingRow
		if err := rows.Scan(&orgID, &row.UserID, &row.OnboardingKey, &row.LastCLIUsedAt); err != nil {
			return nil, fmt.Errorf("scan onboarding: %w", err)
		}
		result[orgID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows onboarding: %w", err)
	}
	return result, nil
}

// ensureOnboardingAPIKey lazily generates an onboarding API key for an org whose owner doesn't
// have one yet. This is a real write (unlike the batched reads above), so it stays a per-org
// call — buildDashboardDataBatch only invokes it for orgs actually missing a key, which in
// practice is rare (new orgs, once).
func (dm *DashboardManager) ensureOnboardingAPIKey(ctx context.Context, orgID, userID int64) (string, error) {
	apiKeyManager := NewAPIKeyManager(dm.db)
	_, newKey, genErr := apiKeyManager.CreateAPIKey(userID, orgID, "Onboarding API Key", []string{}, nil)
	if genErr != nil {
		return "", genErr
	}
	if _, err := dm.db.ExecContext(ctx, `UPDATE users SET onboarding_api_key = $1 WHERE id = $2`, newKey, userID); err != nil {
		return "", err
	}
	return newKey, nil
}

// buildDashboardDataBatch is the all-orgs equivalent of buildDashboardData: 3 batched reads
// total (statistics, connector setup progress, onboarding) instead of buildDashboardData's ~6
// queries repeated per org. Only orgs whose owner is missing an onboarding key fall back to a
// real per-org write, same as the single-org path already did.
func (dm *DashboardManager) buildDashboardDataBatch(ctx context.Context, orgIDs []int64) (map[int64]DashboardData, error) {
	stats, err := dm.fetchStatisticsBatch(ctx, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch statistics batch: %w", err)
	}
	setupProgress, err := dm.fetchConnectorSetupProgressBatch(ctx, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch connector setup progress batch: %w", err)
	}
	onboarding, err := dm.fetchOnboardingDataBatch(ctx, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch onboarding batch: %w", err)
	}

	apiURL := dashboardAPIURL()
	now := time.Now()
	result := make(map[int64]DashboardData, len(orgIDs))

	for _, orgID := range orgIDs {
		data := DashboardData{LastUpdated: now, APIUrl: apiURL}

		if s, ok := stats[orgID]; ok {
			data.TotalReviews = s.TotalReviews
			data.TotalComments = s.TotalComments
			data.ConnectedProviders = s.ConnectedProviders
			data.ActiveAIConnectors = s.ActiveAIConnectors
		}

		data.ConnectorSetupProgress = setupProgress[orgID]

		if ob, ok := onboarding[orgID]; ok {
			key := ob.OnboardingKey
			if !key.Valid || key.String == "" {
				if newKey, genErr := dm.ensureOnboardingAPIKey(ctx, orgID, ob.UserID); genErr == nil {
					key = sql.NullString{String: newKey, Valid: true}
				} else {
					dm.logErrorf("[dashboard] onboarding generate_api_key_failed org_id=%d user_id=%d err=%v", orgID, ob.UserID, genErr)
				}
			}
			if key.Valid && key.String != "" {
				data.OnboardingAPIKey = key.String
			}
			data.CLIInstalled = ob.LastCLIUsedAt.Valid
		}

		result[orgID] = data
	}

	return result, nil
}
