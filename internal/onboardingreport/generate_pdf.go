package onboardingreport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-swiss/fonts"
	_ "github.com/lib/pq"
	"github.com/livereview/internal/vlrender"
	"github.com/phpdave11/gofpdf"
	pdflib "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/util"
)

// chartResult holds the rendered PNG and metadata for one chart.
type chartResult struct {
	Title       string
	Description string
	SQL         string
	PNG         []byte
	ImgHeightMM float64
	Section     string
	Err         error
}

// Brand palette. One source of truth for every color used in the document —
// cover, running header/footer, section rules, chart titles/descriptions,
// and the "no data" callout — so the whole report reads as one designed
// piece instead of a patchwork of ad hoc colors.
var (
	colBrand      = rgba(37, 99, 235)   // blue-600 — primary accent (wordmark, rules, links)
	colInk        = rgba(15, 23, 42)    // slate-900 — headings, chart titles
	colMuted      = rgba(100, 116, 139) // slate-500 — captions, descriptions, footer
	colBorder     = rgba(226, 232, 240) // slate-200 — hairlines
	colWhite      = rgba(255, 255, 255)
	colWarnBg     = rgba(255, 251, 235) // amber-50
	colWarnBorder = rgba(253, 230, 138) // amber-200
	colWarnFg     = rgba(180, 83, 9)    // amber-700
)

func rgba(r, g, b uint8) color.Color { return color.RGBA{R: r, G: g, B: b, A: 255} }

func rgbInts(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func setTextColor(raw *gofpdf.Fpdf, c color.Color) { r, g, b := rgbInts(c); raw.SetTextColor(r, g, b) }
func setDrawColor(raw *gofpdf.Fpdf, c color.Color) { r, g, b := rgbInts(c); raw.SetDrawColor(r, g, b) }
func setFillColor(raw *gofpdf.Fpdf, c color.Color) { r, g, b := rgbInts(c); raw.SetFillColor(r, g, b) }

// mm converts a millimeter length to the points gofpdf's Fpdf actually
// expects (see the mmToPt const doc below), for one-off layout numbers that
// aren't already named constants.
func mm(v float64) float64 { return v * mmToPt }

// Page geometry constants (A4 portrait, all in mm).
const (
	// goldmark-pdf's NewFpdf hardcodes the underlying gofpdf document unit
	// to points (see vendor fpdf.go: gofpdf.New(orientation, "pt", ...)),
	// not millimeters. Every position/dimension constant below is defined
	// as a real-world mm value multiplied by mmToPt so the numbers passed
	// to SetMargins/Line/MultiCell/etc. are correct in the unit gofpdf
	// actually uses. Font sizes and pdflib.Style.Spacing are unaffected —
	// those are always in points regardless of document unit, by both
	// gofpdf and goldmark-pdf convention (see goldmark-pdf style.go).
	mmToPt = 72.0 / 25.4

	pageWidthMM      = 210.0 * mmToPt
	pageHeightMM     = 297.0 * mmToPt
	marginSideMM     = 15.0 * mmToPt
	marginTopContent = 50.0 * mmToPt // room for running header
	marginBottomMM   = 30.0 * mmToPt // room for footer

	// goldmark-pdf renders images at this width (pageWidth - 2*marginSide*2)
	// and at this X offset (2*marginSideMM) — see goldmark-pdf
	// renderer_funcs.go renderImage: maxw = width - mleft*2 - mright*2,
	// x = mleft*2. That insets images further than the normal text margin,
	// so every custom-drawn element around a chart (title, description,
	// "no data" callout, separators) uses the same column so they all line
	// up with the image instead of the wider full-margin text column.
	goldmarkImageWidthMM = pageWidthMM - 4*marginSideMM // 150mm
	chartColX            = marginSideMM * 2             // 30mm
	chartColWidth        = goldmarkImageWidthMM         // 150mm

	// Maximum image render height on page. Reserve ~50mm for title+description+spacing.
	maxImageRenderHeightMM = 140.0 * mmToPt

	// Maximum allowed aspect ratio (height/width) for PNGs so they fit on one page
	// when goldmark-pdf scales them to goldmarkImageWidthMM. A ratio of two
	// lengths is unit-independent, so this value is unaffected by the mm/pt
	// distinction above.
	maxAspectRatio = maxImageRenderHeightMM / goldmarkImageWidthMM // ~0.93

	// Typography/spacing for the per-chart title+description+image block.
	// The two *Size constants are font sizes (points, unconverted); the
	// rest are layout dimensions (mm, converted).
	chartTitleSize      = 12.0
	chartTitleLineH     = 5.0 * mmToPt
	chartDescSize       = 9.0
	chartDescLineH      = 4.2 * mmToPt
	chartTitleGap       = 2.2 * mmToPt
	chartGapBeforeImage = 4.5 * mmToPt
	chartBlockTopPad    = 5.0 * mmToPt
	chartHairlineGap    = 7.0 * mmToPt
	chartWarnBoxH       = 16.0 * mmToPt
)

// renderChartPNG renders a Vega-Lite spec to PNG via vl-convert, and — if
// the result is too tall to fit one PDF page — shrinks the spec's view
// height and re-renders from the vector spec rather than resampling the
// raster.
//
// A previous version of this pipeline rendered once and then downsized an
// overly tall PNG with a manual nearest-neighbor pixel resample. That
// resample had a row-mapping bug (an extra division that made it overshoot
// the source image by several percent, clamping a chunk of bottom rows to a
// single duplicated source row while unevenly compressing the rest) and,
// independent of that bug, nearest-neighbor point-sampling of anti-aliased
// text is lossy in general — solid-color bars survive it because they have
// no fine detail to alias, but every axis label, tick, and title comes out
// as scrambled noise. Re-rendering through vl-convert at the corrected
// height instead produces crisp, correctly anti-aliased text at whatever
// final size is needed, because the renderer draws it fresh rather than
// resampling pixels that were already drawn.
func renderChartPNG(ctx context.Context, spec json.RawMessage, scale string) ([]byte, error) {
	scaleF := 2.0
	if f, err := strconv.ParseFloat(scale, 64); err == nil && f > 0 {
		scaleF = f
	}

	const maxAttempts = 3
	cur := spec
	var pngData []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		data, tmpDir, err := renderVegaSpecOnce(ctx, cur, scale)
		if tmpDir != "" {
			defer os.RemoveAll(tmpDir)
		}
		if err != nil {
			return nil, err
		}
		pngData = data

		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			// Not a decodable PNG; let the caller's validator report it.
			return data, nil
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if w == 0 || float64(h)/float64(w) <= maxAspectRatio {
			return data, nil
		}

		next, ok := shrinkSpecViewHeight(cur, w, h, scaleF, maxAspectRatio)
		if !ok {
			// No adjustable view height (e.g. a concat/facet spec) — accept
			// the image as rendered rather than distorting it.
			return data, nil
		}
		cur = next
	}
	return pngData, nil
}

// renderVegaSpecOnce invokes vl-convert once, with the "powerbi" theme
// disabled so our own config theme's legend/axis colors aren't overridden.
func renderVegaSpecOnce(ctx context.Context, spec json.RawMessage, scale string) ([]byte, string, error) {
	origTheme := os.Getenv("VL_CONVERT_THEME")
	os.Setenv("VL_CONVERT_THEME", "none")
	defer os.Setenv("VL_CONVERT_THEME", origTheme)
	return vlrender.ConvertVegaLiteToPNG(ctx, spec, scale)
}

// shrinkSpecViewHeight returns a copy of spec with config.view.continuousHeight
// reduced so that re-rendering at the same scale should hit targetRatio
// (height/width). actualW/actualH are the pixel dimensions of the most
// recent render. Returns ok=false when the spec has no adjustable view
// height, or when shrinking it further wouldn't help.
func shrinkSpecViewHeight(spec json.RawMessage, actualW, actualH int, scale, targetRatio float64) (json.RawMessage, bool) {
	var m map[string]interface{}
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, false
	}
	config, _ := m["config"].(map[string]interface{})
	if config == nil {
		return nil, false
	}
	view, _ := config["view"].(map[string]interface{})
	if view == nil {
		return nil, false
	}
	curHeight, ok := numberField(view["continuousHeight"])
	if !ok || curHeight <= 0 {
		return nil, false
	}

	// Chrome (axis/legend/title) is font-driven and roughly constant as view
	// height changes, so cutting the overflow directly from the view height
	// (converted from output pixels back to spec units via scale) is a good
	// approximation — and if it undershoots slightly, the next attempt
	// corrects it against the newly measured render.
	targetH := targetRatio * float64(actualW)
	deltaPx := float64(actualH) - targetH
	if deltaPx <= 0 {
		return nil, false
	}
	newHeight := curHeight - deltaPx/scale
	if newHeight < 60 {
		newHeight = 60
	}
	if newHeight >= curHeight {
		return nil, false
	}

	view["continuousHeight"] = newHeight
	out, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(out), true
}

func numberField(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// validateChartPNG checks that a PNG is valid and has reasonable dimensions.
// Returns an error string if something is wrong, empty string if OK.
func validateChartPNG(pngData []byte, title string) string {
	if len(pngData) == 0 {
		return "empty PNG data"
	}
	if len(pngData) < 100 {
		return fmt.Sprintf("PNG too small (%d bytes)", len(pngData))
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return fmt.Sprintf("invalid PNG: %v", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 100 || h < 100 {
		return fmt.Sprintf("PNG too small (%dx%d)", w, h)
	}
	if w > 10000 || h > 10000 {
		return fmt.Sprintf("PNG too large (%dx%d)", w, h)
	}

	// Check aspect ratio - extremely tall or wide charts are suspicious.
	ratio := float64(w) / float64(h)
	if ratio < 0.2 || ratio > 10 {
		return fmt.Sprintf("unusual aspect ratio %.2f (%dx%d)", ratio, w, h)
	}

	return ""
}

// generateChartResults executes every chart template's SQL against db for
// the given org and renders each result to a PNG. It's the shared core of
// both the CLI PDF generator and the HTTP download handlers (PDF and HTML),
// so all three describe the same underlying data. logf receives progress
// lines (nil disables logging, used by the HTTP path where stderr chatter
// per request isn't useful). onProgress, if non-nil, is called once before
// each chart starts (1-indexed current, out of total, with the chart's
// title) — this is what lets the HTTP export endpoints report real
// "chart N of M" progress instead of a bare spinner, since each chart's SQL
// query plus its vl-convert render is the expensive step and can take a
// while across a full catalog.
func generateChartResults(ctx context.Context, db *sql.DB, orgID int64, orgName string, logf func(format string, args ...interface{}), onProgress func(current, total int, label string)) ([]chartResult, []string) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	if onProgress == nil {
		onProgress = func(int, int, string) {}
	}

	cat := Catalog()
	bySection := ChartsBySection()
	logf("Template catalog: %d charts across %d sections\n", cat.TotalCharts, len(cat.Sections))

	var results []chartResult
	var validationErrors []string
	done := 0
	for _, section := range cat.Sections {
		charts := bySection[section.ID]
		logf("\n=== %s (%d charts) ===\n", section.Label, len(charts))

		for i, tmpl := range charts {
			logf("  [%d/%d] %s ... ", i+1, len(charts), tmpl.Title)
			done++
			onProgress(done, cat.TotalCharts, tmpl.Title)

			r := chartResult{
				Title:       tmpl.Title,
				Description: substituteOrgName(tmpl.Description, orgName),
				SQL:         tmpl.SQL,
				Section:     section.Label,
			}

			sqlQuery := tmpl.PrepareSQL(orgID)
			rows, err := db.QueryContext(ctx, sqlQuery)
			if err != nil {
				r.Err = fmt.Errorf("query: %w", err)
				logf("QUERY ERROR: %v\n", err)
				results = append(results, r)
				continue
			}

			columns, _ := rows.Columns()
			var resultRows []map[string]interface{}
			for rows.Next() {
				values := make([]interface{}, len(columns))
				valuePtrs := make([]interface{}, len(columns))
				for j := range values {
					valuePtrs[j] = &values[j]
				}
				if err := rows.Scan(valuePtrs...); err != nil {
					continue
				}
				row := make(map[string]interface{}, len(columns))
				for j, col := range columns {
					if b, ok := values[j].([]byte); ok {
						row[col] = string(b)
					} else {
						row[col] = values[j]
					}
				}
				resultRows = append(resultRows, row)
			}
			rows.Close()

			if len(resultRows) == 0 {
				r.Err = fmt.Errorf("no data")
				logf("NO DATA\n")
				results = append(results, r)
				continue
			}

			logf("%d rows, ", len(resultRows))

			dataJSON, err := json.Marshal(resultRows)
			if err != nil {
				r.Err = fmt.Errorf("marshal: %w", err)
				logf("MARSHAL ERROR: %v\n", err)
				results = append(results, r)
				continue
			}

			vegaSpec := tmpl.PrepareVegaSpec(dataJSON)

			pngData, err := renderChartPNG(ctx, vegaSpec, "2")
			if err != nil {
				r.Err = fmt.Errorf("render: %w", err)
				logf("RENDER ERROR: %v\n", err)
				results = append(results, r)
				continue
			}

			// Validate the final PNG.
			if vErr := validateChartPNG(pngData, tmpl.Title); vErr != "" {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", tmpl.Title, vErr))
				logf("VALIDATION: %s\n", vErr)
			}

			// Record the image's rendered height in mm at goldmark-pdf's
			// fixed display width, so the page-fit logic in buildMarkdown /
			// drawChartBlock knows exactly how much space this chart needs
			// without re-decoding the PNG later.
			if img, decErr := png.Decode(bytes.NewReader(pngData)); decErr == nil {
				b := img.Bounds()
				if b.Dx() > 0 {
					r.ImgHeightMM = goldmarkImageWidthMM * float64(b.Dy()) / float64(b.Dx())
				}
			}

			r.PNG = pngData
			logf("OK (%d bytes PNG)\n", len(pngData))
			results = append(results, r)
		}
	}

	return results, validationErrors
}

// GenerateOnboardingPDF connects to the DB, executes all onboarding report
// templates, renders charts to PNG, and produces a PDF using goldmark-pdf.
func GenerateOnboardingPDF(ctx context.Context, dbURL string, orgID int64, orgName string, outPath string) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	results, validationErrors := generateChartResults(ctx, db, orgID, orgName, func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, format, args...)
	}, nil)

	// Report validation summary.
	if len(validationErrors) > 0 {
		fmt.Fprintf(os.Stderr, "\n=== VALIDATION WARNINGS (%d) ===\n", len(validationErrors))
		for _, v := range validationErrors {
			fmt.Fprintf(os.Stderr, "  ⚠ %s\n", v)
		}
	}

	// Phase 2: Build markdown from results.
	md := buildMarkdown(Catalog(), results, orgName)
	fmt.Fprintf(os.Stderr, "\nMarkdown: %d bytes\n", len(md))

	// Phase 3: Render markdown to PDF using goldmark-pdf.
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	generatedAt := time.Now().Format("2006-01-02 15:04")
	if err := renderMarkdownPDF(ctx, md, orgName, generatedAt, f); err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}

	fmt.Fprintf(os.Stderr, "PDF saved to: %s\n", outPath)
	return nil
}

// GenerateOnboardingPDFToWriter renders the onboarding report PDF for orgID
// straight to w, using an already-open db connection. Used by the HTTP
// download endpoint (internal/api/onboarding_report_handler.go), which has
// a request-scoped org and an existing pool connection rather than a raw
// DATABASE_URL and an output file path like the CLI entry point. onProgress
// (may be nil) is forwarded to generateChartResults — see its doc comment.
func GenerateOnboardingPDFToWriter(ctx context.Context, db *sql.DB, orgID int64, orgName string, w io.Writer, onProgress func(current, total int, label string)) error {
	results, _ := generateChartResults(ctx, db, orgID, orgName, nil, onProgress)
	md := buildMarkdown(Catalog(), results, orgName)
	generatedAt := time.Now().Format("2006-01-02 15:04")
	return renderMarkdownPDF(ctx, md, orgName, generatedAt, w)
}

// chartBlockMeta is the payload embedded in a <!--chart:BASE64--> sentinel.
// The custom node renderer decodes it and draws the chart's title,
// description, and (on error) "no data" callout directly with gofpdf —
// rather than through markdown's bold/italic paragraphs — so it can:
//   - measure exact wrapped line counts and decide, before drawing anything,
//     whether the whole title+description+image block fits in the space
//     left on the current page (forcing a page break before the block, not
//     mid-way through it, when it doesn't)
//   - keep every chart's title/description aligned to the same 150mm column
//     goldmark-pdf places the image in, instead of the wider full-margin
//     text column paragraphs otherwise use
type chartBlockMeta struct {
	Title       string  `json:"t"`
	Description string  `json:"d,omitempty"`
	ImgHeightMM float64 `json:"h,omitempty"`
	Err         string  `json:"e,omitempty"`
	First       bool    `json:"f,omitempty"`
}

const chartMetaPrefix = "<!--chart:"

// templatePlaceholderOrgName is the org the chart templates were extracted
// against (see docs/onboarding-report.md's Template catalog section) —
// baked as a literal string into ~70 description/query_summary fields in
// templates.json rather than a real placeholder token. substituteOrgName
// swaps it for the org actually being reported on, so a report generated
// for another org doesn't describe its charts as being about
// "hexmos-internal".
const templatePlaceholderOrgName = "hexmos-internal"

func substituteOrgName(text, orgName string) string {
	if orgName == "" || orgName == templatePlaceholderOrgName {
		return text
	}
	return strings.ReplaceAll(text, templatePlaceholderOrgName, orgName)
}

// truncateAtWord shortens s to at most limit characters, breaking at the
// last word boundary rather than mid-word, so a cut description reads as
// "...over time" instead of "...over ti...".
func truncateAtWord(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := s[:limit]
	if idx := strings.LastIndex(cut, " "); idx > 0 {
		cut = cut[:idx]
	}
	return strings.TrimRight(cut, ".,;: ") + "..."
}

// friendlyChartError maps an internal chart-generation error to
// customer-facing copy. The underlying errors (a failed SQL query, a
// vl-convert crash) are operational detail that doesn't belong in a report
// an org's admin reads — they're still logged to stderr during generation.
func friendlyChartError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "no data") {
		return "No data available for this chart in the selected period."
	}
	return "This chart could not be generated."
}

// buildMarkdown generates a markdown document from the chart results.
// Uses ASCII-safe characters to avoid encoding issues with gofpdf's
// built-in Helvetica font (which uses Latin-1 encoding).
func buildMarkdown(cat TemplateCatalog, results []chartResult, orgName string) string {
	var b strings.Builder

	// Cover page.
	b.WriteString("# LiveReview Onboarding Report\n\n")
	b.WriteString("<!--cover-rule-->\n\n")
	b.WriteString(fmt.Sprintf("### %s\n\n", orgName))
	b.WriteString(fmt.Sprintf("###### Generated %s - %d charts - %d sections\n\n",
		time.Now().Format("2006-01-02 15:04"), cat.TotalCharts, len(cat.Sections)))

	// Group results by section.
	currentSection := ""
	firstInSection := true
	for _, r := range results {
		if r.Section != currentSection {
			b.WriteString("<!--pagebreak-->\n\n")
			b.WriteString(fmt.Sprintf("## %s\n\n", r.Section))
			b.WriteString("<!--section-rule-->\n\n")
			currentSection = r.Section
			firstInSection = true
		}

		desc := truncateAtWord(r.Description, 200)

		meta := chartBlockMeta{
			Title:       r.Title,
			Description: desc,
			ImgHeightMM: r.ImgHeightMM,
			Err:         friendlyChartError(r.Err),
			First:       firstInSection,
		}
		metaJSON, _ := json.Marshal(meta)
		b.WriteString(fmt.Sprintf("<!--chart:%s-->\n\n", base64.StdEncoding.EncodeToString(metaJSON)))

		if r.Err == nil && len(r.PNG) > 0 {
			dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(r.PNG)
			b.WriteString(fmt.Sprintf("![](%s)\n\n", dataURI))
		}

		firstInSection = false
	}

	return b.String()
}

// onboardingNodeRenderer handles the HTML-comment sentinels buildMarkdown
// embeds: forced page breaks, the cover/section accent rules, and the
// per-chart title+description+"no data" block.
type onboardingNodeRenderer struct{}

func (r *onboardingNodeRenderer) RegisterFuncs(reg pdflib.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
}

func (r *onboardingNodeRenderer) renderHTMLBlock(w *pdflib.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		line := strings.TrimSpace(string(seg.Value(source)))
		switch {
		case strings.Contains(line, "pagebreak"):
			w.Pdf.AddPage()
		case strings.Contains(line, "cover-rule"):
			withRawPdf(w, drawCoverRule)
		case strings.Contains(line, "section-rule"):
			withRawPdf(w, drawSectionRule)
		case strings.HasPrefix(line, chartMetaPrefix):
			withRawPdf(w, func(raw *gofpdf.Fpdf) { renderChartMetaLine(raw, line) })
		}
	}
	return ast.WalkContinue, nil
}

// withRawPdf reaches past goldmark-pdf's restricted PDF interface (which has
// no filled-rectangle or text-wrap-measurement primitives) to the concrete
// *gofpdf.Fpdf underneath, the same way the existing header/footer draw
// functions already do via impl.Fpdf in their HeaderFunc/FooterFunc
// closures.
func withRawPdf(w *pdflib.Writer, fn func(raw *gofpdf.Fpdf)) {
	impl, ok := w.Pdf.(*pdflib.Fpdf)
	if !ok || impl == nil || impl.Fpdf == nil {
		return
	}
	fn(impl.Fpdf)
}

// drawCoverRule draws a short brand-colored accent under the cover title.
func drawCoverRule(raw *gofpdf.Fpdf) {
	y := raw.GetY() + mm(1)
	setDrawColor(raw, colBrand)
	raw.SetLineWidth(mm(1.2))
	raw.Line(marginSideMM, y, marginSideMM+mm(26), y)
	raw.SetY(y + mm(6))
}

// drawSectionRule draws a full-width brand-colored underline below each
// section heading, giving every section the same clear visual start.
func drawSectionRule(raw *gofpdf.Fpdf) {
	y := raw.GetY() + mm(1)
	width, _ := raw.GetPageSize()
	_, _, rm, _ := raw.GetMargins()
	setDrawColor(raw, colBrand)
	raw.SetLineWidth(mm(0.6))
	raw.Line(marginSideMM, y, width-rm, y)
	raw.SetY(y + mm(8))
}

func renderChartMetaLine(raw *gofpdf.Fpdf, line string) {
	payload := strings.TrimSuffix(strings.TrimPrefix(line, chartMetaPrefix), "-->")
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return
	}
	var meta chartBlockMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}
	drawChartBlock(raw, meta)
}

// drawChartBlock draws one chart's title, description, and (on error) "no
// data" callout, deciding first — using gofpdf's own text-wrapping
// (SplitLines) at the exact font/width that will be used, plus the chart's
// already-known image height — whether the whole block fits in the space
// left on the page. If it doesn't, it breaks the page before the title
// rather than letting the title/description land on one page and the image
// spill onto the next (goldmark-pdf's image renderer has no page-break
// awareness of its own — see renderChartPNG's doc comment on why the image
// height is exact). It leaves the cursor exactly where the following
// markdown image node (or nothing, for a "no data" chart) should start.
func drawChartBlock(raw *gofpdf.Fpdf, meta chartBlockMeta) {
	raw.SetFont("Helvetica", "B", chartTitleSize)
	titleLines := len(raw.SplitLines([]byte(meta.Title), chartColWidth))
	if titleLines < 1 {
		titleLines = 1
	}

	descLines := 0
	if meta.Description != "" {
		raw.SetFont("Helvetica", "I", chartDescSize)
		descLines = len(raw.SplitLines([]byte(meta.Description), chartColWidth))
	}

	bodyH := 0.0
	switch {
	case meta.Err != "":
		bodyH = chartWarnBoxH
	case meta.ImgHeightMM > 0:
		bodyH = meta.ImgHeightMM
	}

	textH := float64(titleLines) * chartTitleLineH
	if descLines > 0 {
		textH += chartTitleGap + float64(descLines)*chartDescLineH
	}

	total := chartBlockTopPad + textH + chartGapBeforeImage + bodyH
	if !meta.First {
		total += chartHairlineGap
	}

	_, pageH := raw.GetPageSize()
	remaining := pageH - marginBottomMM - raw.GetY()

	if total > remaining {
		raw.AddPage()
	} else if !meta.First {
		y := raw.GetY() + chartHairlineGap/2
		setDrawColor(raw, colBorder)
		raw.SetLineWidth(mm(0.3))
		raw.Line(chartColX, y, chartColX+chartColWidth, y)
		raw.SetY(y + chartHairlineGap/2)
	}

	raw.SetY(raw.GetY() + chartBlockTopPad)

	raw.SetFont("Helvetica", "B", chartTitleSize)
	setTextColor(raw, colInk)
	raw.SetX(chartColX)
	raw.MultiCell(chartColWidth, chartTitleLineH, meta.Title, "", "L", false)

	if meta.Description != "" {
		raw.SetY(raw.GetY() + chartTitleGap)
		raw.SetFont("Helvetica", "I", chartDescSize)
		setTextColor(raw, colMuted)
		raw.SetX(chartColX)
		raw.MultiCell(chartColWidth, chartDescLineH, meta.Description, "", "L", false)
	}

	raw.SetY(raw.GetY() + chartGapBeforeImage)

	if meta.Err != "" {
		drawWarningBox(raw, meta.Err)
	}
}

// drawWarningBox draws the "no data" / "could not be generated" callout in
// place of a chart image, styled consistently with the rest of the report
// (amber, matching a conventional warning tone) rather than the bare ">"
// blockquote text the report used previously.
func drawWarningBox(raw *gofpdf.Fpdf, message string) {
	y := raw.GetY()
	setFillColor(raw, colWarnBg)
	setDrawColor(raw, colWarnBorder)
	raw.SetLineWidth(mm(0.3))
	raw.Rect(chartColX, y, chartColWidth, chartWarnBoxH, "FD")

	raw.SetXY(chartColX+mm(6), y+mm(5.8))
	raw.SetFont("Helvetica", "", 9)
	setTextColor(raw, colWarnFg)
	raw.MultiCell(chartColWidth-mm(12), mm(4.5), message, "", "L", false)

	raw.SetY(y + chartWarnBoxH)
}

// renderMarkdownPDF renders markdown to PDF using goldmark-pdf.
func renderMarkdownPDF(ctx context.Context, md, orgName, generatedAt string, w io.Writer) error {
	title := "LiveReview Onboarding Report"

	fpdf := pdflib.NewFpdf(ctx, pdflib.FpdfConfig{
		Title:       title,
		Subject:     "Onboarding Report",
		Orientation: "P",
		PaperSize:   "A4",
		HeaderFunc: func(impl pdflib.Fpdf, _ fonts.Cache) func() {
			return func() { drawRunningHeader(impl.Fpdf, orgName) }
		},
		FooterFunc: func(impl pdflib.Fpdf, _ fonts.Cache) func() {
			return func() { drawFooter(impl.Fpdf, orgName, generatedAt) }
		},
	}, nil)

	raw := fpdf.Fpdf
	raw.AliasNbPages("")
	raw.SetMargins(marginSideMM, marginTopContent, marginSideMM)
	raw.SetAutoPageBreak(true, marginBottomMM)
	raw.SetXY(marginSideMM, mm(20))

	renderer := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			pdflib.New(
				pdflib.WithPDF(fpdf),
				pdflib.WithEscapeHTML(false),
				pdflib.OptionFunc(func(c *pdflib.Config) {
					c.Styles = onboardingStyles()
				}),
				pdflib.WithNodeRenderers(
					util.Prioritized(&onboardingNodeRenderer{}, 100),
				),
			),
		),
	)

	return renderer.Convert([]byte(md), w)
}

// onboardingStyles returns the type/color system for the onboarding report.
func onboardingStyles() pdflib.Styles {
	s := pdflib.Styles{}

	sans := pdflib.Font{CanUseForText: true, Family: "Helvetica", Type: pdflib.FontTypeInbuilt}
	mono := pdflib.Font{CanUseForCode: true, Family: "Courier", Type: pdflib.FontTypeInbuilt}

	s.H1 = &pdflib.Style{Font: sans, Size: 24, Spacing: 4, TextColor: colInk, FillColor: colWhite}
	s.H2 = &pdflib.Style{Font: sans, Size: 15, Spacing: 3, TextColor: colInk, FillColor: colWhite}
	s.H3 = &pdflib.Style{Font: sans, Size: 14, Spacing: 6, TextColor: colBrand, FillColor: colWhite}
	s.H4 = &pdflib.Style{Font: sans, Size: 11, Spacing: 5, TextColor: colInk, FillColor: colWhite}
	s.H5 = &pdflib.Style{Font: sans, Size: 10, Spacing: 4, TextColor: colInk, FillColor: colWhite}
	s.H6 = &pdflib.Style{Font: sans, Size: 9, Spacing: 8, TextColor: colMuted, FillColor: colWhite}

	s.Normal = &pdflib.Style{Font: sans, Size: 9, Spacing: 4, TextColor: colInk, FillColor: colWhite}
	s.Blockquote = &pdflib.Style{Font: sans, Size: 9, Spacing: 4, TextColor: colMuted, FillColor: colWhite}

	s.THeader = &pdflib.Style{Font: sans, Size: 8, Spacing: 3, TextColor: colMuted, FillColor: colWhite}
	s.TBody = &pdflib.Style{Font: sans, Size: 8, Spacing: 3, TextColor: colInk, FillColor: colWhite}

	s.CodeFont = mono
	s.LinkColor = colBrand

	return s
}

// drawRunningHeader draws a compact banner on every page after the cover.
func drawRunningHeader(raw *gofpdf.Fpdf, orgName string) {
	if raw.PageNo() <= 1 {
		return
	}

	top := mm(15)
	raw.SetFont("Helvetica", "B", 9)
	setTextColor(raw, colBrand)
	raw.SetXY(marginSideMM, top)
	raw.CellFormat(mm(60), mm(8), "LiveReview", "", 0, "L", false, 0, "")

	raw.SetFont("Helvetica", "", 8)
	setTextColor(raw, colMuted)
	width, _ := raw.GetPageSize()
	_, _, rm, _ := raw.GetMargins()
	raw.SetXY(marginSideMM, top)
	// ASCII-safe dash: gofpdf's built-in Helvetica expects Latin-1, and a
	// Unicode em dash written as a Go string literal comes across as raw
	// UTF-8 bytes, each of which then renders as its own mojibake glyph.
	raw.CellFormat(width-rm-marginSideMM, mm(8), orgName+" - Onboarding Report", "", 0, "R", false, 0, "")

	ruleY := top + mm(10)
	raw.SetLineWidth(mm(0.5))
	setDrawColor(raw, colBorder)
	raw.Line(marginSideMM, ruleY, width-rm, ruleY)

	raw.SetXY(marginSideMM, marginTopContent)
}

// drawFooter draws a page number centered, with the org name and generated
// date as small print on either side — a conventional report footer trio,
// replacing the previous page-number-only footer. Omitted on the cover
// page, matching the running header.
func drawFooter(raw *gofpdf.Fpdf, orgName, generatedAt string) {
	if raw.PageNo() <= 1 {
		return
	}

	width, height := raw.GetPageSize()
	_, _, rm, _ := raw.GetMargins()
	y := height - mm(20)

	raw.SetLineWidth(mm(0.5))
	setDrawColor(raw, colBorder)
	raw.Line(marginSideMM, y, width-rm, y)

	thirdW := (width - rm - marginSideMM) / 3

	raw.SetFont("Helvetica", "", 7)
	setTextColor(raw, colMuted)
	raw.SetXY(marginSideMM, y+mm(4))
	raw.CellFormat(thirdW, mm(6), orgName, "", 0, "L", false, 0, "")

	raw.SetXY(marginSideMM+thirdW, y+mm(4))
	raw.CellFormat(thirdW, mm(6), fmt.Sprintf("Page %d of {nb}", raw.PageNo()), "", 0, "C", false, 0, "")

	raw.SetXY(marginSideMM+2*thirdW, y+mm(4))
	raw.CellFormat(thirdW, mm(6), generatedAt, "", 0, "R", false, 0, "")
}

// RunGeneratePDF is the main entry point for the script. It generates the
// onboarding report for one org (default: hexmos-internal, matching prior
// behavior) and writes it to an output path derived from the org name
// unless overridden.
//
//	go run cmd/onboarding-pdf/main.go
//	go run cmd/onboarding-pdf/main.go --org "Ostrelle Systems"
//	go run cmd/onboarding-pdf/main.go --org "Ostrelle Systems" --out /tmp/report.pdf
func RunGeneratePDF() {
	orgFlag := flag.String("org", "hexmos-internal", "organization name to generate the report for")
	outFlag := flag.String("out", "", "output PDF path (default: derived from the org name under ~/Downloads)")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintf(os.Stderr, "Error: DATABASE_URL not set\n")
		os.Exit(1)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: connect db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	var orgID int64
	var canonicalName string
	err = db.QueryRow(`SELECT id, name FROM orgs WHERE lower(name) = lower($1) LIMIT 1`, *orgFlag).Scan(&orgID, &canonicalName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: find org %q: %v\n", *orgFlag, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Found org_id=%d for %s\n", orgID, canonicalName)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	outPath := *outFlag
	if outPath == "" {
		if strings.EqualFold(canonicalName, "hexmos-internal") {
			outPath = os.ExpandEnv("$HOME/Downloads/livereview-onboarding-report.pdf")
		} else {
			outPath = os.ExpandEnv("$HOME/Downloads/livereview-onboarding-report-" + slugify(canonicalName) + ".pdf")
		}
	}

	if err := GenerateOnboardingPDF(ctx, dbURL, orgID, canonicalName, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// slugify turns an org name into a filesystem-safe lowercase-hyphenated
// token, e.g. "Ostrelle Systems" -> "ostrelle-systems".
func slugify(s string) string {
	var b strings.Builder
	lastDash := true // suppress a leading dash
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
