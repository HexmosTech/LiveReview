package auditlog

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Scope is the caller's verified audit visibility. It is built from
// PermissionContext; never from client-supplied ids.
type Scope struct {
	OrgID        int64
	UserID       int64
	ActorEmail   string
	IsOwner      bool
	IsSuperAdmin bool
	// AdminView is true only for /api/v1/admin endpoints gated by
	// RequireSuperAdmin. It removes the org restriction and ignores the
	// owner's "hide admin-capacity actions" rule.
	AdminView bool
}

// Filters are the supported audit-log filters.
type Filters struct {
	From         *time.Time
	To           *time.Time
	Since        *time.Time // live tail: strictly greater than
	Categories   []Category
	Actions      []Action
	ActorUserID  *int64
	ActorEmail   string
	ResourceType string
	ResourceID   string
	Outcome      Outcome
	Severity     Severity
	Source       Source
	Query        string
	OrgID        *int64 // admin view only
	Limit        int
	Cursor       string
}

// Facets are the filter dropdown values over the current scope/window.
type Facets struct {
	Actions    []FacetValue `json:"actions"`
	Categories []FacetValue `json:"categories"`
	Outcomes   []FacetValue `json:"outcomes"`
	Severities []FacetValue `json:"severities"`
	Actors     []FacetValue `json:"actors"`
}

type FacetValue struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

const eventColumns = `id, occurred_at, org_id, actor_user_id, actor_email, actor_name,
	actor_kind, actor_capacity, auth_method, api_key_id, action, category,
	resource_type, resource_id, outcome, status_code, severity, host(ip), user_agent,
	request_id, method, path, details, source, legacy_id, seq, prev_hash, hash`

// EventColumns is the canonical SELECT list for an audit event. Exported for
// the archive/export code that queries a specific partition.
const EventColumns = eventColumns

// Query is a scope- and filter-aware audit query. Every read path must use it.
type Query struct {
	scope   Scope
	filters Filters
}

func NewQuery(scope Scope, filters Filters) (*Query, error) {
	if scope.AdminView && !scope.IsSuperAdmin {
		return nil, errors.New("auditlog: admin view requires super admin")
	}
	if !scope.AdminView && (scope.OrgID <= 0 || scope.UserID <= 0) {
		return nil, errors.New("auditlog: missing organization/user scope")
	}
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Limit > 500 {
		filters.Limit = 500
	}
	if len(filters.Query) > 200 {
		filters.Query = filters.Query[:200]
	}
	return &Query{scope: scope, filters: filters}, nil
}

type argBuilder struct {
	args []any
}

func (b *argBuilder) add(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

// Where builds the SQL predicate and arguments for this query. It is exported
// so tests can assert that cross-tenant clauses are always present.
func (q *Query) Where() (string, []any) {
	var b argBuilder
	var clauses []string

	// Visibility. This is the single place tenant isolation is enforced for
	// audit reads; handlers must never build their own WHERE.
	if q.scope.AdminView {
		if q.filters.OrgID != nil && *q.filters.OrgID > 0 {
			clauses = append(clauses, "org_id = "+b.add(*q.filters.OrgID))
		}
	} else {
		clauses = append(clauses, "org_id = "+b.add(q.scope.OrgID))
		switch {
		case q.scope.IsSuperAdmin:
			// Super admins see everything in the org, including support actions.
		case q.scope.IsOwner:
			// Owners see their org and all member actions. Super-admin actions
			// (support work) are visible only to super admins. Check both fields
			// so older rows recorded before the capacity rule are still hidden.
			clauses = append(clauses, "(actor_capacity <> 'admin' AND actor_kind <> 'super_admin')")
		default:
			// Members see only their own actions.
			clauses = append(clauses, "actor_user_id = "+b.add(q.scope.UserID))
		}
	}

	if q.filters.From != nil {
		clauses = append(clauses, "occurred_at >= "+b.add(q.filters.From.UTC()))
	}
	if q.filters.To != nil {
		clauses = append(clauses, "occurred_at < "+b.add(q.filters.To.UTC()))
	}
	if q.filters.Since != nil {
		clauses = append(clauses, "occurred_at > "+b.add(q.filters.Since.UTC()))
	}
	if len(q.filters.Categories) > 0 {
		cats := make([]string, 0, len(q.filters.Categories))
		for _, c := range q.filters.Categories {
			cats = append(cats, string(c))
		}
		clauses = append(clauses, "category = ANY("+b.add(pq.Array(cats))+"::text[])")
	}
	if len(q.filters.Actions) > 0 {
		acts := make([]string, 0, len(q.filters.Actions))
		for _, a := range q.filters.Actions {
			acts = append(acts, string(a))
		}
		clauses = append(clauses, "action = ANY("+b.add(pq.Array(acts))+"::text[])")
	}
	if q.filters.ActorUserID != nil {
		clauses = append(clauses, "actor_user_id = "+b.add(*q.filters.ActorUserID))
	}
	if q.filters.ActorEmail != "" {
		clauses = append(clauses, "actor_email ILIKE "+b.add("%"+escapeLike(strings.TrimSpace(q.filters.ActorEmail))+"%")+" ESCAPE '\\'")
	}
	if q.filters.ResourceType != "" {
		clauses = append(clauses, "resource_type = "+b.add(q.filters.ResourceType))
	}
	if q.filters.ResourceID != "" {
		clauses = append(clauses, "resource_id = "+b.add(q.filters.ResourceID))
	}
	if q.filters.Outcome != "" {
		clauses = append(clauses, "outcome = "+b.add(string(q.filters.Outcome)))
	}
	if q.filters.Severity != "" {
		clauses = append(clauses, "severity = "+b.add(string(q.filters.Severity)))
	}
	if q.filters.Source != "" {
		clauses = append(clauses, "source = "+b.add(string(q.filters.Source)))
	}
	if q.filters.Query != "" {
		pattern := "%" + escapeLike(strings.TrimSpace(q.filters.Query)) + "%"
		p := b.add(pattern)
		clauses = append(clauses, "(actor_email ILIKE "+p+" ESCAPE '\\' OR actor_name ILIKE "+p+
			" ESCAPE '\\' OR action ILIKE "+p+" ESCAPE '\\' OR resource_id ILIKE "+p+
			" ESCAPE '\\' OR path ILIKE "+p+" ESCAPE '\\' OR details::text ILIKE "+p+" ESCAPE '\\')")
	}

	if q.filters.Cursor != "" {
		ts, id, err := decodeCursor(q.filters.Cursor)
		if err == nil {
			tsArg := b.add(ts)
			idArg := b.add(id)
			clauses = append(clauses, "(occurred_at, id) < ("+tsArg+", "+idArg+")")
		}
	}

	if len(clauses) == 0 {
		return "TRUE", b.args
	}
	return strings.Join(clauses, " AND "), b.args
}

func (q *Query) timeOnlyWhere() (string, []any) {
	f := q.filters
	f.Categories = nil
	f.Actions = nil
	f.ActorUserID = nil
	f.ActorEmail = ""
	f.ResourceType = ""
	f.ResourceID = ""
	f.Outcome = ""
	f.Severity = ""
	f.Source = ""
	f.Query = ""
	f.Cursor = ""
	f.Since = nil
	clone := &Query{scope: q.scope, filters: f}
	return clone.Where()
}

// List returns one page of events, newest first. nextCursor is empty on the
// last page.
func (q *Query) List(ctx context.Context, db *sql.DB) ([]Event, string, error) {
	where, args := q.Where()
	limit := q.filters.Limit
	args = append(args, limit+1)
	// where is built by Where() and contains only fixed clauses plus $N
	// placeholders; eventColumns is a compile-time constant.
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	sqlText := "SELECT " + eventColumns + " FROM audit_events WHERE " + where +
		" ORDER BY occurred_at DESC, id DESC LIMIT $" + strconv.Itoa(len(args))

	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, "", fmt.Errorf("auditlog: list: %w", err)
	}
	defer rows.Close()

	var events = make([]Event, 0, limit+1)
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, "", err
		}
		ev.FillDerived()
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("auditlog: list rows: %w", err)
	}

	next := ""
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		next = encodeCursor(last.OccurredAt, last.ID)
	}
	return events, next, nil
}

// Get returns one event inside the caller's scope.
func (q *Query) Get(ctx context.Context, db *sql.DB, id int64) (*Event, error) {
	where, args := q.Where()
	idArg := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, id)
	// where is fixed clauses/placeholders; idArg is a generated $N placeholder.
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli, go.lang.security.audit.database.string-formatted-query.string-formatted-query
	sqlText := "SELECT " + eventColumns + " FROM audit_events WHERE " + where +
		" AND id = " + idArg + " ORDER BY occurred_at DESC LIMIT 1"
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	row := db.QueryRowContext(ctx, sqlText, args...)
	ev, err := scanEvent(row)
	if err != nil {
		return nil, err
	}
	ev.FillDerived()
	return &ev, nil
}

// Count returns the number of matching rows (used to cap exports).
func (q *Query) Count(ctx context.Context, db *sql.DB) (int64, error) {
	where, args := q.Where()
	var n int64
	// where is fixed clauses/placeholders built by Where().
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE "+where, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("auditlog: count: %w", err)
	}
	return n, nil
}

// Facets returns filter dropdown values for the current scope/window.
func (q *Query) Facets(ctx context.Context, db *sql.DB) (*Facets, error) {
	where, args := q.timeOnlyWhere()
	facets := &Facets{
		Actions: []FacetValue{}, Categories: []FacetValue{}, Outcomes: []FacetValue{},
		Severities: []FacetValue{}, Actors: []FacetValue{},
	}
	var err error
	if facets.Actions, err = q.facet(ctx, db, "action", where, args, 200); err != nil {
		return nil, err
	}
	if facets.Categories, err = q.facet(ctx, db, "category", where, args, 50); err != nil {
		return nil, err
	}
	if facets.Outcomes, err = q.facet(ctx, db, "outcome", where, args, 10); err != nil {
		return nil, err
	}
	if facets.Severities, err = q.facet(ctx, db, "severity", where, args, 10); err != nil {
		return nil, err
	}
	if facets.Actors, err = q.actorFacet(ctx, db, where, args); err != nil {
		return nil, err
	}
	for i := range facets.Categories {
		facets.Categories[i].Label = CategoryLabel(Category(facets.Categories[i].Value))
	}
	for i := range facets.Actions {
		if meta, ok := LookupAction(Action(facets.Actions[i].Value)); ok {
			facets.Actions[i].Label = meta.Label
		}
	}
	return facets, nil
}

func (q *Query) facet(ctx context.Context, db *sql.DB, column, where string, args []any, limit int) ([]FacetValue, error) {
	// column is a compile-time constant from Facets, never user input; where is
	// fixed clauses/placeholders and limit is an integer.
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli, go.lang.security.audit.database.string-formatted-query.string-formatted-query
	sqlText := "SELECT " + column + ", COUNT(*) FROM audit_events WHERE " + where +
		" GROUP BY " + column + " ORDER BY COUNT(*) DESC, " + column + " ASC LIMIT " + strconv.Itoa(limit)
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("auditlog: facet %s: %w", column, err)
	}
	defer rows.Close()
	out := make([]FacetValue, 0)
	for rows.Next() {
		var value sql.NullString
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		if value.Valid && value.String != "" {
			out = append(out, FacetValue{Value: value.String, Count: count})
		}
	}
	return out, rows.Err()
}

func (q *Query) actorFacet(ctx context.Context, db *sql.DB, where string, args []any) ([]FacetValue, error) {
	// where is fixed clauses/placeholders built by timeOnlyWhere().
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	sqlText := `SELECT COALESCE(NULLIF(actor_email, ''), actor_name, 'unknown') AS actor, COUNT(*)
		FROM audit_events WHERE ` + where + `
		GROUP BY actor ORDER BY COUNT(*) DESC, actor ASC LIMIT 100`
	// nosemgrep: go.lang.security.audit.sqli.gosql-sqli.gosql-sqli
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("auditlog: actor facet: %w", err)
	}
	defer rows.Close()
	out := make([]FacetValue, 0)
	for rows.Next() {
		var actor string
		var count int64
		if err := rows.Scan(&actor, &count); err != nil {
			return nil, err
		}
		out = append(out, FacetValue{Value: actor, Label: actor, Count: count})
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

// RowScanner is the minimal Scan contract shared by *sql.Row and *sql.Rows.
type RowScanner interface {
	Scan(dest ...any) error
}

// ScanEvent scans one row selected with EventColumns.
func ScanEvent(row RowScanner) (Event, error) {
	return scanEvent(row)
}

func scanEvent(row rowScanner) (Event, error) {
	var (
		ev            Event
		actorUserID   sql.NullInt64
		actorEmail    sql.NullString
		actorName     sql.NullString
		actorKind     string
		actorCapacity sql.NullString
		authMethod    sql.NullString
		apiKeyID      sql.NullInt64
		category      string
		resourceType  sql.NullString
		resourceID    sql.NullString
		outcome       string
		statusCode    sql.NullInt64
		severity      string
		ip            sql.NullString
		userAgent     sql.NullString
		requestID     sql.NullString
		method        sql.NullString
		path          sql.NullString
		details       []byte
		source        sql.NullString
		legacyID      sql.NullInt64
		seq           sql.NullInt64
		prevHash      sql.NullString
		hash          sql.NullString
		action        string
	)
	err := row.Scan(
		&ev.ID, &ev.OccurredAt, &ev.OrgID, &actorUserID, &actorEmail, &actorName,
		&actorKind, &actorCapacity, &authMethod, &apiKeyID, &action, &category,
		&resourceType, &resourceID, &outcome, &statusCode, &severity, &ip, &userAgent,
		&requestID, &method, &path, &details, &source, &legacyID, &seq, &prevHash, &hash,
	)
	if err != nil {
		return ev, err
	}
	if actorUserID.Valid {
		v := actorUserID.Int64
		ev.ActorUserID = &v
	}
	ev.ActorEmail = actorEmail.String
	ev.ActorName = actorName.String
	ev.ActorKind = ActorKind(actorKind)
	if actorCapacity.Valid {
		ev.ActorCapacity = ActorCapacity(actorCapacity.String)
	}
	ev.AuthMethod = authMethod.String
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		ev.APIKeyID = &v
	}
	ev.Action = Action(action)
	ev.Category = Category(category)
	ev.ResourceType = resourceType.String
	ev.ResourceID = resourceID.String
	ev.Outcome = Outcome(outcome)
	if statusCode.Valid {
		v := int(statusCode.Int64)
		ev.StatusCode = &v
	}
	ev.Severity = Severity(severity)
	ev.IP = ip.String
	ev.UserAgent = userAgent.String
	ev.RequestID = requestID.String
	ev.Method = method.String
	ev.Path = path.String
	if len(details) > 0 {
		_ = json.Unmarshal(details, &ev.Details)
	}
	ev.Source = Source(source.String)
	if legacyID.Valid {
		v := legacyID.Int64
		ev.LegacyID = &v
	}
	if seq.Valid {
		v := seq.Int64
		ev.Seq = &v
	}
	ev.PrevHash = prevHash.String
	ev.Hash = hash.String
	return ev, nil
}

func encodeCursor(t time.Time, id int64) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return t, id, nil
}

// escapeLike escapes LIKE metacharacters so user input cannot turn into a
// wildcard scan.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
