package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/require"
)

// TestServeChatCSV exercises the DB-backed export endpoint end to end: a
// file persisted against a conversation the caller owns is served; a caller
// outside the owning org (or with a non-numeric id) gets a 404, never the
// data. Runs against the local test database like the other storage
// integration tests.
func TestServeChatCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()

	orgID, userID := insertChatOrgUser(t, ctx, db)
	convID := insertChatConversation(t, ctx, db, orgID, userID)
	msgID := insertChatMessage(t, ctx, db, convID)

	const filename = "export.csv"
	const csvContent = "id,repository,status\n1,repo,completed\n"
	fileID := insertChatFile(t, ctx, db, msgID, filename, csvContent)

	e := echo.New()

	t.Run("serves own org file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/files/"+itoa(fileID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(itoa(fileID))
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
			User:  &models.User{ID: userID, Email: "owner@example.com"},
			OrgID: orgID,
		})

		s := &Server{db: db}
		if err := s.ServeChatCSV(c); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("ServeChatCSV rejected the owning org: status=%d body=%q", rec.Code, rec.Body.String())
		}
		if got := rec.Body.String(); got != csvContent {
			t.Fatalf("ServeChatCSV body = %q, want %q", got, csvContent)
		}
		if cd := rec.Header().Get(echo.HeaderContentDisposition); !strings.Contains(cd, filename) {
			t.Fatalf("ServeChatCSV Content-Disposition = %q, want it to contain %q", cd, filename)
		}
	})

	t.Run("rejects wrong org", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/files/"+itoa(fileID), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(itoa(fileID))
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
			User:  &models.User{ID: 999999, Email: "intruder@example.com"},
			OrgID: orgID + 5000,
		})

		s := &Server{db: db}
		if err := s.ServeChatCSV(c); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("ServeChatCSV served a different org's export: status=%d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects non-numeric id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/files/abc", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("abc")
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
			User:  &models.User{ID: userID, Email: "owner@example.com"},
			OrgID: orgID,
		})

		s := &Server{db: db}
		if err := s.ServeChatCSV(c); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("ServeChatCSV non-numeric id: status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func insertChatOrgUser(t *testing.T, ctx context.Context, db *sql.DB) (int64, int64) {
	t.Helper()
	var orgID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO orgs (name) VALUES ('chat-files-test-org')
		RETURNING id
	`).Scan(&orgID))
	var userID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, is_active) VALUES ('chat-files-test-owner@example.com', 'x', true)
		RETURNING id
	`).Scan(&userID))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
		_, _ = db.ExecContext(ctx, "DELETE FROM orgs WHERE id = $1", orgID)
	})
	return orgID, userID
}

func insertChatConversation(t *testing.T, ctx context.Context, db *sql.DB, orgID, userID int64) int64 {
	t.Helper()
	var convID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO chat_conversations (org_id, user_id, title) VALUES ($1, $2, 'chat-files-test')
		RETURNING id
	`, orgID, userID).Scan(&convID))
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM chat_conversations WHERE id = $1", convID) })
	return convID
}

func insertChatMessage(t *testing.T, ctx context.Context, db *sql.DB, convID int64) int64 {
	t.Helper()
	var msgID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (conversation_id, role, content, raw_history_entry, turn_seq)
		VALUES ($1, 'assistant', 'I have put the results in a file.', '[]', 0)
		RETURNING id
	`, convID).Scan(&msgID))
	return msgID
}

func insertChatFile(t *testing.T, ctx context.Context, db *sql.DB, msgID int64, filename, data string) int64 {
	t.Helper()
	var fileID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO chat_files (message_id, kind, filename, data)
		VALUES ($1, 'csv', $2, $3)
		RETURNING id
	`, msgID, filename, []byte(data)).Scan(&fileID))
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM chat_files WHERE id = $1", fileID) })
	return fileID
}