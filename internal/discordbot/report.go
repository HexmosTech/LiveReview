package discordbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	vlConvertDefault = "vl-convert"
	vlVersion        = "5.21"
	vlThemeDefault   = "powerbi"
)

type VegaLiteReport struct {
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

func renderVegaLiteReports(ctx context.Context, raw string) ([]renderedReport, error) {
	body := extractJSONBlock(raw)

	var multi struct {
		Reports []VegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		return renderReports(ctx, multi.Reports)
	}

	var wrapped VegaLiteReport
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

func renderReports(ctx context.Context, reports []VegaLiteReport) ([]renderedReport, error) {
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
