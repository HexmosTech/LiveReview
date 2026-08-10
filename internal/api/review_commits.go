package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// insertReviewCommits records the commit identifiers a review's diff
// corresponds to, so a later coverage lookup (see review_coverage.go) can
// match on (org_id, ref). repositoryID is left NULL when unknown — matching
// is scoped by org_id + ref only, per the review-coverage design.
func (s *Server) insertReviewCommits(ctx context.Context, reviewID, orgID int64, refs []CommitRef) error {
	return insertReviewCommitsTx(ctx, s.db, reviewID, orgID, nil, refs)
}

// insertReviewCommitsTx builds a parameterized multi-row INSERT. Only
// placeholder indices ($1, $2, ...) driven by the loop counter go through
// fmt.Sprintf here — the actual values (ref, refType, ...) are always
// passed as bound args to db.ExecContext, never interpolated into the query
// string, so this is not susceptible to SQL injection.
func insertReviewCommitsTx(ctx context.Context, db *sql.DB, reviewID, orgID int64, repositoryID *int64, refs []CommitRef) error {
	if len(refs) == 0 {
		return nil
	}

	valueRows := make([]string, 0, len(refs))
	args := make([]interface{}, 0, len(refs)*5)
	argN := 1
	for _, r := range refs {
		ref := strings.TrimSpace(r.Ref)
		if ref == "" {
			continue
		}
		refType := strings.TrimSpace(r.Type)
		if refType != "range" {
			refType = "commit"
		}
		valueRows = append(valueRows, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", argN, argN+1, argN+2, argN+3, argN+4))
		args = append(args, reviewID, orgID, repositoryID, ref, refType)
		argN += 5
	}
	if len(valueRows) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO review_commits (review_id, org_id, repository_id, ref, ref_type) VALUES %s ON CONFLICT (review_id, ref) DO NOTHING`,
		strings.Join(valueRows, ", "),
	)
	_, err := db.ExecContext(ctx, query, args...)
	return err
}
