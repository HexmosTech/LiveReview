package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"

	"github.com/livereview/storage/dashboard"
)

// countsByWindow is a trailing-window count: day=added in the last 1 day, week=last 7,
// month=last 30, all=ever. Same "activity within the window" meaning as ratesByWindow and
// every other period-scoped widget on this dashboard (Review Layers, Coverage%, Averages) —
// counts are not a special case, so they use the identical window concept, just integer-valued.
type countsByWindow struct {
	Day   int `json:"day"`
	Week  int `json:"week"`
	Month int `json:"month"`
	All   int `json:"all"`
}

// ratesByWindow is a trailing-window metric: each period is "activity in the last N
// days" (day=last 1 day, week=last 7, month=last 30, all=no lower bound), computed
// live every tick — never day-bucketed. Distinct-entity metrics like coverage% and
// avg-reviews-per-PR would double-count if summed from independent day buckets (a PR
// reviewed on two different days would count twice), so unlike review_layers this
// domain always recomputes the whole window directly rather than caching daily deltas.
type ratesByWindow struct {
	Day   float64 `json:"day"`
	Week  float64 `json:"week"`
	Month float64 `json:"month"`
	All   float64 `json:"all"`
}

type repoOverviewRow struct {
	RepoName string        `json:"name"`
	PRCount  countsByWindow `json:"pr_count"`
	Coverage ratesByWindow  `json:"coverage_pct"`
}

type hostRepoGroup struct {
	Host  string            `json:"host"`
	Repos []repoOverviewRow `json:"repos"`
}

type connectedProviderRow struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"` // "git" | "ai"
	Provider     string    `json:"provider"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

type systemOverviewSnapshot struct {
	GitHosts            countsByWindow         `json:"git_hosts"`
	AIConnectors        countsByWindow         `json:"ai_connectors"`
	TotalRepos          countsByWindow         `json:"total_repos"`
	TotalPRs            countsByWindow         `json:"total_prs"`
	CoveragePct         ratesByWindow          `json:"coverage_pct"`
	AvgReviewsPerPR     ratesByWindow          `json:"avg_reviews_per_pr"`
	AvgReviewsPerCommit ratesByWindow          `json:"avg_reviews_per_commit"`
	RepoHierarchy       []hostRepoGroup        `json:"repo_hierarchy"`
	ConnectedProviders  []connectedProviderRow `json:"connected_providers"`
}

// systemOverviewCutoffs bundles the trailing-window start timestamps every batched query below
// needs — computed once per tick and reused everywhere, so "week" means the exact same instant
// across every query. All comparisons are ">=" (activity within the window), consistent with
// review_layers and every other period-scoped widget on this dashboard.
type systemOverviewCutoffs struct {
	dayStart   time.Time // now-1d
	weekStart  time.Time // now-7d
	monthStart time.Time // now-30d
}

func newSystemOverviewCutoffs(now time.Time) systemOverviewCutoffs {
	return systemOverviewCutoffs{
		dayStart:   now.AddDate(0, 0, -1),
		weekStart:  now.AddDate(0, 0, -7),
		monthStart: now.AddDate(0, 0, -30),
	}
}

// snapshotCountsQuery counts rows in `table` added within each trailing window, via conditional
// aggregation — one batched round trip per table, not one per window.
func snapshotCountsQuery(table string) string {
	return fmt.Sprintf(`
		SELECT org_id,
		  COUNT(*) FILTER (WHERE created_at >= $2) AS day_count,
		  COUNT(*) FILTER (WHERE created_at >= $3) AS week_count,
		  COUNT(*) FILTER (WHERE created_at >= $4) AS month_count,
		  COUNT(*) AS all_count
		FROM %s
		WHERE org_id = ANY($1)
		GROUP BY org_id
	`, table)
}

func (dm *DashboardManager) fetchSnapshotCounts(ctx context.Context, table string, orgIDs []int64, c systemOverviewCutoffs) (map[int64]countsByWindow, error) {
	rows, err := dm.db.QueryContext(ctx, snapshotCountsQuery(table), pq.Array(orgIDs), c.dayStart, c.weekStart, c.monthStart)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()

	result := map[int64]countsByWindow{}
	for rows.Next() {
		var orgID int64
		var day, week, month, all int
		if err := rows.Scan(&orgID, &day, &week, &month, &all); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		result[orgID] = countsByWindow{Day: day, Week: week, Month: month, All: all}
	}
	return result, rows.Err()
}

// repoHierarchyQuery joins repositories -> integration_tokens (host) -> pull_requests (PR
// count) -> reviews (coverage), grouped per repo. Both PR count and coverage now share the
// same trailing-window boundaries (">=" — activity within the window), consistent with every
// other period-scoped widget on this dashboard.
const repoHierarchyQuery = `
SELECT
  r.org_id,
  it.connection_name AS host,
  r.id AS repo_id,
  r.name AS repo_name,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $2) AS pr_count_day,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $2 AND rv.id IS NOT NULL) AS covered_day,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $3) AS pr_count_week,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $3 AND rv.id IS NOT NULL) AS covered_week,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $4) AS pr_count_month,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $4 AND rv.id IS NOT NULL) AS covered_month,
  COUNT(DISTINCT pr.id) AS pr_count_all,
  COUNT(DISTINCT pr.id) FILTER (WHERE rv.id IS NOT NULL) AS covered_all
FROM repositories r
JOIN integration_tokens it ON it.id = r.connector_id
LEFT JOIN pull_requests pr ON pr.repository_id = r.id
LEFT JOIN reviews rv ON rv.pull_request_id = pr.id
WHERE r.org_id = ANY($1)
GROUP BY r.org_id, it.connection_name, r.id, r.name
`

func coveragePct(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * 100
}

func (dm *DashboardManager) fetchRepoHierarchy(ctx context.Context, orgIDs []int64, c systemOverviewCutoffs) (map[int64][]hostRepoGroup, error) {
	rows, err := dm.db.QueryContext(ctx, repoHierarchyQuery, pq.Array(orgIDs), c.dayStart, c.weekStart, c.monthStart)
	if err != nil {
		return nil, fmt.Errorf("query repo hierarchy: %w", err)
	}
	defer rows.Close()

	type hostKey struct {
		orgID int64
		host  string
	}
	groups := map[hostKey][]repoOverviewRow{}
	hostOrder := map[int64][]string{}

	for rows.Next() {
		var orgID, repoID int64
		var host, repoName string
		var prDay, coveredDay int
		var prWeek, coveredWeek int
		var prMonth, coveredMonth int
		var prAll, coveredAll int
		if err := rows.Scan(&orgID, &host, &repoID, &repoName,
			&prDay, &coveredDay,
			&prWeek, &coveredWeek,
			&prMonth, &coveredMonth,
			&prAll, &coveredAll,
		); err != nil {
			return nil, fmt.Errorf("scan repo hierarchy: %w", err)
		}

		k := hostKey{orgID, host}
		if _, seen := groups[k]; !seen {
			hostOrder[orgID] = append(hostOrder[orgID], host)
		}
		groups[k] = append(groups[k], repoOverviewRow{
			RepoName: repoName,
			PRCount:  countsByWindow{Day: prDay, Week: prWeek, Month: prMonth, All: prAll},
			Coverage: ratesByWindow{
				Day:   coveragePct(coveredDay, prDay),
				Week:  coveragePct(coveredWeek, prWeek),
				Month: coveragePct(coveredMonth, prMonth),
				All:   coveragePct(coveredAll, prAll),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows repo hierarchy: %w", err)
	}

	result := map[int64][]hostRepoGroup{}
	for orgID, hosts := range hostOrder {
		for _, host := range hosts {
			result[orgID] = append(result[orgID], hostRepoGroup{
				Host:  host,
				Repos: groups[hostKey{orgID, host}],
			})
		}
	}
	return result, nil
}

// orgCoverageAndAvgReviewsQuery computes org-wide coverage% and avg-reviews-per-PR together
// in one round trip since both scan the same pull_requests LEFT JOIN reviews shape.
const orgCoverageAndAvgReviewsQuery = `
SELECT
  pr.org_id,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $2) AS pr_count_day,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $2 AND rv.id IS NOT NULL) AS covered_day,
  COUNT(rv.id) FILTER (WHERE pr.created_at >= $2) AS reviews_day,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $3) AS pr_count_week,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $3 AND rv.id IS NOT NULL) AS covered_week,
  COUNT(rv.id) FILTER (WHERE pr.created_at >= $3) AS reviews_week,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $4) AS pr_count_month,
  COUNT(DISTINCT pr.id) FILTER (WHERE pr.created_at >= $4 AND rv.id IS NOT NULL) AS covered_month,
  COUNT(rv.id) FILTER (WHERE pr.created_at >= $4) AS reviews_month,
  COUNT(DISTINCT pr.id) AS pr_count_all,
  COUNT(DISTINCT pr.id) FILTER (WHERE rv.id IS NOT NULL) AS covered_all,
  COUNT(rv.id) AS reviews_all
FROM pull_requests pr
LEFT JOIN reviews rv ON rv.pull_request_id = pr.id
WHERE pr.org_id = ANY($1)
GROUP BY pr.org_id
`

type orgCoverageAndAvgReviews struct {
	Coverage        ratesByWindow
	AvgReviewsPerPR ratesByWindow
}

func avgReviews(reviews, prCount int) float64 {
	if prCount == 0 {
		return 0
	}
	return float64(reviews) / float64(prCount)
}

func (dm *DashboardManager) fetchOrgCoverageAndAvgReviews(ctx context.Context, orgIDs []int64, c systemOverviewCutoffs) (map[int64]orgCoverageAndAvgReviews, error) {
	rows, err := dm.db.QueryContext(ctx, orgCoverageAndAvgReviewsQuery, pq.Array(orgIDs), c.dayStart, c.weekStart, c.monthStart)
	if err != nil {
		return nil, fmt.Errorf("query coverage/avg reviews: %w", err)
	}
	defer rows.Close()

	result := map[int64]orgCoverageAndAvgReviews{}
	for rows.Next() {
		var orgID int64
		var prDay, coveredDay, reviewsDay int
		var prWeek, coveredWeek, reviewsWeek int
		var prMonth, coveredMonth, reviewsMonth int
		var prAll, coveredAll, reviewsAll int
		if err := rows.Scan(&orgID,
			&prDay, &coveredDay, &reviewsDay,
			&prWeek, &coveredWeek, &reviewsWeek,
			&prMonth, &coveredMonth, &reviewsMonth,
			&prAll, &coveredAll, &reviewsAll,
		); err != nil {
			return nil, fmt.Errorf("scan coverage/avg reviews: %w", err)
		}
		result[orgID] = orgCoverageAndAvgReviews{
			Coverage: ratesByWindow{
				Day:   coveragePct(coveredDay, prDay),
				Week:  coveragePct(coveredWeek, prWeek),
				Month: coveragePct(coveredMonth, prMonth),
				All:   coveragePct(coveredAll, prAll),
			},
			AvgReviewsPerPR: ratesByWindow{
				Day:   avgReviews(reviewsDay, prDay),
				Week:  avgReviews(reviewsWeek, prWeek),
				Month: avgReviews(reviewsMonth, prMonth),
				All:   avgReviews(reviewsAll, prAll),
			},
		}
	}
	return result, rows.Err()
}

// avgReviewsPerCommitQuery: cli_diff (precommit) reviews only, avg reviews per distinct commit reviewed, trailing window.
const avgReviewsPerCommitQuery = `
SELECT
  org_id,
  COUNT(*) FILTER (WHERE created_at >= $2) AS reviews_day,
  COUNT(DISTINCT commit_hash) FILTER (WHERE created_at >= $2) AS commits_day,
  COUNT(*) FILTER (WHERE created_at >= $3) AS reviews_week,
  COUNT(DISTINCT commit_hash) FILTER (WHERE created_at >= $3) AS commits_week,
  COUNT(*) FILTER (WHERE created_at >= $4) AS reviews_month,
  COUNT(DISTINCT commit_hash) FILTER (WHERE created_at >= $4) AS commits_month,
  COUNT(*) AS reviews_all,
  COUNT(DISTINCT commit_hash) AS commits_all
FROM reviews
WHERE org_id = ANY($1) AND trigger_type = 'cli_diff'
GROUP BY org_id
`

func (dm *DashboardManager) fetchAvgReviewsPerCommit(ctx context.Context, orgIDs []int64, c systemOverviewCutoffs) (map[int64]ratesByWindow, error) {
	rows, err := dm.db.QueryContext(ctx, avgReviewsPerCommitQuery, pq.Array(orgIDs), c.dayStart, c.weekStart, c.monthStart)
	if err != nil {
		return nil, fmt.Errorf("query avg reviews per commit: %w", err)
	}
	defer rows.Close()

	result := map[int64]ratesByWindow{}
	for rows.Next() {
		var orgID int64
		var reviewsDay, commitsDay int
		var reviewsWeek, commitsWeek int
		var reviewsMonth, commitsMonth int
		var reviewsAll, commitsAll int
		if err := rows.Scan(&orgID,
			&reviewsDay, &commitsDay,
			&reviewsWeek, &commitsWeek,
			&reviewsMonth, &commitsMonth,
			&reviewsAll, &commitsAll,
		); err != nil {
			return nil, fmt.Errorf("scan avg reviews per commit: %w", err)
		}
		result[orgID] = ratesByWindow{
			Day:   avgReviews(reviewsDay, commitsDay),
			Week:  avgReviews(reviewsWeek, commitsWeek),
			Month: avgReviews(reviewsMonth, commitsMonth),
			All:   avgReviews(reviewsAll, commitsAll),
		}
	}
	return result, rows.Err()
}

// connectedProvidersQuery lists git/AI connectors with no period filtering at all — always current state.
const connectedGitProvidersQuery = `SELECT org_id, id, connection_name, provider, updated_at FROM integration_tokens WHERE org_id = ANY($1)`
const connectedAIProvidersQuery = `SELECT org_id, id, connector_name, provider_name, updated_at FROM ai_connectors WHERE org_id = ANY($1)`

func (dm *DashboardManager) fetchConnectedProviders(ctx context.Context, orgIDs []int64) (map[int64][]connectedProviderRow, error) {
	result := map[int64][]connectedProviderRow{}

	gitRows, err := dm.db.QueryContext(ctx, connectedGitProvidersQuery, pq.Array(orgIDs))
	if err != nil {
		return nil, fmt.Errorf("query git providers: %w", err)
	}
	defer gitRows.Close()
	for gitRows.Next() {
		var orgID, id int64
		var name, provider string
		var updatedAt time.Time
		if err := gitRows.Scan(&orgID, &id, &name, &provider, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan git providers: %w", err)
		}
		result[orgID] = append(result[orgID], connectedProviderRow{
			ID: fmt.Sprintf("git-%d", id), Name: name, Kind: "git", Provider: provider, LastSyncedAt: updatedAt,
		})
	}
	if err := gitRows.Err(); err != nil {
		return nil, fmt.Errorf("rows git providers: %w", err)
	}

	aiRows, err := dm.db.QueryContext(ctx, connectedAIProvidersQuery, pq.Array(orgIDs))
	if err != nil {
		return nil, fmt.Errorf("query ai providers: %w", err)
	}
	defer aiRows.Close()
	for aiRows.Next() {
		var orgID, id int64
		var name sql.NullString
		var provider string
		var updatedAt time.Time
		if err := aiRows.Scan(&orgID, &id, &name, &provider, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan ai providers: %w", err)
		}
		displayName := name.String
		if displayName == "" {
			displayName = provider
		}
		result[orgID] = append(result[orgID], connectedProviderRow{
			ID: fmt.Sprintf("ai-%d", id), Name: displayName, Kind: "ai", Provider: provider, LastSyncedAt: updatedAt,
		})
	}
	return result, aiRows.Err()
}

// buildSystemOverviewSnapshots runs every batched source query once (never per-org) and
// assembles one systemOverviewSnapshot per org.
func (dm *DashboardManager) buildSystemOverviewSnapshots(ctx context.Context, orgIDs []int64) (map[int64]systemOverviewSnapshot, error) {
	cutoffs := newSystemOverviewCutoffs(time.Now())

	gitHosts, err := dm.fetchSnapshotCounts(ctx, "integration_tokens", orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	aiConnectors, err := dm.fetchSnapshotCounts(ctx, "ai_connectors", orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	totalRepos, err := dm.fetchSnapshotCounts(ctx, "repositories", orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	totalPRs, err := dm.fetchSnapshotCounts(ctx, "pull_requests", orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	repoHierarchy, err := dm.fetchRepoHierarchy(ctx, orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	coverageAndAvgReviews, err := dm.fetchOrgCoverageAndAvgReviews(ctx, orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	avgReviewsPerCommit, err := dm.fetchAvgReviewsPerCommit(ctx, orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	connectedProviders, err := dm.fetchConnectedProviders(ctx, orgIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]systemOverviewSnapshot, len(orgIDs))
	for _, orgID := range orgIDs {
		result[orgID] = systemOverviewSnapshot{
			GitHosts:            gitHosts[orgID],
			AIConnectors:        aiConnectors[orgID],
			TotalRepos:          totalRepos[orgID],
			TotalPRs:            totalPRs[orgID],
			CoveragePct:         coverageAndAvgReviews[orgID].Coverage,
			AvgReviewsPerPR:     coverageAndAvgReviews[orgID].AvgReviewsPerPR,
			AvgReviewsPerCommit: avgReviewsPerCommit[orgID],
			RepoHierarchy:       repoHierarchy[orgID],
			ConnectedProviders:  connectedProviders[orgID],
		}
	}
	return result, nil
}

// mergeSystemOverviewIntoCacheRow sets final_json.system_overview, preserving json_raw and
// every other final_json key (notably review_layers) byte-for-byte untouched. Deliberately
// mirrors buildCacheRow's "preserve other keys" behavior but never touches json_raw at all —
// system_overview has no day-bucketed history to accumulate.
func mergeSystemOverviewIntoCacheRow(orgID int64, existingRow dashboard.CacheRow, snapshot systemOverviewSnapshot, now time.Time) (dashboard.CacheRow, error) {
	finalMap := map[string]json.RawMessage{}
	if len(existingRow.FinalJSON) > 0 {
		if err := json.Unmarshal(existingRow.FinalJSON, &finalMap); err != nil {
			log.Printf("[dashboard_cache] failed to parse existing final_json org_id=%d err=%v — starting fresh", orgID, err)
			finalMap = map[string]json.RawMessage{}
		}
	}

	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal system_overview: %w", err)
	}
	finalMap["system_overview"] = snapshotBytes

	newFinalJSON, err := json.Marshal(finalMap)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal final_json: %w", err)
	}

	return dashboard.CacheRow{
		OrgID:     orgID,
		JSONRaw:   existingRow.JSONRaw,
		FinalJSON: newFinalJSON,
		UpdatedAt: now,
	}, nil
}

// refreshAllSystemOverview is the system_overview half of the shared 5-minute cycle: batched
// across all orgs, no per-org loop for any query. Unlike review_layers this domain writes no
// json_raw at all — every value is recomputed live from repositories/pull_requests/reviews/
// integration_tokens/ai_connectors each tick, so there's nothing to accumulate day-by-day.
//
// Must run its own GetBatch immediately before its own UpsertBatch (not share a fetch taken
// before review_layers' own write in this same tick) — otherwise this write would silently
// revert review_layers' just-written update back to its pre-tick value.
func (dm *DashboardManager) refreshAllSystemOverview(ctx context.Context, orgIDs []int64) error {
	if len(orgIDs) == 0 {
		return nil
	}

	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	snapshots, err := dm.buildSystemOverviewSnapshots(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("build system overview snapshots: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("get existing cache batch: %w", err)
	}

	now := time.Now()
	rows := make([]dashboard.CacheRow, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		row, err := mergeSystemOverviewIntoCacheRow(orgID, existing[orgID], snapshots[orgID], now)
		if err != nil {
			log.Printf("[dashboard_cache] failed to build system_overview row org_id=%d err=%v — skipping this org this tick", orgID, err)
			continue
		}
		rows = append(rows, row)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, rows); err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed system_overview for %d/%d orgs duration_ms=%d", len(rows), len(orgIDs), time.Since(start).Milliseconds())
	return nil
}

// RefreshOrgSystemOverview recomputes and stores system_overview for one org on demand, reusing the same query logic as the batched tick.
func (dm *DashboardManager) RefreshOrgSystemOverview(ctx context.Context, orgID int64) (systemOverviewSnapshot, error) {
	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	snapshots, err := dm.buildSystemOverviewSnapshots(queryCtx, []int64{orgID})
	if err != nil {
		return systemOverviewSnapshot{}, fmt.Errorf("build system overview snapshot: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, []int64{orgID})
	if err != nil {
		return systemOverviewSnapshot{}, fmt.Errorf("get existing cache row: %w", err)
	}

	row, err := mergeSystemOverviewIntoCacheRow(orgID, existing[orgID], snapshots[orgID], time.Now())
	if err != nil {
		return systemOverviewSnapshot{}, fmt.Errorf("build cache row: %w", err)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, []dashboard.CacheRow{row}); err != nil {
		return systemOverviewSnapshot{}, fmt.Errorf("upsert cache row: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed system_overview trigger=manual org_id=%d duration_ms=%d", orgID, time.Since(start).Milliseconds())
	return snapshots[orgID], nil
}
