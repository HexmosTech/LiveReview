// Package hexmos holds outbound calls to Hexmos' fw-parse (Parse Server) used for cloud identity.
package hexmos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ParseAppID is the Parse application id sent as X-Parse-Application-Id.
func ParseAppID() string {
	if id := os.Getenv("FW_PARSE_APP_ID"); id != "" {
		return id
	}
	return "impressionserver"
}

// ParseUser is the user fields LiveReview needs from a Parse signup result.
type ParseUser struct {
	Email         string `json:"email"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Username      string `json:"username"`
	ProfilePicURL string `json:"profilePicUrl"`
}

// ParseSignupResult is the outcome of externalProviderSignup: a Hexmos JWT for the Parse user.
type ParseSignupResult struct {
	JWT  string
	User ParseUser
}

// ExternalProviderSignup calls Parse's externalProviderSignup function, which finds or creates the Parse _User
// for the token's identity and returns a Hexmos JWT - the same function the Hexmos onelogin uses. For provider
// "github_app", accessToken is an already-exchanged GitHub App user token.
func ExternalProviderSignup(ctx context.Context, provider, accessToken, app string) (*ParseSignupResult, error) {
	baseURL := strings.TrimRight(os.Getenv("FW_PARSE_BASE_URL"), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("FW_PARSE_BASE_URL is not set")
	}
	endpoint := baseURL + "/parse/functions/externalProviderSignup"

	payload, err := json.Marshal(map[string]interface{}{
		"externalServiceProvider": provider,
		"accessToken":             accessToken,
		"client_details":          map[string]string{"application": app, "deviceUniqueId": "default"},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Parse-Application-Id", ParseAppID())
	if secret := os.Getenv("FW_PARSE_ADMIN_SECRET"); secret != "" {
		req.Header.Set("X-Internal-Admin-Secret", secret)
	}

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call fw-parse: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fw-parse returned %d", resp.StatusCode)
	}

	// Parse reports business failures as HTTP 200 {"result":{"success":false,"message":...}}, so success is a
	// non-empty JWT, not the status code.
	var out struct {
		Result struct {
			JWT     string    `json:"jwt"`
			Success *bool     `json:"success"`
			Message string    `json:"message"`
			Data    ParseUser `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode fw-parse response: %w", err)
	}
	if out.Result.JWT == "" {
		return nil, fmt.Errorf("fw-parse signup failed: %s", out.Result.Message)
	}
	return &ParseSignupResult{JWT: out.Result.JWT, User: out.Result.Data}, nil
}
