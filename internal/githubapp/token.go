package githubapp

import (
	"database/sql"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	networkgithub "github.com/livereview/network/providers/github"
	storagegithub "github.com/livereview/storage/providers/github"
)

// refreshMargin: a cached installation token that expires sooner than this is replaced before use.
const refreshMargin = 5 * time.Minute

var (
	ErrNotConfigured       = errors.New("github app not configured on this server")
	ErrInstallationRemoved = errors.New("github app installation removed")
)

// ConnectorToken returns a usable API token for any GitHub connector. PAT rows return pat_token unchanged;
// GITHUB_APP rows return the cached installation token, minting a new one when it expires within 5 minutes.
func ConnectorToken(ctx context.Context, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (string, error) {
	return ConnectorTokenWith(ctx, Load(), store, rec)
}

// ConnectorTokenWith is ConnectorToken for callers that already hold the App configuration.
func ConnectorTokenWith(ctx context.Context, cfg *Config, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (string, error) {
	if rec.TokenType != storagegithub.TokenTypeGitHubApp {
		return rec.PatToken, nil
	}
	return connectorToken(ctx, cfg, store, rec, false)
}

// RefreshConnectorToken mints and stores a new installation token regardless of the cached one's age.
func RefreshConnectorToken(ctx context.Context, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (string, error) {
	return connectorToken(ctx, Load(), store, rec, true)
}

type appConnectorMeta struct {
	InstallationID int64 `json:"installation_id"`
	Removed        bool  `json:"removed"`
	Suspended      bool  `json:"suspended"`
}

func connectorToken(ctx context.Context, cfg *Config, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord, force bool) (string, error) {
	var meta appConnectorMeta
	if len(rec.Metadata) > 0 {
		if err := json.Unmarshal(rec.Metadata, &meta); err != nil {
			return "", fmt.Errorf("connector %d: unreadable metadata: %w", rec.ID, err)
		}
	}
	if meta.Removed || meta.Suspended {
		return "", ErrInstallationRemoved
	}
	if !force && rec.PatToken != "" && rec.ExpiresAt.Valid && time.Until(rec.ExpiresAt.Time) > refreshMargin {
		return rec.PatToken, nil
	}

	// Configuration is only needed to mint; a still-valid cached token needs no App credentials.
	if cfg == nil {
		return "", ErrNotConfigured
	}
	installationID := meta.InstallationID
	if installationID == 0 {
		return "", fmt.Errorf("connector %d: no installation id in metadata", rec.ID)
	}

	appJWT, err := networkgithub.MintAppJWT(cfg.AppID, cfg.PrivateKey, time.Now())
	if err != nil {
		return "", fmt.Errorf("mint app jwt: %w", err)
	}
	tok, err := networkgithub.CreateInstallationToken(ctx, cfg.APIBaseURL(), appJWT, installationID)
	if err != nil {
		var apiErr *networkgithub.APIError
		// 404 means the installation no longer exists (uninstalled while we missed the webhook).
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			removed := true
			if flagErr := store.SetAppConnectorFlags(rec.ID, &removed, nil); flagErr != nil {
				return "", fmt.Errorf("mark installation %s removed: %w", strconv.FormatInt(installationID, 10), flagErr)
			}
			return "", ErrInstallationRemoved
		}
		return "", fmt.Errorf("create installation token: %w", err)
	}
	if err := store.UpdateCachedInstallationToken(rec.ID, tok.Token, tok.ExpiresAt); err != nil {
		return "", fmt.Errorf("cache installation token: %w", err)
	}
	rec.PatToken = tok.Token
	rec.ExpiresAt.Time, rec.ExpiresAt.Valid = tok.ExpiresAt, true
	return tok.Token, nil
}

// ResolveIfApp is for code that already read a connector row and holds its stored token: PAT/OAuth rows get
// that token back unchanged, GitHub App rows get a valid installation token (minted when the cached one is near
// expiry; the stored one may be an hour old). connectorID is only used for App rows.
func ResolveIfApp(ctx context.Context, db *sql.DB, connectorID int64, tokenType, stored string) (string, error) {
	if tokenType != storagegithub.TokenTypeGitHubApp {
		return stored, nil
	}
	store := storagegithub.NewTokenStore(db)
	rec, err := store.GetGitHubTokenByConnectorID(connectorID)
	if err != nil {
		return "", err
	}
	return ConnectorToken(ctx, store, rec)
}
