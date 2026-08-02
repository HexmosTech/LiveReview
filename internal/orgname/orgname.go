// Package orgname provides a shared helper for resolving an organization's name.
package orgname

import (
	"context"
	"database/sql"
	"errors"
)

// OrgNameByID returns the organization name for orgID. It returns ("", nil) when
// the organization does not exist, and ("", err) on an actual database failure so
// callers can distinguish "not found" from a real error.
func OrgNameByID(ctx context.Context, db *sql.DB, orgID int64) (string, error) {
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM orgs WHERE id = $1`, orgID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return name, nil
}
