package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/livereview/storage/dashboard"
)

// reviewLayerTaxonomyCategories is the fixed 10-item taxonomy final_json always fills out (0 for unused categories); json_raw stores sparsely.
var reviewLayerTaxonomyCategories = []string{
	"Security", "Reliability", "Correctness", "Performance", "Cost",
	"Scalability", "Maintainability", "Architecture", "Developer Experience",
	"Compliance & Governance",
}

var reviewLayerLabels = map[string]string{
	"precommit": "Pre-commit",
	"mr-pr":     "MR / PR",
	"api-mcp":   "API / MCP",
}

// reviewLayerOrder is precommit/mr-pr/api-mcp for now — scheduled comes once that trigger type is real.
var reviewLayerOrder = []string{"precommit", "mr-pr", "api-mcp"}

// reviewLayersSourceQuery is the validated batched shape (~50x faster than looping per org, see cmd/dashboard-perf-test); trigger_type is mapped at read time since the column still holds the old raw values.
const reviewLayersSourceQuery = `
SELECT
  r.org_id,
  r.id AS review_id,
  CASE r.trigger_type
    WHEN 'cli_diff' THEN 'precommit'
    WHEN 'manual' THEN 'mr-pr'
    WHEN 'api' THEN 'api-mcp'
    WHEN 'mcp' THEN 'api-mcp'
  END AS trigger_type,
  COALESCE(NULLIF(c.comment->>'category',''), NULLIF(c.comment->>'Category','')) AS category
FROM reviews r
LEFT JOIN LATERAL jsonb_array_elements(
  CASE
    WHEN jsonb_typeof(r.metadata->'review_result'->'comments') = 'array'  THEN r.metadata->'review_result'->'comments'
    WHEN jsonb_typeof(r.metadata->'review_result'->'comments') = 'object' THEN jsonb_build_array(r.metadata->'review_result'->'comments')
    ELSE '[]'::jsonb
  END
) AS c(comment) ON true
WHERE r.org_id = ANY($1)
  AND r.trigger_type IN ('cli_diff', 'manual', 'api', 'mcp')
  AND r.created_at >= $2
  AND r.created_at <  $2::date + INTERVAL '1 day'
`

// reviewLayerAggregate is one trigger-type layer's numbers for a single day.
type reviewLayerAggregate struct {
	ReviewsRun  int            `json:"reviews_run"`
	IssuesFound int            `json:"issues_found"`
	Categories  map[string]int `json:"categories"`
}

// dayLayerEntry is the "review_layers" subfield of one date's entry in json_raw: trigger_type -> that day's aggregate.
type dayLayerEntry map[string]reviewLayerAggregate

// finalLayerRow is one layer's row in final_json.review_layers[period]: id, label, totals, and a full 10-item category array.
type finalLayerRow struct {
	ID          string             `json:"id"`
	Label       string             `json:"label"`
	ReviewsRun  int                `json:"reviews_run"`
	IssuesFound int                `json:"issues_found"`
	Categories  []categoryCountRow `json:"categories"`
}

type categoryCountRow struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// refreshAllReviewLayers is the review_layers half of the shared 5-minute cycle: ~3 batched round trips, using orgIDs already fetched by the caller.
func (dm *DashboardManager) refreshAllReviewLayers(ctx context.Context, orgIDs []int64) error {
	if len(orgIDs) == 0 {
		return nil
	}

	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	today := time.Now().UTC().Format("2006-01-02")

	todayData, err := dm.fetchReviewLayersToday(queryCtx, orgIDs, today)
	if err != nil {
		return fmt.Errorf("fetch review layers: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("get existing cache batch: %w", err)
	}

	now := time.Now()
	rows := make([]dashboard.CacheRow, 0, len(orgIDs))

	for _, orgID := range orgIDs {
		row, err := dm.buildCacheRow(orgID, existing[orgID], todayData[orgID], today, now)
		if err != nil {
			log.Printf("[dashboard_cache] failed to build cache row org_id=%d err=%v — skipping this org this tick", orgID, err)
			continue
		}
		rows = append(rows, row)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, rows); err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed review_layers for %d/%d orgs duration_ms=%d", len(rows), len(orgIDs), time.Since(start).Milliseconds())
	return nil
}

// activeOrgIDsSince returns every org with at least one review since the given day (inclusive) — used to skip orgs with nothing to backfill.
func (dm *DashboardManager) activeOrgIDsSince(ctx context.Context, sinceDay string) ([]int64, error) {
	rows, err := dm.db.QueryContext(ctx, `SELECT DISTINCT org_id FROM reviews WHERE created_at >= $1`, sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgIDs []int64
	for rows.Next() {
		var orgID int64
		if err := rows.Scan(&orgID); err != nil {
			return nil, err
		}
		orgIDs = append(orgIDs, orgID)
	}
	return orgIDs, rows.Err()
}

// BackfillReviewLayers is a one-off operational backfill (not part of the regular 5-minute
// tick) that populates dashboard_cache with `days` days of history for every org active in
// that window. It reuses fetchReviewLayersToday/buildCacheRow exactly as the real tick does,
// one day at a time oldest-to-newest, so each day's merge builds on the previous day's row —
// identical logic to refreshAllReviewLayers, just replayed across a date range instead of "today".
func (dm *DashboardManager) BackfillReviewLayers(ctx context.Context, days int) error {
	if dm.cacheStore == nil {
		return fmt.Errorf("cacheStore not configured")
	}
	if days <= 0 {
		return fmt.Errorf("days must be positive, got %d", days)
	}

	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(days)*dashboardDBQueryTimeout)
	defer cancel()

	today := time.Now().UTC()
	sinceDay := today.AddDate(0, 0, -days+1).Format("2006-01-02")

	orgIDs, err := dm.activeOrgIDsSince(queryCtx, sinceDay)
	if err != nil {
		return fmt.Errorf("list active orgs: %w", err)
	}
	if len(orgIDs) == 0 {
		log.Printf("[dashboard_cache] backfill: no active orgs since %s", sinceDay)
		return nil
	}
	log.Printf("[dashboard_cache] backfill: %d day(s) for %d active org(s)", days, len(orgIDs))

	existing, err := dm.cacheStore.GetBatch(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("get existing cache batch: %w", err)
	}

	rowsByOrg := make(map[int64]dashboard.CacheRow, len(orgIDs))
	for _, orgID := range orgIDs {
		rowsByOrg[orgID] = existing[orgID]
	}

	for i := days - 1; i >= 0; i-- {
		day := today.AddDate(0, 0, -i).Format("2006-01-02")
		dayData, err := dm.fetchReviewLayersToday(queryCtx, orgIDs, day)
		if err != nil {
			return fmt.Errorf("fetch day %s: %w", day, err)
		}
		now := time.Now()
		for _, orgID := range orgIDs {
			row, err := dm.buildCacheRow(orgID, rowsByOrg[orgID], dayData[orgID], day, now)
			if err != nil {
				log.Printf("[dashboard_cache] backfill: failed to build row org_id=%d day=%s err=%v — skipping this org/day", orgID, day, err)
				continue
			}
			rowsByOrg[orgID] = row
		}
		log.Printf("[dashboard_cache] backfill: merged day=%s", day)
	}

	finalRows := make([]dashboard.CacheRow, 0, len(rowsByOrg))
	for _, row := range rowsByOrg {
		finalRows = append(finalRows, row)
	}
	if err := dm.cacheStore.UpsertBatch(queryCtx, finalRows); err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}

	log.Printf("[dashboard_cache] backfill: wrote dashboard_cache for %d org(s)", len(finalRows))
	return nil
}

// buildCacheRow merges today's freshly computed layer data into the org's existing json_raw (older entries untouched), then derives final_json from it.
func (dm *DashboardManager) buildCacheRow(orgID int64, existingRow dashboard.CacheRow, layerData map[string]reviewLayerAggregate, today string, now time.Time) (dashboard.CacheRow, error) {
	jsonRawMap := map[string]json.RawMessage{}
	if len(existingRow.JSONRaw) > 0 {
		if err := json.Unmarshal(existingRow.JSONRaw, &jsonRawMap); err != nil {
			log.Printf("[dashboard_cache] failed to parse existing json_raw org_id=%d err=%v — starting fresh", orgID, err)
			jsonRawMap = map[string]json.RawMessage{}
		}
	}

	if layerData == nil {
		layerData = map[string]reviewLayerAggregate{}
	}

	todayEntry := map[string]json.RawMessage{}
	if existingDay, ok := jsonRawMap[today]; ok {
		if err := json.Unmarshal(existingDay, &todayEntry); err != nil {
			log.Printf("[dashboard_cache] failed to parse existing today entry org_id=%d err=%v — starting fresh", orgID, err)
			todayEntry = map[string]json.RawMessage{}
		}
	}
	layerBytes, err := json.Marshal(layerData)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal today's layer data: %w", err)
	}
	todayEntry["review_layers"] = layerBytes

	todayEntryBytes, err := json.Marshal(todayEntry)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal today's entry: %w", err)
	}
	jsonRawMap[today] = todayEntryBytes

	newJSONRaw, err := json.Marshal(jsonRawMap)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal json_raw: %w", err)
	}

	finalMap := map[string]json.RawMessage{}
	if len(existingRow.FinalJSON) > 0 {
		if err := json.Unmarshal(existingRow.FinalJSON, &finalMap); err != nil {
			log.Printf("[dashboard_cache] failed to parse existing final_json org_id=%d err=%v — starting fresh", orgID, err)
			finalMap = map[string]json.RawMessage{}
		}
	}
	finalReviewLayersBytes, err := json.Marshal(deriveReviewLayersPeriods(jsonRawMap, today))
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal final review_layers: %w", err)
	}
	finalMap["review_layers"] = finalReviewLayersBytes

	newFinalJSON, err := json.Marshal(finalMap)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal final_json: %w", err)
	}

	return dashboard.CacheRow{
		OrgID:     orgID,
		JSONRaw:   newJSONRaw,
		FinalJSON: newFinalJSON,
		UpdatedAt: now,
	}, nil
}

// RefreshOrgReviewLayers recomputes and stores review-layers for one org on demand, reusing the same query/merge/derive steps as the batched tick.
func (dm *DashboardManager) RefreshOrgReviewLayers(ctx context.Context, orgID int64) (map[string][]finalLayerRow, error) {
	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	today := time.Now().UTC().Format("2006-01-02")

	todayData, err := dm.fetchReviewLayersToday(queryCtx, []int64{orgID}, today)
	if err != nil {
		return nil, fmt.Errorf("fetch review layers: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, []int64{orgID})
	if err != nil {
		return nil, fmt.Errorf("get existing cache row: %w", err)
	}

	row, err := dm.buildCacheRow(orgID, existing[orgID], todayData[orgID], today, time.Now())
	if err != nil {
		return nil, fmt.Errorf("build cache row: %w", err)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, []dashboard.CacheRow{row}); err != nil {
		return nil, fmt.Errorf("upsert cache row: %w", err)
	}

	var finalWrapper struct {
		ReviewLayers map[string][]finalLayerRow `json:"review_layers"`
	}
	if err := json.Unmarshal(row.FinalJSON, &finalWrapper); err != nil {
		return nil, fmt.Errorf("unmarshal final_json: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed review_layers trigger=manual org_id=%d duration_ms=%d", orgID, time.Since(start).Milliseconds())
	return finalWrapper.ReviewLayers, nil
}

// fetchReviewLayersToday runs reviewLayersSourceQuery once for every org; reviews_run counts distinct review IDs since one review can produce multiple issue rows.
func (dm *DashboardManager) fetchReviewLayersToday(ctx context.Context, orgIDs []int64, dayUTC string) (map[int64]map[string]reviewLayerAggregate, error) {
	dayStart, err := time.Parse("2006-01-02", dayUTC)
	if err != nil {
		return nil, fmt.Errorf("parse day: %w", err)
	}

	rows, err := dm.db.QueryContext(ctx, reviewLayersSourceQuery, pq.Array(orgIDs), dayStart)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	type key struct {
		orgID       int64
		triggerType string
	}
	reviewIDs := map[key]map[int64]struct{}{}
	issuesFound := map[key]int{}
	categoryCounts := map[key]map[string]int{}

	for rows.Next() {
		var orgID, reviewID int64
		var triggerType string
		var category sql.NullString
		if err := rows.Scan(&orgID, &reviewID, &triggerType, &category); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		k := key{orgID, triggerType}
		if reviewIDs[k] == nil {
			reviewIDs[k] = map[int64]struct{}{}
		}
		reviewIDs[k][reviewID] = struct{}{}

		if category.Valid {
			issuesFound[k]++
			if categoryCounts[k] == nil {
				categoryCounts[k] = map[string]int{}
			}
			categoryCounts[k][category.String]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	result := map[int64]map[string]reviewLayerAggregate{}
	for k, ids := range reviewIDs {
		if result[k.orgID] == nil {
			result[k.orgID] = map[string]reviewLayerAggregate{}
		}
		result[k.orgID][k.triggerType] = reviewLayerAggregate{
			ReviewsRun:  len(ids),
			IssuesFound: issuesFound[k],
			Categories:  categoryCounts[k],
		}
	}
	return result, nil
}

// deriveReviewLayersPeriods sums json_raw's daily entries into day/week/month/all — safe since each count belongs to exactly one calendar day.
func deriveReviewLayersPeriods(jsonRawMap map[string]json.RawMessage, today string) map[string][]finalLayerRow {
	dates := make([]string, 0, len(jsonRawMap))
	for d := range jsonRawMap {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	todayIdx := -1
	for i, d := range dates {
		if d == today {
			todayIdx = i
			break
		}
	}

	windowDates := func(days int) []string {
		if todayIdx < 0 {
			return nil
		}
		end := todayIdx + days
		if end > len(dates) {
			end = len(dates)
		}
		return dates[todayIdx:end]
	}

	sum := func(days []string) []finalLayerRow {
		totals := map[string]*reviewLayerAggregate{}
		for _, d := range days {
			for triggerType, agg := range extractDayLayers(jsonRawMap[d]) {
				t, ok := totals[triggerType]
				if !ok {
					t = &reviewLayerAggregate{Categories: map[string]int{}}
					totals[triggerType] = t
				}
				t.ReviewsRun += agg.ReviewsRun
				t.IssuesFound += agg.IssuesFound
				for cat, count := range agg.Categories {
					t.Categories[cat] += count
				}
			}
		}
		return toFinalRows(totals)
	}

	return map[string][]finalLayerRow{
		"day":   sum(windowDates(1)),
		"week":  sum(windowDates(7)),
		"month": sum(windowDates(30)),
		"all":   sum(dates),
	}
}

func extractDayLayers(dayEntryRaw json.RawMessage) dayLayerEntry {
	if len(dayEntryRaw) == 0 {
		return dayLayerEntry{}
	}
	var wrapper struct {
		ReviewLayers dayLayerEntry `json:"review_layers"`
	}
	if err := json.Unmarshal(dayEntryRaw, &wrapper); err != nil || wrapper.ReviewLayers == nil {
		return dayLayerEntry{}
	}
	return wrapper.ReviewLayers
}

func toFinalRows(totals map[string]*reviewLayerAggregate) []finalLayerRow {
	rows := make([]finalLayerRow, 0, len(reviewLayerOrder))
	for _, triggerType := range reviewLayerOrder {
		agg := totals[triggerType]
		if agg == nil {
			agg = &reviewLayerAggregate{Categories: map[string]int{}}
		}
		categories := make([]categoryCountRow, 0, len(reviewLayerTaxonomyCategories))
		for _, cat := range reviewLayerTaxonomyCategories {
			categories = append(categories, categoryCountRow{Category: cat, Count: agg.Categories[cat]})
		}
		rows = append(rows, finalLayerRow{
			ID:          triggerType,
			Label:       reviewLayerLabels[triggerType],
			ReviewsRun:  agg.ReviewsRun,
			IssuesFound: agg.IssuesFound,
			Categories:  categories,
		})
	}
	return rows
}
