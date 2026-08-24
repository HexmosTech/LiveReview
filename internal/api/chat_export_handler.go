package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatexport"
	"github.com/livereview/pkg/models"
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
	if format != "pdf" && format != "html" && format != "json" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported format: " + format})
	}

	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	conv, err := chatStore.GetConversation(ctx, pc.OrgID, pc.User.ID, convID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}

	isDebug := conv.Surface == storagechat.SurfaceChatDebug
	if format == "json" && !isDebug {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "JSON export is only available for debug conversations"})
	}

	convDoc, err := chatexport.BuildDoc(ctx, chatStore, pc.OrgID, pc.User.ID, convID, chatexport.BuildOptions{
		IncludeDebugArtifacts: isDebug,
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
	case "json":
		firstName, lastName, fullName := userNames(pc.User)
		var subPlan *string
		if pc.CurrentOrg != nil {
			subPlan = pc.CurrentOrg.SubscriptionPlan
		}
		jsonDoc := chatexport.BuildCompileJSON(
			doc,
			pc.User.ID, pc.User.Email, firstName, lastName, fullName,
			pc.OrgID, orgName(pc), orgDesc(pc), orgActive(pc), subPlan,
			pc.Role, pc.IsSuperAdmin, pc.IsOwner, pc.IsMember,
		)
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonDoc); err != nil {
			log.Error().Err(err).Int64("conversation_id", convID).Msg("ExportConversation: failed to encode JSON")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode json"})
		}
		contentType, ext = "application/json", "json"
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

// userNames extracts first/last/full name from a User model.
func userNames(u *models.User) (firstName, lastName, fullName string) {
	if u == nil {
		return "", "", ""
	}
	if u.FirstName != nil {
		firstName = *u.FirstName
	}
	if u.LastName != nil {
		lastName = *u.LastName
	}
	fullName = u.FullName()
	return
}

// orgName returns the org name from a PermissionContext.
func orgName(pc *auth.PermissionContext) string {
	if pc.CurrentOrg != nil {
		return pc.CurrentOrg.Name
	}
	return ""
}

// orgDesc returns the org description from a PermissionContext.
func orgDesc(pc *auth.PermissionContext) string {
	if pc.CurrentOrg != nil && pc.CurrentOrg.Description != nil {
		return *pc.CurrentOrg.Description
	}
	return ""
}

// orgActive returns whether the org is active.
func orgActive(pc *auth.PermissionContext) bool {
	if pc.CurrentOrg != nil {
		return pc.CurrentOrg.IsActive
	}
	return true
}
