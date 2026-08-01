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

func buildAttachmentsFromVegaLite(ctx context.Context, baseURL string, text string) ([]Attachment, string) {
	reports, err := vlrender.RenderVegaLiteReports(ctx, text)
	if err != nil {
		log.Printf("[TeamsBot] Vega-Lite render failed: %s", err)
		return nil, text
	}

	var descriptions []string
	var attachments []Attachment

	for _, r := range reports {
		chartID := make([]byte, 8)
		rand.Read(chartID)
		id := hex.EncodeToString(chartID)
		pngPath := filepath.Join(r.PNGPath, "report.png")
		RegisterChartFile(id, pngPath)
		imgURL := fmt.Sprintf("%s/charts/%s", strings.TrimRight(baseURL, "/"), id)

		if r.Description != "" {
			descriptions = append(descriptions, r.Description)
		}

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

		attachments = append(attachments, Attachment{
			ContentType: "application/vnd.microsoft.card.adaptive",
			Content:     card,
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

	if len(descriptions) > 0 {
		if cleanText != "" {
			cleanText += "\n\n" + strings.Join(descriptions, "\n\n")
		} else {
			cleanText = strings.Join(descriptions, "\n\n")
		}
	}

	if cleanText == "" {
		cleanText = "Here are the results:"
	}

	return attachments, cleanText
}
