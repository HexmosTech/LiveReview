// Package chat is the Postgres-backed store for persisted Livi chat
// conversations. Every method takes orgID/userID and scopes its WHERE/JOIN
// clause on both - a conversation id alone is never trusted as
// authorization, matching the pattern already used for chat CSV exports (see
// internal/api.ServeChatCSV).
package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/lib/pq"
	domainchat "github.com/livereview/internal/chat"
	"github.com/livereview/internal/mcpagent"
)

// ErrNotFound is returned when a conversation/chart doesn't exist, is soft
// deleted, or is not owned by the given org/user.
var ErrNotFound = errors.New("chat: not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// MessageInput is one side of a turn (user prompt or assistant reply) to
// persist via AppendUserMessage or AppendAssistantMessage.
type MessageInput struct {
	Role              string
	Content           string
	RawHistoryEntries []mcpagent.HistoryEntry
	DebugArtifacts    json.RawMessage // nil for user messages and non-debug turns
}

// ChartInput is a chart artifact to persist alongside the assistant message
// of a turn.
type ChartInput struct {
	Title                 string
	Description           string
	Query                 string
	TimeRange             string
	Granularity           string
	TriggeringUserMessage string
	VegaSpec              json.RawMessage
	RawLLMOutput          string
}

// FileInput is a downloadable export to persist alongside the assistant
// message of a turn.
type FileInput struct {
	Kind        string
	Filename    string
	Title       string
	Description string
	Query       string
	TimeRange   string
	Granularity string
	Rows        int
	Data        []byte
}

// CreateConversation inserts a new conversation, defaulting its title when
// empty, and returns its id.
func (s *Store) CreateConversation(ctx context.Context, orgID, userID int64, title, sessionID string) (int64, error) {
	if title == "" {
		title = "New conversation"
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chat_conversations (org_id, user_id, title, session_id)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id
	`, orgID, userID, title, sessionID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert chat_conversations: %w", err)
	}
	return id, nil
}

// GetConversation returns a single conversation owned by orgID/userID.
func (s *Store) GetConversation(ctx context.Context, orgID, userID, convID int64) (*domainchat.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, org_id, user_id, title, COALESCE(session_id, ''), created_at, updated_at
		FROM chat_conversations
		WHERE id = $1 AND org_id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, convID, orgID, userID)
	c, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select chat_conversations: %w", err)
	}
	return c, nil
}

// ListConversations returns a user's conversations ordered by most recently
// active, most recent first.
func (s *Store) ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]domainchat.Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, user_id, title, COALESCE(session_id, ''), created_at, updated_at
		FROM chat_conversations
		WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4
	`, orgID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list chat_conversations: %w", err)
	}
	defer rows.Close()

	var out []domainchat.Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat_conversations: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// SearchConversations searches both conversation titles and message bodies,
// ranking title/body matches together and returning a snippet of the
// best-matching message (if the match came from a message rather than the
// title). Matching is prefix-based (searching "rev" finds "review" - plain
// to_tsquery/websearch_to_tsquery only match whole stemmed words, which is
// too strict for an incremental search box) with a trigram-similarity
// fallback on the title for typo tolerance.
func (s *Store) SearchConversations(ctx context.Context, orgID, userID int64, query string, limit int) ([]domainchat.ConversationSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	prefixQuery := buildPrefixTSQuery(query)
	rows, err := s.db.QueryContext(ctx, `
		WITH q AS (SELECT to_tsquery('english', $3) AS tsq)
		SELECT c.id, c.org_id, c.user_id, c.title, COALESCE(c.session_id, ''), c.created_at, c.updated_at,
			COALESCE(hit.snippet, '')
		FROM chat_conversations c, q
		LEFT JOIN LATERAL (
			SELECT
				-- StartSel/StopSel blanked out: the snippet is rendered as plain
				-- text on the frontend (no dangerouslySetInnerHTML), since message
				-- content is user/LLM text that must not be treated as trusted HTML.
				ts_headline('english', m.content, q.tsq, 'StartSel="", StopSel="", MaxFragments=1, MaxWords=20, MinWords=5') AS snippet,
				ts_rank(m.search_vector, q.tsq) AS rank
			FROM chat_messages m
			WHERE m.conversation_id = c.id AND m.search_vector @@ q.tsq
			ORDER BY ts_rank(m.search_vector, q.tsq) DESC
			LIMIT 1
		) hit ON true
		WHERE c.org_id = $1 AND c.user_id = $2 AND c.deleted_at IS NULL
			AND (
				c.search_vector @@ q.tsq
				OR hit.snippet IS NOT NULL
				OR similarity(c.title, $4) > 0.2
			)
		ORDER BY GREATEST(
			ts_rank(c.search_vector, q.tsq),
			COALESCE(hit.rank, 0),
			similarity(c.title, $4)
		) DESC, c.updated_at DESC
		LIMIT $5
	`, orgID, userID, prefixQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search chat_conversations: %w", err)
	}
	defer rows.Close()

	var out []domainchat.ConversationSearchResult
	for rows.Next() {
		var r domainchat.ConversationSearchResult
		if err := rows.Scan(&r.ID, &r.OrgID, &r.UserID, &r.Title, &r.SessionID, &r.CreatedAt, &r.UpdatedAt, &r.Snippet); err != nil {
			return nil, fmt.Errorf("scan chat_conversations search: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RenameConversation updates a conversation's title.
func (s *Store) RenameConversation(ctx context.Context, orgID, userID, convID int64, title string) error {
	if title == "" {
		return errors.New("title is required")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE chat_conversations SET title = $1
		WHERE id = $2 AND org_id = $3 AND user_id = $4 AND deleted_at IS NULL
	`, title, convID, orgID, userID)
	if err != nil {
		return fmt.Errorf("rename chat_conversations: %w", err)
	}
	return checkRowsAffected(res)
}

// SoftDeleteConversation hides a conversation from list/search/get without
// removing its rows.
func (s *Store) SoftDeleteConversation(ctx context.Context, orgID, userID, convID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE chat_conversations SET deleted_at = NOW()
		WHERE id = $1 AND org_id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, convID, orgID, userID)
	if err != nil {
		return fmt.Errorf("soft delete chat_conversations: %w", err)
	}
	return checkRowsAffected(res)
}

// AppendUserMessage persists just the user side of a turn and bumps the
// conversation's updated_at, so the question survives (and the conversation
// resurfaces in the sidebar) even if everything after this call - the AI
// connector, the MCP session, the agent loop itself - fails or the client
// disconnects before a reply exists. Call this the moment a turn reaches the
// handler, before any of that; see AppendAssistantMessage for the reply half.
func (s *Store) AppendUserMessage(ctx context.Context, convID int64, userMsg MessageInput) (msgID int64, seq int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(turn_seq) + 1, 0) FROM chat_messages WHERE conversation_id = $1
	`, convID).Scan(&seq); err != nil {
		return 0, 0, fmt.Errorf("next turn_seq: %w", err)
	}

	msgID, err = insertMessage(ctx, tx, convID, seq, userMsg)
	if err != nil {
		return 0, 0, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE chat_conversations SET updated_at = NOW() WHERE id = $1`, convID); err != nil {
		return 0, 0, fmt.Errorf("bump chat_conversations.updated_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit user message: %w", err)
	}
	return msgID, seq, nil
}

// AppendAssistantMessage persists the reply half of a turn (plus any charts
// and file exports) at the next available turn_seq - always after the
// user message AppendUserMessage already saved, since that call already
// consumed the seq the reply would otherwise have raced for. Safe to call
// even when the reply is an error message: a turn that ends in "Error: ..."
// is still worth keeping, so the user can see what happened on reopening
// the conversation rather than finding a question with no visible outcome.
func (s *Store) AppendAssistantMessage(ctx context.Context, convID int64, assistantMsg MessageInput, charts []ChartInput, files []FileInput) (msgID int64, fileIDs []int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var seq int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(turn_seq) + 1, 0) FROM chat_messages WHERE conversation_id = $1
	`, convID).Scan(&seq); err != nil {
		return 0, nil, fmt.Errorf("next turn_seq: %w", err)
	}

	msgID, err = insertMessage(ctx, tx, convID, seq, assistantMsg)
	if err != nil {
		return 0, nil, err
	}

	for _, ch := range charts {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_charts (message_id, title, description, query, time_range, granularity, triggering_user_message, vega_spec, raw_llm_output)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, msgID, ch.Title, ch.Description, ch.Query, ch.TimeRange, ch.Granularity, ch.TriggeringUserMessage, []byte(ch.VegaSpec), ch.RawLLMOutput); err != nil {
			return 0, nil, fmt.Errorf("insert chat_charts: %w", err)
		}
	}

	fileIDs = make([]int64, 0, len(files))
	for _, f := range files {
		var fileID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO chat_files (message_id, kind, filename, title, description, query, time_range, granularity, rows, data)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id
		`, msgID, f.Kind, f.Filename, nullable(f.Title), nullable(f.Description), nullable(f.Query), nullable(f.TimeRange), nullable(f.Granularity), f.Rows, f.Data).Scan(&fileID); err != nil {
			return 0, nil, fmt.Errorf("insert chat_files: %w", err)
		}
		fileIDs = append(fileIDs, fileID)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE chat_conversations SET updated_at = NOW() WHERE id = $1`, convID); err != nil {
		return 0, nil, fmt.Errorf("bump chat_conversations.updated_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit assistant message: %w", err)
	}
	return msgID, fileIDs, nil
}

// nullable returns a pointer so empty strings persist as SQL NULL rather than
// empty text, matching how chat_charts stores its optional metadata columns.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func insertMessage(ctx context.Context, tx *sql.Tx, convID int64, seq int, m MessageInput) (int64, error) {
	rawJSON, err := json.Marshal(m.RawHistoryEntries)
	if err != nil {
		return 0, fmt.Errorf("marshal raw_history_entry: %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO chat_messages (conversation_id, role, content, raw_history_entry, turn_seq, debug_artifacts)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, convID, m.Role, m.Content, rawJSON, seq, m.DebugArtifacts).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert chat_messages: %w", err)
	}
	return id, nil
}

// ListMessages returns a conversation's full message history in order, each
// with its charts eager-loaded. Ownership is checked first so a wrong-org id
// returns ErrNotFound instead of an empty (and indistinguishable from
// "no messages yet") list.
func (s *Store) ListMessages(ctx context.Context, orgID, userID, convID int64) ([]domainchat.Message, error) {
	if _, err := s.GetConversation(ctx, orgID, userID, convID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, raw_history_entry, turn_seq, created_at, debug_artifacts
		FROM chat_messages
		WHERE conversation_id = $1
		ORDER BY turn_seq ASC
	`, convID)
	if err != nil {
		return nil, fmt.Errorf("list chat_messages: %w", err)
	}
	defer rows.Close()

	var messages []domainchat.Message
	msgIdx := map[int64]int{}
	for rows.Next() {
		var m domainchat.Message
		var rawJSON []byte
		var debugJSON []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &rawJSON, &m.TurnSeq, &m.CreatedAt, &debugJSON); err != nil {
			return nil, fmt.Errorf("scan chat_messages: %w", err)
		}
		if err := json.Unmarshal(rawJSON, &m.RawHistoryEntries); err != nil {
			return nil, fmt.Errorf("unmarshal raw_history_entry: %w", err)
		}
		if len(debugJSON) > 0 {
			m.DebugArtifacts = json.RawMessage(debugJSON)
		}
		msgIdx[m.ID] = len(messages)
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return messages, nil
	}

	ids := make([]int64, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}

	chartRows, err := s.db.QueryContext(ctx, `
		SELECT id, message_id, COALESCE(title, ''), COALESCE(description, ''), COALESCE(query, ''),
			COALESCE(time_range, ''), COALESCE(granularity, ''), triggering_user_message, vega_spec, raw_llm_output, created_at
		FROM chat_charts
		WHERE message_id = ANY($1)
	`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list chat_charts: %w", err)
	}
	defer chartRows.Close()
	for chartRows.Next() {
		var ch domainchat.Chart
		var specJSON []byte
		if err := chartRows.Scan(&ch.ID, &ch.MessageID, &ch.Title, &ch.Description, &ch.Query, &ch.TimeRange,
			&ch.Granularity, &ch.TriggeringUserMessage, &specJSON, &ch.RawLLMOutput, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat_charts: %w", err)
		}
		ch.VegaSpec = specJSON
		if idx, ok := msgIdx[ch.MessageID]; ok {
			messages[idx].Charts = append(messages[idx].Charts, ch)
		}
	}

	fileRows, err := s.db.QueryContext(ctx, `
		SELECT id, message_id, COALESCE(kind, 'csv'), filename,
			COALESCE(title, ''), COALESCE(description, ''), COALESCE(query, ''),
			COALESCE(time_range, ''), COALESCE(granularity, ''), COALESCE(rows, 0), data, created_at
		FROM chat_files
		WHERE message_id = ANY($1)
		ORDER BY id ASC
	`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list chat_files: %w", err)
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var f domainchat.File
		if err := fileRows.Scan(&f.ID, &f.MessageID, &f.Kind, &f.Filename, &f.Title, &f.Description, &f.Query,
			&f.TimeRange, &f.Granularity, &f.Rows, &f.Data, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat_files: %w", err)
		}
		if idx, ok := msgIdx[f.MessageID]; ok {
			messages[idx].Files = append(messages[idx].Files, f)
		}
	}
	return messages, fileRows.Err()
}

// GetFile returns one file export, ownership-checked via a join back through
// its message to the owning conversation.
func (s *Store) GetFile(ctx context.Context, orgID, userID, fileID int64) (*domainchat.File, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT f.id, f.message_id, COALESCE(f.kind, 'csv'), f.filename,
			COALESCE(f.title, ''), COALESCE(f.description, ''), COALESCE(f.query, ''),
			COALESCE(f.time_range, ''), COALESCE(f.granularity, ''), COALESCE(f.rows, 0), f.data, f.created_at
		FROM chat_files f
		JOIN chat_messages m ON m.id = f.message_id
		JOIN chat_conversations c ON c.id = m.conversation_id
		WHERE f.id = $1 AND c.org_id = $2 AND c.user_id = $3 AND c.deleted_at IS NULL
	`, fileID, orgID, userID)

	var f domainchat.File
	if err := row.Scan(&f.ID, &f.MessageID, &f.Kind, &f.Filename, &f.Title, &f.Description, &f.Query,
		&f.TimeRange, &f.Granularity, &f.Rows, &f.Data, &f.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("select chat_files: %w", err)
	}
	return &f, nil
}

// GetChart returns one chart, ownership-checked via a join back through its
// message to the owning conversation.
func (s *Store) GetChart(ctx context.Context, orgID, userID, chartID int64) (*domainchat.Chart, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ch.id, ch.message_id, COALESCE(ch.title, ''), COALESCE(ch.description, ''), COALESCE(ch.query, ''),
			COALESCE(ch.time_range, ''), COALESCE(ch.granularity, ''), ch.triggering_user_message, ch.vega_spec, ch.raw_llm_output, ch.created_at
		FROM chat_charts ch
		JOIN chat_messages m ON m.id = ch.message_id
		JOIN chat_conversations c ON c.id = m.conversation_id
		WHERE ch.id = $1 AND c.org_id = $2 AND c.user_id = $3 AND c.deleted_at IS NULL
	`, chartID, orgID, userID)

	var ch domainchat.Chart
	var specJSON []byte
	err := row.Scan(&ch.ID, &ch.MessageID, &ch.Title, &ch.Description, &ch.Query, &ch.TimeRange,
		&ch.Granularity, &ch.TriggeringUserMessage, &specJSON, &ch.RawLLMOutput, &ch.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select chat_charts: %w", err)
	}
	ch.VegaSpec = specJSON
	return &ch, nil
}

// buildPrefixTSQuery turns free-text search input into a tsquery string that
// prefix-matches every word ("rev ac" -> "rev:* & ac:*"), so an incremental
// search box finds "review activity" without the user typing whole words.
// Tokens are alphanumeric-only (split on everything else), which also means
// the result never contains tsquery operator characters that would need
// escaping. Returns "" (a valid, always-empty tsquery) when input has no
// alphanumeric content at all.
func buildPrefixTSQuery(input string) string {
	tokens := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = t + ":*"
	}
	return strings.Join(parts, " & ")
}

type scannable interface {
	Scan(dest ...any) error
}

func scanConversation(row scannable) (*domainchat.Conversation, error) {
	var c domainchat.Conversation
	if err := row.Scan(&c.ID, &c.OrgID, &c.UserID, &c.Title, &c.SessionID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func checkRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
