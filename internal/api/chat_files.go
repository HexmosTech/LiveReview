package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/mcpagent"
	"github.com/livereview/internal/vlrender"
	storagechat "github.com/livereview/storage/chat"
)

// Downloadable chat exports (CSV). Unlike a chart spec, which the frontend
// renders directly and never touches the backend again, an export is bulk
// organization data, so it is served only to an authenticated caller in the
// org that produced it. Exports are persisted in the chat_files table (see
// webchat_handler.go's persistAssistantMessage) and served by stable database id - the id
// alone is not treated as sufficient authorization.

// WebChatFile is a downloadable artifact offered alongside a chat answer.
type WebChatFile struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Query       string `json:"query,omitempty"`
	TimeRange   string `json:"time_range,omitempty"`
	Granularity string `json:"granularity,omitempty"`
	Context     vlrender.ChartContext `json:"context"`
	Rows        int                   `json:"rows,omitempty"`
}

// chatFileURL builds the stable, auth-scoped download URL for a persisted file.
func chatFileURL(fileID int64) string {
	return "/api/v1/chat/files/" + strconv.FormatInt(fileID, 10)
}

// chatFileFromArtifact maps a freshly produced artifact plus its persisted DB
// id to a WebChatFile descriptor the frontend can offer for download.
func chatFileFromArtifact(art mcpagent.Artifact, fileID int64) WebChatFile {
	return WebChatFile{
		URL:         chatFileURL(fileID),
		Filename:    art.Filename,
		Title:       art.Title,
		Description: art.Description,
		Query:       art.Query,
		TimeRange:   art.TimeRange,
		Granularity: art.Granularity,
		Context:     art.Context,
		Rows:        art.Rows,
	}
}

// ServeChatCSV returns a previously persisted export. It requires
// authentication and a matching organization: the id is unguessable, but an
// unguessable id is secrecy, not authorization, and this endpoint returns an
// organization's review data in bulk.
func (s *Server) ServeChatCSV(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		// Report a non-numeric id as absent rather than a bad request, so the
		// endpoint cannot be used to probe id formatting.
		return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found or expired"})
	}

	f, err := storagechat.NewStore(s.db).GetFile(c.Request().Context(), pc.OrgID, pc.User.ID, id)
	if err != nil {
		if err == storagechat.ErrNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found or expired"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load file"})
	}

	c.Response().Header().Set(echo.HeaderContentDisposition,
		fmt.Sprintf("attachment; filename=%q", f.Filename))
	return c.Blob(http.StatusOK, "text/csv; charset=utf-8", f.Data)
}

// toFileInput converts a freshly produced artifact into the storage shape for
// persistence.
func toFileInput(art mcpagent.Artifact) storagechat.FileInput {
	return storagechat.FileInput{
		Kind:        art.Kind,
		Filename:    art.Filename,
		Title:       art.Title,
		Description: art.Description,
		Query:       art.Query,
		TimeRange:   art.TimeRange,
		Granularity: art.Granularity,
		Context:     art.Context,
		Rows:        art.Rows,
		Data:        art.Data,
	}
}
