package learnings

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type LearningsStore struct {
	db *sql.DB
}

type InsertLearningInput struct {
	ShortID           string
	OrgID             int64
	ScopeKind         string
	RepoID            *string
	Title             string
	Body              string
	Tags              []string
	Status            string
	Confidence        int
	Simhash           int64
	Embedding         []byte
	SourceURLs        []string
	SourceContextJSON []byte
	CreatedBy         *int64
	UpdatedBy         *int64
}

type UpdateLearningInput struct {
	ID                string
	Title             string
	Body              string
	Tags              []string
	Status            string
	Confidence        int
	Simhash           int64
	Embedding         []byte
	SourceURLs        []string
	SourceContextJSON []byte
	ScopeKind         string
	RepoID            *string
}

type InsertLearningEventInput struct {
	LearningID    string
	OrgID         int64
	Action        string
	Provider      string
	ThreadID      string
	CommentID     string
	Repository    string
	CommitSHA     string
	FilePath      string
	LineStart     int
	LineEnd       int
	ActorID       *int64
	ReasonSnippet string
	Classifier    string
	ContextJSON   []byte
}

func NewLearningsStore(db *sql.DB) *LearningsStore {
	return &LearningsStore{db: db}
}

func (s *LearningsStore) InsertLearning(ctx context.Context, input InsertLearningInput) (string, time.Time, time.Time, error) {
	var id string
	var createdAt, updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO learnings (short_id, org_id, scope_kind, repo_id, title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id, created_at, updated_at
	`, input.ShortID, input.OrgID, input.ScopeKind, input.RepoID, input.Title, input.Body, pq.Array(input.Tags), input.Status, input.Confidence, input.Simhash, input.Embedding, pq.Array(input.SourceURLs), input.SourceContextJSON, input.CreatedBy, input.UpdatedBy).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	return id, createdAt, updatedAt, nil
}

func (s *LearningsStore) UpdateLearning(ctx context.Context, input UpdateLearningInput) (time.Time, error) {
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		UPDATE learnings
		SET title=$1, body=$2, tags=$3, status=$4, confidence=$5, simhash=$6, embedding=$7, source_urls=$8, source_context=$9, scope_kind=$10, repo_id=$11, updated_at=now()
		WHERE id=$12
		RETURNING updated_at
	`, input.Title, input.Body, pq.Array(input.Tags), input.Status, input.Confidence, input.Simhash, input.Embedding, pq.Array(input.SourceURLs), input.SourceContextJSON, input.ScopeKind, input.RepoID, input.ID).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, err
	}
	return updatedAt, nil
}

func (s *LearningsStore) QueryLearningByID(ctx context.Context, id string) *sql.Row {
	return s.db.QueryRowContext(ctx, `
		SELECT id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
		FROM learnings WHERE id=$1
	`, id)
}

func (s *LearningsStore) QueryLearningByShortID(ctx context.Context, orgID int64, shortID string) *sql.Row {
	return s.db.QueryRowContext(ctx, `
		SELECT id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
		FROM learnings WHERE org_id=$1 AND short_id=$2
	`, orgID, shortID)
}

func (s *LearningsStore) ListByOrgWithPagination(ctx context.Context, orgID int64, offset, limit int, search string, includeArchived bool) (*sql.Rows, error) {
	query := `
		SELECT DISTINCT ON (body) id, short_id, org_id, scope_kind, coalesce(repo_id,''), title, body, tags, status, confidence, simhash, embedding, source_urls, source_context, created_at, updated_at
		FROM learnings WHERE org_id=$1`

	args := []interface{}{orgID}
	argIndex := 2

	if search != "" {
		query += ` AND (title ILIKE $` + fmt.Sprintf("%d", argIndex) + ` OR body ILIKE $` + fmt.Sprintf("%d", argIndex) + ` OR $` + fmt.Sprintf("%d", argIndex) + ` = ANY(tags) OR short_id ILIKE $` + fmt.Sprintf("%d", argIndex) + `)`
		args = append(args, "%"+search+"%")
		argIndex++
	}

	if !includeArchived {
		query += ` AND status != 'archived'`
	}

	query += ` ORDER BY body, confidence DESC, updated_at DESC`

	if limit > 0 {
		query += ` LIMIT $` + fmt.Sprintf("%d", argIndex)
		args = append(args, limit)
		argIndex++
	}
	if offset > 0 {
		query += ` OFFSET $` + fmt.Sprintf("%d", argIndex)
		args = append(args, offset)
	}

	return s.db.QueryContext(ctx, query, args...)
}

func (s *LearningsStore) CountByOrg(ctx context.Context, orgID int64, search string, includeArchived bool) (int, error) {
	query := `SELECT COUNT(*) FROM learnings WHERE org_id=$1`
	args := []interface{}{orgID}
	argIndex := 2

	if search != "" {
		query += ` AND (title ILIKE $` + fmt.Sprintf("%d", argIndex) + ` OR body ILIKE $` + fmt.Sprintf("%d", argIndex) + ` OR $` + fmt.Sprintf("%d", argIndex) + ` = ANY(tags) OR short_id ILIKE $` + fmt.Sprintf("%d", argIndex) + `)`
		args = append(args, "%"+search+"%")
		argIndex++
	}

	if !includeArchived {
		query += ` AND status != 'archived'`
	}

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (s *LearningsStore) InsertLearningEvent(ctx context.Context, input InsertLearningEventInput) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO learning_events (learning_id, org_id, action, provider, thread_id, comment_id, repository, commit_sha, file_path, line_start, line_end, actor_id, reason_snippet, classifier, context)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id
	`, input.LearningID, input.OrgID, input.Action, input.Provider, input.ThreadID, input.CommentID, input.Repository, input.CommitSHA, input.FilePath, input.LineStart, input.LineEnd, input.ActorID, input.ReasonSnippet, input.Classifier, input.ContextJSON).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}
