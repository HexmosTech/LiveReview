// Package githubapp holds configuration and token handling for the LiveReview GitHub App. It makes no
// outbound calls itself; HTTP lives in network/providers/github and SQL in storage/providers/github.
package githubapp

import (
	"crypto/rsa"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"

	githubprovider "github.com/livereview/internal/providers/github"
)

const defaultBaseURL = "https://github.com"

// Config is the GitHub App configuration read from GITHUB_APP_* environment variables.
type Config struct {
	AppID         int64
	Slug          string
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	BaseURL       string // e.g. https://github.com, no trailing slash
	PrivateKey    *rsa.PrivateKey
}

var (
	loadOnce sync.Once
	loaded   *Config
)

// Load reads GITHUB_APP_* once. It returns nil when the feature is off (GITHUB_APP_ID unset) or
// misconfigured; a misconfiguration is logged once, naming what is wrong.
func Load() *Config {
	loadOnce.Do(func() {
		cfg, err := fromEnv()
		if err != nil {
			log.Printf("[GitHubApp] disabled: %v", err)
			return
		}
		if cfg != nil {
			log.Printf("[GitHubApp] enabled: app_id=%d slug=%s base_url=%s", cfg.AppID, cfg.Slug, cfg.BaseURL)
		}
		loaded = cfg
	})
	return loaded
}

// fromEnv returns (nil, nil) when GITHUB_APP_ID is empty (feature off), and an error when it is set but
// the rest of the configuration is incomplete or unparsable.
func fromEnv() (*Config, error) {
	rawID := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	if rawID == "" {
		return nil, nil
	}

	var missing []string
	need := func(name string) string {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	cfg := &Config{
		Slug:          need("GITHUB_APP_SLUG"),
		ClientID:      need("GITHUB_APP_CLIENT_ID"),
		ClientSecret:  need("GITHUB_APP_CLIENT_SECRET"),
		WebhookSecret: need("GITHUB_APP_WEBHOOK_SECRET"),
	}
	rawKey := need("GITHUB_APP_PRIVATE_KEY")
	if len(missing) > 0 {
		return nil, fmt.Errorf("GITHUB_APP_ID is set but missing %s", strings.Join(missing, ", "))
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("GITHUB_APP_ID must be a positive integer, got %q", rawID)
	}
	cfg.AppID = id

	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_APP_BASE_URL")), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	key, err := parsePrivateKey(rawKey)
	if err != nil {
		return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY: %w", err)
	}
	cfg.PrivateKey = key
	return cfg, nil
}

// parsePrivateKey accepts either the PEM text (env files often store it with literal \n) or a file path.
func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	pem := raw
	if !strings.HasPrefix(strings.TrimSpace(raw), "-----BEGIN") {
		b, err := os.ReadFile(raw)
		if err != nil {
			return nil, fmt.Errorf("not a PEM and cannot be read as a file: %w", err)
		}
		pem = string(b)
	}
	pem = strings.ReplaceAll(pem, `\n`, "\n")
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pem))
	if err != nil {
		return nil, fmt.Errorf("invalid RSA private key: %w", err)
	}
	return key, nil
}

// APIBaseURL is the REST API base for this GitHub host (api.github.com, or <host>/api/v3 on GHES).
func (c *Config) APIBaseURL() string { return githubprovider.APIBaseURL(c.BaseURL) }

// IsGitHubDotCom reports whether the App lives on github.com rather than a GitHub Enterprise Server.
func (c *Config) IsGitHubDotCom() bool { return c.BaseURL == defaultBaseURL }

// BotLogin is the login GitHub gives the App's bot account.
func (c *Config) BotLogin() string { return c.Slug + "[bot]" }

// InstallURL is where a new installation starts; GHES serves the same page under /github-apps/.
func (c *Config) InstallURL(state string) string {
	prefix := "/apps/"
	if !c.IsGitHubDotCom() {
		prefix = "/github-apps/"
	}
	return fmt.Sprintf("%s%s%s/installations/new?state=%s", c.BaseURL, prefix, url.PathEscape(c.Slug), url.QueryEscape(state))
}

// InstallURLForTarget starts an installation with the GitHub account or organization already chosen (targetID is
// its numeric GitHub id), skipping the "pick an account" screen.
func (c *Config) InstallURLForTarget(state string, targetID int64) string {
	prefix := "/apps/"
	if !c.IsGitHubDotCom() {
		prefix = "/github-apps/"
	}
	return fmt.Sprintf("%s%s%s/installations/new/permissions?target_id=%d&state=%s", c.BaseURL, prefix, url.PathEscape(c.Slug), targetID, url.QueryEscape(state))
}

// AuthorizeURL is the OAuth authorize page, used when the App is already installed.
func (c *Config) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("state", state)
	return c.BaseURL + "/login/oauth/authorize?" + q.Encode()
}

// InstallationSettingsURL is the GitHub page where the owner manages which repositories the App can see.
func (c *Config) InstallationSettingsURL(accountType, accountLogin string, installationID int64) string {
	if strings.EqualFold(accountType, "Organization") {
		return fmt.Sprintf("%s/organizations/%s/settings/installations/%d", c.BaseURL, url.PathEscape(accountLogin), installationID)
	}
	return fmt.Sprintf("%s/settings/installations/%d", c.BaseURL, installationID)
}
