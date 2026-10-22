package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"strings"

	"github.com/livereview/internal/githubapp"
	storagegithub "github.com/livereview/storage/providers/github"
)

// githubPullRepo extracts "owner/repo" from a GitHub pull request URL (/owner/repo/pull/N); "" when it isn't one.
func githubPullRepo(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// preferredGitHubConnector picks the connector for a pull request URL from the org's known repositories, so a
// GitHub App connector beats an older PAT connector for the same repo. It returns nil when the repository is
// not (yet) in the repositories table or the connector is on a different host; the caller then keeps its
// URL-based lookup.
func (s *Server) preferredGitHubConnector(requestURL, baseURL string, orgID int64) *IntegrationToken {
	repo := githubPullRepo(requestURL)
	if repo == "" {
		return nil
	}
	id, err := storagegithub.NewTokenStore(s.db).FindPreferredConnectorForRepo(orgID, repo)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[WARN] preferredGitHubConnector: lookup failed for %s: %v", repo, err)
		}
		return nil
	}
	token, err := s.loadIntegrationTokenByID(id)
	if err != nil || token.OrgID != orgID {
		return nil
	}
	// Same host only: a substring match would accept e.g. https://github.com.example.net for https://github.com.
	if !strings.EqualFold(strings.TrimRight(token.ProviderURL, "/"), strings.TrimRight(baseURL, "/")) {
		return nil
	}
	return token
}

// resolveGitHubAppToken replaces an App connector's cached token with a valid one (minting if it is near
// expiry). PAT connectors are left untouched.
func (s *Server) resolveGitHubAppToken(ctx context.Context, token *IntegrationToken) error {
	if token.TokenType != storagegithub.TokenTypeGitHubApp {
		return nil
	}
	store := storagegithub.NewTokenStore(s.db)
	rec, err := store.GetGitHubTokenByConnectorID(token.ID)
	if err != nil {
		return err
	}
	apiToken, err := githubapp.ConnectorToken(ctx, store, rec)
	if err != nil {
		return err
	}
	token.PatToken = apiToken
	return nil
}

const githubAppManagedMessage = "This connection is managed by the LiveReview GitHub App. Manage repositories on GitHub."

// isGitHubAppConnector reports whether the connector is a GitHub App installation (not a PAT/OAuth connector).
func (s *Server) isGitHubAppConnector(connectorID int) bool {
	var tokenType sql.NullString
	if err := s.db.QueryRow(`SELECT token_type FROM integration_tokens WHERE id = $1`, connectorID).Scan(&tokenType); err != nil {
		return false
	}
	return tokenType.String == storagegithub.TokenTypeGitHubApp
}
