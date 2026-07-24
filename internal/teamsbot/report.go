package teamsbot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	vlConvertDefault = "vl-convert"
	vlVersion        = "5.21"
	vlThemeDefault   = "powerbi"
)

type vegaReport struct {
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle,omitempty"`
	Description string          `json:"description,omitempty"`
	Spec        json.RawMessage `json:"spec"`
}

type renderedReport struct {
	PNGData     []byte
	Title       string
	Description string
	PNGPath     string
}

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

func renderVegaLiteReports(ctx context.Context, raw string) ([]renderedReport, error) {
	body := extractJSONBlock(raw)

	var multi struct {
		Reports []vegaReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		return renderReports(ctx, multi.Reports)
	}

	var wrapped vegaReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		spec, err := normalizeVegaLiteSpec(wrapped.Spec)
		if err != nil {
			return nil, err
		}
		png, pngPath, err := convertVegaLiteToPNG(ctx, spec)
		if err != nil {
			return nil, err
		}
		return []renderedReport{{
			PNGData:     png,
			PNGPath:     pngPath,
			Title:       friendlyTitle(wrapped.Title, wrapped.Subtitle),
			Description: wrapped.Description,
		}}, nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := rawMap["$schema"]; !ok && rawMap["mark"] == nil && rawMap["layer"] == nil && rawMap["vconcat"] == nil && rawMap["hconcat"] == nil {
		return nil, fmt.Errorf("not a Vega-Lite specification")
	}
	spec, err := normalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, err
	}
	png, pngPath, err := convertVegaLiteToPNG(ctx, spec)
	if err != nil {
		return nil, err
	}
	return []renderedReport{{PNGData: png, PNGPath: pngPath, Title: "LiveReview Chart"}}, nil
}

func renderReports(ctx context.Context, reports []vegaReport) ([]renderedReport, error) {
	var out []renderedReport
	for _, r := range reports {
		spec, err := normalizeVegaLiteSpec(r.Spec)
		if err != nil {
			continue
		}
		png, pngPath, err := convertVegaLiteToPNG(ctx, spec)
		if err != nil {
			continue
		}
		out = append(out, renderedReport{
			PNGData:     png,
			PNGPath:     pngPath,
			Title:       friendlyTitle(r.Title, r.Subtitle),
			Description: r.Description,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no reports could be rendered")
	}
	return out, nil
}

func normalizeVegaLiteSpec(spec []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, err
	}
	injectAxisAngle(m)
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func injectAxisAngle(m map[string]any) {
	if m == nil {
		return
	}
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					injectAxisAngle(child)
				}
			}
		}
	}
	if child, ok := m["spec"].(map[string]any); ok {
		injectAxisAngle(child)
	}
	encoding, ok := m["encoding"].(map[string]any)
	if !ok {
		return
	}
	for channel, v := range encoding {
		if channel != "x" && channel != "xOffset" && channel != "x2" {
			continue
		}
		channelMap, ok := v.(map[string]any)
		if !ok {
			continue
		}
		t := ""
		if typ, ok := channelMap["type"].(string); ok {
			t = typ
		}
		if t == "quantitative" {
			continue
		}
		axis, ok := channelMap["axis"].(map[string]any)
		if !ok {
			axis = map[string]any{}
			channelMap["axis"] = axis
		}
		if _, exists := axis["labelAngle"]; !exists {
			axis["labelAngle"] = float64(45)
		}
	}
}

func friendlyTitle(title, subtitle string) string {
	title = strings.TrimSpace(title)
	subtitle = strings.TrimSpace(subtitle)
	if title == "" {
		return "LiveReview Chart"
	}
	if subtitle != "" {
		return title + " — " + subtitle
	}
	return title
}

func extractJSONBlock(raw string) string {
	s := strings.TrimSpace(raw)
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(s[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + len("```")
		end := strings.Index(s[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	return s
}

func convertVegaLiteToPNG(ctx context.Context, spec []byte) ([]byte, string, error) {
	tmpDir, err := os.MkdirTemp("", "vl-report-*")
	if err != nil {
		return nil, "", fmt.Errorf("create temp dir: %w", err)
	}

	inputPath := filepath.Join(tmpDir, "report.vl.json")
	outputPath := filepath.Join(tmpDir, "report.png")

	if err := os.WriteFile(inputPath, spec, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("write spec: %w", err)
	}

	binary := os.Getenv("VL_CONVERT_BIN")
	if binary == "" {
		binary = vlConvertDefault
	}

	theme := os.Getenv("VL_CONVERT_THEME")
	if theme == "" {
		theme = vlThemeDefault
	}

	cmd := exec.CommandContext(ctx, binary, "vl2png",
		"-i", inputPath,
		"-o", outputPath,
		"-v", vlVersion,
		"--scale", "1.0",
		"--theme", theme,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("vl-convert failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	pngData, err := os.ReadFile(outputPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("read png: %w", err)
	}

	return pngData, tmpDir, nil
}

func hasVegaLiteSpec(text string) bool {
	return strings.Contains(text, `"$schema"`) ||
		(strings.Contains(text, `"mark"`) && strings.Contains(text, `"encoding"`)) ||
		(strings.Contains(text, `"title"`) && strings.Contains(text, `"spec"`)) ||
		strings.Contains(text, `"reports"`)
}

func buildAttachmentsFromVegaLite(ctx context.Context, baseURL string, text string) ([]Attachment, string) {
	reports, err := renderVegaLiteReports(ctx, text)
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
					"type":    "TextBlock",
					"text":    r.Title,
					"weight":  "bolder",
					"size":    "medium",
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
