package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/canonicaldoc"
	storagereviews "github.com/livereview/storage/reviews"
)

// CIRulesetsHandler implements the CI/CD gate ruleset CRUD + evaluation API.
//
// A ruleset is a named, org-scoped jq expression run against a canonical
// per-review findings document (see buildCanonicalReviewDoc). CI callers hit
// GET /api/v1/ci-rulesets/:id/evaluate?review_id=... and act on the HTTP
// status code alone (200 allow, 422 block, 202 review still running) -- no
// client-side jq or JSON parsing required. The UI's builder page uses
// POST /api/v1/ci-rulesets/preview to try an unsaved expression against a
// real past review before saving.
type CIRulesetsHandler struct {
	db       *sql.DB
	taxonomy *storagereviews.TaxonomyReportStore
}

func NewCIRulesetsHandler(db *sql.DB) *CIRulesetsHandler {
	return &CIRulesetsHandler{db: db, taxonomy: storagereviews.NewTaxonomyReportStore(db)}
}

// CIRuleset is the persisted/returned shape of a ruleset row.
type CIRuleset struct {
	ID          int64     `json:"id"`
	OrgID       int64     `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	JQExpr      string    `json:"jq_expr"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type upsertCIRulesetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	JQExpr      string `json:"jq_expr"`
}

// ---- canonical review document ----

// The canonical document types and jq evaluation are shared with the
// risk-policy engine's static gate stage via internal/canonicaldoc, so there
// is exactly one rule engine (see docs/risk-policy-engine.md §4). These
// aliases keep the existing handler API and its tests unchanged.
type canonicalFinding = canonicaldoc.Finding
type canonicalReviewDoc = canonicaldoc.ReviewDoc
type canonicalReviewDocCounts = canonicaldoc.Counts

var knownSeverities = canonicaldoc.KnownSeverities
var runJQBool = canonicaldoc.RunJQBool

// buildCanonicalReviewDoc delegates to internal/canonicaldoc.Build, scoping the
// review strictly to orgID -- never trust a caller-supplied review_id without
// confirming org ownership first.
func (h *CIRulesetsHandler) buildCanonicalReviewDoc(ctx context.Context, orgID, reviewID int64) (*canonicalReviewDoc, error) {
	return canonicaldoc.Build(ctx, h.db, orgID, reviewID)
}

// ---- CRUD ----

func (h *CIRulesetsHandler) List(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	rows, err := h.db.QueryContext(c.Request().Context(), `
		SELECT id, org_id, name, description, jq_expr, created_by, created_at, updated_at
		FROM ci_rulesets WHERE org_id = $1 ORDER BY updated_at DESC`, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("list failed: %v", err))
	}
	defer rows.Close()

	out := make([]CIRuleset, 0)
	for rows.Next() {
		var r CIRuleset
		var createdBy sql.NullInt64
		if err := rows.Scan(&r.ID, &r.OrgID, &r.Name, &r.Description, &r.JQExpr, &createdBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
		}
		if createdBy.Valid {
			r.CreatedBy = &createdBy.Int64
		}
		out = append(out, r)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": out})
}

func (h *CIRulesetsHandler) Get(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid ruleset id")
	}
	r, err := h.getRuleset(c.Request().Context(), orgID, id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "ruleset not found")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"ruleset": r})
}

func (h *CIRulesetsHandler) getRuleset(ctx context.Context, orgID, id int64) (*CIRuleset, error) {
	var r CIRuleset
	var createdBy sql.NullInt64
	err := h.db.QueryRowContext(ctx, `
		SELECT id, org_id, name, description, jq_expr, created_by, created_at, updated_at
		FROM ci_rulesets WHERE id = $1 AND org_id = $2`, id, orgID).
		Scan(&r.ID, &r.OrgID, &r.Name, &r.Description, &r.JQExpr, &createdBy, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if createdBy.Valid {
		r.CreatedBy = &createdBy.Int64
	}
	return &r, nil
}

func (h *CIRulesetsHandler) Create(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()

	var body upsertCIRulesetRequest
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.Name = strings.TrimSpace(body.Name)
	body.JQExpr = strings.TrimSpace(body.JQExpr)
	if body.Name == "" || body.JQExpr == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "name and jq_expr are required")
	}
	if _, err := gojq.Parse(body.JQExpr); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("invalid jq expression: %v", err))
	}

	var createdBy *int64
	if pc.User != nil {
		id := pc.User.ID
		createdBy = &id
	}

	var id int64
	err := h.db.QueryRowContext(c.Request().Context(), `
		INSERT INTO ci_rulesets (org_id, name, description, jq_expr, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, body.Name, body.Description, body.JQExpr, createdBy).Scan(&id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("create failed: %v", err))
	}
	r, err := h.getRuleset(c.Request().Context(), orgID, id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "created but failed to reload")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"ruleset": r})
}

func (h *CIRulesetsHandler) Update(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid ruleset id")
	}
	if _, err := h.getRuleset(c.Request().Context(), orgID, id); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "ruleset not found")
	}

	var body upsertCIRulesetRequest
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.Name = strings.TrimSpace(body.Name)
	body.JQExpr = strings.TrimSpace(body.JQExpr)
	if body.Name == "" || body.JQExpr == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "name and jq_expr are required")
	}
	if _, err := gojq.Parse(body.JQExpr); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("invalid jq expression: %v", err))
	}

	_, err = h.db.ExecContext(c.Request().Context(), `
		UPDATE ci_rulesets SET name = $1, description = $2, jq_expr = $3, updated_at = now()
		WHERE id = $4 AND org_id = $5`, body.Name, body.Description, body.JQExpr, id, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
	}
	r, err := h.getRuleset(c.Request().Context(), orgID, id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "updated but failed to reload")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"ruleset": r})
}

func (h *CIRulesetsHandler) Delete(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid ruleset id")
	}
	res, err := h.db.ExecContext(c.Request().Context(), `DELETE FROM ci_rulesets WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("delete failed: %v", err))
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "ruleset not found")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "deleted"})
}

// ---- preview (builder) ----

type previewCIRulesetRequest struct {
	JQExpr   string      `json:"jq_expr"`
	ReviewID int64       `json:"review_id"`
	Document interface{} `json:"document"` // used instead of ReviewID when previewing against the synthetic sample doc
}

// Preview runs an unsaved jq expression against either a real past review's
// canonical document (ReviewID) or a caller-supplied document (e.g. the
// synthetic sample doc, before any review exists), for the builder page's
// live-preview panel. Member access is enough since it's read-only and
// doesn't touch saved rulesets.
func (h *CIRulesetsHandler) Preview(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	var body previewCIRulesetRequest
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.JQExpr = strings.TrimSpace(body.JQExpr)
	if body.JQExpr == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "jq_expr is required")
	}

	var doc interface{}
	if body.ReviewID > 0 {
		d, err := h.buildCanonicalReviewDoc(c.Request().Context(), orgID, body.ReviewID)
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusNotFound, "review not found in this organization")
		}
		doc = d
	} else if body.Document != nil {
		doc = body.Document
	} else {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "either review_id or document is required")
	}

	blocked, result, err := runJQBool(body.JQExpr, doc)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"block":    blocked,
		"result":   result,
		"document": doc,
	})
}

// ---- evaluate (CI-facing) ----

// Evaluate handles GET /api/v1/ci-rulesets/:id/evaluate?review_id=... -- the
// single primitive every CI integration (curl, lrc, or an MCP tool) calls.
// Status code is a complete integration on its own:
//
//	200 -> allow (rule did not match)
//	422 -> block (rule matched)
//	202 -> review still running; retry
//	4xx -> bad ruleset/review id, or org mismatch
func (h *CIRulesetsHandler) Evaluate(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	rulesetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid ruleset id")
	}
	reviewID, err := strconv.ParseInt(c.QueryParam("review_id"), 10, 64)
	if err != nil || reviewID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "review_id query parameter is required")
	}

	ruleset, err := h.getRuleset(c.Request().Context(), orgID, rulesetID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "ruleset not found in this organization")
	}

	doc, err := h.buildCanonicalReviewDoc(c.Request().Context(), orgID, reviewID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "review not found in this organization")
	}

	if doc.Status != "" && doc.Status != "completed" && doc.Status != "failed" && doc.Status != "error" {
		return JSONWithEnvelope(c, http.StatusAccepted, map[string]interface{}{
			"status":     doc.Status,
			"block":      false,
			"ruleset_id": ruleset.ID,
			"review_id":  doc.ReviewID,
			"message":    "review not finished yet; retry",
		})
	}

	blocked, result, err := runJQBool(ruleset.JQExpr, doc)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("ruleset evaluation failed: %v", err))
	}

	status := http.StatusOK
	if blocked {
		status = http.StatusUnprocessableEntity
	}
	return JSONWithEnvelope(c, status, map[string]interface{}{
		"block":        blocked,
		"result":       result,
		"ruleset_id":   ruleset.ID,
		"ruleset_name": ruleset.Name,
		"review_id":    doc.ReviewID,
		"status":       doc.Status,
		"counts":       doc.Counts,
	})
}

// ciTaxonomyDimensions is the field->store-dimension map for Taxonomy below.
// "severity" is intentionally excluded: its 5 values are fixed (see
// knownSeverities) and always zero-filled, unlike the free-text dimensions.
var ciTaxonomyDimensions = map[string]string{
	"confidences":   "confidence",
	"categories":    "category",
	"subcategories": "subcategory",
	"types":         "type",
}

// ciCategoryNode is one category branch of the category->subcategory tree,
// mirroring git-lrc's issue_filter_state.mjs buildIssueCategoryGroups shape
// (category chip with count, nested subcategory chips with their own
// counts) -- but computed org-wide from GetCategorySubcategoryRelations
// instead of per-review, since this is a stable reference, not a live filter.
type ciCategoryNode struct {
	Category      string              `json:"category"`
	Count         int                 `json:"count"`
	Subcategories []ciSubcategoryLeaf `json:"subcategories"`
}

type ciSubcategoryLeaf struct {
	Subcategory string `json:"subcategory"`
	Count       int    `json:"count"`
}

// Taxonomy returns every distinct value actually seen in this org's findings
// for each dimension, plus the fixed severity list and the category ->
// subcategory hierarchy -- the reference panel, the insert-palette, and the
// LLM prompt builder all pull from this so users (and the LLM) see real
// vocabulary instead of a generic guess.
func (h *CIRulesetsHandler) Taxonomy(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	ctx := c.Request().Context()
	out := map[string]interface{}{"severities": knownSeverities}
	for field, dimension := range ciTaxonomyDimensions {
		rows, err := h.taxonomy.GetDistribution(ctx, dimension, storagereviews.TaxonomyFilter{OrgID: orgID})
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("taxonomy query failed: %v", err))
		}
		values := make([]string, 0, len(rows))
		for _, r := range rows {
			if v := strings.ToLower(strings.TrimSpace(r.Value)); v != "" {
				values = append(values, v)
			}
		}
		out[field] = values
	}

	relations, err := h.taxonomy.GetCategorySubcategoryRelations(ctx, storagereviews.TaxonomyFilter{OrgID: orgID})
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("taxonomy relations query failed: %v", err))
	}
	byCategory := map[string]*ciCategoryNode{}
	var order []string
	for _, rel := range relations {
		cat := strings.ToLower(strings.TrimSpace(rel.Category))
		if cat == "" {
			continue
		}
		node, ok := byCategory[cat]
		if !ok {
			node = &ciCategoryNode{Category: cat, Subcategories: []ciSubcategoryLeaf{}}
			byCategory[cat] = node
			order = append(order, cat)
		}
		node.Count += int(rel.Count)
		if sub := strings.ToLower(strings.TrimSpace(rel.Subcategory)); sub != "" {
			node.Subcategories = append(node.Subcategories, ciSubcategoryLeaf{Subcategory: sub, Count: int(rel.Count)})
		}
	}
	tree := make([]ciCategoryNode, 0, len(order))
	for _, cat := range order {
		tree = append(tree, *byCategory[cat])
	}
	out["category_tree"] = tree

	return JSONWithEnvelope(c, http.StatusOK, out)
}

// SampleDocument returns a synthetic canonical document (no real review
// needed) so the builder page has something to preview against before an
// org has any completed reviews yet.
func (h *CIRulesetsHandler) SampleDocument(c echo.Context) error {
	doc := canonicalReviewDoc{
		ReviewID:   123,
		OrgID:      1,
		Repository: "acme/webapp",
		Provider:   "github",
		Status:     "completed",
		Findings: []canonicalFinding{
			{Severity: "critical", Confidence: "high", Category: "security", Subcategory: "sql-injection", Type: "bug", FilePath: "internal/api/users.go", LineNumber: ciSampleIntPtr(42)},
			{Severity: "warning", Confidence: "medium", Category: "performance", Subcategory: "n-plus-one", Type: "bug", FilePath: "internal/api/reviews.go", LineNumber: ciSampleIntPtr(88)},
			{Severity: "info", Confidence: "high", Category: "style", Subcategory: "naming", Type: "suggestion", FilePath: "internal/api/orgs.go", LineNumber: ciSampleIntPtr(12)},
		},
		Counts: canonicalReviewDocCounts{
			BySeverity: map[string]int{"critical": 1, "warning": 1, "info": 1},
			ByCategory: map[string]int{"security": 1, "performance": 1, "style": 1},
			Total:      3,
		},
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"document": doc})
}

func ciSampleIntPtr(v int) *int { return &v }
