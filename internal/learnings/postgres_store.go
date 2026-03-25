package learnings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	storagelearnings "github.com/livereview/storage/learnings"
)

type PostgresStore struct {
	store *storagelearnings.LearningsStore
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{store: storagelearnings.NewLearningsStore(db)}
}

func (s *PostgresStore) Create(ctx context.Context, l *Learning) error {
	var scJSON []byte
	var err error
	if l.SourceContext != nil {
		scJSON, err = json.Marshal(l.SourceContext)
		if err != nil {
			return err
		}
	}
	id, createdAt, updatedAt, err := s.store.InsertLearning(
		ctx,
		storagelearnings.InsertLearningInput{
			ShortID:           l.ShortID,
			OrgID:             l.OrgID,
			ScopeKind:         string(l.Scope),
			RepoID:            nullableStringPtr(l.RepoID),
			Title:             l.Title,
			Body:              l.Body,
			Tags:              ensureSliceNotNil(l.Tags),
			Status:            string(l.Status),
			Confidence:        l.Confidence,
			Simhash:           l.Simhash,
			Embedding:         l.Embedding,
			SourceURLs:        ensureSliceNotNil(l.SourceURLs),
			SourceContextJSON: scJSON,
			CreatedBy:         nullableInt64Ptr(l.CreatedBy()),
			UpdatedBy:         nullableInt64Ptr(l.UpdatedBy()),
		},
	)
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
	updatedAt, err := s.store.UpdateLearning(
		ctx,
		storagelearnings.UpdateLearningInput{
			ID:                l.ID,
			Title:             l.Title,
			Body:              l.Body,
			Tags:              ensureSliceNotNil(l.Tags),
			Status:            string(l.Status),
			Confidence:        l.Confidence,
			Simhash:           l.Simhash,
			Embedding:         l.Embedding,
			SourceURLs:        ensureSliceNotNil(l.SourceURLs),
			SourceContextJSON: scJSON,
			ScopeKind:         string(l.Scope),
			RepoID:            nullableStringPtr(l.RepoID),
		},
	)
	if err != nil {
		return err
	}
	l.UpdatedAt = updatedAt
	return nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Learning, error) {
	row := s.store.QueryLearningByID(ctx, id)
	return scanLearning(row)
}

func (s *PostgresStore) GetByShortID(ctx context.Context, orgID int64, shortID string) (*Learning, error) {
	row := s.store.QueryLearningByShortID(ctx, orgID, shortID)
	return scanLearning(row)
}

func (s *PostgresStore) ListByOrg(ctx context.Context, orgID int64) ([]*Learning, error) {
	return s.ListByOrgWithPagination(ctx, orgID, 0, 100, "", true)
}

func (s *PostgresStore) ListByOrgWithPagination(ctx context.Context, orgID int64, offset, limit int, search string, includeArchived bool) ([]*Learning, error) {
	rows, err := s.store.ListByOrgWithPagination(ctx, orgID, offset, limit, search, includeArchived)
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

func (s *PostgresStore) CountByOrg(ctx context.Context, orgID int64, search string, includeArchived bool) (int, error) {
	return s.store.CountByOrg(ctx, orgID, search, includeArchived)
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
	id, err := s.store.InsertLearningEvent(
		ctx,
		storagelearnings.InsertLearningEventInput{
			LearningID:    ev.LearningID,
			OrgID:         ev.OrgID,
			Action:        string(ev.Action),
			Provider:      ev.Provider,
			ThreadID:      ev.ThreadID,
			CommentID:     ev.CommentID,
			Repository:    ev.Repository,
			CommitSHA:     ev.CommitSHA,
			FilePath:      ev.FilePath,
			LineStart:     ev.LineStart,
			LineEnd:       ev.LineEnd,
			ActorID:       nil,
			ReasonSnippet: ev.Reason,
			Classifier:    ev.Classifier,
			ContextJSON:   ctxJSON,
		},
	)
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

func nullableStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nullableInt64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ensureSliceNotNil ensures a string slice is never nil to avoid NOT NULL constraint violations
func ensureSliceNotNil(slice []string) []string {
	if slice == nil {
		return []string{}
	}
	return slice
}
