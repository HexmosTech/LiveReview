package sso

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Identity is the normalized claim set extracted from a successful SSO
// login, OIDC or SAML - CompleteSSOLogin (internal/api/auth) only ever deals
// with this, never protocol-specific structures.
type Identity struct {
	Email string
	Name  string
	// Subject is the IdP's own identifier for this identity (OIDC `sub` /
	// SAML NameID). Needed to resolve an incoming OIDC back-channel logout
	// token or SAML LogoutRequest - which only carry this, never a
	// LiveReview user id - back to a user (see internal/sso identity_store.go
	// and Phase 4 of docs/sso.md's deprovisioning section).
	Subject string
	// RefreshToken is only populated for OIDC configs with the
	// reconciliation poll enabled (see Config.ReconciliationEnabled) - it
	// requests the "offline_access" scope specifically so the poll can
	// silently re-check "is this account still valid" between logins.
	RefreshToken string
}

// oidcClaims mirrors the standard OIDC claims LiveReview needs. Most IdPs
// (Okta, Azure AD/Entra, Google Workspace, Keycloak) populate all three.
type oidcClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// BuildOIDCOAuth2Config runs OIDC discovery against the configured issuer
// and returns an oauth2.Config plus the ID-token verifier for that issuer.
// Discovery happens per login (not cached) - login traffic is low-volume
// and this keeps the design simple; an admin rotating issuer URLs needs no
// cache invalidation story.
func BuildOIDCOAuth2Config(ctx context.Context, cfg *Config, redirectURL string) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	if cfg.ProviderType != ProviderOIDC {
		return nil, nil, fmt.Errorf("sso: config %d is not an OIDC config", cfg.ID)
	}
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, nil, fmt.Errorf("sso: oidc discovery against %q failed: %w", cfg.OIDCIssuer, err)
	}
	scopes := []string{oidc.ScopeOpenID, "email", "profile"}
	if strings.TrimSpace(cfg.OIDCScopes) != "" {
		scopes = strings.Fields(cfg.OIDCScopes)
	}
	if cfg.ReconciliationEnabled && !containsFold(scopes, "offline_access") {
		scopes = append(scopes, "offline_access")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	return oauthCfg, verifier, nil
}

// ExchangeOIDCCode exchanges the authorization code for tokens, verifies the
// ID token's signature/issuer/audience/expiry, and extracts the identity.
func ExchangeOIDCCode(ctx context.Context, oauthCfg *oauth2.Config, verifier *oidc.IDTokenVerifier, code string) (*Identity, error) {
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("sso: oidc code exchange failed: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("sso: oidc token response missing id_token")
	}
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("sso: oidc id_token verification failed: %w", err)
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("sso: parsing oidc id_token claims: %w", err)
	}
	if claims.Email == "" {
		return nil, fmt.Errorf("sso: oidc id_token has no email claim")
	}
	return &Identity{
		Email:        claims.Email,
		Name:         claims.Name,
		Subject:      idToken.Subject,
		RefreshToken: token.RefreshToken,
	}, nil
}

// backChannelLogoutEvent is the OIDC Back-Channel Logout 1.0 event URI a
// valid logout_token's "events" claim must contain.
const backChannelLogoutEvent = "http://schemas.openid.net/event/backchannel-logout"

type backChannelLogoutClaims struct {
	Subject string                 `json:"sub"`
	Events  map[string]interface{} `json:"events"`
}

// VerifyBackChannelLogoutToken verifies an incoming OIDC back-channel
// logout token (RFC: OIDC Back-Channel Logout 1.0) the same way an ID
// token is verified - same issuer, signature (JWKS), audience, and expiry
// checks via go-oidc - then confirms the logout-specific "events" claim is
// present, and returns the subject whose sessions should be revoked.
func VerifyBackChannelLogoutToken(ctx context.Context, cfg *Config, rawToken string) (string, error) {
	if cfg.ProviderType != ProviderOIDC {
		return "", fmt.Errorf("sso: config %d is not an OIDC config", cfg.ID)
	}
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return "", fmt.Errorf("sso: oidc discovery against %q failed: %w", cfg.OIDCIssuer, err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	idToken, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return "", fmt.Errorf("sso: logout_token verification failed: %w", err)
	}
	var claims backChannelLogoutClaims
	if err := idToken.Claims(&claims); err != nil {
		return "", fmt.Errorf("sso: parsing logout_token claims: %w", err)
	}
	if _, ok := claims.Events[backChannelLogoutEvent]; !ok {
		return "", fmt.Errorf("sso: logout_token missing required backchannel-logout event claim")
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("sso: logout_token has no sub claim")
	}
	return claims.Subject, nil
}

// TestOIDCDiscovery is used by the config "test connection" endpoint - it
// only proves the issuer is reachable and speaks OIDC discovery, without a
// full login round trip.
func TestOIDCDiscovery(ctx context.Context, issuer string) error {
	_, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return fmt.Errorf("sso: oidc discovery against %q failed: %w", issuer, err)
	}
	return nil
}
