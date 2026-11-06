package notify

import (
	"context"
	"database/sql"
)

// ClaimFirstGitHubConnect atomically marks orgID as having connected GitHub
// for the first time, returning true only for the caller that won the race.
// Same idiom as pushnotify.ClaimFirstFinding: a single atomic
// UPDATE ... WHERE ... IS NULL, never a SELECT-then-UPDATE, so concurrent
// connect attempts for the same org can't both fire the notification.
func ClaimFirstGitHubConnect(ctx context.Context, db *sql.DB, orgID int64) (bool, error) {
	return claimOrgMilestone(ctx, db, "first_github_connect_notified_at", orgID)
}

// ClaimFirstReview is the same idiom as ClaimFirstGitHubConnect, for the
// "org submitted its first review" milestone.
func ClaimFirstReview(ctx context.Context, db *sql.DB, orgID int64) (bool, error) {
	return claimOrgMilestone(ctx, db, "first_review_notified_at", orgID)
}

func claimOrgMilestone(ctx context.Context, db *sql.DB, column string, orgID int64) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE orgs SET `+column+` = NOW()
		WHERE id = $1 AND `+column+` IS NULL`, orgID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
