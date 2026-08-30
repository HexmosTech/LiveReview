package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatstats"
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
	Stats       json.RawMessage `json:"stats,omitempty"`
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

// onboardingExportJob tracks one in-flight (or finished) PDF/HTML export.
// Generating either export re-runs every chart's SQL query and re-renders
// its chart to a PNG via vl-convert — the same expensive pipeline the CLI
// tool uses — which for a full catalog can take well over a minute.
// Running that behind a single synchronous HTTP response left users
// looking at a bare spinner with no way to tell a slow-but-working request
// from a stuck one, so generation instead runs in a background goroutine
// (see runExportJob) with progress polled via ExportStatus, and the
// finished bytes fetched once via ExportFile.
type onboardingExportJob struct {
	mu          sync.Mutex
	orgID       int64
	status      string // "running", "done", "error"
	current     int
	total       int
	label       string
	data        []byte
	filename    string
	contentType string
	errMsg      string
	createdAt   time.Time
}

// onboardingExportJobTTL bounds how long a finished or abandoned job's
// bytes stay in memory before being swept, since jobs live in a plain
// process-local map rather than a persistent store.
const onboardingExportJobTTL = 20 * time.Minute

var (
	onboardingExportJobsMu sync.Mutex
	onboardingExportJobs   = map[string]*onboardingExportJob{}
)

func sweepOnboardingExportJobs() {
	cutoff := time.Now().Add(-onboardingExportJobTTL)
	onboardingExportJobsMu.Lock()
	defer onboardingExportJobsMu.Unlock()
	for id, job := range onboardingExportJobs {
		job.mu.Lock()
		expired := job.createdAt.Before(cutoff)
		job.mu.Unlock()
		if expired {
			delete(onboardingExportJobs, id)
		}
	}
}

// StartExport kicks off asynchronous generation of the onboarding report
// (?format=pdf|html) for the caller's org and returns a job ID immediately.
// Poll ExportStatus for progress, then fetch ExportFile once status is "done".
func (h *OnboardingReportHandler) StartExport(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "authentication required")
	}

	format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))
	if format != "pdf" && format != "html" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, `format must be "pdf" or "html"`)
	}

	sweepOnboardingExportJobs()

	orgName := orgNameFromContext(pc)
	job := &onboardingExportJob{
		orgID:     pc.OrgID,
		status:    "running",
		total:     onboardingreport.Catalog().TotalCharts,
		createdAt: time.Now(),
	}
	jobID := uuid.NewString()

	onboardingExportJobsMu.Lock()
	onboardingExportJobs[jobID] = job
	onboardingExportJobsMu.Unlock()

	go h.runExportJob(job, format, pc.OrgID, orgName)

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"total":  job.total,
	})
}

// runExportJob does the actual generation work in the background, updating
// job's progress fields as generateChartResults reports each chart. It runs
// detached from the triggering request's context (which ends the moment
// StartExport responds) with its own generous timeout instead.
func (h *OnboardingReportHandler) runExportJob(job *onboardingExportJob, format string, orgID int64, orgName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	onProgress := func(current, total int, label string) {
		job.mu.Lock()
		job.current = current
		job.total = total
		job.label = label
		job.mu.Unlock()
	}

	var buf bytes.Buffer
	var err error
	var contentType, ext string
	switch format {
	case "pdf":
		contentType, ext = "application/pdf", "pdf"
		err = onboardingreport.GenerateOnboardingPDFToWriter(ctx, h.db, orgID, orgName, &buf, onProgress)
	default:
		contentType, ext = "text/html; charset=utf-8", "html"
		err = onboardingreport.GenerateOnboardingHTMLToWriter(ctx, h.db, orgID, orgName, &buf, onProgress)
	}

	job.mu.Lock()
	defer job.mu.Unlock()
	if err != nil {
		log.Error().Err(err).Int64("org_id", orgID).Str("format", format).Msg("onboarding report: export generation failed")
		job.status = "error"
		job.errMsg = "Report generation failed. Please try again."
		return
	}

	job.data = buf.Bytes()
	job.filename = fmt.Sprintf("livereview-onboarding-report-%s.%s", downloadSlug(orgName), ext)
	job.contentType = contentType
	job.status = "done"
	job.label = ""
}

// lookupExportJob finds the job for :jobId, scoped to the caller's org so
// one org can't poll or fetch another org's report.
func (h *OnboardingReportHandler) lookupExportJob(c echo.Context, pc *auth.PermissionContext) *onboardingExportJob {
	jobID := c.Param("jobId")
	onboardingExportJobsMu.Lock()
	job, ok := onboardingExportJobs[jobID]
	onboardingExportJobsMu.Unlock()
	if !ok {
		return nil
	}
	job.mu.Lock()
	orgMatches := job.orgID == pc.OrgID
	job.mu.Unlock()
	if !orgMatches {
		return nil
	}
	return job
}

// ExportStatus reports an export job's progress so the frontend can show
// "Rendering chart N of M: <title>" instead of a bare spinner.
func (h *OnboardingReportHandler) ExportStatus(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "authentication required")
	}

	job := h.lookupExportJob(c, pc)
	if job == nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "export job not found")
	}

	job.mu.Lock()
	defer job.mu.Unlock()
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"status":  job.status,
		"current": job.current,
		"total":   job.total,
		"label":   job.label,
		"error":   job.errMsg,
	})
}

// ExportFile streams a finished export job's bytes back as a file
// attachment. Returns 409 if the job hasn't finished yet.
func (h *OnboardingReportHandler) ExportFile(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "authentication required")
	}

	job := h.lookupExportJob(c, pc)
	if job == nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "export job not found")
	}

	job.mu.Lock()
	defer job.mu.Unlock()
	if job.status == "error" {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, job.errMsg)
	}
	if job.status != "done" {
		return JSONErrorWithEnvelope(c, http.StatusConflict, "export not ready yet")
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, job.filename))
	return c.Blob(http.StatusOK, job.contentType, job.data)
}

// orgNameFromContext pulls the display name of the org being reported on
// out of the request's permission context, falling back to a generic label
// if it's somehow unset (defensive only — auth middleware always resolves it).
func orgNameFromContext(pc *auth.PermissionContext) string {
	if pc.CurrentOrg != nil && pc.CurrentOrg.Name != "" {
		return pc.CurrentOrg.Name
	}
	return "your-organization"
}

// downloadSlug turns an org name into a filesystem-safe token for a
// Content-Disposition filename, e.g. "Ostrelle Systems" -> "ostrelle-systems".
func downloadSlug(s string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
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

	// Compute KPI stats (Top3, Bottom3, Avg, Peak, Low, Trend%, etc.)
	if statsJSON, err := chatstats.ComputeAllStats(cr.VegaSpec); err == nil && statsJSON != nil {
		cr.Stats = statsJSON
	}

	return cr
}
