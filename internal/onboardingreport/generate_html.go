package onboardingreport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"time"
)

// GenerateOnboardingHTMLToWriter renders the onboarding report as a single
// self-contained HTML document — charts embedded as base64 PNGs, no
// external assets — so it can be viewed in a browser or archived outside a
// PDF viewer. It shares the chart-execution pipeline (generateChartResults)
// and color palette with the PDF export (generate_pdf.go) so both describe
// the same data with a consistent look.
func GenerateOnboardingHTMLToWriter(ctx context.Context, db *sql.DB, orgID int64, orgName string, w io.Writer, onProgress func(current, total int, label string)) error {
	cat := Catalog()
	results, _ := generateChartResults(ctx, db, orgID, orgName, nil, onProgress)
	generatedAt := time.Now().Format("2006-01-02 15:04")

	var b bytes.Buffer
	fmt.Fprintf(&b, `<!doctype html>
<html><head><meta charset="utf-8">
<title>%s - Onboarding Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; background:#f8fafc; color:#0f172a; margin:0; }
  .wrap { max-width: 880px; margin: 0 auto; padding: 32px 24px 80px; }
  .cover { padding: 24px 0; border-bottom: 3px solid #2563eb; margin-bottom: 32px; }
  .cover h1 { font-size: 28px; margin: 0 0 8px; }
  .cover .org { font-size: 16px; color:#334155; margin: 0 0 4px; }
  .cover .meta { font-size: 12px; color:#64748b; }
  h2.section { font-size: 20px; border-bottom: 2px solid #2563eb; padding-bottom: 8px; margin: 40px 0 20px; color:#0f172a; }
  .chart { background:#fff; border:1px solid #e2e8f0; border-radius:8px; padding:20px; margin-bottom:20px; }
  .chart h3 { margin: 0 0 6px; font-size: 14px; color:#0f172a; }
  .chart p.desc { margin: 0 0 14px; font-size: 12px; color:#64748b; font-style: italic; }
  .chart img { max-width: 100%%; display:block; border-radius:4px; }
  .warn { background:#fffbeb; border:1px solid #fde68a; color:#b45309; border-radius:6px; padding:12px 14px; font-size:12px; }
  footer { color:#94a3b8; font-size:11px; text-align:center; margin-top:40px; }
</style>
</head><body><div class="wrap">
<div class="cover">
  <h1>LiveReview Onboarding Report</h1>
  <p class="org">%s</p>
  <p class="meta">Generated %s &middot; %d charts &middot; %d sections</p>
</div>
`, html.EscapeString(orgName), html.EscapeString(orgName), html.EscapeString(generatedAt), cat.TotalCharts, len(cat.Sections))

	currentSection := ""
	for _, r := range results {
		if r.Section != currentSection {
			fmt.Fprintf(&b, "<h2 class=\"section\">%s</h2>\n", html.EscapeString(r.Section))
			currentSection = r.Section
		}

		desc := truncateAtWord(r.Description, 200)
		fmt.Fprintf(&b, "<div class=\"chart\">\n<h3>%s</h3>\n", html.EscapeString(r.Title))
		if desc != "" {
			fmt.Fprintf(&b, "<p class=\"desc\">%s</p>\n", html.EscapeString(desc))
		}

		if msg := friendlyChartError(r.Err); msg != "" {
			fmt.Fprintf(&b, "<div class=\"warn\">%s</div>\n", html.EscapeString(msg))
		} else if len(r.PNG) > 0 {
			fmt.Fprintf(&b, "<img src=\"data:image/png;base64,%s\" alt=\"%s\">\n",
				base64.StdEncoding.EncodeToString(r.PNG), html.EscapeString(r.Title))
		}
		b.WriteString("</div>\n")
	}

	fmt.Fprintf(&b, "<footer>LiveReview &middot; %s</footer>\n</div></body></html>\n", html.EscapeString(generatedAt))

	_, err := w.Write(b.Bytes())
	return err
}
