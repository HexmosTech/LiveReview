package policyengine

import (
	"context"
	"database/sql"
	"strings"
)

// AuthorInfo describes how the PR author was resolved to a review level.
type AuthorInfo struct {
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Provider string `json:"provider,omitempty"`
	IsBot    bool   `json:"is_bot"`
	UserID   int64  `json:"user_id,omitempty"` // 0 = unmapped to a LiveReview user
	Level    int    `json:"level"`             // resolved level (default 1 when unmapped)
	Matched  bool   `json:"matched"`           // a user_identities mapping was found
}

// resolveAuthor maps the PR author to a LiveReview user (via user_identities) and
// resolves their review_level. Unmapped non-bot authors default to level 1
// (fail-closed: the lowest bar); bots default to level 0.
func (e *Engine) resolveAuthor(ctx context.Context, orgID, reviewID int64) (*AuthorInfo, error) {
	var prID sql.NullInt64
	if err := e.db.QueryRowContext(ctx,
		`SELECT pull_request_id FROM reviews WHERE id = $1 AND org_id = $2`, reviewID, orgID).Scan(&prID); err != nil {
		return nil, err
	}

	var username, email, provider string
	if prID.Valid {
		var u, em, p sql.NullString
		err := e.db.QueryRowContext(ctx,
			`SELECT COALESCE(author_username, ''), COALESCE(author_email, ''), COALESCE(provider, '')
			 FROM pull_requests WHERE id = $1 AND org_id = $2`, prID.Int64, orgID).Scan(&u, &em, &p)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		username, email, provider = u.String, em.String, p.String
	} else {
		// No linked PR: fall back to the review's own author fields.
		var u, em, p sql.NullString
		if err := e.db.QueryRowContext(ctx,
			`SELECT COALESCE(author_username, ''), COALESCE(user_email, ''), COALESCE(provider, '')
			 FROM reviews WHERE id = $1 AND org_id = $2`, reviewID, orgID).Scan(&u, &em, &p); err != nil {
			return nil, err
		}
		username, email, provider = u.String, em.String, p.String
	}

	ai := &AuthorInfo{
		Username: strings.TrimSpace(username),
		Email:    strings.ToLower(strings.TrimSpace(email)),
		Provider: provider,
		Level:    1, // fail-closed default
	}
	ai.IsBot = isBotLogin(ai.Username)

	// Look up user_identities by username and (if present) email.
	usernameKey := strings.ToLower(strings.TrimSpace(username))
	emailKey := ai.Email

	var userID sql.NullInt64
	err := e.db.QueryRowContext(ctx, `
		SELECT user_id FROM user_identities
		WHERE org_id = $1 AND kind = 'username' AND value = $2
		LIMIT 1`, orgID, usernameKey).Scan(&userID)
	if err == nil && userID.Valid {
		return e.withMappedUser(ctx, ai, userID.Int64)
	}
	if emailKey != "" {
		err = e.db.QueryRowContext(ctx, `
			SELECT user_id FROM user_identities
			WHERE org_id = $1 AND kind = 'email' AND value = $2
			LIMIT 1`, orgID, emailKey).Scan(&userID)
		if err == nil && userID.Valid {
			return e.withMappedUser(ctx, ai, userID.Int64)
		}
	}

	return ai, nil
}

// withMappedUser fills in the mapped user's level (bots keep level 0 unless the
// mapping makes them a real user, in which case their level applies).
func (e *Engine) withMappedUser(ctx context.Context, ai *AuthorInfo, userID int64) (*AuthorInfo, error) {
	var level int
	if err := e.db.QueryRowContext(ctx, `SELECT review_level FROM users WHERE id = $1`, userID).Scan(&level); err != nil {
		return nil, err
	}
	ai.UserID = userID
	ai.Matched = true
	ai.Level = level
	return ai, nil
}

// isBotLogin detects provider bot accounts (GitHub's "[bot]" suffix, and common
// automation suffixes) from a login.
func isBotLogin(username string) bool {
	u := strings.ToLower(strings.TrimSpace(username))
	if u == "" {
		return false
	}
	return strings.HasSuffix(u, "[bot]") ||
		strings.HasSuffix(u, "-bot") ||
		strings.HasSuffix(u, "_bot")
}
