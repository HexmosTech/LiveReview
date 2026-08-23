package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatexport"
	storagechat "github.com/livereview/storage/chat"
)

// conversationSummaryLimit caps how many conversations the compile picker
// (see CompileExport) can browse in one surface - the same cap
// ListConversations already clamps requests to.
const conversationSummaryLimit = 200

// ConversationSummaryOut is one row in the compile-picker list: enough to
// select and usefully reorder a conversation without fetching its full
// message history (no chart PNG rendering happens here - that's what makes
// this cheap enough to compute for every conversation in a surface at once).
type ConversationSummaryOut struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	UpdatedAt  string   `json:"updatedAt"`
	TurnCount  int      `json:"turnCount"`
	ChartTypes []string `json:"chartTypes,omitempty"`
}

// ListConversationSummaries is a separate, richer listing from
// ListConversations (used by the always-visible sidebar) so the sidebar's
// hot path stays cheap - this one is only called when the compile dialog
// opens. For each conversation it walks the already-loaded chart specs
// (ListMessages already returns them) through chatexport.DescribeChartShape
// - no PNG rendering, unlike an actual export.
func (s *Server) ListConversationSummaries(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)
	surface := normalizeSurface(c.QueryParam("surface"))

	conversations, err := chatStore.ListConversations(ctx, pc.OrgID, pc.User.ID, surface, conversationSummaryLimit, 0)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
	}

	out := make([]ConversationSummaryOut, 0, len(conversations))
	for _, conv := range conversations {
		messages, err := chatStore.ListMessages(ctx, pc.OrgID, pc.User.ID, conv.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load conversation details"})
		}

		summary := ConversationSummaryOut{
			ID:        conv.ID,
			Title:     conv.Title,
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
			TurnCount: len(messages),
		}
		for _, m := range messages {
			for _, ch := range m.Charts {
				summary.ChartTypes = append(summary.ChartTypes, chatexport.DescribeChartShape(ch.VegaSpec))
			}
		}
		out = append(out, summary)
	}

	return c.JSON(http.StatusOK, out)
}
