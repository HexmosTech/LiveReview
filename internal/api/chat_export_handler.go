package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatexport"
	storagechat "github.com/livereview/storage/chat"
	"github.com/rs/zerolog/log"
)

// ExportConversation renders a persisted conversation to a downloadable,
// self-contained PDF or HTML document (see internal/chatexport). Debug
// artifacts are included only for chat_debug conversations, decided here
// from the conversation's own stored surface - never from client input -
// so the "debug is the only intentional difference between surfaces" rule
// (see the Chat UI section of AGENTS.md) extends to exports too.
func (s *Server) ExportConversation(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	convID, err := parseChatIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid conversation id"})
	}
	format := c.QueryParam("format")
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "html" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported format: " + format})
	}

	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	conv, err := chatStore.GetConversation(ctx, pc.OrgID, pc.User.ID, convID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}

	convDoc, err := chatexport.BuildDoc(ctx, chatStore, pc.OrgID, pc.User.ID, convID, chatexport.BuildOptions{
		IncludeDebugArtifacts: conv.Surface == storagechat.SurfaceChatDebug,
	})
	if err != nil {
		log.Error().Err(err).Int64("conversation_id", convID).Msg("ExportConversation: failed to build export doc")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to build export"})
	}
	// RenderPDF/RenderHTML take a CompiledDoc (the general, multi-
	// conversation shape - see CompileExport) - a single-conversation
	// export is just a CompiledDoc with one Conversation and no Subtitle.
	doc := &chatexport.CompiledDoc{
		Title:         convDoc.Conversation.Title,
		Conversations: []chatexport.ExportDoc{*convDoc},
	}

	var buf bytes.Buffer
	var contentType, ext string
	switch format {
	case "pdf":
		if err := chatexport.RenderPDF(ctx, doc, &buf); err != nil {
			log.Error().Err(err).Int64("conversation_id", convID).Msg("ExportConversation: failed to render PDF")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to render pdf"})
		}
		contentType, ext = "application/pdf", "pdf"
	case "html":
		if err := chatexport.RenderHTML(doc, &buf); err != nil {
			log.Error().Err(err).Int64("conversation_id", convID).Msg("ExportConversation: failed to render HTML")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to render html"})
		}
		contentType, ext = "text/html; charset=utf-8", "html"
	}

	filename := exportFilename(conv.Title, convID, ext)
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filename))
	return c.Blob(http.StatusOK, contentType, buf.Bytes())
}

// exportFilename builds a filesystem-safe download name from the
// conversation title, e.g. "billing-status-42.pdf".
func exportFilename(title string, convID int64, ext string) string {
	slug := slugify(title)
	if slug == "" {
		slug = "conversation"
	}
	return fmt.Sprintf("%s-%d.%s", slug, convID, ext)
}

func slugify(s string) string {
	var b strings.Builder
	lastHyphen := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}
