package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/livereview/storage/dashboard"
)

// calendarLookbackDays matches the "Daily review activity over the last 7 months" widget description.
const calendarLookbackDays = 210

// peopleContributor is one person's rollup for a single trailing-window period.
type peopleContributor struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	ReviewsGiven int    `json:"reviews_given"`
	LOCReviewed  int    `json:"loc_reviewed"`
}

// peopleContributorsByWindow: same trailing-window shape as system_overview's ratesByWindow —
// "top reviewer" should reflect recent activity, not a cumulative-as-of-cutoff ranking.
type peopleContributorsByWindow struct {
	Day   []peopleContributor `json:"day"`
	Week  []peopleContributor `json:"week"`
	Month []peopleContributor `json:"month"`
	All   []peopleContributor `json:"all"`
}

type calendarDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type peopleSnapshot struct {
	Contributors         peopleContributorsByWindow `json:"contributors"`
	ContributionCalendar []calendarDay              `json:"contribution_calendar"`
}

// contributorsQuery rolls up review count + LOC reviewed per person, per trailing window, in one
// pass over reviews (LEFT JOIN users only for a nicer display name — falls back to email when unset).
// LOC is cast via double precision, not bigint, since it's read back out of JSONB text and a plain
// integer-looking cast would fail if any value were ever written with a decimal point.
const contributorsQuery = `
SELECT
  r.org_id,
  r.user_email,
  u.first_name,
  u.last_name,
  COUNT(*) FILTER (WHERE r.created_at >= $2) AS reviews_day,
  COALESCE(SUM((r.metadata->>'operation_billable_loc')::double precision) FILTER (WHERE r.created_at >= $2), 0) AS loc_day,
  COUNT(*) FILTER (WHERE r.created_at >= $3) AS reviews_week,
  COALESCE(SUM((r.metadata->>'operation_billable_loc')::double precision) FILTER (WHERE r.created_at >= $3), 0) AS loc_week,
  COUNT(*) FILTER (WHERE r.created_at >= $4) AS reviews_month,
  COALESCE(SUM((r.metadata->>'operation_billable_loc')::double precision) FILTER (WHERE r.created_at >= $4), 0) AS loc_month,
  COUNT(*) AS reviews_all,
  COALESCE(SUM((r.metadata->>'operation_billable_loc')::double precision), 0) AS loc_all
FROM reviews r
LEFT JOIN users u ON u.email = r.user_email
WHERE r.org_id = ANY($1) AND r.user_email IS NOT NULL AND r.user_email <> ''
GROUP BY r.org_id, r.user_email, u.first_name, u.last_name
`

func contributorDisplayName(email string, firstName, lastName *string) string {
	parts := make([]string, 0, 2)
	if firstName != nil && strings.TrimSpace(*firstName) != "" {
		parts = append(parts, strings.TrimSpace(*firstName))
	}
	if lastName != nil && strings.TrimSpace(*lastName) != "" {
		parts = append(parts, strings.TrimSpace(*lastName))
	}
	if len(parts) == 0 {
		return email
	}
	return strings.Join(parts, " ")
}

func (dm *DashboardManager) fetchContributors(ctx context.Context, orgIDs []int64, c systemOverviewCutoffs) (map[int64]peopleContributorsByWindow, error) {
	rows, err := dm.db.QueryContext(ctx, contributorsQuery, pq.Array(orgIDs), c.dayStart, c.weekStart, c.monthStart)
	if err != nil {
		return nil, fmt.Errorf("query contributors: %w", err)
	}
	defer rows.Close()

	byOrg := map[int64][]peopleContributor4{}
	for rows.Next() {
		var orgID int64
		var email string
		var firstName, lastName *string
		var reviewsDay, reviewsWeek, reviewsMonth, reviewsAll int
		var locDay, locWeek, locMonth, locAll float64
		if err := rows.Scan(&orgID, &email, &firstName, &lastName,
			&reviewsDay, &locDay,
			&reviewsWeek, &locWeek,
			&reviewsMonth, &locMonth,
			&reviewsAll, &locAll,
		); err != nil {
			return nil, fmt.Errorf("scan contributors: %w", err)
		}
		name := contributorDisplayName(email, firstName, lastName)
		byOrg[orgID] = append(byOrg[orgID], peopleContributor4{
			Day:   peopleContributor{Email: email, Name: name, ReviewsGiven: reviewsDay, LOCReviewed: int(locDay)},
			Week:  peopleContributor{Email: email, Name: name, ReviewsGiven: reviewsWeek, LOCReviewed: int(locWeek)},
			Month: peopleContributor{Email: email, Name: name, ReviewsGiven: reviewsMonth, LOCReviewed: int(locMonth)},
			All:   peopleContributor{Email: email, Name: name, ReviewsGiven: reviewsAll, LOCReviewed: int(locAll)},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows contributors: %w", err)
	}

	result := map[int64]peopleContributorsByWindow{}
	for orgID, contributors := range byOrg {
		day := make([]peopleContributor, 0, len(contributors))
		week := make([]peopleContributor, 0, len(contributors))
		month := make([]peopleContributor, 0, len(contributors))
		all := make([]peopleContributor, 0, len(contributors))
		for _, c := range contributors {
			day = append(day, c.Day)
			week = append(week, c.Week)
			month = append(month, c.Month)
			all = append(all, c.All)
		}
		sortContributorsByReviewsGiven(day)
		sortContributorsByReviewsGiven(week)
		sortContributorsByReviewsGiven(month)
		sortContributorsByReviewsGiven(all)
		result[orgID] = peopleContributorsByWindow{Day: day, Week: week, Month: month, All: all}
	}
	return result, nil
}

// peopleContributor4 bundles one person's four period variants together while scanning, before splitting into the four period-keyed lists.
type peopleContributor4 struct {
	Day, Week, Month, All peopleContributor
}

func sortContributorsByReviewsGiven(contributors []peopleContributor) {
	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].ReviewsGiven > contributors[j].ReviewsGiven
	})
}

// contributionCalendarQuery: plain indexed COUNT/GROUP BY, no JSON parsing — cheap enough to
// recompute live every tick over the full lookback window, same reasoning as system_overview's counts.
const contributionCalendarQuery = `
SELECT org_id, DATE(created_at) AS day, COUNT(*)
FROM reviews
WHERE org_id = ANY($1) AND created_at >= $2
GROUP BY org_id, DATE(created_at)
`

func (dm *DashboardManager) fetchContributionCalendar(ctx context.Context, orgIDs []int64) (map[int64][]calendarDay, error) {
	since := time.Now().AddDate(0, 0, -calendarLookbackDays)
	rows, err := dm.db.QueryContext(ctx, contributionCalendarQuery, pq.Array(orgIDs), since)
	if err != nil {
		return nil, fmt.Errorf("query contribution calendar: %w", err)
	}
	defer rows.Close()

	result := map[int64][]calendarDay{}
	for rows.Next() {
		var orgID int64
		var day time.Time
		var count int
		if err := rows.Scan(&orgID, &day, &count); err != nil {
			return nil, fmt.Errorf("scan contribution calendar: %w", err)
		}
		result[orgID] = append(result[orgID], calendarDay{Date: day.Format("2006-01-02"), Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows contribution calendar: %w", err)
	}

	for orgID := range result {
		sort.Slice(result[orgID], func(i, j int) bool { return result[orgID][i].Date < result[orgID][j].Date })
	}
	return result, nil
}

// buildPeopleSnapshots runs every batched source query once (never per-org) and assembles one peopleSnapshot per org.
func (dm *DashboardManager) buildPeopleSnapshots(ctx context.Context, orgIDs []int64) (map[int64]peopleSnapshot, error) {
	cutoffs := newSystemOverviewCutoffs(time.Now())

	contributors, err := dm.fetchContributors(ctx, orgIDs, cutoffs)
	if err != nil {
		return nil, err
	}
	calendars, err := dm.fetchContributionCalendar(ctx, orgIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]peopleSnapshot, len(orgIDs))
	for _, orgID := range orgIDs {
		result[orgID] = peopleSnapshot{
			Contributors:         contributors[orgID],
			ContributionCalendar: calendars[orgID],
		}
	}
	return result, nil
}

// mergePeopleIntoCacheRow sets final_json.people, preserving json_raw and every other final_json
// key (review_layers, system_overview) byte-for-byte untouched — same pattern as mergeSystemOverviewIntoCacheRow.
func mergePeopleIntoCacheRow(orgID int64, existingRow dashboard.CacheRow, snapshot peopleSnapshot, now time.Time) (dashboard.CacheRow, error) {
	finalMap := map[string]json.RawMessage{}
	if len(existingRow.FinalJSON) > 0 {
		if err := json.Unmarshal(existingRow.FinalJSON, &finalMap); err != nil {
			log.Printf("[dashboard_cache] failed to parse existing final_json org_id=%d err=%v — starting fresh", orgID, err)
			finalMap = map[string]json.RawMessage{}
		}
	}

	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return dashboard.CacheRow{}, fmt.Errorf("marshal people: %w", err)
	}
	finalMap["people"] = snapshotBytes

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

// refreshAllPeople is the people half of the shared 5-minute cycle: batched across all orgs, no
// per-org loop for any query, no json_raw involvement — same reasoning as system_overview, both
// contributor rollups and the calendar are cheap indexed aggregates recomputed live every tick.
//
// Must run its own GetBatch immediately before its own UpsertBatch, same ordering requirement as
// refreshAllSystemOverview — otherwise this write could revert another domain's just-written update.
func (dm *DashboardManager) refreshAllPeople(ctx context.Context, orgIDs []int64) error {
	if len(orgIDs) == 0 {
		return nil
	}

	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	snapshots, err := dm.buildPeopleSnapshots(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("build people snapshots: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, orgIDs)
	if err != nil {
		return fmt.Errorf("get existing cache batch: %w", err)
	}

	now := time.Now()
	rows := make([]dashboard.CacheRow, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		row, err := mergePeopleIntoCacheRow(orgID, existing[orgID], snapshots[orgID], now)
		if err != nil {
			log.Printf("[dashboard_cache] failed to build people row org_id=%d err=%v — skipping this org this tick", orgID, err)
			continue
		}
		rows = append(rows, row)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, rows); err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed people for %d/%d orgs duration_ms=%d", len(rows), len(orgIDs), time.Since(start).Milliseconds())
	return nil
}

// RefreshOrgPeople recomputes and stores people for one org on demand, reusing the same query logic as the batched tick.
func (dm *DashboardManager) RefreshOrgPeople(ctx context.Context, orgID int64) (peopleSnapshot, error) {
	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	snapshots, err := dm.buildPeopleSnapshots(queryCtx, []int64{orgID})
	if err != nil {
		return peopleSnapshot{}, fmt.Errorf("build people snapshot: %w", err)
	}

	existing, err := dm.cacheStore.GetBatch(queryCtx, []int64{orgID})
	if err != nil {
		return peopleSnapshot{}, fmt.Errorf("get existing cache row: %w", err)
	}

	row, err := mergePeopleIntoCacheRow(orgID, existing[orgID], snapshots[orgID], time.Now())
	if err != nil {
		return peopleSnapshot{}, fmt.Errorf("build cache row: %w", err)
	}

	if err := dm.cacheStore.UpsertBatch(queryCtx, []dashboard.CacheRow{row}); err != nil {
		return peopleSnapshot{}, fmt.Errorf("upsert cache row: %w", err)
	}

	log.Printf("[dashboard_cache] refreshed people trigger=manual org_id=%d duration_ms=%d", orgID, time.Since(start).Milliseconds())
	return snapshots[orgID], nil
}
