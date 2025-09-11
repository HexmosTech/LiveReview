package prompts

import (
	"context"
	"database/sql"
	"errors"
)

// Store provides DB access for application context resolution and chunk CRUD.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ResolveApplicationContext implements the precedence:
// repo > group > (ai+git) > ai only > git only > org
// It returns the ID of the single most specific matching row for the org.
func (s *Store) ResolveApplicationContext(ctx context.Context, c Context) (int64, error) {
	if c.OrgID == 0 {
		return 0, errors.New("prompts: ResolveApplicationContext requires OrgID")
	}

	// 1) Repository
	if c.Repository != nil {
		var id int64
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM prompt_application_context WHERE org_id = $1 AND repository = $2 AND group_identifier IS NULL AND ai_connector_id IS NULL AND integration_token_id IS NULL ORDER BY id LIMIT 1`,
			c.OrgID, *c.Repository,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	// 2) Group — omitted in v0.1 (Context has no group field yet)

	// 3) AI + Git
	if c.AIConnectorID != nil && c.IntegrationTokenID != nil {
		var id int64
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM prompt_application_context WHERE org_id = $1 AND ai_connector_id = $2 AND integration_token_id = $3 AND group_identifier IS NULL AND repository IS NULL ORDER BY id LIMIT 1`,
			c.OrgID, *c.AIConnectorID, *c.IntegrationTokenID,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	// 4) AI only
	if c.AIConnectorID != nil {
		var id int64
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM prompt_application_context WHERE org_id = $1 AND ai_connector_id = $2 AND integration_token_id IS NULL AND group_identifier IS NULL AND repository IS NULL ORDER BY id LIMIT 1`,
			c.OrgID, *c.AIConnectorID,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	// 5) Git only
	if c.IntegrationTokenID != nil {
		var id int64
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM prompt_application_context WHERE org_id = $1 AND integration_token_id = $2 AND ai_connector_id IS NULL AND group_identifier IS NULL AND repository IS NULL ORDER BY id LIMIT 1`,
			c.OrgID, *c.IntegrationTokenID,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	// 6) Org (global)
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM prompt_application_context WHERE org_id = $1 AND ai_connector_id IS NULL AND integration_token_id IS NULL AND group_identifier IS NULL AND repository IS NULL ORDER BY id LIMIT 1`,
		c.OrgID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	// 7) Create default org-level context if none exists
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO prompt_application_context (org_id) VALUES ($1) RETURNING id`,
		c.OrgID,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ListChunks returns chunks ordered by sequence_index ASC, created_at ASC.
func (s *Store) ListChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string) ([]Chunk, error) {
	const q = `
SELECT id, org_id, application_context_id, prompt_key, variable_name, chunk_type, COALESCE(title, ''), body,
       sequence_index, enabled, allow_markdown, redact_on_log, created_by, updated_by
FROM prompt_chunks
WHERE org_id = $1 AND application_context_id = $2 AND prompt_key = $3 AND variable_name = $4
ORDER BY sequence_index ASC, created_at ASC`

	rows, err := s.db.QueryContext(ctx, q, orgID, applicationContextID, promptKey, variableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var ch Chunk
		var title string
		if err := rows.Scan(&ch.ID, &ch.OrgID, &ch.ApplicationContextID, &ch.PromptKey, &ch.VariableName, &ch.Type, &title, &ch.Body,
			&ch.SequenceIndex, &ch.Enabled, &ch.AllowMarkdown, &ch.RedactOnLog, &ch.CreatedBy, &ch.UpdatedBy); err != nil {
			return nil, err
		}
		ch.Title = title
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) CreateChunk(ctx context.Context, ch Chunk) (int64, error) {
	// With unique index (org_id, application_context_id, prompt_key, variable_name)
	// perform an upsert so callers can always call CreateChunk for single-value semantics.
	const q = `
INSERT INTO prompt_chunks (org_id, application_context_id, prompt_key, variable_name, chunk_type, title, body, sequence_index, enabled, allow_markdown, redact_on_log, created_by, updated_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (org_id, application_context_id, prompt_key, variable_name)
DO UPDATE SET
	title = EXCLUDED.title,
	body = EXCLUDED.body,
	sequence_index = EXCLUDED.sequence_index,
	enabled = EXCLUDED.enabled,
	allow_markdown = EXCLUDED.allow_markdown,
	redact_on_log = EXCLUDED.redact_on_log,
	updated_by = EXCLUDED.updated_by,
	updated_at = now()
RETURNING id`
	var id int64
	err := s.db.QueryRowContext(ctx, q, ch.OrgID, ch.ApplicationContextID, ch.PromptKey, ch.VariableName, ch.Type, nullIfEmpty(ch.Title), ch.Body, ch.SequenceIndex, ch.Enabled, ch.AllowMarkdown, ch.RedactOnLog, ch.CreatedBy, ch.UpdatedBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) UpdateChunk(ctx context.Context, ch Chunk) error {
	if ch.ID == 0 || ch.OrgID == 0 {
		return errors.New("prompts: UpdateChunk requires ID and OrgID")
	}
	const q = `
UPDATE prompt_chunks
SET title = $1, body = $2, sequence_index = $3, enabled = $4, allow_markdown = $5, redact_on_log = $6, updated_by = $7, updated_at = now()
WHERE id = $8 AND org_id = $9`
	_, err := s.db.ExecContext(ctx, q, nullIfEmpty(ch.Title), ch.Body, ch.SequenceIndex, ch.Enabled, ch.AllowMarkdown, ch.RedactOnLog, ch.UpdatedBy, ch.ID, ch.OrgID)
	return err
}

func (s *Store) DeleteChunk(ctx context.Context, orgID, chunkID int64) error {
	if chunkID == 0 || orgID == 0 {
		return errors.New("prompts: DeleteChunk requires IDs")
	}
	const q = `DELETE FROM prompt_chunks WHERE id = $1 AND org_id = $2`
	_, err := s.db.ExecContext(ctx, q, chunkID, orgID)
	return err
}

// ReorderChunks sets sequence_index for the provided IDs in order.
func (s *Store) ReorderChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string, orderedIDs []int64) error {
	if orgID == 0 || applicationContextID == 0 {
		return errors.New("prompts: ReorderChunks requires org and context IDs")
	}
	if len(orderedIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const q = `
UPDATE prompt_chunks
SET sequence_index = $1, updated_at = now()
WHERE id = $2 AND org_id = $3 AND application_context_id = $4 AND prompt_key = $5 AND variable_name = $6`

	// Space indices by 10 to allow future insertions between items
	seq := 1000
	for _, id := range orderedIDs {
		if _, err = tx.ExecContext(ctx, q, seq, id, orgID, applicationContextID, promptKey, variableName); err != nil {
			return err
		}
		seq += 10
	}
	return tx.Commit()
}

// providerFromAIConnector looks up provider_name for an ai_connectors.id
func (s *Store) providerFromAIConnector(ctx context.Context, aiConnectorID int64) (string, error) {
	const q = `SELECT provider_name FROM ai_connectors WHERE id = $1`
	var provider string
	if err := s.db.QueryRowContext(ctx, q, aiConnectorID).Scan(&provider); err != nil {
		return "", err
	}
	return provider, nil
}

// Helpers
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Removed complex SQL rebind helpers; sequential queries are simpler and clearer.
