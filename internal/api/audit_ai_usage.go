package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/auditlog"
)

// AuditAIUsageRow is one model invocation recorded in the billing ledger. It is
// the durable, org-scoped record of which provider/model handled a review and
// how many tokens it cost.
type AuditAIUsageRow struct {
	AccountedAt   time.Time `json:"accounted_at"`
	ReviewID      *int64    `json:"review_id,omitempty"`
	UserID        *int64    `json:"user_id,omitempty"`
	OperationType string    `json:"operation_type"`
	TriggerSource string    `json:"trigger_source"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	InputTokens   *int64    `json:"input_tokens,omitempty"`
	OutputTokens  *int64    `json:"output_tokens,omitempty"`
	CostUSD       *float64  `json:"cost_usd,omitempty"`
	ActorKind     string    `json:"actor_kind,omitempty"`
	ActorEmail    string    `json:"actor_email,omitempty"`
}

// GetAuditAIUsage handles GET /api/v1/orgs/:org_id/audit-events/ai-usage.
// It reads loc_usage_ledger rather than duplicating token accounting into the
// audit table: the ledger is already the immutable per-operation record.
func (s *Server) GetAuditAIUsage(c echo.Context) error {
	scope, ok := s.auditReadScope(c, false)
	if !ok {
		return auditForbidden(c)
	}
	from := time.Now().UTC().AddDate(0, 0, -30)
	if v := c.QueryParam("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	to := time.Now().UTC()
	if v := c.QueryParam("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	limit := 200
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	clauses := []string{"org_id = $1", "accounted_at >= $2", "accounted_at < $3", "status = 'accounted'"}
	args := []any{scope.OrgID, from, to}
	if !scope.IsOwner && !scope.IsSuperAdmin {
		clauses = append(clauses, "user_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, scope.UserID)
	} else if scope.IsOwner {
		// Owners see the org's member and system usage, but not support work
		// performed by a platform super admin.
		clauses = append(clauses, `(user_id IS NULL OR user_id NOT IN (
			SELECT ur.user_id FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE r.name = 'super_admin'
		))`)
	}
	if v := c.QueryParam("provider"); v != "" {
		clauses = append(clauses, "provider = $"+strconv.Itoa(len(args)+1))
		args = append(args, v)
	}
	if v := c.QueryParam("review_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			clauses = append(clauses, "review_id = $"+strconv.Itoa(len(args)+1))
			args = append(args, id)
		}
	}
	args = append(args, limit)

	sqlText := `SELECT accounted_at, review_id, user_id, operation_type, trigger_source,
		COALESCE(provider, ''), COALESCE(model, ''), input_tokens, output_tokens, llm_cost_usd,
		COALESCE(actor_kind, ''), COALESCE(actor_email_snapshot, '')
		FROM loc_usage_ledger WHERE ` + joinAnd(clauses) + `
		ORDER BY accounted_at DESC LIMIT $` + strconv.Itoa(len(args))
	rows, err := s.db.QueryContext(c.Request().Context(), sqlText, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load AI usage"})
	}
	defer rows.Close()

	out := make([]AuditAIUsageRow, 0)
	for rows.Next() {
		var (
			row      AuditAIUsageRow
			reviewID sql.NullInt64
			userID   sql.NullInt64
			inTok    sql.NullInt64
			outTok   sql.NullInt64
			cost     sql.NullFloat64
		)
		if err := rows.Scan(&row.AccountedAt, &reviewID, &userID, &row.OperationType, &row.TriggerSource,
			&row.Provider, &row.Model, &inTok, &outTok, &cost, &row.ActorKind, &row.ActorEmail); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read AI usage"})
		}
		if reviewID.Valid {
			row.ReviewID = &reviewID.Int64
		}
		if userID.Valid {
			row.UserID = &userID.Int64
		}
		if inTok.Valid {
			row.InputTokens = &inTok.Int64
		}
		if outTok.Valid {
			row.OutputTokens = &outTok.Int64
		}
		if cost.Valid {
			row.CostUSD = &cost.Float64
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read AI usage"})
	}
	s.recordAuditRead(c, auditlog.ActionAuditRead, auditlog.Details("view", "ai_usage", "rows", len(out)))
	return c.JSON(http.StatusOK, map[string]any{"rows": out})
}

func joinAnd(clauses []string) string {
	out := ""
	for i, c := range clauses {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}
