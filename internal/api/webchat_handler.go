package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/mcpagent"
	"github.com/livereview/internal/vlrender"
	"github.com/rs/zerolog/log"
)

var (
	chartFiles   = map[string]*chartFileEntry{}
	chartFilesMu sync.RWMutex
	lastCleanup  = time.Now()
)

const (
	chartTTL        = 10 * time.Minute
	cleanupInterval = 1 * time.Minute
)

type chartFileEntry struct {
	PNGPath   string
	TmpDir    string
	CreatedAt time.Time
}

func registerChartFile(id, tmpDir, pngPath string) {
	chartFilesMu.Lock()
	// Replace any prior entry for this id (id collision) so its temp dir is cleaned up.
	if old, ok := chartFiles[id]; ok {
		os.RemoveAll(old.TmpDir)
	}
	chartFiles[id] = &chartFileEntry{PNGPath: pngPath, TmpDir: tmpDir, CreatedAt: time.Now()}
	chartFilesMu.Unlock()
	cleanupExpiredCharts()
}

func lookupChartFile(id string) (string, bool) {
	cleanupExpiredCharts()
	chartFilesMu.RLock()
	defer chartFilesMu.RUnlock()
	e, ok := chartFiles[id]
	if !ok {
		return "", false
	}
	return e.PNGPath, true
}

func cleanupExpiredCharts() {
	chartFilesMu.Lock()
	defer chartFilesMu.Unlock()
	// Throttle the full-map scan: it runs on every register/lookup, so only
	// re-scan at most once per cleanupInterval to avoid an O(n) walk under
	// high-frequency image serving. Expired entries are still removed within
	// TTL + cleanupInterval.
	if time.Since(lastCleanup) < cleanupInterval {
		return
	}
	lastCleanup = time.Now()
	now := time.Now()
	for id, e := range chartFiles {
		if now.Sub(e.CreatedAt) > chartTTL {
			os.RemoveAll(e.TmpDir)
			delete(chartFiles, id)
		}
	}
}

type WebChatRequest struct {
	Message string                  `json:"message"`
	History []mcpagent.HistoryEntry `json:"history,omitempty"`
}

type WebChatImage struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Query       string `json:"query,omitempty"`
}

type WebChatResponse struct {
	Response string                  `json:"response"`
	History  []mcpagent.HistoryEntry `json:"history"`
	Images   []WebChatImage          `json:"images,omitempty"`
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

	authHeader := c.Request().Header.Get("Authorization")
	mcpHeaders := map[string]string{}
	if authHeader != "" {
		mcpHeaders["Authorization"] = authHeader
	}
	if orgCtx := c.Request().Header.Get("X-Org-Context"); orgCtx != "" {
		mcpHeaders["X-Org-Context"] = orgCtx
	}

	mcpSession, err := mcpagent.ConnectMCP(ctx, mcpURL, mcpHeaders)
	if err != nil {
		log.Error().Err(err).Str("url", mcpURL).Msg("WebChat: failed to connect to MCP server")
		return c.JSON(http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Failed to connect: %s", err.Error())})
	}

	if pc.CurrentOrg != nil && pc.CurrentOrg.Name != "" {
		mcpSession.OrgName = pc.CurrentOrg.Name
	}
	if pc.User != nil {
		mcpSession.UserName = pc.User.FullName()
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

	if vlrender.HasVegaLiteSpec(responseText) {
		images, cleanText := s.renderImagesFromVega(ctx, responseText)
		if len(images) > 0 {
			resp.Images = images
			// The Vega-Lite JSON must never surface in the chat. If stripping
			// removes everything, keep only the surrounding text (or empty).
			resp.Response = cleanText
		} else {
			// Rendering failed even after retries: never show the raw JSON.
			resp.Response = "Having an issue generating the data, please try again."
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func (s *Server) renderImagesFromVega(ctx context.Context, text string) ([]WebChatImage, string) {
	body := vlrender.ExtractJSONBlock(text)

	var multi struct {
		Reports []vlrender.VegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		return renderReportsToImages(ctx, multi.Reports)
	}

	var wrapped vlrender.VegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		images, err := renderSpecToImages(ctx, wrapped)
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
	spec, err := vlrender.NormalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, text
	}
	images, err := renderRawSpecToImages(ctx, spec)
	if err != nil {
		return nil, text
	}
	return images, stripVegaBlocks(text)
}

func renderReportsToImages(ctx context.Context, reports []vlrender.VegaLiteReport) ([]WebChatImage, string) {
	var images []WebChatImage
	for _, r := range reports {
		spec, err := vlrender.NormalizeVegaLiteSpec(r.Spec)
		if err != nil {
			continue
		}
		img, err := convertSpecToImage(ctx, spec)
		if err != nil {
			continue
		}
		img.Title = vlrender.FriendlyTitle(r.Title, r.Subtitle)
		img.Description = r.Description
		img.Query = r.Query
		images = append(images, img)
	}
	return images, ""
}

func renderSpecToImages(ctx context.Context, r vlrender.VegaLiteReport) ([]WebChatImage, error) {
	spec, err := vlrender.NormalizeVegaLiteSpec(r.Spec)
	if err != nil {
		return nil, err
	}
	img, err := convertSpecToImage(ctx, spec)
	if err != nil {
		return nil, err
	}
	img.Title = vlrender.FriendlyTitle(r.Title, r.Subtitle)
	img.Description = r.Description
	img.Query = r.Query
	return []WebChatImage{img}, nil
}

func renderRawSpecToImages(ctx context.Context, spec []byte) ([]WebChatImage, error) {
	img, err := convertSpecToImage(ctx, spec)
	if err != nil {
		return nil, err
	}
	img.Title = "LiveReview Chart"
	return []WebChatImage{img}, nil
}

func convertSpecToImage(ctx context.Context, spec []byte) (WebChatImage, error) {
	_, tmpDir, err := vlrender.ConvertVegaLiteToPNG(ctx, spec, "2.0")
	if err != nil {
		return WebChatImage{}, err
	}
	outputPath := filepath.Join(tmpDir, "report.png")

	chartID := make([]byte, 8)
	rand.Read(chartID)
	id := hex.EncodeToString(chartID)
	registerChartFile(id, tmpDir, outputPath)
	imgURL := fmt.Sprintf("/api/v1/chat/charts/%s", id)

	return WebChatImage{URL: imgURL}, nil
}

// stripVegaBlocks recursively removes leading Vega-Lite JSON objects and any
// leftover code fences from the text, returning the remaining prose.
func stripVegaBlocks(raw string) string {
	idx := strings.Index(raw, "\n{")
	if idx < 0 {
		idx = strings.Index(raw, "{")
	}
	if idx < 0 {
		return strings.TrimSpace(removeEmptyCodeFences(raw))
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
				if vlrender.IsVegaJSON([]byte(raw[start : i+1])) {
					raw = strings.TrimSpace(raw[:start] + raw[i+1:])
					return stripVegaBlocks(raw)
				}
				start = -1
			}
		}
	}
	return strings.TrimSpace(removeEmptyCodeFences(raw))
}

// removeEmptyCodeFences removes any leftover ```json / ``` fence markers that
// may remain after a Vega-Lite JSON block was stripped.
func removeEmptyCodeFences(raw string) string {
	if !strings.Contains(raw, "```") {
		return raw
	}
	var lines []string
	for _, ln := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(ln)
		if t == "```" || t == "```json" || t == "```python" || t == "```json ```" {
			continue
		}
		lines = append(lines, ln)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
