package reviews

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TaxonomyReportStore provides read-only aggregate queries over persisted review taxonomy fields.
// Findings are read from reviews.metadata.review_result.comments JSON.
type TaxonomyReportStore struct {
	db *sql.DB
}

func NewTaxonomyReportStore(db *sql.DB) *TaxonomyReportStore {
	return &TaxonomyReportStore{db: db}
}

// TaxonomyFilter holds query-time filter criteria. Zero values mean "no filter".
type TaxonomyFilter struct {
	OrgID       int64 // 0 = all orgs (super-admin only)
	Since       time.Time
	Until       time.Time
	Repository  string
	Provider    string
	Severity    string
	Confidence  string
	IssueType   string // "type" is a reserved word; use IssueType
	Category    string
	Subcategory string
}

// TaxonomySummary is the aggregate KPI response.
type TaxonomySummary struct {
	TotalFindings    int64 `json:"total_findings"`
	TotalReviews     int64 `json:"total_reviews"`
	CriticalCount    int64 `json:"critical_count"`
	HighCount        int64 `json:"high_count"`
	MediumCount      int64 `json:"medium_count"`
	LowCount         int64 `json:"low_count"`
	InfoCount        int64 `json:"info_count"`
	HighConfidence   int64 `json:"high_confidence_count"`
	MediumConfidence int64 `json:"medium_confidence_count"`
	LowConfidence    int64 `json:"low_confidence_count"`
}

// TaxonomyDistributionRow is one bucket in a distribution (severity/confidence/type/category/subcategory).
type TaxonomyDistributionRow struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
	Count     int64  `json:"count"`
}

// TaxonomyTrendRow is one time bucket in a trend series.
type TaxonomyTrendRow struct {
	Bucket      string `json:"bucket"`
	Count       int64  `json:"count"`
	ReviewCount int64  `json:"review_count"`
}

// TaxonomyBreakdownRow is one row in an org/repo/provider breakdown.
type TaxonomyBreakdownRow struct {
	OrgID       *int64  `json:"org_id,omitempty"`
	OrgName     *string `json:"org_name,omitempty"`
	Repository  string  `json:"repository"`
	Provider    string  `json:"provider"`
	Count       int64   `json:"count"`
	ReviewCount int64   `json:"review_count"`
}

// TaxonomyFindingRow is one raw finding row for the explorer table.
type TaxonomyFindingRow struct {
	CommentID   int64   `json:"comment_id"`
	ReviewID    int64   `json:"review_id"`
	OrgID       int64   `json:"org_id"`
	Repository  string  `json:"repository"`
	Provider    string  `json:"provider"`
	FilePath    *string `json:"file_path"`
	LineNumber  *int    `json:"line_number"`
	Severity    string  `json:"severity"`
	Confidence  string  `json:"confidence"`
	IssueType   string  `json:"type"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Content     string  `json:"content"`
	CreatedAt   string  `json:"created_at"`
}

// TaxonomyFindingsOptions controls server-side sorting and column-level filtering
// for findings explorer queries.
type TaxonomyFindingsOptions struct {
	SortBy        string
	SortDirection string
	ColumnFilters map[string]string
}

// TaxonomyRelationRow represents category -> subcategory relationship counts.
type TaxonomyRelationRow struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Count       int64  `json:"count"`
}

// buildWhereClause builds a parameterised WHERE clause from the filter.
// baseArg is the index of the next SQL argument ($1, $2 …). Returns clause and args.
func (s *TaxonomyReportStore) buildWhereClause(f TaxonomyFilter, baseArg int) (string, []interface{}) {
	var parts []string
	var args []interface{}
	idx := baseArg

	collectMulti := func(raw string) []string {
		chunks := strings.Split(raw, ",")
		out := make([]string, 0, len(chunks))
		seen := map[string]bool{}
		for _, c := range chunks {
			v := strings.TrimSpace(c)
			if v == "" {
				continue
			}
			k := strings.ToLower(v)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, v)
		}
		return out
	}

	addMultiFilter := func(expr string, raw string, caseInsensitive bool) {
		vals := collectMulti(raw)
		if len(vals) == 0 {
			return
		}
		if len(vals) == 1 {
			if caseInsensitive {
				parts = append(parts, fmt.Sprintf("lower(%s) = lower($%d)", expr, idx))
			} else {
				parts = append(parts, fmt.Sprintf("%s = $%d", expr, idx))
			}
			args = append(args, vals[0])
			idx++
			return
		}
		placeholders := make([]string, 0, len(vals))
		for _, v := range vals {
			if caseInsensitive {
				placeholders = append(placeholders, fmt.Sprintf("lower($%d)", idx))
			} else {
				placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			}
			args = append(args, v)
			idx++
		}
		if caseInsensitive {
			parts = append(parts, fmt.Sprintf("lower(%s) IN (%s)", expr, strings.Join(placeholders, ",")))
		} else {
			parts = append(parts, fmt.Sprintf("%s IN (%s)", expr, strings.Join(placeholders, ",")))
		}
	}

	// Only rows that look like persisted line findings.
	parts = append(parts, fmt.Sprintf("%s <> ''", findingFilePathExpr))
	parts = append(parts, fmt.Sprintf("%s IS NOT NULL", findingLineExpr))

	if f.OrgID > 0 {
		parts = append(parts, fmt.Sprintf("rc.org_id = $%d", idx))
		args = append(args, f.OrgID)
		idx++
	}

	if !f.Since.IsZero() {
		parts = append(parts, fmt.Sprintf("rc.created_at >= $%d", idx))
		args = append(args, f.Since)
		idx++
	}
	if !f.Until.IsZero() {
		parts = append(parts, fmt.Sprintf("rc.created_at < $%d", idx))
		args = append(args, f.Until)
		idx++
	}

	addMultiFilter("rc.repository", f.Repository, false)
	addMultiFilter("rc.provider", f.Provider, false)
	addMultiFilter(findingSeverityExpr, f.Severity, true)
	addMultiFilter(findingConfidenceExpr, f.Confidence, true)
	addMultiFilter(findingTypeExpr, f.IssueType, true)
	addMultiFilter(findingCategoryExpr, f.Category, true)
	addMultiFilter(findingSubcategoryExpr, f.Subcategory, true)

	// Suppress compiler warning for idx; it's used dynamically above.
	_ = idx

	clause := strings.Join(parts, " AND ")
	return clause, args
}

const findingFilePathExpr = "COALESCE(NULLIF(c.comment->>'file_path',''), NULLIF(c.comment->>'FilePath',''))"
const findingLineExpr = "CASE WHEN COALESCE(NULLIF(c.comment->>'line',''), NULLIF(c.comment->>'Line','')) ~ '^[0-9]+$' THEN COALESCE(NULLIF(c.comment->>'line',''), NULLIF(c.comment->>'Line',''))::int END"
const findingSeverityExpr = "COALESCE(NULLIF(c.comment->>'severity',''), NULLIF(c.comment->>'Severity',''))"
const findingConfidenceExpr = "COALESCE(NULLIF(c.comment->>'confidence',''), NULLIF(c.comment->>'Confidence',''))"
const findingTypeExpr = "COALESCE(NULLIF(c.comment->>'type',''), NULLIF(c.comment->>'Type',''))"
const findingCategoryExpr = "COALESCE(NULLIF(c.comment->>'category',''), NULLIF(c.comment->>'Category',''))"
const findingSubcategoryExpr = "COALESCE(NULLIF(c.comment->>'subcategory',''), NULLIF(c.comment->>'Subcategory',''))"
const findingContentExpr = "COALESCE(NULLIF(c.comment->>'content',''), NULLIF(c.comment->>'Content',''))"

// baseFrom is the normalized source used by all taxonomy queries.
// review_result.comments can be array, object, or other scalar JSON values.
const baseFrom = `
FROM (
	SELECT
		r.id,
		r.org_id,
		r.repository,
		r.provider,
		r.created_at,
		CASE
			WHEN jsonb_typeof(r.metadata->'review_result'->'comments') = 'array' THEN r.metadata->'review_result'->'comments'
			WHEN jsonb_typeof(r.metadata->'review_result'->'comments') = 'object' THEN jsonb_build_array(r.metadata->'review_result'->'comments')
			ELSE '[]'::jsonb
		END AS comments_json
	FROM reviews r
) rc
JOIN LATERAL jsonb_array_elements(rc.comments_json) AS c(comment) ON true
`

// GetSummary returns aggregate KPIs for the given filter.
func (s *TaxonomyReportStore) GetSummary(ctx context.Context, f TaxonomyFilter) (*TaxonomySummary, error) {
	fmt.Printf("[DEBUG] GetSummary called with filter={OrgID:%d, Since:%v, Until:%v, Repository:%q, Provider:%q, Severity:%q, Confidence:%q, IssueType:%q, Category:%q, Subcategory:%q}\n",
		f.OrgID, f.Since, f.Until, f.Repository, f.Provider, f.Severity, f.Confidence, f.IssueType, f.Category, f.Subcategory)

	where, args := s.buildWhereClause(f, 1)
	fmt.Printf("[DEBUG] GetSummary buildWhereClause returned: where=%q, args=%v\n", where, args)

	q := fmt.Sprintf(`
SELECT
  COUNT(*)                                                                          AS total_findings,
	COUNT(DISTINCT rc.id)                                                            AS total_reviews,
	COUNT(*) FILTER (WHERE lower(%s) = 'critical')                                  AS critical_count,
	COUNT(*) FILTER (WHERE lower(%s) IN ('high','error'))                           AS high_count,
	COUNT(*) FILTER (WHERE lower(%s) IN ('medium','warning'))                       AS medium_count,
	COUNT(*) FILTER (WHERE lower(%s) = 'low')                                       AS low_count,
	COUNT(*) FILTER (WHERE lower(%s) = 'info')                                      AS info_count,
	COUNT(*) FILTER (WHERE lower(%s) = 'high')                                      AS high_confidence,
	COUNT(*) FILTER (WHERE lower(%s) = 'medium')                                    AS medium_confidence,
	COUNT(*) FILTER (WHERE lower(%s) = 'low')                                       AS low_confidence
%s
WHERE %s
`, findingSeverityExpr, findingSeverityExpr, findingSeverityExpr, findingSeverityExpr, findingSeverityExpr, findingConfidenceExpr, findingConfidenceExpr, findingConfidenceExpr, baseFrom, where)

	fmt.Printf("[DEBUG] Executing summary query with args: %v\n", args)

	var row TaxonomySummary
	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&row.TotalFindings,
		&row.TotalReviews,
		&row.CriticalCount,
		&row.HighCount,
		&row.MediumCount,
		&row.LowCount,
		&row.InfoCount,
		&row.HighConfidence,
		&row.MediumConfidence,
		&row.LowConfidence,
	)
	if err != nil {
		return nil, fmt.Errorf("taxonomy summary query: %w", err)
	}
	fmt.Printf("[DEBUG] GetSummary result: TotalFindings=%d, TotalReviews=%d\n", row.TotalFindings, row.TotalReviews)
	return &row, nil
}

// GetDistribution returns per-value counts for one dimension.
// dimension must be one of: severity, confidence, type, category, subcategory.
func (s *TaxonomyReportStore) GetDistribution(ctx context.Context, dimension string, f TaxonomyFilter) ([]TaxonomyDistributionRow, error) {
	allowed := map[string]bool{
		"severity":    true,
		"confidence":  true,
		"type":        true,
		"category":    true,
		"subcategory": true,
	}
	if !allowed[dimension] {
		return nil, fmt.Errorf("invalid dimension %q", dimension)
	}

	where, args := s.buildWhereClause(f, 1)
	q := fmt.Sprintf(`
SELECT
  $%d::text                           AS dimension,
	COALESCE(NULLIF(
		CASE $%d
			WHEN 'severity' THEN %s
			WHEN 'confidence' THEN %s
			WHEN 'type' THEN %s
			WHEN 'category' THEN %s
			WHEN 'subcategory' THEN %s
			ELSE ''
		END,
	''), '')                            AS value,
  COUNT(*)                            AS count
%s
WHERE %s
GROUP BY value
ORDER BY count DESC
`, len(args)+1, len(args)+2, findingSeverityExpr, findingConfidenceExpr, findingTypeExpr, findingCategoryExpr, findingSubcategoryExpr, baseFrom, where)

	args = append(args, dimension, dimension)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("taxonomy distribution query (%s): %w", dimension, err)
	}
	defer rows.Close()

	var out []TaxonomyDistributionRow
	for rows.Next() {
		var r TaxonomyDistributionRow
		if err := rows.Scan(&r.Dimension, &r.Value, &r.Count); err != nil {
			return nil, fmt.Errorf("taxonomy distribution scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTrend returns per-bucket finding counts. grain must be one of: day, week, month.
func (s *TaxonomyReportStore) GetTrend(ctx context.Context, grain string, f TaxonomyFilter) ([]TaxonomyTrendRow, error) {
	allowed := map[string]bool{"day": true, "week": true, "month": true}
	if !allowed[grain] {
		return nil, fmt.Errorf("invalid grain %q", grain)
	}

	fmt.Printf("[DEBUG] GetTrend called with grain=%q, filter={OrgID:%d, Since:%v, Until:%v, Repository:%q, Provider:%q, Severity:%q, Confidence:%q, IssueType:%q, Category:%q, Subcategory:%q}\n",
		grain, f.OrgID, f.Since, f.Until, f.Repository, f.Provider, f.Severity, f.Confidence, f.IssueType, f.Category, f.Subcategory)

	where, args := s.buildWhereClause(f, 1)
	fmt.Printf("[DEBUG] buildWhereClause returned: where=%q, args=%v\n", where, args)

	// Use date_trunc with a literal interval value safely via a fixed string (no user input).
	q := fmt.Sprintf(`
SELECT
	date_trunc('%s', rc.created_at)::text AS bucket,
	COUNT(*)                             AS count,
	COUNT(DISTINCT rc.id)                AS review_count
%s
WHERE %s
GROUP BY bucket
ORDER BY bucket ASC
`, grain, baseFrom, where)

	fmt.Printf("[DEBUG] Executing trend query:\n%s\nWith args: %v\n", q, args)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("taxonomy trend query: %w", err)
	}
	defer rows.Close()

	var out []TaxonomyTrendRow
	rowCount := 0
	for rows.Next() {
		var r TaxonomyTrendRow
		if err := rows.Scan(&r.Bucket, &r.Count, &r.ReviewCount); err != nil {
			return nil, fmt.Errorf("taxonomy trend scan: %w", err)
		}
		fmt.Printf("[DEBUG] Scanned trend row: bucket=%q, count=%d, review_count=%d\n", r.Bucket, r.Count, r.ReviewCount)
		out = append(out, r)
		rowCount++
	}
	fmt.Printf("[DEBUG] GetTrend completed: scanned %d rows, returning %d rows. Final result: %+v\n", rowCount, len(out), out)
	return out, rows.Err()
}

// GetBreakdown returns per-org/repo/provider finding counts.
// includeOrgName controls whether to JOIN orgs for the name (super-admin mode).
func (s *TaxonomyReportStore) GetBreakdown(ctx context.Context, f TaxonomyFilter, includeOrgName bool) ([]TaxonomyBreakdownRow, error) {
	where, args := s.buildWhereClause(f, 1)

	orgCols := "NULL::bigint, NULL::text"
	joinOrgs := ""
	if includeOrgName {
		orgCols = "rc.org_id, o.name"
		joinOrgs = "LEFT JOIN orgs o ON o.id = rc.org_id"
	}

	q := fmt.Sprintf(`
SELECT
  %s,
	COALESCE(rc.repository, ''),
	COALESCE(rc.provider, ''),
	COUNT(*) AS count,
	COUNT(DISTINCT rc.id) AS review_count
%s
%s
WHERE %s
GROUP BY rc.org_id, o_name, rc.repository, rc.provider
ORDER BY count DESC
LIMIT 200
`, orgCols, baseFrom, joinOrgs, where)

	// Replace the placeholder GROUP BY alias for the org name column.
	if includeOrgName {
		q = strings.Replace(q, "o_name", "o.name", 1)
	} else {
		q = strings.Replace(q, "o_name", "1", 1)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("taxonomy breakdown query: %w", err)
	}
	defer rows.Close()

	var out []TaxonomyBreakdownRow
	for rows.Next() {
		var r TaxonomyBreakdownRow
		var orgID sql.NullInt64
		var orgName sql.NullString
		if err := rows.Scan(&orgID, &orgName, &r.Repository, &r.Provider, &r.Count, &r.ReviewCount); err != nil {
			return nil, fmt.Errorf("taxonomy breakdown scan: %w", err)
		}
		if orgID.Valid {
			v := orgID.Int64
			r.OrgID = &v
		}
		if orgName.Valid {
			v := orgName.String
			r.OrgName = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFindings returns paginated raw finding rows for the explorer table.
func (s *TaxonomyReportStore) ListFindings(ctx context.Context, f TaxonomyFilter, limit, offset int, opts TaxonomyFindingsOptions) ([]TaxonomyFindingRow, int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	where, args := s.buildWhereClause(f, 1)
	parts := []string{where}
	idx := len(args) + 1

	addTextFilter := func(expr, key string) {
		v := strings.TrimSpace(opts.ColumnFilters[key])
		if v == "" {
			return
		}
		parts = append(parts, fmt.Sprintf("%s ILIKE $%d", expr, idx))
		args = append(args, "%"+v+"%")
		idx++
	}

	addTextFilter(findingSeverityExpr, "severity")
	addTextFilter(findingConfidenceExpr, "confidence")
	addTextFilter(findingTypeExpr, "type")
	addTextFilter(findingCategoryExpr, "category")
	addTextFilter(findingSubcategoryExpr, "subcategory")
	addTextFilter("rc.repository", "repository")
	addTextFilter("rc.provider", "provider")
	addTextFilter(findingFilePathExpr, "file_path")
	addTextFilter(findingContentExpr, "content")
	addTextFilter("to_char(rc.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')", "created_at")

	if v := strings.TrimSpace(opts.ColumnFilters["line_number"]); v != "" {
		parts = append(parts, fmt.Sprintf("CAST(%s AS TEXT) = $%d", findingLineExpr, idx))
		args = append(args, v)
		idx++
	}

	combinedWhere := strings.Join(parts, " AND ")

	allowedSortExprs := map[string]string{
		"severity":    fmt.Sprintf("lower(%s)", findingSeverityExpr),
		"confidence":  fmt.Sprintf("lower(%s)", findingConfidenceExpr),
		"type":        fmt.Sprintf("lower(%s)", findingTypeExpr),
		"category":    fmt.Sprintf("lower(%s)", findingCategoryExpr),
		"subcategory": fmt.Sprintf("lower(%s)", findingSubcategoryExpr),
		"repository":  "lower(rc.repository)",
		"provider":    "lower(rc.provider)",
		"file_path":   fmt.Sprintf("lower(%s)", findingFilePathExpr),
		"line_number": findingLineExpr,
		"created_at":  "rc.created_at",
	}
	sortExpr := "rc.created_at"
	if expr, ok := allowedSortExprs[strings.TrimSpace(opts.SortBy)]; ok {
		sortExpr = expr
	}
	sortDirection := "DESC"
	if strings.EqualFold(strings.TrimSpace(opts.SortDirection), "asc") {
		sortDirection = "ASC"
	}
	orderBy := fmt.Sprintf("%s %s, rc.created_at DESC, rc.id DESC", sortExpr, sortDirection)

	countQ := fmt.Sprintf(`SELECT COUNT(*) %s WHERE %s`, baseFrom, combinedWhere)
	var total int64
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("taxonomy findings count: %w", err)
	}

	// Build the paginated select; limit/offset are appended as the last two args.
	dataQ := fmt.Sprintf(`
SELECT
	ROW_NUMBER() OVER (ORDER BY %s)::bigint,
	rc.id,
	rc.org_id,
  COALESCE(rc.repository, ''),
  COALESCE(rc.provider, ''),
	NULLIF(%s, ''),
	%s,
	COALESCE(%s, ''),
	COALESCE(%s, ''),
	COALESCE(%s, ''),
	COALESCE(%s, ''),
	COALESCE(%s, ''),
	COALESCE(%s, ''),
	rc.created_at
%s
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, orderBy, findingFilePathExpr, findingLineExpr, findingSeverityExpr, findingConfidenceExpr, findingTypeExpr, findingCategoryExpr, findingSubcategoryExpr, findingContentExpr, baseFrom, combinedWhere, orderBy, len(args)+1, len(args)+2)

	dataArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("taxonomy findings query: %w", err)
	}
	defer rows.Close()

	var out []TaxonomyFindingRow
	for rows.Next() {
		var r TaxonomyFindingRow
		var createdAt time.Time
		if err := rows.Scan(
			&r.CommentID, &r.ReviewID, &r.OrgID,
			&r.Repository, &r.Provider,
			&r.FilePath, &r.LineNumber,
			&r.Severity, &r.Confidence, &r.IssueType,
			&r.Category, &r.Subcategory,
			&r.Content,
			&createdAt,
		); err != nil {
			return nil, 0, fmt.Errorf("taxonomy findings scan: %w", err)
		}
		r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// GetCategorySubcategoryRelations returns relation rows for category/subcategory with counts.
func (s *TaxonomyReportStore) GetCategorySubcategoryRelations(ctx context.Context, f TaxonomyFilter) ([]TaxonomyRelationRow, error) {
	where, args := s.buildWhereClause(f, 1)
	q := fmt.Sprintf(`
SELECT
	COALESCE(%s, '') AS category,
	COALESCE(%s, '') AS subcategory,
	COUNT(*) AS count
%s
WHERE %s
GROUP BY category, subcategory
ORDER BY category ASC, count DESC
`, findingCategoryExpr, findingSubcategoryExpr, baseFrom, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("taxonomy relations query: %w", err)
	}
	defer rows.Close()

	out := make([]TaxonomyRelationRow, 0)
	for rows.Next() {
		var r TaxonomyRelationRow
		if err := rows.Scan(&r.Category, &r.Subcategory, &r.Count); err != nil {
			return nil, fmt.Errorf("taxonomy relations scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
