package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/blobstore"
	"github.com/livereview/pkg/models"
	"github.com/rs/zerolog/log"
)

const auditExportMaxRows = 200000
const auditExportMaxDays = 366
const auditPackageMaxRows = 100000

// auditReadScope builds the verified scope for an audit read. Members are
// allowed but the query builder restricts them to their own events. API keys
// must be the dedicated audit_reader kind; cross-org admin reads are
// session-only.
func (s *Server) auditReadScope(c echo.Context, adminView bool) (auditlog.Scope, bool) {
	pc := auth.GetPermissionContext(c)
	if pc == nil {
		if !adminView {
			return auditlog.Scope{}, false
		}
		// /admin routes are gated by RequireSuperAdmin and intentionally do
		// not build a PermissionContext. Cross-org data is session-only:
		// even a super admin's API key must not read or change it.
		if method, _ := c.Get(authMethodContextKey).(string); method == authMethodAPIKey {
			return auditlog.Scope{}, false
		}
		user, ok := c.Get("user").(*models.User)
		if !ok || user == nil {
			return auditlog.Scope{}, false
		}
		return auditlog.Scope{UserID: user.ID, ActorEmail: user.Email, IsSuperAdmin: true, AdminView: true}, true
	}
	if adminView && !pc.IsSuperAdmin {
		return auditlog.Scope{}, false
	}
	if method, _ := c.Get(authMethodContextKey).(string); method == authMethodAPIKey {
		kind, _ := c.Get("api_key_kind").(string)
		if kind != "audit_reader" || adminView {
			return auditlog.Scope{}, false
		}
	}
	if !adminView && pc.OrgID <= 0 {
		return auditlog.Scope{}, false
	}
	email := ""
	if pc.User != nil {
		email = pc.User.Email
	}
	return auditlog.Scope{
		OrgID:        pc.OrgID,
		UserID:       pc.GetUserID(),
		ActorEmail:   email,
		IsOwner:      pc.IsOwner,
		IsSuperAdmin: pc.IsSuperAdmin,
		AdminView:    adminView,
	}, true
}

// auditIsSuperAdmin works on both org routes (PermissionContext) and /admin
// routes (RequireSuperAdmin, no PermissionContext).
func auditIsSuperAdmin(c echo.Context) bool {
	if pc := auth.GetPermissionContext(c); pc != nil {
		return pc.IsSuperAdmin
	}
	_, ok := c.Get("user").(*models.User)
	return ok
}

// auditSettingsAllowed gates the retention policy: super admin, session only.
func auditSettingsAllowed(c echo.Context) bool {
	if !auditIsSuperAdmin(c) {
		return false
	}
	if method, _ := c.Get(authMethodContextKey).(string); method == authMethodAPIKey {
		return false
	}
	return true
}

func auditForbidden(c echo.Context) error {
	return c.JSON(http.StatusForbidden, map[string]string{
		"error":      "audit log access denied",
		"error_code": "AUDIT_ACCESS_DENIED",
	})
}

func parseAuditFilters(c echo.Context) (auditlog.Filters, error) {
	f := auditlog.Filters{
		Categories:   parseCSV[auditlog.Category](c.QueryParam("category")),
		Actions:      parseCSV[auditlog.Action](c.QueryParam("action")),
		ActorEmail:   strings.TrimSpace(c.QueryParam("actor_email")),
		ResourceType: strings.TrimSpace(c.QueryParam("resource_type")),
		ResourceID:   strings.TrimSpace(c.QueryParam("resource_id")),
		Outcome:      auditlog.Outcome(strings.TrimSpace(c.QueryParam("outcome"))),
		Severity:     auditlog.Severity(strings.TrimSpace(c.QueryParam("severity"))),
		Source:       auditlog.Source(strings.TrimSpace(c.QueryParam("source"))),
		Query:        strings.TrimSpace(c.QueryParam("q")),
		Cursor:       strings.TrimSpace(c.QueryParam("cursor")),
	}
	if v := c.QueryParam("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, fmt.Errorf("invalid limit")
		}
		f.Limit = n
	}
	if v := c.QueryParam("actor_user_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return f, fmt.Errorf("invalid actor_user_id")
		}
		f.ActorUserID = &n
	}
	if v := c.QueryParam("org_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return f, fmt.Errorf("invalid org_id")
		}
		f.OrgID = &n
	}
	if v := c.QueryParam("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("invalid from (use RFC3339)")
		}
		f.From = &t
	}
	if v := c.QueryParam("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("invalid to (use RFC3339)")
		}
		f.To = &t
	}
	if v := c.QueryParam("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("invalid since (use RFC3339)")
		}
		f.Since = &t
	}
	if f.From == nil && f.Since == nil {
		from := time.Now().UTC().AddDate(0, 0, -30)
		f.From = &from
	}
	switch f.Outcome {
	case "", auditlog.OutcomeSuccess, auditlog.OutcomeFailure, auditlog.OutcomeDenied, auditlog.OutcomeError:
	default:
		return f, fmt.Errorf("invalid outcome")
	}
	switch f.Severity {
	case "", auditlog.SeverityInfo, auditlog.SeverityWarning, auditlog.SeverityCritical:
	default:
		return f, fmt.Errorf("invalid severity")
	}
	return f, nil
}

func parseCSV[T ~string](raw string) []T {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]T, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, T(p))
		}
	}
	return out
}

// GetAuditEvents handles GET /api/v1/orgs/:org_id/audit-events.
func (s *Server) GetAuditEvents(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	filters, err := parseAuditFilters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	q, err := auditlog.NewQuery(scope, filters)
	if err != nil {
		return auditForbidden(c)
	}
	events, next, err := q.List(c.Request().Context(), s.db)
	if err != nil {
		log.Error().Err(err).Msg("audit: list failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load audit events"})
	}
	// One meta-audit row per page open, not per live poll.
	if filters.Since == nil && filters.Cursor == "" {
		s.recordAuditRead(c, auditlog.ActionAuditRead, auditlog.Details(
			"filters", auditFiltersSummary(filters), "rows", len(events)))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"events":      events,
		"next_cursor": next,
		"has_more":    next != "",
	})
}

func auditFiltersSummary(f auditlog.Filters) string {
	parts := make([]string, 0, 6)
	if f.From != nil {
		parts = append(parts, "from="+f.From.Format(time.RFC3339))
	}
	if f.To != nil {
		parts = append(parts, "to="+f.To.Format(time.RFC3339))
	}
	if len(f.Categories) > 0 {
		parts = append(parts, "category="+joinCats(f.Categories))
	}
	if len(f.Actions) > 0 {
		parts = append(parts, "action="+joinActions(f.Actions))
	}
	if f.ActorEmail != "" {
		parts = append(parts, "actor="+f.ActorEmail)
	}
	if f.Query != "" {
		parts = append(parts, "q="+f.Query)
	}
	return strings.Join(parts, "&")
}

func joinCats(cats []auditlog.Category) string {
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = string(c)
	}
	return strings.Join(out, ",")
}

func joinActions(actions []auditlog.Action) string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = string(a)
	}
	return strings.Join(out, ",")
}

// GetAuditEvent handles GET /api/v1/orgs/:org_id/audit-events/:id.
func (s *Server) GetAuditEvent(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid event id"})
	}
	q, err := auditlog.NewQuery(scope, auditlog.Filters{Limit: 1})
	if err != nil {
		return auditForbidden(c)
	}
	ev, err := q.Get(c.Request().Context(), s.db, id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "audit event not found"})
	}
	if err != nil {
		log.Error().Err(err).Msg("audit: get failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load audit event"})
	}
	s.recordAuditRead(c, auditlog.ActionAuditRead, auditlog.Details("event_id", id))
	return c.JSON(http.StatusOK, ev)
}

// GetAuditFacets handles GET /api/v1/orgs/:org_id/audit-events/facets.
func (s *Server) GetAuditFacets(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	filters, err := parseAuditFilters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	q, err := auditlog.NewQuery(scope, filters)
	if err != nil {
		return auditForbidden(c)
	}
	facets, err := q.Facets(c.Request().Context(), s.db)
	if err != nil {
		log.Error().Err(err).Msg("audit: facets failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load audit facets"})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"facets":     facets,
		"categories": auditlog.AllCategories(),
	})
}

// GetAuditMeta handles GET /api/v1/orgs/:org_id/audit-events/meta. It exposes
// the retention policy read-only so the UI can warn before data expires.
func (s *Server) GetAuditMeta(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	cfg := loadAuditSettings(c.Request().Context(), s.db)
	return c.JSON(http.StatusOK, auditRetentionMeta{
		HotDays:        cfg.HotDays,
		RetentionDays:  cfg.RetentionDays,
		ArchiveEnabled: cfg.ArchiveEnabled,
		AnchorEnabled:  cfg.AnchorEnabled,
		CanManage:      scope.IsSuperAdmin,
		Now:            time.Now().UTC(),
	})
}

// recordAuditRead records a sensitive read best-effort. It never blocks the
// read itself.
func (s *Server) recordAuditRead(c echo.Context, action auditlog.Action, details auditlog.Fields) {
	recordSensitiveRead(c, s.auditRecorder, action, details)
}

// recordSensitiveRead is the package-level form used by handlers that do not
// hold a *Server (taxonomy reports, onboarding reports).
func recordSensitiveRead(c echo.Context, rec *auditlog.Recorder, action auditlog.Action, details auditlog.Fields) {
	if rec == nil {
		return
	}
	if err := rec.SensitiveRead(c, action, details); err != nil {
		auditlog.LogRecordError(action, err)
	}
}

// ExportAuditEvents handles GET /api/v1/orgs/:org_id/audit-events/export.
func (s *Server) ExportAuditEvents(c echo.Context) error {
	return s.exportAuditEvents(c, false)
}

func (s *Server) exportAuditEvents(c echo.Context, adminView bool) error {
	scope, ok := s.auditReadScope(c, adminView)
	if !ok {
		return auditForbidden(c)
	}
	filters, err := parseAuditFilters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	format := strings.ToLower(c.QueryParam("format"))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "ndjson" && format != "package" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "format must be csv, ndjson, or package"})
	}
	if filters.From != nil && filters.To != nil && filters.To.Sub(*filters.From) > auditExportMaxDays*24*time.Hour {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("export range cannot exceed %d days", auditExportMaxDays)})
	}
	filters.Cursor = ""
	maxRows := int64(auditExportMaxRows)
	if format == "package" {
		maxRows = auditPackageMaxRows
	}
	q, err := auditlog.NewQuery(scope, filters)
	if err != nil {
		return auditForbidden(c)
	}
	count, err := q.Count(c.Request().Context(), s.db)
	if err != nil {
		log.Error().Err(err).Msg("audit: export count failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to prepare export"})
	}
	if count > maxRows {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("export contains %d rows; narrow the filters or use package (max %d)", count, maxRows),
		})
	}
	action := auditlog.ActionExportAudit
	if format == "package" {
		action = auditlog.ActionExportPackage
	}
	s.recordAuditRead(c, action, auditlog.Details(
		"format", format, "rows", count, "filters", auditFiltersSummary(filters)))

	switch format {
	case "csv":
		return s.streamAuditCSV(c, scope, filters)
	case "ndjson":
		return s.streamAuditNDJSON(c, scope, filters, count)
	default:
		return s.streamAuditPackage(c, scope, filters, count)
	}
}

func auditFilename(scope auditlog.Scope, ext string) string {
	org := scope.OrgID
	if scope.AdminView {
		org = 0
	}
	return fmt.Sprintf("livereview-audit-%d-%s.%s", org, time.Now().UTC().Format("20060102-150405"), ext)
}

func (s *Server) streamAuditCSV(c echo.Context, scope auditlog.Scope, filters auditlog.Filters) error {
	c.Response().Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+auditFilename(scope, "csv")+`"`)
	w := csv.NewWriter(c.Response())
	defer w.Flush()
	_ = w.Write([]string{
		"id", "occurred_at", "org_id", "actor_user_id", "actor_email", "actor_name",
		"actor_kind", "actor_capacity", "auth_method", "api_key_id", "action", "category",
		"resource_type", "resource_id", "outcome", "status_code", "severity", "ip",
		"user_agent", "request_id", "method", "path", "source", "details",
	})
	return s.iterateAuditEvents(c.Request().Context(), scope, filters, auditExportMaxRows, func(ev auditlog.Event) error {
		details, _ := json.Marshal(ev.Details)
		status := ""
		if ev.StatusCode != nil {
			status = strconv.Itoa(*ev.StatusCode)
		}
		actorID := ""
		if ev.ActorUserID != nil {
			actorID = strconv.FormatInt(*ev.ActorUserID, 10)
		}
		apiKeyID := ""
		if ev.APIKeyID != nil {
			apiKeyID = strconv.FormatInt(*ev.APIKeyID, 10)
		}
		return w.Write([]string{
			strconv.FormatInt(ev.ID, 10), ev.OccurredAt.UTC().Format(time.RFC3339Nano),
			strconv.FormatInt(ev.OrgID, 10), actorID, ev.ActorEmail, ev.ActorName,
			string(ev.ActorKind), string(ev.ActorCapacity), ev.AuthMethod, apiKeyID,
			string(ev.Action), string(ev.Category), ev.ResourceType, ev.ResourceID,
			string(ev.Outcome), status, string(ev.Severity), ev.IP, ev.UserAgent,
			ev.RequestID, ev.Method, ev.Path, string(ev.Source), string(details),
		})
	})
}

func (s *Server) streamAuditNDJSON(c echo.Context, scope auditlog.Scope, filters auditlog.Filters, count int64) error {
	c.Response().Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+auditFilename(scope, "ndjson")+`"`)
	enc := json.NewEncoder(c.Response())
	return s.iterateAuditEvents(c.Request().Context(), scope, filters, auditExportMaxRows, func(ev auditlog.Event) error {
		return enc.Encode(ev)
	})
}

func (s *Server) streamAuditPackage(c echo.Context, scope auditlog.Scope, filters auditlog.Filters, count int64) error {
	tmp, err := os.CreateTemp("", "audit-package-*.ndjson")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to build package"})
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	hasher := sha256.New()
	enc := json.NewEncoder(io.MultiWriter(tmp, hasher))
	if err := s.iterateAuditEvents(c.Request().Context(), scope, filters, auditPackageMaxRows, func(ev auditlog.Event) error {
		return enc.Encode(ev)
	}); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to build package"})
	}

	manifest := map[string]any{
		"format_version": 1,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"org_id":         scope.OrgID,
		"rows":           count,
		"sha256":         hex.EncodeToString(hasher.Sum(nil)),
		"from":           filters.From,
		"to":             filters.To,
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")

	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+auditFilename(scope, "zip")+`"`)
	zw := zip.NewWriter(c.Response())
	nd, err := zw.Create("audit.ndjson")
	if err != nil {
		return err
	}
	reader, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	defer reader.Close()
	if _, err := io.Copy(nd, reader); err != nil {
		return err
	}
	mf, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err := mf.Write(manifestBytes); err != nil {
		return err
	}
	return zw.Close()
}

func (s *Server) iterateAuditEvents(ctx context.Context, scope auditlog.Scope, filters auditlog.Filters, maxRows int, fn func(auditlog.Event) error) error {
	const pageSize = 500
	cursor := ""
	var seen int
	for {
		pageFilters := filters
		pageFilters.Limit = pageSize
		pageFilters.Cursor = cursor
		q, err := auditlog.NewQuery(scope, pageFilters)
		if err != nil {
			return err
		}
		events, next, err := q.List(ctx, s.db)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if seen >= maxRows {
				return nil
			}
			if err := fn(ev); err != nil {
				return err
			}
			seen++
		}
		if next == "" || len(events) == 0 {
			return nil
		}
		cursor = next
	}
}

// DownloadAuditArchive handles GET /api/v1/orgs/:org_id/audit-events/archive.
func (s *Server) DownloadAuditArchive(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	if !scope.IsOwner && !scope.IsSuperAdmin {
		return auditForbidden(c)
	}
	month := strings.TrimSpace(c.QueryParam("month"))
	if !archiveMonthRe.MatchString(month) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "month must be YYYY-MM"})
	}
	var key string
	err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT object_key FROM audit_archives WHERE org_id = $1 AND month = $2`,
		scope.OrgID, month).Scan(&key)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no archive for that month yet"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to look up archive"})
	}
	bucket, err := blobstore.OpenBucketFromDB(c.Request().Context(), s.db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "storage unavailable"})
	}
	defer bucket.Close()
	reader, err := bucket.NewReader(c.Request().Context(), key, nil)
	if err != nil {
		if blobstore.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "archive object missing"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open archive"})
	}
	defer reader.Close()
	c.Response().Header().Set("Content-Type", "application/gzip")
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="livereview-audit-%d-%s.ndjson.gz"`, scope.OrgID, month))
	s.recordAuditRead(c, auditlog.ActionAuditArchiveDownload,
		auditlog.Details("month", month, "object_key", key))
	_, err = io.Copy(c.Response(), reader)
	return err
}

var archiveMonthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// AdminListAuditEvents handles GET /api/v1/admin/audit-events.
func (s *Server) AdminListAuditEvents(c echo.Context) error {
	scope, ok := s.auditReadScope(c, true)
	if !ok {
		return auditForbidden(c)
	}
	filters, err := parseAuditFilters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	q, err := auditlog.NewQuery(scope, filters)
	if err != nil {
		return auditForbidden(c)
	}
	events, next, err := q.List(c.Request().Context(), s.db)
	if err != nil {
		log.Error().Err(err).Msg("audit: admin list failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load audit events"})
	}
	if filters.Since == nil && filters.Cursor == "" {
		s.recordAuditRead(c, auditlog.ActionAuditRead, auditlog.Details(
			"scope", "all_orgs", "filters", auditFiltersSummary(filters), "rows", len(events)))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"events":      events,
		"next_cursor": next,
		"has_more":    next != "",
	})
}

// AdminAuditIntegrity handles GET /api/v1/admin/audit-events/integrity.
func (s *Server) AdminAuditIntegrity(c echo.Context) error {
	scope, ok := s.auditReadScope(c, true)
	if !ok {
		return auditForbidden(c)
	}
	_ = scope
	orgID, err := strconv.ParseInt(c.QueryParam("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "org_id is required"})
	}
	fromSeq, _ := strconv.ParseInt(c.QueryParam("from_seq"), 10, 64)
	limit := 0
	if v := c.QueryParam("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	report, err := auditlog.VerifyChain(c.Request().Context(), s.db, orgID, fromSeq, limit)
	if err != nil {
		log.Error().Err(err).Msg("audit: integrity check failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "integrity check failed"})
	}
	s.recordAuditRead(c, auditlog.ActionAuditIntegrityCheck, auditlog.Details(
		"org_id", orgID, "rows", report.Rows, "intact", report.Intact))
	return c.JSON(http.StatusOK, report)
}

// GetAuditSettings handles GET /api/v1/admin/audit-settings.
func (s *Server) GetAuditSettings(c echo.Context) error {
	if !auditSettingsAllowed(c) {
		return auditForbidden(c)
	}
	cfg := loadAuditSettings(c.Request().Context(), s.db)
	return c.JSON(http.StatusOK, auditSettingsForResponse(cfg))
}

// UpdateAuditSettings handles PUT /api/v1/admin/audit-settings.
func (s *Server) UpdateAuditSettings(c echo.Context) error {
	if !auditSettingsAllowed(c) {
		return auditForbidden(c)
	}
	pc := auth.GetPermissionContext(c)
	eventOrgID := int64(0)
	if pc != nil {
		eventOrgID = pc.OrgID
	}
	var cfg AuditSettings
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := cfg.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveAuditSettings(c.Request().Context(), tx, cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
	}
	if s.auditRecorder != nil {
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:    eventOrgID,
			Action:   auditlog.ActionSettingsRetention,
			Category: auditlog.CategorySettings,
			Outcome:  auditlog.OutcomeSuccess,
			Severity: auditlog.SeverityCritical,
			Details: auditlog.Details(
				"hot_days", cfg.HotDays, "retention_days", cfg.RetentionDays,
				"archive_enabled", cfg.ArchiveEnabled, "hash_chain_enabled", cfg.HashChainEnabled,
				"cron_expression", cfg.CronExpression),
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
	}
	if s.auditMaintenance != nil {
		s.auditMaintenance.Reload()
	}
	return c.JSON(http.StatusOK, auditSettingsForResponse(cfg))
}

// RunAuditMaintenance handles POST /api/v1/admin/audit-maintenance/run.
// The cycle runs in the background; the API returns immediately.
func (s *Server) RunAuditMaintenance(c echo.Context) error {
	if !auditSettingsAllowed(c) {
		return auditForbidden(c)
	}
	if s.auditMaintenance == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "audit maintenance unavailable"})
	}
	go s.auditMaintenance.RunNow()
	return c.JSON(http.StatusAccepted, map[string]string{"message": "audit maintenance started"})
}

// AdminExportAuditEvents handles GET /api/v1/admin/audit-events/export.
// Cross-org audit export is session-only (auditReadScope enforces that).
func (s *Server) AdminExportAuditEvents(c echo.Context) error {
	return s.exportAuditEvents(c, true)
}
