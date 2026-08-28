package teamsbot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/livereview/internal/chatstats"
	"github.com/livereview/internal/vlrender"
)

// renderedReport mirrors vlrender.Report. Teams retains PNGPath for serving the
// chart via the charts HTTP endpoint, so no temp-dir cleanup happens here.
type renderedReport vlrender.Report

var (
	chartFiles   = map[string]string{}
	chartFilesMu sync.RWMutex
)

func RegisterChartFile(id, path string) {
	chartFilesMu.Lock()
	chartFiles[id] = path
	chartFilesMu.Unlock()
}

func LookupChartFile(id string) (string, bool) {
	chartFilesMu.RLock()
	p, ok := chartFiles[id]
	chartFilesMu.RUnlock()
	return p, ok
}

// chartReply pairs one chart's image card with its own description/metadata
// text, so the bot can send them as image-then-text, per chart, matching
// Discord's layout instead of one combined text block followed by all images.
type chartReply struct {
	Attachment Attachment
	Text       string
}

func buildChartRepliesFromVegaLite(ctx context.Context, baseURL string, text string) ([]chartReply, string) {
	reports, err := vlrender.RenderVegaLiteReportsWithRetry(ctx, text, "1.0", 3)
	if err != nil {
		log.Printf("[TeamsBot] Vega-Lite render failed: %s", err)
		return nil, text
	}

	var replies []chartReply
	for _, r := range reports {
		chartID := make([]byte, 8)
		rand.Read(chartID)
		id := hex.EncodeToString(chartID)
		pngPath := filepath.Join(r.PNGPath, "report.png")
		RegisterChartFile(id, pngPath)
		imgURL := fmt.Sprintf("%s/charts/%s", strings.TrimRight(baseURL, "/"), id)

		card := map[string]any{
			"type":    "AdaptiveCard",
			"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
			"version": "1.2",
			"body": []map[string]any{
				{
					"type":   "TextBlock",
					"text":   r.Title,
					"weight": "bolder",
					"size":   "medium",
				},
				{
					"type":    "Image",
					"url":     imgURL,
					"altText": r.Title,
				},
			},
		}

		var lines []string
		if r.Description != "" {
			lines = append(lines, r.Description)
		}
		for _, chip := range chatstats.FormatChips(r.Stats) {
			lines = append(lines, fmt.Sprintf("**%s:** %s", chip.Label, chip.Value))
		}
		if r.Query != "" {
			lines = append(lines, "_Query: "+r.Query+"_")
		}
		if r.TimeRange != "" {
			lines = append(lines, "_Time range: "+r.TimeRange+"_")
		}
		if r.Granularity != "" {
			lines = append(lines, "_Granularity: "+r.Granularity+"_")
		}
		if formattedCtx := r.Context.Format(); formattedCtx != "" {
			lines = append(lines, "_Context: "+formattedCtx+"_")
		}

		replies = append(replies, chartReply{
			Attachment: Attachment{ContentType: "application/vnd.microsoft.card.adaptive", Content: card},
			Text:       strings.Join(lines, "\n\n"),
		})
	}

	cleanText := text
	for {
		start := strings.Index(cleanText, "```json")
		if start < 0 {
			break
		}
		end := strings.Index(cleanText[start+len("```json"):], "```")
		if end < 0 {
			break
		}
		cleanText = cleanText[:start] + cleanText[start+end+len("```json")+3:]
	}
	cleanText = vlrender.StripVegaJSON(cleanText)
	cleanText = strings.TrimSpace(cleanText)

	return replies, cleanText
}
