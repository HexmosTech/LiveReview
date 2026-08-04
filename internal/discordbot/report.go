package discordbot

import (
	"context"

	"github.com/livereview/internal/vlrender"
)

// renderedReport mirrors vlrender.Report. It is kept as a distinct type so the
// Discord bot can manage temp-directory cleanup independently of rendering.
type renderedReport vlrender.Report

// renderVegaLiteReports delegates to the shared renderer and cleans up temp
// files after rendering, since Discord uploads charts from in-memory PNGData.
func renderVegaLiteReports(ctx context.Context, raw string) ([]renderedReport, error) {
	reports, err := vlrender.RenderVegaLiteReportsWithRetry(ctx, raw, "1.0", 3)
	if err != nil {
		// Defensive: RenderVegaLiteReports currently returns nil reports on
		// error, but clean up any partial temp dirs so they never leak.
		vlrender.CleanupReports(reports)
		return nil, err
	}
	cleaned := make([]renderedReport, len(reports))
	for i, r := range reports {
		cleaned[i] = renderedReport(r)
	}
	vlrender.CleanupReports(reports)
	return cleaned, nil
}

func hasVegaLiteSpec(text string) bool {
	return vlrender.HasVegaLiteSpec(text)
}

func stripTopLevelVegaJSON(raw string) string {
	return vlrender.StripVegaJSON(raw)
}
