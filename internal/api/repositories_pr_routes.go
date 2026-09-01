package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// multiValueFilterClause builds an " AND column IN ($n, $n+1, ...)" clause from
// a comma-separated (and/or repeated) query param value - e.g.
// "github,gitlab" or ?provider=github&provider=gitlab - appending each
// distinct value to args and advancing *argIdx. Mirrors the multi-select
// convention already used by the taxonomy report filters
// (collectMultiQueryParam / addMultiFilter in taxonomy_report_handler.go).
// Returns "" if csv has no values, so callers can unconditionally append the
// result to both a count query and a select query sharing the same args.
func multiValueFilterClause(column, csv string, args *[]interface{}, argIdx *int) string {
	seen := make(map[string]bool)
	vals := make([]string, 0)
	for _, part := range strings.Split(csv, ",") {
		v := strings.TrimSpace(part)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return ""
	}
	placeholders := make([]string, len(vals))
	for i, v := range vals {
		placeholders[i] = fmt.Sprintf("$%d", *argIdx)
		*args = append(*args, v)
		*argIdx++
	}
	return fmt.Sprintf(" AND %s IN (%s)", column, strings.Join(placeholders, ","))
}

// providerFilterClause is like multiValueFilterClause but for the provider
// column specifically: stored values are compound strings like "github-com",
// "github-enterprise", "gitlab-com", "gitlab-self-hosted" (see
// internal/jobqueue/repo_sync_worker.go), not the plain "github"/"gitlab"
// short keys the UI's checkbox filter sends. An exact IN(...) match (as
// multiValueFilterClause does) would never match those compound values, so
// this does a prefix match per selected key instead.
func providerFilterClause(csv string, args *[]interface{}, argIdx *int) string {
	seen := make(map[string]bool)
	vals := make([]string, 0)
	for _, part := range strings.Split(csv, ",") {
		v := strings.TrimSpace(part)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return ""
	}
	conds := make([]string, len(vals))
	for i, v := range vals {
		conds[i] = fmt.Sprintf("provider ILIKE $%d", *argIdx)
		*args = append(*args, v+"%")
		*argIdx++
	}
	return " AND (" + strings.Join(conds, " OR ") + ")"
}

// RepositoryResponse is the JSON shape for a repositories row.
type RepositoryResponse struct {
	ID             int64      `json:"id"`
	OrgID          int64      `json:"org_id"`
	ConnectorID    int64      `json:"connector_id"`
	Provider       string     `json:"provider"`
	ProviderRepoID string     `json:"provider_repo_id"`
	FullName       string     `json:"full_name"`
	Name           string     `json:"name"`
	WebURL         string     `json:"web_url"`
	DefaultBranch  string     `json:"default_branch,omitempty"`
	IsPrivate      bool       `json:"is_private"`
	Description    string     `json:"description,omitempty"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	LastSyncStatus string     `json:"last_sync_status"`
	LastSyncError  string     `json:"last_sync_error,omitempty"`
	OpenPRCount    int        `json:"open_pr_count"`
	// LastActivityAt is the most recent provider_updated_at across this
	// repo's pull_requests - nil if it has none. Used as the default sort
	// key (most recently active repos first) since it reflects real PR/MR
	// activity, unlike LastSyncedAt which only reflects LiveReview's own
	// polling cadence.
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RepositoriesListResponse wraps a paginated repositories listing.
type RepositoriesListResponse struct {
	Repositories []RepositoryResponse `json:"repositories"`
	Total        int                  `json:"total"`
	Page         int                  `json:"page"`
	PerPage      int                  `json:"per_page"`
	TotalPages   int                  `json:"total_pages"`
}

// PullRequestResponse is the JSON shape for a pull_requests row.
type PullRequestResponse struct {
	ID                int64      `json:"id"`
	RepositoryID      int64      `json:"repository_id"`
	OrgID             int64      `json:"org_id"`
	Provider          string     `json:"provider"`
	ProviderPRID      string     `json:"provider_pr_id"`
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	Description       string     `json:"description,omitempty"`
	State             string     `json:"state"`
	AuthorUsername    string     `json:"author_username,omitempty"`
	AuthorName        string     `json:"author_name,omitempty"`
	AuthorAvatarURL   string     `json:"author_avatar_url,omitempty"`
	SourceBranch      string     `json:"source_branch,omitempty"`
	TargetBranch      string     `json:"target_branch,omitempty"`
	WebURL            string     `json:"web_url"`
	ProviderCreatedAt *time.Time `json:"provider_created_at,omitempty"`
	ProviderUpdatedAt *time.Time `json:"provider_updated_at,omitempty"`
	LastSyncedAt      time.Time  `json:"last_synced_at"`
	LastSyncedSource  string     `json:"last_synced_source"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// PullRequestsListResponse wraps a paginated pull_requests listing.
type PullRequestsListResponse struct {
	PullRequests []PullRequestResponse `json:"pull_requests"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	PerPage      int                   `json:"per_page"`
	TotalPages   int                   `json:"total_pages"`
}

// PullRequestReviewSummary is one review-run entry in a PR/MR's review history.
type PullRequestReviewSummary struct {
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	TriggerType string     `json:"trigger_type"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// PullRequestDetailResponse is a single PR/MR plus its review history.
type PullRequestDetailResponse struct {
	PullRequestResponse
	Reviews []PullRequestReviewSummary `json:"reviews"`
}

func parsePageParams(c echo.Context) (page, perPage int) {
	page = 1
	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 0 {
		page = p
	}
	perPage = 20
	if pp, err := strconv.Atoi(c.QueryParam("per_page")); err == nil && pp > 0 && pp <= 200 {
		perPage = pp
	}
	return page, perPage
}

func scanRepository(scan func(dest ...interface{}) error) (RepositoryResponse, error) {
	var r RepositoryResponse
	var defaultBranch, description, lastSyncError sql.NullString
	var lastSyncedAt sql.NullTime
	err := scan(
		&r.ID, &r.OrgID, &r.ConnectorID, &r.Provider, &r.ProviderRepoID, &r.FullName, &r.Name,
		&r.WebURL, &defaultBranch, &r.IsPrivate, &description, &lastSyncedAt, &r.LastSyncStatus,
		&lastSyncError, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return r, err
	}
	r.DefaultBranch = defaultBranch.String
	r.Description = description.String
	r.LastSyncError = lastSyncError.String
	if lastSyncedAt.Valid {
		r.LastSyncedAt = &lastSyncedAt.Time
	}
	return r, nil
}

const repositoryColumns = `id, org_id, connector_id, provider, provider_repo_id, full_name, name,
	web_url, default_branch, is_private, description, last_synced_at, last_sync_status,
	last_sync_error, created_at, updated_at`

// repositoryColumnsQualified is repositoryColumns qualified for ListRepositories' optional join against scheduled_review_configs (sort=schedule), which has its own id/created_at/updated_at.
const repositoryColumnsQualified = `repositories.id, repositories.org_id, repositories.connector_id, repositories.provider,
	repositories.provider_repo_id, repositories.full_name, repositories.name, repositories.web_url,
	repositories.default_branch, repositories.is_private, repositories.description, repositories.last_synced_at,
	repositories.last_sync_status, repositories.last_sync_error, repositories.created_at, repositories.updated_at`

// ListRepositories handles GET /api/v1/repositories.
func (s *Server) ListRepositories(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	page, perPage := parsePageParams(c)

	// sort=schedule needs a join against scheduled_review_configs; added conditionally so other sort keys don't pay for it.
	scheduleJoin := c.QueryParam("sort") == "schedule"
	fromClause := "FROM repositories"
	selectColumns := repositoryColumns
	if scheduleJoin {
		fromClause += " LEFT JOIN scheduled_review_configs sched ON sched.repository_id = repositories.id"
		selectColumns = repositoryColumnsQualified
	}

	// Safe: selectColumns and fromClause are compile-time constants (lines 190-202),
	// never derived from user input. All user-supplied values use $N placeholders.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query := `SELECT ` + selectColumns + `,
		(SELECT count(*) FROM pull_requests pr WHERE pr.repository_id = repositories.id AND pr.state = 'open') AS open_pr_count,
		(SELECT max(pr.provider_updated_at) FROM pull_requests pr WHERE pr.repository_id = repositories.id) AS last_pr_activity_at
		` + fromClause + ` WHERE repositories.org_id = $1`
	countQuery := `SELECT count(*) FROM repositories WHERE org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if clause := multiValueFilterClause("connector_id", c.QueryParam("connector_id"), &args, &argIdx); clause != "" {
		query += clause
		countQuery += clause
	}
	if clause := providerFilterClause(c.QueryParam("provider"), &args, &argIdx); clause != "" {
		query += clause
		countQuery += clause
	}
	if search := c.QueryParam("search"); search != "" {
		query += fmt.Sprintf(" AND full_name ILIKE $%d", argIdx)
		countQuery += fmt.Sprintf(" AND full_name ILIKE $%d", argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if clause := multiValueFilterClause("last_sync_status", c.QueryParam("sync_status"), &args, &argIdx); clause != "" {
		query += clause
		countQuery += clause
	}
	if hasOpenPRs := c.QueryParam("has_open_prs"); hasOpenPRs != "" {
		existsClause := " AND EXISTS (SELECT 1 FROM pull_requests pr WHERE pr.repository_id = repositories.id AND pr.state = 'open')"
		if hasOpenPRs == "false" {
			existsClause = " AND NOT EXISTS (SELECT 1 FROM pull_requests pr WHERE pr.repository_id = repositories.id AND pr.state = 'open')"
		}
		query += existsClause
		countQuery += existsClause
	}

	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to count repositories")
	}

	// Whitelisted column expressions only - "sort" is never interpolated
	// directly into the query, so this can't become a SQL injection vector
	// no matter what the query param contains.
	sortColumns := map[string]string{
		"repository":     "full_name",
		"provider":       "provider",
		"default_branch": "default_branch",
		"sync_status":    "last_sync_status",
		"open_pr_count":  "open_pr_count",
		"last_activity":  "last_pr_activity_at",
	}
	sortKey := c.QueryParam("sort")
	if scheduleJoin {
		// Fixed compound order (enabled first, latest run first); "order" param is intentionally ignored here.
		query += " ORDER BY COALESCE(sched.enabled, false) DESC, sched.last_run_at DESC NULLS LAST, repositories.full_name ASC"
	} else {
		sortColumn, ok := sortColumns[sortKey]
		if !ok {
			sortKey = "last_activity"
			sortColumn = sortColumns[sortKey]
		}
		requestedOrder := strings.ToLower(c.QueryParam("order"))
		descByDefault := sortKey == "open_pr_count" || sortKey == "last_activity"
		// Resolve to one of two hardcoded literals rather than passing the
		// (validated) request value through, so the SQL builder never touches
		// user-controlled input even indirectly.
		orderClause := "ASC"
		if requestedOrder == "desc" || (requestedOrder != "asc" && descByDefault) {
			orderClause = "DESC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s NULLS LAST, full_name ASC", sortColumn, orderClause)
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch repositories")
	}
	defer rows.Close()

	repos := make([]RepositoryResponse, 0)
	for rows.Next() {
		var openPRCount int
		var lastActivityAt sql.NullTime
		r, err := scanRepository(func(dest ...interface{}) error {
			return rows.Scan(append(dest, &openPRCount, &lastActivityAt)...)
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to scan repository")
		}
		r.OpenPRCount = openPRCount
		if lastActivityAt.Valid {
			r.LastActivityAt = &lastActivityAt.Time
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Error iterating repositories")
	}

	return c.JSON(http.StatusOK, RepositoriesListResponse{
		Repositories: repos,
		Total:        total,
		Page:         page,
		PerPage:      perPage,
		TotalPages:   totalPages(total, perPage),
	})
}

// GetRepository handles GET /api/v1/repositories/:repoId.
func (s *Server) GetRepository(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}

	row := s.db.QueryRow(`SELECT `+repositoryColumns+`,
		(SELECT count(*) FROM pull_requests pr WHERE pr.repository_id = repositories.id AND pr.state = 'open') AS open_pr_count,
		(SELECT max(pr.provider_updated_at) FROM pull_requests pr WHERE pr.repository_id = repositories.id) AS last_pr_activity_at
		FROM repositories WHERE id = $1 AND org_id = $2`, repoID, orgID)
	var openPRCount int
	var lastActivityAt sql.NullTime
	r, err := scanRepository(func(dest ...interface{}) error {
		return row.Scan(append(dest, &openPRCount, &lastActivityAt)...)
	})
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch repository")
	}
	r.OpenPRCount = openPRCount
	if lastActivityAt.Valid {
		r.LastActivityAt = &lastActivityAt.Time
	}
	return c.JSON(http.StatusOK, r)
}

// TriggerRepositoryPRSync handles POST /api/v1/repositories/:repoId/sync -
// queues an immediate (non-initial-backfill) PR/MR sync for one repository.
func (s *Server) TriggerRepositoryPRSync(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}

	var exists bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1 AND org_id = $2)`, repoID, orgID).Scan(&exists); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to look up repository")
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}

	if s.jobQueue == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Job queue not initialized")
	}
	if err := s.jobQueue.QueueRepoPRSyncJob(c.Request().Context(), repoID, false); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to queue repository sync")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"status": "queued", "repository_id": repoID})
}

// TriggerRepositorySync handles POST /api/v1/connectors/:connectorId/repositories/sync
// - re-runs repository discovery for the whole connector and queues a PR/MR
// sync for every repository found (existing rows are upserted, not
// duplicated; see AutoWebhookInstaller.syncRepositoriesAndQueueBackfill).
func (s *Server) TriggerRepositorySync(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	connectorID, err := strconv.Atoi(c.Param("connectorId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid connector ID")
	}
	if s.autoWebhookInstaller == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Auto webhook installer not initialized")
	}

	connector, err := s.autoWebhookInstaller.getConnectorDetails(connectorID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Connector not found")
	}
	if connector.OrgID != orgID {
		return echo.NewHTTPError(http.StatusNotFound, "Connector not found")
	}

	if err := s.autoWebhookInstaller.syncRepositoriesAndQueueBackfill(c.Request().Context(), connectorID, connector); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to sync repositories: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"status": "queued", "connector_id": connectorID})
}

func scanPullRequest(scan func(dest ...interface{}) error) (PullRequestResponse, error) {
	var pr PullRequestResponse
	var description, authorUsername, authorName, authorAvatarURL, sourceBranch, targetBranch sql.NullString
	var providerCreatedAt, providerUpdatedAt sql.NullTime
	err := scan(
		&pr.ID, &pr.RepositoryID, &pr.OrgID, &pr.Provider, &pr.ProviderPRID, &pr.Number, &pr.Title,
		&description, &pr.State, &authorUsername, &authorName, &authorAvatarURL, &sourceBranch, &targetBranch,
		&pr.WebURL, &providerCreatedAt, &providerUpdatedAt, &pr.LastSyncedAt, &pr.LastSyncedSource,
		&pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		return pr, err
	}
	pr.Description = description.String
	pr.AuthorUsername = authorUsername.String
	pr.AuthorName = authorName.String
	pr.AuthorAvatarURL = authorAvatarURL.String
	pr.SourceBranch = sourceBranch.String
	pr.TargetBranch = targetBranch.String
	if providerCreatedAt.Valid {
		pr.ProviderCreatedAt = &providerCreatedAt.Time
	}
	if providerUpdatedAt.Valid {
		pr.ProviderUpdatedAt = &providerUpdatedAt.Time
	}
	return pr, nil
}

const pullRequestColumns = `id, repository_id, org_id, provider, provider_pr_id, number, title,
	description, state, author_username, author_name, author_avatar_url, source_branch, target_branch,
	web_url, provider_created_at, provider_updated_at, last_synced_at, last_synced_source,
	created_at, updated_at`

// pullRequestColumnsQualified is pullRequestColumns with each column
// qualified by the "pr" table alias, for queries that JOIN pull_requests
// against another table (e.g. repositories) and would otherwise have
// ambiguous column references (both tables have an "id" column, etc).
const pullRequestColumnsQualified = `pr.id, pr.repository_id, pr.org_id, pr.provider, pr.provider_pr_id, pr.number, pr.title,
	pr.description, pr.state, pr.author_username, pr.author_name, pr.author_avatar_url, pr.source_branch, pr.target_branch,
	pr.web_url, pr.provider_created_at, pr.provider_updated_at, pr.last_synced_at, pr.last_synced_source,
	pr.created_at, pr.updated_at`

// ListPullRequestsForRepo handles GET /api/v1/repositories/:repoId/pull-requests.
func (s *Server) ListPullRequestsForRepo(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}

	var repoExists bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1 AND org_id = $2)`, repoID, orgID).Scan(&repoExists); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to look up repository")
	}
	if !repoExists {
		return echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}

	page, perPage := parsePageParams(c)
	query := `SELECT ` + pullRequestColumns + ` FROM pull_requests WHERE repository_id = $1 AND org_id = $2`
	countQuery := `SELECT count(*) FROM pull_requests WHERE repository_id = $1 AND org_id = $2`
	args := []interface{}{repoID, orgID}
	argIdx := 3

	if state := c.QueryParam("state"); state != "" && state != "all" {
		query += fmt.Sprintf(" AND state = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, state)
		argIdx++
	}
	if search := c.QueryParam("search"); search != "" {
		query += fmt.Sprintf(" AND (title ILIKE $%d OR author_username ILIKE $%d)", argIdx, argIdx)
		countQuery += fmt.Sprintf(" AND (title ILIKE $%d OR author_username ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to count pull requests")
	}

	query += " ORDER BY provider_updated_at DESC NULLS LAST, number DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch pull requests")
	}
	defer rows.Close()

	prs := make([]PullRequestResponse, 0)
	for rows.Next() {
		pr, err := scanPullRequest(rows.Scan)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to scan pull request")
		}
		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Error iterating pull requests")
	}

	return c.JSON(http.StatusOK, PullRequestsListResponse{
		PullRequests: prs,
		Total:        total,
		Page:         page,
		PerPage:      perPage,
		TotalPages:   totalPages(total, perPage),
	})
}

// PullRequestWithRepoResponse is a pull_requests row plus enough of its
// owning repository to render in a cross-repo (unified, all-connectors) PR/MR
// listing without a follow-up lookup.
type PullRequestWithRepoResponse struct {
	PullRequestResponse
	RepositoryFullName string `json:"repository_full_name"`
	RepositoryWebURL   string `json:"repository_web_url"`
}

// PullRequestsWithRepoListResponse wraps a paginated, cross-repository
// pull_requests listing.
type PullRequestsWithRepoListResponse struct {
	PullRequests []PullRequestWithRepoResponse `json:"pull_requests"`
	Total        int                           `json:"total"`
	Page         int                           `json:"page"`
	PerPage      int                           `json:"per_page"`
	TotalPages   int                           `json:"total_pages"`
}

// ListPullRequests handles GET /api/v1/pull-requests - a unified PR/MR
// listing across every repository the org's connectors have discovered
// (unlike ListPullRequestsForRepo, which is scoped to one repository).
func (s *Server) ListPullRequests(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	page, perPage := parsePageParams(c)

	// countQuery and query share identical FROM/JOIN/WHERE clauses (built up
	// in lockstep below) so the reported total can never drift from what the
	// page actually contains.
	fromAndFilters := `FROM pull_requests pr
		JOIN repositories r ON r.id = pr.repository_id
		WHERE pr.org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if repositoryID := c.QueryParam("repository_id"); repositoryID != "" {
		fromAndFilters += fmt.Sprintf(" AND pr.repository_id = $%d", argIdx)
		args = append(args, repositoryID)
		argIdx++
	}
	if connectorID := c.QueryParam("connector_id"); connectorID != "" {
		fromAndFilters += fmt.Sprintf(" AND r.connector_id = $%d", argIdx)
		args = append(args, connectorID)
		argIdx++
	}
	if clause := multiValueFilterClause("pr.provider", c.QueryParam("provider"), &args, &argIdx); clause != "" {
		fromAndFilters += clause
	}
	if state := c.QueryParam("state"); state != "" && state != "all" {
		if clause := multiValueFilterClause("pr.state", state, &args, &argIdx); clause != "" {
			fromAndFilters += clause
		}
	}
	if search := c.QueryParam("search"); search != "" {
		fromAndFilters += fmt.Sprintf(" AND (pr.title ILIKE $%d OR pr.author_username ILIKE $%d OR r.full_name ILIKE $%d)", argIdx, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	var total int
	if err := s.db.QueryRow(`SELECT count(*) `+fromAndFilters, args...).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to count pull requests")
	}

	// SQL string is built only from parameter placeholders ($n); all user values are passed via args.
	query := `SELECT ` + pullRequestColumnsQualified + `, r.full_name, r.web_url ` + fromAndFilters // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	query += " ORDER BY pr.provider_updated_at DESC NULLS LAST, pr.id DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch pull requests")
	}
	defer rows.Close()

	prs := make([]PullRequestWithRepoResponse, 0)
	for rows.Next() {
		var pr PullRequestWithRepoResponse
		scanned, err := scanPullRequest(func(dest ...interface{}) error {
			combined := append(dest, &pr.RepositoryFullName, &pr.RepositoryWebURL)
			return rows.Scan(combined...)
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to scan pull request")
		}
		pr.PullRequestResponse = scanned
		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Error iterating pull requests")
	}

	return c.JSON(http.StatusOK, PullRequestsWithRepoListResponse{
		PullRequests: prs,
		Total:        total,
		Page:         page,
		PerPage:      perPage,
		TotalPages:   totalPages(total, perPage),
	})
}

// GetPullRequest handles GET /api/v1/repositories/:repoId/pull-requests/:prId,
// including its review history (joined from the reviews table).
func (s *Server) GetPullRequest(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}
	prID, err := strconv.ParseInt(c.Param("prId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid pull request ID")
	}

	row := s.db.QueryRow(
		`SELECT `+pullRequestColumns+` FROM pull_requests WHERE id = $1 AND repository_id = $2 AND org_id = $3`,
		prID, repoID, orgID,
	)
	pr, err := scanPullRequest(row.Scan)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "Pull request not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch pull request")
	}

	rows, err := s.db.Query(
		`SELECT id, status, trigger_type, created_at, started_at, completed_at
		 FROM reviews WHERE pull_request_id = $1 AND org_id = $2
		 ORDER BY created_at DESC`,
		prID, orgID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch review history")
	}
	defer rows.Close()

	reviews := make([]PullRequestReviewSummary, 0)
	for rows.Next() {
		var rv PullRequestReviewSummary
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&rv.ID, &rv.Status, &rv.TriggerType, &rv.CreatedAt, &startedAt, &completedAt); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to scan review history")
		}
		if startedAt.Valid {
			rv.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			rv.CompletedAt = &completedAt.Time
		}
		reviews = append(reviews, rv)
	}
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Error iterating review history")
	}

	return c.JSON(http.StatusOK, PullRequestDetailResponse{
		PullRequestResponse: pr,
		Reviews:             reviews,
	})
}

func totalPages(total, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}
