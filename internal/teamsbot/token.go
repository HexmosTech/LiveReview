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

// botFrameworkTokenURL is the OAuth2 client-credentials endpoint bots use to
// authenticate their own outbound calls to the Bot Framework Connector API
// (posting replies back to Teams/Slack-style channels). This is Microsoft's
// well-known multi-tenant "botframework.com" endpoint used for Connector API
// auth specifically - distinct from a bot's own Azure AD tenant, and the
// same regardless of whether the underlying Azure Bot app registration is
// single- or multi-tenant.
const botFrameworkTokenURL = "https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token"

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

// token returns a valid Bearer token for the given App ID/secret pair,
// reusing a cached one if it still has more than a minute of validity left.
func (tc *tokenCache) token(ctx context.Context, appID, appPassword string) (string, error) {
	if appID == "" || appPassword == "" {
		return "", fmt.Errorf("app ID/password not configured")
	}

	tc.mu.Lock()
	if entry, ok := tc.tokens[appID]; ok && time.Now().Before(entry.expiresAt.Add(-1*time.Minute)) {
		tc.mu.Unlock()
		return entry.accessToken, nil
	}
	tc.mu.Unlock()

	accessToken, expiresIn, err := fetchBotFrameworkToken(ctx, tc.client, appID, appPassword)
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
func fetchBotFrameworkToken(ctx context.Context, client *http.Client, appID, appPassword string) (string, int, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {appID},
		"client_secret": {appPassword},
		"scope":         {botFrameworkTokenScope},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, botFrameworkTokenURL, strings.NewReader(form.Encode()))
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

// authorizationHeader returns "Bearer <token>" for the given org's bot
// credentials, or "" if a token couldn't be acquired (missing/placeholder
// credentials, network issue, etc.) - callers treat that as "send
// unauthenticated, best-effort" rather than a hard failure, since local
// testing tools (emulators, the 365 Agents Playground with auth disabled)
// don't require it, only real Bot Framework channels do.
func (tc *tokenCache) authorizationHeader(ctx context.Context, appID, appPassword string) string {
	token, err := tc.token(ctx, appID, appPassword)
	if err != nil {
		log.Printf("[TeamsBot] Could not acquire outbound auth token for app %s, sending unauthenticated: %v", appID, err)
		return ""
	}
	return "Bearer " + token
}
