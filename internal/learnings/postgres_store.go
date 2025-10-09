package learnings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) Create(ctx context.Context, l *Learning) error {
	var scJSON []byte
	var err error
	if l.SourceContext != nil {
		scJSON, err = json.Marshal(l.SourceContext)
		if err != nil {
			return err
		}
	}
	var id string
	var createdAt, updatedAt time.Time
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO learnings (short_id, org_id, scope_kind, repo_id, title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_by, updated_by)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
        RETURNING id, created_at, updated_at
    `,
		l.ShortID, l.OrgID, string(l.Scope), nullIfEmpty(l.RepoID), l.Title, l.Body, pq.Array(ensureSliceNotNil(l.Tags)), string(l.Status), l.Confidence, l.Simhash, l.Embedding, pq.Array(ensureSliceNotNil(l.SourceURLs)), scJSON, nullIfZero(l.CreatedBy()), nullIfZero(l.UpdatedBy()),
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	l.ID = id
	l.CreatedAt = createdAt
	l.UpdatedAt = updatedAt
	return nil
}

func (l *Learning) CreatedBy() int64 { return 0 }
func (l *Learning) UpdatedBy() int64 { return 0 }

func (s *PostgresStore) Update(ctx context.Context, l *Learning) error {
	var scJSON []byte
	var err error
	if l.SourceContext != nil {
		scJSON, err = json.Marshal(l.SourceContext)
		if err != nil {
			return err
		}
	}
	var updatedAt time.Time
	res := s.db.QueryRowContext(ctx, `
        UPDATE learnings
        SET title=$1, body=$2, tags=$3, status=$4, confidence=$5, simhash=$6, embedding=$7, source_urls=$8, source_context=$9, scope_kind=$10, repo_id=$11, updated_at=now()
        WHERE id=$12
        RETURNING updated_at
    `, l.Title, l.Body, pq.Array(ensureSliceNotNil(l.Tags)), string(l.Status), l.Confidence, l.Simhash, l.Embedding, pq.Array(ensureSliceNotNil(l.SourceURLs)), scJSON, string(l.Scope), nullIfEmpty(l.RepoID), l.ID)
	if err := res.Scan(&updatedAt); err != nil {
		return err
	}
	l.UpdatedAt = updatedAt
	return nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Learning, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
        FROM learnings WHERE id=$1
    `, id)
	return scanLearning(row)
}

func (s *PostgresStore) GetByShortID(ctx context.Context, orgID int64, shortID string) (*Learning, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
        FROM learnings WHERE org_id=$1 AND short_id=$2
    `, orgID, shortID)
	return scanLearning(row)
}

func (s *PostgresStore) ListByOrg(ctx context.Context, orgID int64) ([]*Learning, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
        FROM learnings WHERE org_id=$1 ORDER BY updated_at DESC
    `, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Always return a non-nil slice so JSON encodes as [] instead of null
	out := make([]*Learning, 0)
	for rows.Next() {
		l, err := scanLearning(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CreateEvent(ctx context.Context, ev *LearningEvent) error {
	var ctxJSON []byte
	var err error
	if ev.Context != nil {
		ctxJSON, err = json.Marshal(ev.Context)
		if err != nil {
			return err
		}
	}
	var id string
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO learning_events (learning_id, org_id, action, provider, thread_id, comment_id, repository, commit_sha, file_path, line_start, line_end, actor_id, reason_snippet, classifier, context)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
        RETURNING id
    `,
		ev.LearningID, ev.OrgID, string(ev.Action), ev.Provider, ev.ThreadID, ev.CommentID, ev.Repository, ev.CommitSHA, ev.FilePath, ev.LineStart, ev.LineEnd, nullIfZero64(0), ev.Reason, ev.Classifier, ctxJSON,
	).Scan(&id)
	if err != nil {
		return err
	}
	ev.ID = id
	ev.CreatedAt = time.Now()
	return nil
}

func scanLearning(scanner interface{ Scan(dest ...any) error }) (*Learning, error) {
	var l Learning
	var scope, status string
	var tags []string
	var srcURLs []string
	var embedding []byte
	var scJSON sql.NullString
	if err := scanner.Scan(&l.ID, &l.ShortID, &l.OrgID, &scope, &l.RepoID, &l.Title, &l.Body, pq.Array(&tags), &status, &l.Confidence, &l.Simhash, &embedding, pq.Array(&srcURLs), &scJSON, &l.CreatedAt, &l.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.Scope = ScopeKind(scope)
	l.Status = Status(status)
	l.Tags = append([]string(nil), tags...)
	l.SourceURLs = append([]string(nil), srcURLs...)
	if len(embedding) > 0 {
		l.Embedding = append([]byte(nil), embedding...)
	}
	if scJSON.Valid && scJSON.String != "" {
		var sc SourceContext
		if err := json.Unmarshal([]byte(scJSON.String), &sc); err == nil {
			l.SourceContext = &sc
		}
	}
	return &l, nil
}

// helpers - pqStringArray and pqArrayString are no longer needed, using pq.Array() directly
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
func nullIfZero(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
func nullIfZero64(v int64) interface{} { return nullIfZero(v) }

// ensureSliceNotNil ensures a string slice is never nil to avoid NOT NULL constraint violations
func ensureSliceNotNil(slice []string) []string {
	if slice == nil {
		return []string{}
	}
	return slice
}
