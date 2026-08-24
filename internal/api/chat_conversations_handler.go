package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/vlrender"
	storagechat "github.com/livereview/storage/chat"
	"github.com/rs/zerolog/log"
)

// ChatConversationSummary is one row in the conversation list/search sidebar.
type ChatConversationSummary struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	Snippet   string `json:"snippet,omitempty"`
}

// ChatMessageOut is one persisted message, with any charts and file exports
// it produced.
type ChatMessageOut struct {
	ID             int64           `json:"id"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	Charts         []WebChatChart  `json:"charts,omitempty"`
	Files          []WebChatFile   `json:"files,omitempty"`
	DebugArtifacts json.RawMessage `json:"debug_artifacts,omitempty"`
}

// ChatConversationDetail is a full conversation with its message history, for
// loading a thread from the sidebar.
type ChatConversationDetail struct {
	ID        int64            `json:"id"`
	Title     string           `json:"title"`
	UpdatedAt string           `json:"updatedAt"`
	Messages  []ChatMessageOut `json:"messages"`
}

func parseChatIDParam(c echo.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

type createConversationRequest struct {
	Message string `json:"message"`
	Surface string `json:"surface"`
}

// normalizeSurface validates the client-supplied surface value, defaulting
// to SurfaceChat for anything empty or unrecognized so a stale/tampered
// client can't wedge conversations onto an invalid partition.
func normalizeSurface(surface string) string {
	if surface == storagechat.SurfaceChatDebug {
		return storagechat.SurfaceChatDebug
	}
	return storagechat.SurfaceChat
}

// CreateConversation creates an empty conversation up front (titled from the
// first message, no agent turn run yet) so the sidebar can show it the
// moment the user hits send, rather than only once the first answer comes
// back. The caller then sends the actual message with this id via
// POST /api/v1/chat/send.
func (s *Server) CreateConversation(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	var req createConversationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	chatStore := storagechat.NewStore(s.db)
	id, err := chatStore.CreateConversation(c.Request().Context(), pc.OrgID, pc.User.ID, titleFromFirstMessage(req.Message), "", normalizeSurface(req.Surface))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
	}
	conv, err := chatStore.GetConversation(c.Request().Context(), pc.OrgID, pc.User.ID, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load created conversation"})
	}
	return c.JSON(http.StatusCreated, ChatConversationSummary{
		ID:        conv.ID,
		Title:     conv.Title,
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
	})
}

// ListConversations returns the authenticated user's conversations, most
// recently active first, optionally full-text filtered by ?search=.
func (s *Server) ListConversations(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	search := strings.TrimSpace(c.QueryParam("search"))
	surface := normalizeSurface(c.QueryParam("surface"))

	if search != "" {
		results, err := chatStore.SearchConversations(ctx, pc.OrgID, pc.User.ID, surface, search, limit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "search failed"})
		}
		out := make([]ChatConversationSummary, 0, len(results))
		for _, r := range results {
			out = append(out, ChatConversationSummary{
				ID:        r.ID,
				Title:     r.Title,
				UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
				Snippet:   r.Snippet,
			})
		}
		return c.JSON(http.StatusOK, out)
	}

	conversations, err := chatStore.ListConversations(ctx, pc.OrgID, pc.User.ID, surface, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
	}
	out := make([]ChatConversationSummary, 0, len(conversations))
	for _, conv := range conversations {
		out = append(out, ChatConversationSummary{
			ID:        conv.ID,
			Title:     conv.Title,
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GetConversation returns one conversation with its full message/chart
// history, for opening a thread from the sidebar.
func (s *Server) GetConversation(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	convID, err := parseChatIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid conversation id"})
	}
	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	conv, err := chatStore.GetConversation(ctx, pc.OrgID, pc.User.ID, convID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}
	messages, err := chatStore.ListMessages(ctx, pc.OrgID, pc.User.ID, convID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
	}

	out := ChatConversationDetail{
		ID:        conv.ID,
		Title:     conv.Title,
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		Messages:  make([]ChatMessageOut, 0, len(messages)),
	}
	for _, m := range messages {
		charts := make([]WebChatChart, 0, len(m.Charts))
		for _, ch := range m.Charts {
			charts = append(charts, WebChatChart{
				Title:       ch.Title,
				Description: ch.Description,
				Query:       ch.Query,
				TimeRange:   ch.TimeRange,
				Granularity: ch.Granularity,
				Context:     ch.Context,
				Spec:        json.RawMessage(ch.VegaSpec),
				Stats:       json.RawMessage(ch.Stats),
				ID:          ch.ID,
			})
		}
		files := make([]WebChatFile, 0, len(m.Files))
		for _, f := range m.Files {
			files = append(files, WebChatFile{
				URL:         chatFileURL(f.ID),
				Filename:    f.Filename,
				Title:       f.Title,
				Description: f.Description,
				Query:       f.Query,
				TimeRange:   f.TimeRange,
				Granularity: f.Granularity,
				Context:     f.Context,
				Rows:        f.Rows,
			})
		}
		out.Messages = append(out.Messages, ChatMessageOut{
			ID:             m.ID,
			Role:           m.Role,
			Content:        m.Content,
			Charts:         charts,
			Files:          files,
			DebugArtifacts: m.DebugArtifacts,
		})
	}
	return c.JSON(http.StatusOK, out)
}

type renameConversationRequest struct {
	Title string `json:"title"`
}

// RenameConversation updates a conversation's title.
func (s *Server) RenameConversation(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	convID, err := parseChatIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid conversation id"})
	}
	var req renameConversationRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}

	chatStore := storagechat.NewStore(s.db)
	if err := chatStore.RenameConversation(c.Request().Context(), pc.OrgID, pc.User.ID, convID, strings.TrimSpace(req.Title)); err != nil {
		if errors.Is(err, storagechat.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to rename conversation"})
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteConversation soft-deletes a conversation.
func (s *Server) DeleteConversation(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	convID, err := parseChatIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid conversation id"})
	}

	chatStore := storagechat.NewStore(s.db)
	if err := chatStore.SoftDeleteConversation(c.Request().Context(), pc.OrgID, pc.User.ID, convID); err != nil {
		if errors.Is(err, storagechat.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete conversation"})
	}
	return c.NoContent(http.StatusNoContent)
}

// RenderChart re-renders a persisted chart's Vega-Lite spec on demand -
// format=json returns the stored spec/metadata (same shape InteractiveChart
// already consumes); format=png reuses the vlrender/vl-convert path that
// already produces Slack/Discord/Teams bot chart images, so a chart persisted
// from a live turn can be exported without ever re-running the LLM.
func (s *Server) RenderChart(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	chartID, err := parseChatIDParam(c, "chartId")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid chart id"})
	}

	chatStore := storagechat.NewStore(s.db)
	chart, err := chatStore.GetChart(c.Request().Context(), pc.OrgID, pc.User.ID, chartID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "chart not found"})
	}

	format := c.QueryParam("format")
	switch format {
	case "", "json":
		return c.JSON(http.StatusOK, WebChatChart{
			Title:       chart.Title,
			Description: chart.Description,
			Query:       chart.Query,
			TimeRange:   chart.TimeRange,
			Granularity: chart.Granularity,
			Context:     chart.Context,
			Spec:        json.RawMessage(chart.VegaSpec),
			Stats:       json.RawMessage(chart.Stats),
			ID:          chart.ID,
		})
	case "png":
		png, _, err := vlrender.ConvertVegaLiteToPNG(c.Request().Context(), chart.VegaSpec, "2.0")
		if err != nil {
			log.Error().Err(err).Int64("chart_id", chartID).Msg("RenderChart: vl-convert PNG render failed")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to render chart"})
		}
		return c.Blob(http.StatusOK, "image/png", png)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported format: " + format})
	}
}
