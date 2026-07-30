package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/mcpagent"
	"github.com/rs/zerolog/log"
)

var (
	chartFiles   = map[string]string{}
	chartFilesMu sync.RWMutex
)

func registerChartFile(id, path string) {
	chartFilesMu.Lock()
	chartFiles[id] = path
	chartFilesMu.Unlock()
}

func lookupChartFile(id string) (string, bool) {
	chartFilesMu.RLock()
	p, ok := chartFiles[id]
	chartFilesMu.RUnlock()
	return p, ok
}

type WebChatRequest struct {
	Message string                 `json:"message"`
	History []mcpagent.HistoryEntry `json:"history,omitempty"`
}

type WebChatImage struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type WebChatResponse struct {
	Response string            `json:"response"`
	History  []mcpagent.HistoryEntry `json:"history"`
	Images   []WebChatImage    `json:"images,omitempty"`
}

func (s *Server) HandleWebChat(c echo.Context) error {
	var req WebChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "message is required"})
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	orgID := pc.OrgID
	ctx := c.Request().Context()

	connector, err := s.resolveOrgConnector(ctx, orgID)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}

	mcpURL := os.Getenv("SLACK_MCP_SERVER_URL")
	if mcpURL == "" {
		mcpURL = fmt.Sprintf("http://localhost:%d/api/mcp", s.deploymentConfig.BackendPort)
	}
	maxSteps := 20
	if s := os.Getenv("SLACK_MAX_AGENT_STEPS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxSteps = n
		}
	}

	mcpSession, err := mcpagent.ConnectMCP(ctx, mcpURL, nil)
	if err != nil {
		log.Error().Err(err).Str("url", mcpURL).Msg("WebChat: failed to connect to MCP server")
		return c.JSON(http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Failed to connect: %s", err.Error())})
	}

	provider := mcpagent.NewProvider(connector)
	agent := mcpagent.NewAgent(provider, mcpSession, maxSteps)

	responseText, updatedHistory, err := agent.RunTurn(ctx, req.History, req.Message)
	if err != nil {
		log.Error().Err(err).Msg("WebChat: agent loop failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Agent loop failed: %s", err.Error())})
	}

	resp := WebChatResponse{
		Response: responseText,
		History:  updatedHistory,
	}

	if hasVegaLiteSpec(responseText) {
		images, cleanText := s.renderImagesFromVega(ctx, c, responseText)
		if len(images) > 0 {
			resp.Images = images
		}
		if cleanText != "" {
			resp.Response = cleanText
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func (s *Server) renderImagesFromVega(ctx context.Context, c echo.Context, text string) ([]WebChatImage, string) {
	body := extractJSONBlock(text)

	var multi struct {
		Reports []vegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		return renderReportsToImages(ctx, multi.Reports, s.deploymentConfig.BackendPort)
	}

	var wrapped vegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		images, err := renderSpecToImages(ctx, wrapped, s.deploymentConfig.BackendPort)
		if err != nil {
			return nil, text
		}
		return images, stripVegaBlocks(text)
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		return nil, text
	}
	if _, ok := rawMap["$schema"]; !ok && rawMap["mark"] == nil && rawMap["layer"] == nil && rawMap["vconcat"] == nil && rawMap["hconcat"] == nil {
		return nil, text
	}
	spec, err := normalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, text
	}
	images, err := renderRawSpecToImages(ctx, spec, s.deploymentConfig.BackendPort)
	if err != nil {
		return nil, text
	}
	return images, stripVegaBlocks(text)
}

type vegaLiteReport struct {
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle,omitempty"`
	Description string          `json:"description,omitempty"`
	Spec        json.RawMessage `json:"spec"`
}

func renderReportsToImages(ctx context.Context, reports []vegaLiteReport, port int) ([]WebChatImage, string) {
	var images []WebChatImage
	for _, r := range reports {
		spec, err := normalizeVegaLiteSpec(r.Spec)
		if err != nil {
			continue
		}
		img, err := convertSpecToImage(ctx, spec, port)
		if err != nil {
			continue
		}
		img.Title = friendlyTitle(r.Title, r.Subtitle)
		img.Description = r.Description
		images = append(images, img)
	}
	return images, ""
}

func renderSpecToImages(ctx context.Context, r vegaLiteReport, port int) ([]WebChatImage, error) {
	spec, err := normalizeVegaLiteSpec(r.Spec)
	if err != nil {
		return nil, err
	}
	img, err := convertSpecToImage(ctx, spec, port)
	if err != nil {
		return nil, err
	}
	img.Title = friendlyTitle(r.Title, r.Subtitle)
	img.Description = r.Description
	return []WebChatImage{img}, nil
}

func renderRawSpecToImages(ctx context.Context, spec []byte, port int) ([]WebChatImage, error) {
	img, err := convertSpecToImage(ctx, spec, port)
	if err != nil {
		return nil, err
	}
	img.Title = "LiveReview Chart"
	return []WebChatImage{img}, nil
}

func convertSpecToImage(ctx context.Context, spec []byte, port int) (WebChatImage, error) {
	tmpDir, err := os.MkdirTemp("", "vl-report-*")
	if err != nil {
		return WebChatImage{}, fmt.Errorf("create temp dir: %w", err)
	}

	inputPath := filepath.Join(tmpDir, "report.vl.json")
	outputPath := filepath.Join(tmpDir, "report.png")

	if err := os.WriteFile(inputPath, spec, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return WebChatImage{}, fmt.Errorf("write spec: %w", err)
	}

	binary := os.Getenv("VL_CONVERT_BIN")
	if binary == "" {
		binary = "vl-convert"
	}

	theme := os.Getenv("VL_CONVERT_THEME")
	if theme == "" {
		theme = "powerbi"
	}

	cmd := exec.CommandContext(ctx, binary, "vl2png",
		"-i", inputPath,
		"-o", outputPath,
		"-v", "5.21",
		"--scale", "2.0",
		"--theme", theme,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tmpDir)
		return WebChatImage{}, fmt.Errorf("vl-convert failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	chartID := make([]byte, 8)
	rand.Read(chartID)
	id := hex.EncodeToString(chartID)
	registerChartFile(id, outputPath)
	imgURL := fmt.Sprintf("http://localhost:%d/api/v1/chat/charts/%s", port, id)

	return WebChatImage{URL: imgURL}, nil
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

func hasVegaLiteSpec(text string) bool {
	return strings.Contains(text, `"$schema"`) ||
		(strings.Contains(text, `"mark"`) && strings.Contains(text, `"encoding"`)) ||
		(strings.Contains(text, `"title"`) && strings.Contains(text, `"spec"`)) ||
		strings.Contains(text, `"reports"`)
}

func stripVegaBlocks(raw string) string {
	idx := strings.Index(raw, "\n{")
	if idx < 0 {
		idx = strings.Index(raw, "{")
	}
	if idx < 0 {
		return raw
	}

	depth := 0
	inStr := false
	esc := false
	start := -1

	for i := idx; i < len(raw); i++ {
		ch := raw[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			esc = false
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				if isVegaJSON([]byte(raw[start : i+1])) {
					raw = strings.TrimSpace(raw[:start] + raw[i+1:])
					return stripVegaBlocks(raw)
				}
				start = -1
			}
		}
	}
	return raw
}

func (s *Server) resolveOrgConnector(ctx context.Context, orgID int64) (*aiconnectors.Connector, error) {
	storage := aiconnectors.NewStorage(s.db)
	connectors, err := storage.GetAllConnectors(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI connectors: %w", err)
	}
	if len(connectors) == 0 {
		return nil, fmt.Errorf("no AI connectors configured in this organization")
	}
	for _, record := range connectors {
		options := storage.GetConnectorOptions(ctx, record)
		c, err := aiconnectors.NewConnector(ctx, options)
		if err != nil {
			continue
		}
		return c, nil
	}
	return nil, fmt.Errorf("all AI connectors failed to initialize")
}

func isVegaJSON(data []byte) bool {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	switch v := raw.(type) {
	case map[string]any:
		if _, ok := v["reports"]; ok {
			return true
		}
		if _, ok := v["$schema"]; ok {
			return true
		}
		if _, ok := v["spec"]; ok {
			if _, hasTitle := v["title"]; hasTitle {
				return true
			}
		}
		if mark := v["mark"]; mark != nil {
			_, hasEnc := v["encoding"]
			_, hasTitle := v["title"]
			if hasEnc || hasTitle {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if _, ok := m["$schema"]; ok {
					return true
				}
				if _, ok := m["spec"]; ok {
					return true
				}
			}
		}
	}
	return false
}

func (s *Server) ServeChartPNG(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.NoContent(http.StatusBadRequest)
	}
	path, ok := lookupChartFile(id)
	if !ok {
		return c.NoContent(http.StatusNotFound)
	}
	return c.File(path)
}
