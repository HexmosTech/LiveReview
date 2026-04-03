package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ActorLookupStore struct {
	db *sql.DB
}

func NewActorLookupStore(db *sql.DB) *ActorLookupStore {
	return &ActorLookupStore{db: db}
}

func (s *ActorLookupStore) ResolveOrgMemberUserIDByEmail(ctx context.Context, orgID int64, email string) (*int64, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("missing db handle")
	}
	if orgID <= 0 {
		return nil, fmt.Errorf("org id must be > 0")
	}
	normalizedEmail := strings.TrimSpace(strings.ToLower(email))
	if normalizedEmail == "" {
		return nil, nil
	}

	var userID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		WHERE ur.org_id = $1
		  AND lower(u.email) = $2
		  AND u.is_active = TRUE
		LIMIT 1
	`, orgID, normalizedEmail).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve member user by email: %w", err)
	}
	return &userID, nil
}
