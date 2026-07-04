package slackbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/slack-go/slack"
)

const (
	vlConvertDefault = "vl-convert"
	vlVersion        = "5.21"
	vlThemeDefault   = "powerbi"
)

// VegaLiteReport is the expected JSON wrapper from the LLM.
type VegaLiteReport struct {
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle,omitempty"`
	Spec     json.RawMessage `json:"spec"`
}

// renderVegaLiteReport extracts a Vega-Lite spec from the LLM response,
// runs vl-convert vl2png, and returns the PNG bytes and a friendly title.
func renderVegaLiteReport(ctx context.Context, raw string) ([]byte, string, error) {
	body := extractJSONBlock(raw)

	// Try wrapped format: { "title": "...", "spec": { ...vega-lite... } }
	var wrapped VegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		spec, err := normalizeVegaLiteSpec(wrapped.Spec)
		if err != nil {
			return nil, "", err
		}
		png, err := convertVegaLiteToPNG(ctx, spec)
		return png, friendlyTitle(wrapped.Title, wrapped.Subtitle), err
	}

	// Try raw Vega-Lite spec: { "$schema": "...", "mark": "bar", ... }
	var rawMap map[string]any
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		return nil, "", fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := rawMap["$schema"]; !ok && rawMap["mark"] == nil && rawMap["layer"] == nil && rawMap["vconcat"] == nil && rawMap["hconcat"] == nil {
		return nil, "", fmt.Errorf("not a Vega-Lite specification")
	}

	spec, err := normalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, "", err
	}
	png, err := convertVegaLiteToPNG(ctx, spec)
	return png, "LiveReview Chart", err
}

// normalizeVegaLiteSpec injects consistent styling into a Vega-Lite spec.
// Currently it sets x-axis labelAngle to 45 degrees for better readability.
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

	// Handle layer, vconcat, hconcat, concat, repeat recursively
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					injectAxisAngle(child)
				}
			}
		}
	}

	// Handle repeat's spec
	if child, ok := m["spec"].(map[string]any); ok {
		injectAxisAngle(child)
	}

	encoding, ok := m["encoding"].(map[string]any)
	if !ok {
		return
	}

	for channel, v := range encoding {
		// Only adjust x-axis channels
		if channel != "x" && channel != "xOffset" && channel != "x2" {
			continue
		}
		channelMap, ok := v.(map[string]any)
		if !ok {
			continue
		}

		// Only adjust ordinal/nominal/temporal x fields, or if no type specified
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
		// Only set if not already set, respecting LLM overrides
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

func convertVegaLiteToPNG(ctx context.Context, spec []byte) ([]byte, error) {
	debugDir := os.Getenv("VL_CONVERT_DEBUG_DIR")

	var tmpDir string
	var err error
	if debugDir != "" {
		if err := os.MkdirAll(debugDir, 0755); err == nil {
			tmpDir, _ = os.MkdirTemp(debugDir, "vl-report-*")
		}
	}
	if tmpDir == "" {
		tmpDir, err = os.MkdirTemp("", "vl-report-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
	}
	if debugDir == "" {
		defer os.RemoveAll(tmpDir)
	} else {
		log.Printf("[SlackBot] Vega-Lite debug files kept in: %s", tmpDir)
	}

	inputPath := filepath.Join(tmpDir, "report.vl.json")
	outputPath := filepath.Join(tmpDir, "report.png")

	if err := os.WriteFile(inputPath, spec, 0644); err != nil {
		return nil, fmt.Errorf("write spec: %w", err)
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
		"--scale", "2.0",
		"--theme", theme,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("vl-convert failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	pngData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read png: %w", err)
	}
	return pngData, nil
}

// uploadReportToSlack uploads a PNG image to the Slack channel as a file reply.
func (oh *orgHandler) uploadReportToSlack(channel, threadTS string, pngData []byte, title string) {
	params := slack.UploadFileParameters{
		Channel:         channel,
		Content:         string(pngData),
		Filename:        "report.png",
		Title:           title,
		FileSize:        len(pngData),
		ThreadTimestamp: threadTS,
	}
	if _, err := oh.slackClient.UploadFileContext(context.Background(), params); err != nil {
		if strings.Contains(err.Error(), "missing_scope") {
			log.Printf("[SlackBot] Failed to upload report image: Slack bot token is missing the 'files:write' scope. Add it in your Slack app settings and reinstall the app.")
		} else {
			log.Printf("[SlackBot] Failed to upload report image: %s", err)
		}
	}
}

// parseAndRenderVegaLiteReport tries to parse the LLM output as a Vega-Lite spec
// and render it as a PNG image. Returns (pngData, title, ok).
func parseAndRenderVegaLiteReport(ctx context.Context, text string) ([]byte, string, bool) {
	data, title, err := renderVegaLiteReport(ctx, text)
	if err != nil {
		log.Printf("[SlackBot] Vega-Lite render failed: %s", err)
		return nil, "", false
	}
	return data, title, true
}
