package slackbot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"

	"github.com/livereview/internal/vlrender"
)

// renderedReport holds a single rendered chart with its metadata. Slack uploads
// charts from in-memory PNGData, so no temp path is retained here.
type renderedReport struct {
	PNGData     []byte
	Title       string
	Description string
	Query       string
}

// renderVegaLiteReports parses the LLM response and renders 1+ charts at 2x
// scale, cleaning up temp files afterwards (unless VL_CONVERT_DEBUG_DIR is set).
func renderVegaLiteReports(ctx context.Context, raw string) ([]renderedReport, error) {
	reports, err := vlrender.RenderVegaLiteReportsWithRetry(ctx, raw, "2.0", 3)
	if err != nil {
		return nil, err
	}
	out := make([]renderedReport, len(reports))
	for i, r := range reports {
		out[i] = renderedReport{
			PNGData:     r.PNGData,
			Title:       r.Title,
			Description: r.Description,
			Query:       r.Query,
		}
		if os.Getenv("VL_CONVERT_DEBUG_DIR") != "" && r.PNGPath != "" {
			log.Printf("[SlackBot] Vega-Lite debug files kept in: %s", r.PNGPath)
		} else if r.PNGPath != "" {
			os.RemoveAll(r.PNGPath)
		}
	}
	return out, nil
}

// uploadReportsToSlack uploads one or more PNG images to the Slack channel.
// Each report's description is sent as the initial comment alongside the image.
func (oh *orgHandler) uploadReportsToSlack(channel, threadTS string, reports []renderedReport) {
	for i, r := range reports {
		filename := "report.png"
		if len(reports) > 1 {
			filename = fmt.Sprintf("report_%d.png", i+1)
		}
		initialComment := r.Description
		if r.Query != "" {
			if initialComment != "" {
				initialComment += "\n\n"
			}
			initialComment += "Query used: " + r.Query
		}
		params := slack.UploadFileParameters{
			Channel:         channel,
			Content:         string(r.PNGData),
			Filename:        filename,
			Title:           r.Title,
			FileSize:        len(r.PNGData),
			InitialComment:  initialComment,
			ThreadTimestamp: threadTS,
		}
		if _, err := oh.slackClient.UploadFileContext(context.Background(), params); err != nil {
			if strings.Contains(err.Error(), "missing_scope") {
				log.Printf("[SlackBot] Failed to upload report image: Slack bot token is missing the 'files:write' scope.")
			} else {
				log.Printf("[SlackBot] Failed to upload report image: %s", err)
			}
		}
	}
}

// parseAndRenderVegaLiteReports tries to parse the LLM output as one or more
// Vega-Lite specs and render each as a PNG image.
func parseAndRenderVegaLiteReports(ctx context.Context, text string) ([]renderedReport, bool) {
	reports, err := renderVegaLiteReports(ctx, text)
	if err != nil {
		log.Printf("[SlackBot] Vega-Lite render failed: %s", err)
		return nil, false
	}
	return reports, true
}
