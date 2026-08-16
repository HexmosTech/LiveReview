package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/mcpagent"
	"github.com/rs/zerolog/log"
)

// Downloadable chat exports (CSV). Unlike a chart spec, which the frontend
// renders directly and never touches the backend again, an export is bulk
// organization data, so it is served only to an authenticated caller in the
// org that produced it. The random id alone is not treated as sufficient
// authorization.

const (
	exportTTL             = 10 * time.Minute
	exportCleanupInterval = 1 * time.Minute
)

var (
	chatExports       = map[string]*chatExportEntry{}
	chatExportsMu     sync.RWMutex
	lastExportCleanup = time.Now()
)

type chatExportEntry struct {
	Path      string
	TmpDir    string
	CreatedAt time.Time
	Filename  string
	OrgID     int64
}

// cleanupExpiredExports throttles the full-map scan to once per
// exportCleanupInterval, so it can be called on every register/lookup without
// an O(n) walk on each request. Expired entries are still removed within
// exportTTL + exportCleanupInterval.
func cleanupExpiredExports() {
	chatExportsMu.Lock()
	defer chatExportsMu.Unlock()
	if time.Since(lastExportCleanup) < exportCleanupInterval {
		return
	}
	lastExportCleanup = time.Now()
	now := time.Now()
	for id, e := range chatExports {
		if now.Sub(e.CreatedAt) > exportTTL {
			os.RemoveAll(e.TmpDir)
			delete(chatExports, id)
		}
	}
}

// WebChatFile is a downloadable artifact offered alongside a chat answer.
type WebChatFile struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Query       string `json:"query,omitempty"`
	TimeRange   string `json:"time_range,omitempty"`
	Granularity string `json:"granularity,omitempty"`
	Rows        int    `json:"rows,omitempty"`
}

func newChatFileID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// registerChatExports writes each artifact to disk and returns the file
// descriptors to hand back to the client. Failures are logged and skipped: a
// missing download is better than a failed answer.
func registerChatExports(artifacts []mcpagent.Artifact, orgID int64) []WebChatFile {
	if len(artifacts) == 0 {
		return nil
	}
	files := make([]WebChatFile, 0, len(artifacts))
	for _, art := range artifacts {
		tmpDir, err := os.MkdirTemp("", "livereview-chat-export-")
		if err != nil {
			log.Error().Err(err).Msg("WebChat: failed to create export temp dir")
			continue
		}
		path := filepath.Join(tmpDir, art.Filename)
		if err := os.WriteFile(path, art.Data, 0600); err != nil {
			log.Error().Err(err).Msg("WebChat: failed to write export file")
			os.RemoveAll(tmpDir)
			continue
		}

		id := newChatFileID()
		chatExportsMu.Lock()
		if old, ok := chatExports[id]; ok {
			os.RemoveAll(old.TmpDir)
		}
		chatExports[id] = &chatExportEntry{
			Path:      path,
			TmpDir:    tmpDir,
			CreatedAt: time.Now(),
			Filename:  art.Filename,
			OrgID:     orgID,
		}
		chatExportsMu.Unlock()
		cleanupExpiredExports()

		files = append(files, WebChatFile{
			URL:         "/api/v1/chat/files/" + id,
			Filename:    art.Filename,
			Title:       art.Title,
			Description: art.Description,
			Query:       art.Query,
			TimeRange:   art.TimeRange,
			Granularity: art.Granularity,
			Rows:        art.Rows,
		})
	}
	return files
}

// ServeChatCSV returns a previously generated export. It requires
// authentication and a matching organization: the id is unguessable, but an
// unguessable id is secrecy, not authorization, and this endpoint returns an
// organization's review data in bulk.
func (s *Server) ServeChatCSV(c echo.Context) error {
	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}

	id := c.Param("id")
	cleanupExpiredExports()

	chatExportsMu.RLock()
	entry, ok := chatExports[id]
	chatExportsMu.RUnlock()

	// Report a wrong-org request as absent rather than forbidden, so the
	// endpoint cannot be used to confirm that an id exists.
	if !ok || entry.OrgID != pc.OrgID {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found or expired"})
	}
	return c.Attachment(entry.Path, entry.Filename)
}
