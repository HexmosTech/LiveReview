package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatexport"
	storagechat "github.com/livereview/storage/chat"
	"github.com/rs/zerolog/log"
)

// compileExportMaxConversations bounds how many conversations one compile
// request can combine - generous for real use, but stops a single request
// from rendering an unbounded number of charts to PNG.
const compileExportMaxConversations = 50

type compileExportRequest struct {
	ConversationIDs []int64 `json:"conversationIds"`
	Title           string  `json:"title"`
	Subtitle        string  `json:"subtitle"`
	Format          string  `json:"format"`
}

// CompileExport combines several persisted conversations - in the given
// order - into one downloadable PDF or HTML document (see
// internal/chatexport.BuildCompiledDoc). Debug artifacts are included only
// when every selected conversation's own stored surface is chat_debug -
// BuildCompiledDoc itself rejects a request mixing surfaces, so this can't
// be used to smuggle chat_debug data into what looks like a plain export.
func (s *Server) CompileExport(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}

	var req compileExportRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if len(req.ConversationIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "select at least one conversation"})
	}
	if len(req.ConversationIDs) > compileExportMaxConversations {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "too many conversations selected"})
	}
	if req.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}
	format := req.Format
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "html" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported format: " + format})
	}

	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	firstConv, err := chatStore.GetConversation(ctx, pc.OrgID, pc.User.ID, req.ConversationIDs[0])
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}

	doc, err := chatexport.BuildCompiledDoc(ctx, chatStore, pc.OrgID, pc.User.ID, req.ConversationIDs, req.Title, req.Subtitle, chatexport.BuildOptions{
		IncludeDebugArtifacts: firstConv.Surface == storagechat.SurfaceChatDebug,
	})
	if err != nil {
		log.Error().Err(err).Ints64("conversation_ids", req.ConversationIDs).Msg("CompileExport: failed to build compiled doc")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "could not build export from the selected conversations"})
	}

	var buf bytes.Buffer
	var contentType, ext string
	switch format {
	case "pdf":
		if err := chatexport.RenderPDF(ctx, doc, &buf); err != nil {
			log.Error().Err(err).Msg("CompileExport: failed to render PDF")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to render pdf"})
		}
		contentType, ext = "application/pdf", "pdf"
	case "html":
		if err := chatexport.RenderHTML(doc, &buf); err != nil {
			log.Error().Err(err).Msg("CompileExport: failed to render HTML")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to render html"})
		}
		contentType, ext = "text/html; charset=utf-8", "html"
	}

	slug := slugify(req.Title)
	if slug == "" {
		slug = "compiled-conversations"
	}
	filename := fmt.Sprintf("%s.%s", slug, ext)
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filename))
	return c.Blob(http.StatusOK, contentType, buf.Bytes())
}
