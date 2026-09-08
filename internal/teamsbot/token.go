package teamsbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenURLFor returns the OAuth2 client-credentials endpoint a bot uses to
// authenticate its own outbound calls to the Bot Framework Connector API
// (posting replies back to Teams). Per Microsoft's Bot Framework Connector
// authentication docs, this differs by the bot's Azure AD app type:
//   - Multi-tenant (or a legacy config with no tenant ID on file): the
//     fixed public "botframework.com" authority, regardless of the bot's
//     own tenant.
//   - Single-tenant: the bot's own tenant-specific authority
//     (https://login.microsoftonline.com/<TenantID>/oauth2/v2.0/token) -
//     the fixed botframework.com endpoint does NOT work for single-tenant
//     bots.
//
// An empty tenantID (today's only case, and any pre-existing multi-tenant
// config saved before Single Tenant support was added) keeps using the
// fixed endpoint, so this is backward compatible with every config saved
// before this function existed.
func tokenURLFor(tenantID string) string {
	if tenantID == "" {
		return "https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token"
	}
	return "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
}

// botFrameworkTokenScope is the fixed audience/scope for Connector API
// tokens - not tenant- or app-specific.
const botFrameworkTokenScope = "https://api.botframework.com/.default"

// cachedToken is one entry in the token cache, keyed by bot App ID.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// tokenCache acquires and caches OAuth2 access tokens for outbound Connector
// API calls, keyed by Bot App ID (each org has its own Azure Bot app
// registration and thus its own token). A single Bot may serve many orgs,
// so this is shared rather than per-org, refreshing lazily on expiry.
type tokenCache struct {
	mu     sync.Mutex
	tokens map[string]cachedToken
	client *http.Client
}

func newTokenCache() *tokenCache {
	return &tokenCache{
		tokens: make(map[string]cachedToken),
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// token returns a valid Bearer token for the given org's bot credentials,
// reusing a cached one if it still has more than a minute of validity left.
// tenantID selects which token endpoint to use (see tokenURLFor) - it isn't
// part of the cache key because an org's appID is already 1:1 with its bot,
// and its tenantID doesn't change without the appID/password changing too.
func (tc *tokenCache) token(ctx context.Context, appID, appPassword, tenantID string) (string, error) {
	if appID == "" || appPassword == "" {
		return "", fmt.Errorf("app ID/password not configured")
	}

	tc.mu.Lock()
	if entry, ok := tc.tokens[appID]; ok && time.Now().Before(entry.expiresAt.Add(-1*time.Minute)) {
		tc.mu.Unlock()
		return entry.accessToken, nil
	}
	tc.mu.Unlock()

	accessToken, expiresIn, err := fetchBotFrameworkToken(ctx, tc.client, appID, appPassword, tenantID)
	if err != nil {
		return "", err
	}

	tc.mu.Lock()
	tc.tokens[appID] = cachedToken{
		accessToken: accessToken,
		expiresAt:   time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	tc.mu.Unlock()

	return accessToken, nil
}

// fetchBotFrameworkToken performs the OAuth2 client-credentials request.
func fetchBotFrameworkToken(ctx context.Context, client *http.Client, appID, appPassword, tenantID string) (string, int, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {appID},
		"client_secret": {appPassword},
		"scope":         {botFrameworkTokenScope},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURLFor(tenantID), strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", 0, fmt.Errorf("decode token response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 300 || body.AccessToken == "" {
		return "", 0, fmt.Errorf("token endpoint returned status %d: %s: %s", resp.StatusCode, body.Error, body.ErrorDesc)
	}

	return body.AccessToken, body.ExpiresIn, nil
}

// ValidateCredentials performs a real OAuth2 client-credentials request
// against Entra ID with the given App ID/Tenant ID/Secret, without caching
// or persisting anything. Used to verify Teams bot credentials at save time
// (before writing them to the DB) rather than discovering a typo only when
// a real Teams message tries to route through them - see AADSTS7000215
// (wrong secret) and AADSTS700016 (wrong tenant) class errors this caught
// live. Returns a description of the specific Azure error on failure.
func ValidateCredentials(ctx context.Context, appID, appPassword, tenantID string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	_, _, err := fetchBotFrameworkToken(ctx, client, appID, appPassword, tenantID)
	return err
}

// authorizationHeader returns "Bearer <token>" for the given org's bot
// credentials, or "" if a token couldn't be acquired (missing/placeholder
// credentials, network issue, etc.) - callers treat that as "send
// unauthenticated, best-effort" rather than a hard failure, since local
// testing tools (emulators, the 365 Agents Playground with auth disabled)
// don't require it, only real Bot Framework channels do.
func (tc *tokenCache) authorizationHeader(ctx context.Context, appID, appPassword, tenantID string) string {
	token, err := tc.token(ctx, appID, appPassword, tenantID)
	if err != nil {
		log.Printf("[TeamsBot] Could not acquire outbound auth token for app %s, sending unauthenticated: %v", appID, err)
		return ""
	}
	return "Bearer " + token
}
