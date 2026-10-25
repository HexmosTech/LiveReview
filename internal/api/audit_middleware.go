package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/auditlog"
	"github.com/rs/zerolog/log"
)

// auditMiddleware is the automatic coverage layer. It records every non-GET
// /api/v1 request, plus GET requests whose route has a SensitiveRead spec.
// A handler that already wrote its canonical domain event is not
// double-recorded on success; failures are always recorded (the business
// transaction may have rolled back).
func (s *Server) auditMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		reqID := auditlog.RequestID(c)
		handlerErr := next(c)
		s.recordAuditRequest(c, handlerErr, reqID)
		return handlerErr
	}
}

func (s *Server) recordAuditRequest(c echo.Context, handlerErr error, reqID string) {
	if s == nil || s.auditRecorder == nil || s.auditRegistry == nil {
		return
	}
	path := c.Path()
	if path == "" || !strings.HasPrefix(path, "/api/v1/") {
		return
	}
	method := c.Request().Method
	isRead := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions

	spec, hasSpec := s.auditRegistry.Lookup(method, path)
	if isRead && !(hasSpec && spec.SensitiveRead) {
		return
	}
	// Auth endpoints are instrumented explicitly in the auth handlers, where
	// the attempted email and success/failure reason are available.
	if isAuthAuditSkipped(path) {
		return
	}

	status := responseStatus(c, handlerErr)
	if status >= 200 && status < 300 && auditlog.WasRecorded(c) {
		return
	}

	outcome := outcomeForStatus(status)
	action := auditlog.Action("http." + strings.ToLower(method))
	category := auditlog.CategoryOther
	resourceType, resourceID := "", ""
	severity := auditlog.SeverityInfo
	var details auditlog.Fields
	if hasSpec {
		action = spec.Action
		category = spec.Category
		resourceType = spec.ResourceType
		severity = spec.Severity
		if spec.ResourceIDParam != "" {
			resourceID = auditlog.ParamID(c, spec.ResourceIDParam)
		}
		if spec.Details != nil {
			details = spec.Details(c)
		}
		if !isRead && status >= 200 && status < 300 {
			// The handler returned success but did not emit its domain event.
			// Keep the coverage row, and make the gap visible to developers.
			log.Warn().Str("method", method).Str("path", path).
				Str("action", string(action)).
				Msg("audit: route has a spec but the handler did not record a domain event; wrote fallback row")
		}
	}
	if status >= 400 {
		severity = auditlog.SeverityWarning
	}
	if status >= 500 {
		severity = auditlog.SeverityCritical
	}

	ev := auditlog.Event{
		OccurredAt:   time.Now().UTC(),
		Action:       action,
		Category:     category,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Outcome:      outcome,
		StatusCode:   &status,
		Severity:     severity,
		Details:      details,
		RequestID:    reqID,
		Method:       method,
		Path:         path,
		Source:       auditlog.SourceAPI,
	}
	auditlog.EnrichFromEcho(c, &ev)
	if ev.OrgID == 0 && !strings.HasPrefix(path, "/api/v1/admin/") {
		// No verified org context (e.g. a public endpoint that is not one of
		// the auth routes). Record it under org 0; only super admins see it.
		ev.OrgID = 0
	}

	// Best-effort write with a short timeout: an audit outage must not block
	// ordinary product actions. Privileged handlers use RecordTx, which is
	// fail-closed.
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()
	ev.RequestID = reqID
	if err := s.auditRecorder.Record(ctx, ev); err != nil {
		auditlog.LogRecordError(action, err)
	}
}

func responseStatus(c echo.Context, handlerErr error) int {
	if handlerErr != nil {
		var he *echo.HTTPError
		if errors.As(handlerErr, &he) {
			if he.Code != 0 {
				return he.Code
			}
		}
		return http.StatusInternalServerError
	}
	status := c.Response().Status
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func outcomeForStatus(status int) auditlog.Outcome {
	switch {
	case status >= 200 && status < 300:
		return auditlog.OutcomeSuccess
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return auditlog.OutcomeDenied
	case status >= 400 && status < 500:
		return auditlog.OutcomeFailure
	default:
		return auditlog.OutcomeError
	}
}

func isAuthAuditSkipped(path string) bool {
	switch path {
	case "/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/change-password":
		// Instrumented explicitly where the attempted email and the
		// success/failure reason are available.
		return true
	case "/api/v1/auth/refresh":
		// Refreshes happen every few minutes per session; recording them
		// would bury meaningful events without adding security value.
		return true
	}
	// setup/onboard/ensure-cloud-user are state-changing and have no explicit
	// event, so they fall through to generic middleware coverage.
	return false
}

// audited registers a route and its audit spec atomically, so the two cannot
// drift. It returns the Echo route (with the full path) for callers that need
// it.
func (s *Server) audited(g *echo.Group, method, path string, spec auditlog.Spec, h echo.HandlerFunc) *echo.Route {
	r := g.Add(method, path, h)
	if s.auditRegistry != nil {
		s.auditRegistry.Register(method, r.Path, spec)
	}
	return r
}

// auditScopeFromContext builds the verified visibility scope for audit reads.
// org_id and role come exclusively from the middleware-built PermissionContext.
func auditScopeFromContext(c echo.Context, adminView bool) (auditlog.Scope, bool) {
	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return auditlog.Scope{}, false
	}
	if adminView && !pc.IsSuperAdmin {
		return auditlog.Scope{}, false
	}
	return auditlog.Scope{
		OrgID:        pc.OrgID,
		UserID:       pc.GetUserID(),
		IsOwner:      pc.IsOwner,
		IsSuperAdmin: pc.IsSuperAdmin,
		AdminView:    adminView,
	}, true
}

// validateAuditCoverage reports mutating routes that still lack a semantic
// spec. It is intentionally non-fatal: coverage exists via the middleware
// regardless. In development it logs the first few gaps so they get burned
// down over time.
func (s *Server) validateAuditCoverage() {
	if s == nil || s.auditRegistry == nil || s.echo == nil {
		return
	}
	problems := s.auditRegistry.ValidateRegistry()
	for _, p := range problems {
		log.Error().Str("problem", p).Msg("audit: invalid route spec")
	}
	violations := auditlog.ValidateRouteCoverage(s.echo.Routes(), auditRouteAllowlist, s.auditRegistry)
	if len(violations) == 0 {
		return
	}
	if os.Getenv("AUDIT_COVERAGE_PRINT") == "1" {
		for _, v := range violations {
			if v.Method == "echo_route_not_found" || strings.Contains(v.Path, "*") {
				continue
			}
			log.Info().Msgf("\t%q: %q,", v.Method+" "+v.Path, "grandfathered before the audit module")
		}
	}
	logger := log.Warn()
	if s.devMode {
		logger = log.Warn()
	}
	logger.Int("count", len(violations)).
		Msg("audit: mutating routes without a semantic spec (covered generically by the middleware; add s.audited() or an allowlist reason)")
	for i, v := range violations {
		if i >= 15 {
			break
		}
		log.Warn().Str("method", v.Method).Str("path", v.Path).Msg("audit: unmapped mutation")
	}
}
