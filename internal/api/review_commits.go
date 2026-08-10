package api

import (
	"context"
	"database/sql"

	reviewprocessor "github.com/livereview/internal/review_processor"
)

// insertReviewCommits records the commit identifiers a review's diff corresponds to, so a
// later coverage lookup (see review_coverage.go) can match on (org_id, ref).
func (s *Server) insertReviewCommits(ctx context.Context, reviewID, orgID int64, refs []CommitRef) error {
	return insertReviewCommitsTx(ctx, s.db, reviewID, orgID, nil, refs)
}

// insertReviewCommitsTx delegates to reviewprocessor.InsertReviewCommits (shared with internal/jobqueue's scheduled-review worker), converting api.CommitRef (JSON-bound) to its internal equivalent.
func insertReviewCommitsTx(ctx context.Context, db *sql.DB, reviewID, orgID int64, repositoryID *int64, refs []CommitRef) error {
	converted := make([]reviewprocessor.CommitRef, len(refs))
	for i, r := range refs {
		converted[i] = reviewprocessor.CommitRef{Ref: r.Ref, Type: r.Type}
	}
	return reviewprocessor.InsertReviewCommits(ctx, db, reviewID, orgID, repositoryID, converted)
}
