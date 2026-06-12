package api

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	storagereports "github.com/livereview/storage/reviews"
	"github.com/xuri/excelize/v2"
)

func collectMultiQueryParam(c echo.Context, key string) string {
	values := c.QueryParams()[key]
	if len(values) == 0 {
		return ""
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			v := strings.TrimSpace(part)
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
	}
	return strings.Join(out, ",")
}

// TaxonomyReportHandler serves both JSON exploration and CSV export endpoints
// for review-finding taxonomy data.
type TaxonomyReportHandler struct {
	store *storagereports.TaxonomyReportStore
}

func NewTaxonomyReportHandler(db *sql.DB) *TaxonomyReportHandler {
	return &TaxonomyReportHandler{
		store: storagereports.NewTaxonomyReportStore(db),
	}
}

// parseTaxonomyFilter builds a TaxonomyFilter from the request query params and
// the org context.  orgID=0 means all orgs (super-admin only); handlers that
// enforce org scoping should pass the context org.
func parseTaxonomyFilter(c echo.Context, orgID int64) (storagereports.TaxonomyFilter, error) {
	f := storagereports.TaxonomyFilter{OrgID: orgID}

	if v := strings.TrimSpace(c.QueryParam("since")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			// Try date-only.
			t, err = time.Parse("2006-01-02", v)
			if err != nil {
				return f, fmt.Errorf("invalid since: must be RFC3339 or YYYY-MM-DD")
			}
			// Date-only "since" should start at local day boundary.
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
		f.Since = t
	}
	if v := strings.TrimSpace(c.QueryParam("until")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse("2006-01-02", v)
			if err != nil {
				return f, fmt.Errorf("invalid until: must be RFC3339 or YYYY-MM-DD")
			}
			// Date-only "until" should include the entire day, and SQL uses "< until".
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Add(24 * time.Hour)
		}
		f.Until = t
	}
	f.Repository = strings.TrimSpace(c.QueryParam("repository"))
	f.Provider = collectMultiQueryParam(c, "provider")
	f.Severity = collectMultiQueryParam(c, "severity")
	f.Confidence = collectMultiQueryParam(c, "confidence")
	f.IssueType = collectMultiQueryParam(c, "type")
	f.Category = collectMultiQueryParam(c, "category")
	f.Subcategory = collectMultiQueryParam(c, "subcategory")
	return f, nil
}

func parsePagination(c echo.Context, defaultLimit int) (limit, offset int, err error) {
	limit = defaultLimit
	if v := strings.TrimSpace(c.QueryParam("limit")); v != "" {
		parsed, perr := strconv.Atoi(v)
		if perr != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("invalid limit")
		}
		limit = parsed
	}
	if v := strings.TrimSpace(c.QueryParam("offset")); v != "" {
		parsed, perr := strconv.Atoi(v)
		if perr != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("invalid offset")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func parseFindingsOptions(c echo.Context) storagereports.TaxonomyFindingsOptions {
	filters := map[string]string{}
	for _, k := range []string{
		"severity",
		"confidence",
		"type",
		"category",
		"subcategory",
		"repository",
		"provider",
		"file_path",
		"line_number",
		"content",
		"created_at",
	} {
		if v := strings.TrimSpace(c.QueryParam("findings_filter_" + k)); v != "" {
			filters[k] = v
		}
	}

	sortBy := strings.TrimSpace(c.QueryParam("findings_sort_by"))
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortDirection := strings.ToLower(strings.TrimSpace(c.QueryParam("findings_sort_dir")))
	if sortDirection != "asc" {
		sortDirection = "desc"
	}

	return storagereports.TaxonomyFindingsOptions{
		SortBy:        sortBy,
		SortDirection: sortDirection,
		ColumnFilters: filters,
	}
}

// ---- Org-scoped handlers (owner/admin of current org) ----

// GetOrgTaxonomySummary returns KPI summary for the caller's current org.
func (h *TaxonomyReportHandler) GetOrgTaxonomySummary(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	summary, err := h.store.GetSummary(c.Request().Context(), f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("summary query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"total_findings":          summary.TotalFindings,
		"total_reviews":           summary.TotalReviews,
		"critical_count":          summary.CriticalCount,
		"high_count":              summary.HighCount,
		"medium_count":            summary.MediumCount,
		"low_count":               summary.LowCount,
		"info_count":              summary.InfoCount,
		"high_confidence_count":   summary.HighConfidence,
		"medium_confidence_count": summary.MediumConfidence,
		"low_confidence_count":    summary.LowConfidence,
	})
}

// GetOrgTaxonomyDistribution returns per-value counts for one taxonomy dimension.
func (h *TaxonomyReportHandler) GetOrgTaxonomyDistribution(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	dimension := strings.TrimSpace(c.Param("dimension"))
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetDistribution(c.Request().Context(), dimension, f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("distribution query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"dimension": dimension,
		"rows":      rows,
	})
}

// GetOrgTaxonomyTrend returns finding counts bucketed by time grain.
func (h *TaxonomyReportHandler) GetOrgTaxonomyTrend(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	grain := strings.TrimSpace(c.QueryParam("grain"))
	if grain == "" {
		grain = "day"
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetTrend(c.Request().Context(), grain, f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("trend query failed: %v", err))
	}
	fmt.Printf("[HANDLER] GetOrgTaxonomyTrend: received %d rows from store\n", len(rows))
	if len(rows) > 0 {
		fmt.Printf("[HANDLER] First row: %+v\n", rows[0])
	}
	payload := map[string]interface{}{
		"grain": grain,
		"rows":  rows,
	}
	fmt.Printf("[HANDLER] Payload before JSONWithEnvelope: %+v\n", payload)
	return JSONWithEnvelope(c, http.StatusOK, payload)
}

// GetOrgTaxonomyBreakdown returns per-repo/provider finding counts for the current org.
func (h *TaxonomyReportHandler) GetOrgTaxonomyBreakdown(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetBreakdown(c.Request().Context(), f, false)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("breakdown query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": rows})
}

// ListOrgTaxonomyFindings returns paginated raw finding rows for the current org.
func (h *TaxonomyReportHandler) ListOrgTaxonomyFindings(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	limit, offset, err := parsePagination(c, 50)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, total, err := h.store.ListFindings(c.Request().Context(), f, limit, offset, parseFindingsOptions(c))
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("findings query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"rows":   rows,
	})
}

// GetOrgTaxonomyRelations returns category -> subcategory relation rows.
func (h *TaxonomyReportHandler) GetOrgTaxonomyRelations(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetCategorySubcategoryRelations(c.Request().Context(), f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("relations query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": rows})
}

// GetOrgTaxonomyExportPreview returns row estimates for each export dataset.
func (h *TaxonomyReportHandler) GetOrgTaxonomyExportPreview(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	return h.getExportPreview(c, orgID, false)
}

// ---- Super-admin global handlers ----

// GetAdminTaxonomySummary returns global KPI summary (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomySummary(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	summary, err := h.store.GetSummary(c.Request().Context(), f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("summary query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"total_findings":          summary.TotalFindings,
		"total_reviews":           summary.TotalReviews,
		"critical_count":          summary.CriticalCount,
		"high_count":              summary.HighCount,
		"medium_count":            summary.MediumCount,
		"low_count":               summary.LowCount,
		"info_count":              summary.InfoCount,
		"high_confidence_count":   summary.HighConfidence,
		"medium_confidence_count": summary.MediumConfidence,
		"low_confidence_count":    summary.LowConfidence,
	})
}

// GetAdminTaxonomyDistribution returns global distribution (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomyDistribution(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	dimension := strings.TrimSpace(c.Param("dimension"))
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetDistribution(c.Request().Context(), dimension, f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("distribution query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"dimension": dimension,
		"rows":      rows,
	})
}

// GetAdminTaxonomyTrend returns global trend (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomyTrend(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	grain := strings.TrimSpace(c.QueryParam("grain"))
	if grain == "" {
		grain = "day"
	}
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetTrend(c.Request().Context(), grain, f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("trend query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"grain": grain,
		"rows":  rows,
	})
}

// GetAdminTaxonomyBreakdown returns global org/repo/provider breakdown (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomyBreakdown(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetBreakdown(c.Request().Context(), f, true)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("breakdown query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": rows})
}

// ListAdminTaxonomyFindings returns paginated global finding rows (super-admin).
func (h *TaxonomyReportHandler) ListAdminTaxonomyFindings(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	limit, offset, err := parsePagination(c, 50)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, total, err := h.store.ListFindings(c.Request().Context(), f, limit, offset, parseFindingsOptions(c))
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("findings query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"rows":   rows,
	})
}

// GetAdminTaxonomyRelations returns category -> subcategory relation rows (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomyRelations(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	rows, err := h.store.GetCategorySubcategoryRelations(c.Request().Context(), f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("relations query failed: %v", err))
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": rows})
}

// GetAdminTaxonomyExportPreview returns row estimates for each export dataset (super-admin).
func (h *TaxonomyReportHandler) GetAdminTaxonomyExportPreview(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	return h.getExportPreview(c, orgID, true)
}

// ---- CSV export helpers ----

// ExportOrgTaxonomyCSV streams a CSV of raw findings for the current org.
func (h *TaxonomyReportHandler) ExportOrgTaxonomyCSV(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "org context required"})
	}
	dataset := strings.TrimSpace(c.QueryParam("dataset"))
	return h.streamCSV(c, orgID, dataset, false)
}

// ExportOrgTaxonomyXLSX streams a multi-sheet xlsx export for the current org.
func (h *TaxonomyReportHandler) ExportOrgTaxonomyXLSX(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "org context required"})
	}
	return h.streamXLSX(c, orgID, false)
}

// ExportAdminTaxonomyCSV streams a CSV export for super-admin.
func (h *TaxonomyReportHandler) ExportAdminTaxonomyCSV(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	dataset := strings.TrimSpace(c.QueryParam("dataset"))
	return h.streamCSV(c, orgID, dataset, true)
}

// ExportAdminTaxonomyXLSX streams a multi-sheet xlsx export for super-admin.
func (h *TaxonomyReportHandler) ExportAdminTaxonomyXLSX(c echo.Context) error {
	orgID := parseOptionalOrgID(c)
	return h.streamXLSX(c, orgID, true)
}

// streamCSV generates and streams the chosen dataset as CSV.
// dataset: findings | category_distribution | severity_distribution | trend | breakdown
func (h *TaxonomyReportHandler) streamCSV(c echo.Context, orgID int64, dataset string, includeOrgName bool) error {
	if dataset == "" {
		dataset = "findings"
	}

	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	grain := strings.TrimSpace(c.QueryParam("grain"))
	if grain == "" {
		grain = "day"
	}

	filename := fmt.Sprintf("livereview-impact-report-%s-%s.csv", dataset, time.Now().UTC().Format("20060102"))
	c.Response().Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().WriteHeader(http.StatusOK)

	w := csv.NewWriter(c.Response())

	ctx := c.Request().Context()

	switch dataset {
	case "findings":
		_ = w.Write([]string{
			"comment_id", "review_id", "org_id", "repository", "provider",
			"file_path", "line_number", "severity", "confidence", "type",
			"category", "subcategory", "content", "created_at",
		})
		limit := 5000
		offset := 0
		for {
			rows, _, err2 := h.store.ListFindings(ctx, f, limit, offset, storagereports.TaxonomyFindingsOptions{})
			if err2 != nil {
				break
			}
			for _, r := range rows {
				fp := ""
				if r.FilePath != nil {
					fp = *r.FilePath
				}
				ln := ""
				if r.LineNumber != nil {
					ln = strconv.Itoa(*r.LineNumber)
				}
				_ = w.Write([]string{
					strconv.FormatInt(r.CommentID, 10),
					strconv.FormatInt(r.ReviewID, 10),
					strconv.FormatInt(r.OrgID, 10),
					r.Repository, r.Provider,
					fp, ln,
					r.Severity, r.Confidence, r.IssueType,
					r.Category, r.Subcategory,
					r.Content, r.CreatedAt,
				})
			}
			if len(rows) < limit {
				break
			}
			offset += limit
		}

	case "category_distribution":
		_ = w.Write([]string{"dimension", "value", "count"})
		for _, dim := range []string{"category", "subcategory"} {
			rows, err2 := h.store.GetDistribution(ctx, dim, f)
			if err2 != nil {
				break
			}
			for _, r := range rows {
				_ = w.Write([]string{r.Dimension, r.Value, strconv.FormatInt(r.Count, 10)})
			}
		}

	case "severity_distribution":
		_ = w.Write([]string{"dimension", "value", "count"})
		for _, dim := range []string{"severity", "confidence", "type"} {
			rows, err2 := h.store.GetDistribution(ctx, dim, f)
			if err2 != nil {
				break
			}
			for _, r := range rows {
				_ = w.Write([]string{r.Dimension, r.Value, strconv.FormatInt(r.Count, 10)})
			}
		}

	case "trend":
		_ = w.Write([]string{"bucket", "findings_count", "reviews_count"})
		rows, err2 := h.store.GetTrend(ctx, grain, f)
		if err2 == nil {
			for _, r := range rows {
				_ = w.Write([]string{r.Bucket, strconv.FormatInt(r.Count, 10), strconv.FormatInt(r.ReviewCount, 10)})
			}
		}

	case "breakdown":
		_ = w.Write([]string{"org_id", "org_name", "repository", "provider", "findings_count", "reviews_count"})
		rows, err2 := h.store.GetBreakdown(ctx, f, includeOrgName)
		if err2 == nil {
			for _, r := range rows {
				oid := ""
				if r.OrgID != nil {
					oid = strconv.FormatInt(*r.OrgID, 10)
				}
				oname := ""
				if r.OrgName != nil {
					oname = *r.OrgName
				}
				_ = w.Write([]string{oid, oname, r.Repository, r.Provider, strconv.FormatInt(r.Count, 10), strconv.FormatInt(r.ReviewCount, 10)})
			}
		}

	default:
		_ = w.Write([]string{"error"})
		_ = w.Write([]string{fmt.Sprintf("unknown dataset %q; valid: findings, category_distribution, severity_distribution, trend, breakdown", dataset)})
	}

	w.Flush()
	return nil
}

func parseDatasets(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"findings", "severity_distribution", "category_distribution", "trend", "breakdown"}
	}
	parts := strings.Split(raw, ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		ds := strings.TrimSpace(p)
		if ds == "" || seen[ds] {
			continue
		}
		switch ds {
		case "findings", "severity_distribution", "category_distribution", "trend", "breakdown":
			out = append(out, ds)
			seen[ds] = true
		}
	}
	if len(out) == 0 {
		return []string{"findings"}
	}
	return out
}

func (h *TaxonomyReportHandler) streamXLSX(c echo.Context, orgID int64, includeOrgName bool) error {
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	grain := strings.TrimSpace(c.QueryParam("grain"))
	if grain == "" {
		grain = "day"
	}
	datasets := parseDatasets(c.QueryParam("datasets"))

	wb := excelize.NewFile()
	defer wb.Close()
	// Remove default empty sheet; we add one sheet per dataset.
	defaultSheet := wb.GetSheetName(0)
	if defaultSheet != "" {
		_ = wb.DeleteSheet(defaultSheet)
	}

	ctx := c.Request().Context()

	for _, ds := range datasets {
		sheetName := ds
		if len(sheetName) > 31 {
			sheetName = sheetName[:31]
		}
		_, _ = wb.NewSheet(sheetName)

		switch ds {
		case "findings":
			headers := []string{"comment_id", "review_id", "org_id", "repository", "provider", "file_path", "line_number", "severity", "confidence", "type", "category", "subcategory", "content", "created_at"}
			for i, hcol := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				_ = wb.SetCellValue(sheetName, cell, hcol)
			}
			limit := 5000
			offset := 0
			rowNum := 2
			for {
				rows, _, err2 := h.store.ListFindings(ctx, f, limit, offset, storagereports.TaxonomyFindingsOptions{})
				if err2 != nil {
					break
				}
				for _, r := range rows {
					line := ""
					if r.LineNumber != nil {
						line = strconv.Itoa(*r.LineNumber)
					}
					filePath := ""
					if r.FilePath != nil {
						filePath = *r.FilePath
					}
					vals := []interface{}{r.CommentID, r.ReviewID, r.OrgID, r.Repository, r.Provider, filePath, line, r.Severity, r.Confidence, r.IssueType, r.Category, r.Subcategory, r.Content, r.CreatedAt}
					for col, v := range vals {
						cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
						_ = wb.SetCellValue(sheetName, cell, v)
					}
					rowNum++
				}
				if len(rows) < limit {
					break
				}
				offset += limit
			}

		case "severity_distribution":
			headers := []string{"dimension", "value", "count"}
			for i, hcol := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				_ = wb.SetCellValue(sheetName, cell, hcol)
			}
			rowNum := 2
			for _, dim := range []string{"severity", "confidence", "type"} {
				rows, err2 := h.store.GetDistribution(ctx, dim, f)
				if err2 != nil {
					continue
				}
				for _, r := range rows {
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), r.Dimension)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), r.Value)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), r.Count)
					rowNum++
				}
			}

		case "category_distribution":
			headers := []string{"dimension", "value", "count"}
			for i, hcol := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				_ = wb.SetCellValue(sheetName, cell, hcol)
			}
			rowNum := 2
			for _, dim := range []string{"category", "subcategory"} {
				rows, err2 := h.store.GetDistribution(ctx, dim, f)
				if err2 != nil {
					continue
				}
				for _, r := range rows {
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), r.Dimension)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), r.Value)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), r.Count)
					rowNum++
				}
			}

		case "trend":
			headers := []string{"bucket", "findings_count", "reviews_count"}
			for i, hcol := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				_ = wb.SetCellValue(sheetName, cell, hcol)
			}
			rows, err2 := h.store.GetTrend(ctx, grain, f)
			if err2 == nil {
				for i, r := range rows {
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("A%d", i+2), r.Bucket)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("B%d", i+2), r.Count)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("C%d", i+2), r.ReviewCount)
				}
			}

		case "breakdown":
			headers := []string{"org_id", "org_name", "repository", "provider", "findings_count", "reviews_count"}
			for i, hcol := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				_ = wb.SetCellValue(sheetName, cell, hcol)
			}
			rows, err2 := h.store.GetBreakdown(ctx, f, includeOrgName)
			if err2 == nil {
				for i, r := range rows {
					orgIDVal := ""
					if r.OrgID != nil {
						orgIDVal = strconv.FormatInt(*r.OrgID, 10)
					}
					orgNameVal := ""
					if r.OrgName != nil {
						orgNameVal = *r.OrgName
					}
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("A%d", i+2), orgIDVal)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("B%d", i+2), orgNameVal)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("C%d", i+2), r.Repository)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("D%d", i+2), r.Provider)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("E%d", i+2), r.Count)
					_ = wb.SetCellValue(sheetName, fmt.Sprintf("F%d", i+2), r.ReviewCount)
				}
			}
		}
	}

	buf := bytes.NewBuffer(nil)
	if err := wb.Write(buf); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("xlsx generation failed: %v", err)})
	}

	filename := fmt.Sprintf("livereview-impact-report-export-%s.xlsx", time.Now().UTC().Format("20060102"))
	c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().WriteHeader(http.StatusOK)
	_, _ = c.Response().Write(buf.Bytes())
	return nil
}

// parseOptionalOrgID reads an optional ?org_id= query param. Returns 0 if absent.
func parseOptionalOrgID(c echo.Context) int64 {
	v := strings.TrimSpace(c.QueryParam("org_id"))
	if v == "" {
		return 0
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func (h *TaxonomyReportHandler) getExportPreview(c echo.Context, orgID int64, includeOrgName bool) error {
	f, err := parseTaxonomyFilter(c, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	grain := strings.TrimSpace(c.QueryParam("grain"))
	if grain == "" {
		grain = "day"
	}

	ctx := c.Request().Context()
	_, findingsTotal, err := h.store.ListFindings(ctx, f, 1, 0, storagereports.TaxonomyFindingsOptions{})
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview findings failed: %v", err))
	}
	sevRows, err := h.store.GetDistribution(ctx, "severity", f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview severity failed: %v", err))
	}
	confRows, err := h.store.GetDistribution(ctx, "confidence", f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview confidence failed: %v", err))
	}
	typeRows, err := h.store.GetDistribution(ctx, "type", f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview type failed: %v", err))
	}
	catRows, err := h.store.GetDistribution(ctx, "category", f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview category failed: %v", err))
	}
	subRows, err := h.store.GetDistribution(ctx, "subcategory", f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview subcategory failed: %v", err))
	}
	trendRows, err := h.store.GetTrend(ctx, grain, f)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview trend failed: %v", err))
	}
	breakRows, err := h.store.GetBreakdown(ctx, f, includeOrgName)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("preview breakdown failed: %v", err))
	}

	preview := map[string]int64{
		"findings":              findingsTotal,
		"severity_distribution": int64(len(sevRows) + len(confRows) + len(typeRows)),
		"category_distribution": int64(len(catRows) + len(subRows)),
		"trend":                 int64(len(trendRows)),
		"breakdown":             int64(len(breakRows)),
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": preview})
}
