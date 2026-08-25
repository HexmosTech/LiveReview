package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/onboardingreport"
	"github.com/rs/zerolog/log"
)

// OnboardingReportHandler serves the Onboarding Report — a 1-click,
// LLM-free report that executes parameterized SQL templates and returns
// filled Vega-Lite chart specs for any org.
type OnboardingReportHandler struct {
	db *sql.DB
}

func NewOnboardingReportHandler(db *sql.DB) *OnboardingReportHandler {
	return &OnboardingReportHandler{db: db}
}

// chartResult is one chart with its query results injected into the Vega spec.
type chartResult struct {
	ID          string          `json:"id"`
	Section     string          `json:"section"`
	SectionLabel string         `json:"section_label"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	QuerySummary string         `json:"query_summary"`
	ChartType   string          `json:"chart_type"`
	Granularity string          `json:"granularity"`
	TimeRange   string          `json:"time_range"`
	VegaSpec    json.RawMessage `json:"vega_spec"`
	RowCount    int             `json:"row_count"`
	Error       string          `json:"error,omitempty"`
}

// sectionResult is the response for one section.
type sectionResult struct {
	Section     string        `json:"section"`
	SectionLabel string       `json:"section_label"`
	Charts      []chartResult `json:"charts"`
	TotalCharts int           `json:"total_charts"`
}

// GetSections returns the list of available sections (no SQL execution).
func (h *OnboardingReportHandler) GetSections(c echo.Context) error {
	sections := onboardingreport.Sections()
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"sections": sections,
		"total":    onboardingreport.Catalog().TotalCharts,
	})
}

// GetSectionCharts executes all chart templates for a given section and
// returns the filled Vega-Lite specs.
func (h *OnboardingReportHandler) GetSectionCharts(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "authentication required")
	}

	sectionID := strings.TrimSpace(c.Param("section"))
	if sectionID == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "section parameter required")
	}

	// Validate section exists
	validSection := false
	for _, s := range onboardingreport.Sections() {
		if s.ID == sectionID {
			validSection = true
			break
		}
	}
	if !validSection {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("unknown section: %s", sectionID))
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	charts := onboardingreport.ChartsBySection()[sectionID]
	results := make([]chartResult, 0, len(charts))

	for _, tmpl := range charts {
		cr := h.executeChart(ctx, pc.OrgID, tmpl)
		results = append(results, cr)
	}

	sectionLabel := ""
	for _, s := range onboardingreport.Sections() {
		if s.ID == sectionID {
			sectionLabel = s.Label
			break
		}
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"section":       sectionID,
		"section_label": sectionLabel,
		"charts":        results,
		"total_charts":  len(results),
	})
}

// GetAllCharts executes all chart templates across all sections.
func (h *OnboardingReportHandler) GetAllCharts(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "authentication required")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Minute)
	defer cancel()

	allCharts := onboardingreport.Charts()
	results := make([]chartResult, 0, len(allCharts))

	for _, tmpl := range allCharts {
		cr := h.executeChart(ctx, pc.OrgID, tmpl)
		results = append(results, cr)
	}

	// Group results by section
	sectionMap := make(map[string][]chartResult)
	for _, r := range results {
		sectionMap[r.Section] = append(sectionMap[r.Section], r)
	}

	sections := make([]sectionResult, 0, len(onboardingreport.Sections()))
	for _, s := range onboardingreport.Sections() {
		charts := sectionMap[s.ID]
		sections = append(sections, sectionResult{
			Section:      s.ID,
			SectionLabel: s.Label,
			Charts:       charts,
			TotalCharts:  len(charts),
		})
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"sections":      sections,
		"total_charts":  len(results),
		"generated_at":  time.Now().UTC().Format(time.RFC3339),
	})
}

// executeChart runs one chart template's SQL and injects results into the
// Vega-Lite spec. Errors are captured per-chart so one failure doesn't
// break the entire report.
func (h *OnboardingReportHandler) executeChart(ctx context.Context, orgID int64, tmpl onboardingreport.ChartTemplate) chartResult {
	cr := chartResult{
		ID:           tmpl.ID,
		Section:      tmpl.Section,
		SectionLabel: tmpl.SectionLabel,
		Title:        tmpl.Title,
		Description:  tmpl.Description,
		QuerySummary: tmpl.QuerySummary,
		ChartType:    tmpl.ChartType,
		Granularity:  tmpl.Granularity,
		TimeRange:    tmpl.TimeRange,
	}

	sqlQuery := tmpl.PrepareSQL(orgID)

	rows, err := h.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		log.Error().Err(err).Str("chart_id", tmpl.ID).Int64("org_id", orgID).Msg("onboarding report: query failed")
		cr.Error = fmt.Sprintf("query failed: %v", err)
		cr.VegaSpec = tmpl.VegaSpec // return spec without data
		return cr
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		cr.Error = fmt.Sprintf("failed to get columns: %v", err)
		cr.VegaSpec = tmpl.VegaSpec
		return cr
	}

	var resultRows []map[string]interface{}
	for rows.Next() {
		// Create a slice of interface{} to hold column values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			log.Error().Err(err).Str("chart_id", tmpl.ID).Msg("onboarding report: scan failed")
			continue
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for JSON serialization
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		cr.Error = fmt.Sprintf("row iteration error: %v", err)
		cr.VegaSpec = tmpl.VegaSpec
		return cr
	}

	cr.RowCount = len(resultRows)

	// Marshal the result rows and inject into Vega spec
	dataJSON, err := json.Marshal(resultRows)
	if err != nil {
		cr.Error = fmt.Sprintf("failed to marshal results: %v", err)
		cr.VegaSpec = tmpl.VegaSpec
		return cr
	}

	cr.VegaSpec = tmpl.PrepareVegaSpec(dataJSON)
	return cr
}
