package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GitHub App calls. Every function takes its API/web base URL from the caller (config), never a literal host.

const appHTTPTimeout = 15 * time.Second

// APIError is a non-2xx GitHub response.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api returned %d: %s", e.Status, e.Body)
}

func doJSON(ctx context.Context, method, requestURL, authHeader string, form url.Values, wantStatus int, out interface{}) (*http.Response, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "LiveReview")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
	}

	resp, err := Do(NewHTTPClient(appHTTPTimeout), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != wantStatus {
		return resp, &APIError{Status: resp.StatusCode, Body: truncate(string(raw), 300)}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp, fmt.Errorf("decode github response: %w", err)
		}
	}
	return resp, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// nextLink returns the rel="next" URL, or "" when there is none or it points at a different host than
// apiBase (so a bearer token is never replayed to a host GitHub did not name in the API base).
func nextLink(resp *http.Response, apiBase string) string {
	if resp == nil {
		return ""
	}
	m := nextLinkRe.FindStringSubmatch(resp.Header.Get("Link"))
	if m == nil {
		return ""
	}
	next, err := url.Parse(m[1])
	if err != nil {
		return ""
	}
	base, err := url.Parse(apiBase)
	if err != nil || next.Host != base.Host {
		return ""
	}
	return m[1]
}

// MintAppJWT signs the short-lived RS256 JWT that authenticates as the App itself.
func MintAppJWT(appID int64, key *rsa.PrivateKey, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    strconv.FormatInt(appID, 10),
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // allow for clock drift
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),   // GitHub rejects > 10 minutes
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
}

type InstallationToken struct {
	Token     string
	ExpiresAt time.Time
}

// CreateInstallationToken mints a 1-hour token scoped to one installation.
func CreateInstallationToken(ctx context.Context, apiBase, appJWT string, installationID int64) (*InstallationToken, error) {
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	endpoint := fmt.Sprintf("%s/app/installations/%d/access_tokens", apiBase, installationID)
	if _, err := doJSON(ctx, http.MethodPost, endpoint, "Bearer "+appJWT, nil, http.StatusCreated, &out); err != nil {
		return nil, err
	}
	if out.Token == "" {
		return nil, fmt.Errorf("github returned an empty installation token")
	}
	return &InstallationToken{Token: out.Token, ExpiresAt: out.ExpiresAt}, nil
}

type UserToken struct {
	AccessToken string
}

// ExchangeUserCode trades the OAuth code for a user token. GitHub answers 200 with {"error": ...} on
// failure (e.g. an expired or reused code), so that is checked explicitly.
func ExchangeUserCode(ctx context.Context, base, clientID, clientSecret, code string) (*UserToken, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	endpoint := strings.TrimRight(base, "/") + "/login/oauth/access_token"
	if _, err := doJSON(ctx, http.MethodPost, endpoint, "", form, http.StatusOK, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("github oauth: %s (%s)", out.Error, out.Description)
	}
	if out.AccessToken == "" {
		return nil, fmt.Errorf("github oauth: no access token in response")
	}
	return &UserToken{AccessToken: out.AccessToken}, nil
}

type UserEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func ListUserEmails(ctx context.Context, apiBase, userToken string) ([]UserEmail, error) {
	var out []UserEmail
	if _, err := doJSON(ctx, http.MethodGet, apiBase+"/user/emails", "Bearer "+userToken, nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type UserProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// GetAuthenticatedUser calls GET /user. It only works with a user token; installation tokens get a 403.
func GetAuthenticatedUser(ctx context.Context, apiBase, userToken string) (*UserProfile, error) {
	var out UserProfile
	if _, err := doJSON(ctx, http.MethodGet, apiBase+"/user", "Bearer "+userToken, nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type Installation struct {
	ID                  int64
	AppID               int64
	AccountLogin        string
	AccountType         string
	RepositorySelection string
	Suspended           bool
}

// ListUserInstallations returns the installations (of every app) the user can access, following pagination.
func ListUserInstallations(ctx context.Context, apiBase, userToken string) ([]Installation, error) {
	var all []Installation
	next := apiBase + "/user/installations?per_page=100"
	for page := 0; next != "" && page < 20; page++ {
		var out struct {
			Installations []struct {
				ID      int64 `json:"id"`
				AppID   int64 `json:"app_id"`
				Account struct {
					Login string `json:"login"`
					Type  string `json:"type"`
				} `json:"account"`
				RepositorySelection string     `json:"repository_selection"`
				SuspendedAt         *time.Time `json:"suspended_at"`
			} `json:"installations"`
		}
		resp, err := doJSON(ctx, http.MethodGet, next, "Bearer "+userToken, nil, http.StatusOK, &out)
		if err != nil {
			return nil, err
		}
		for _, i := range out.Installations {
			all = append(all, Installation{
				ID: i.ID, AppID: i.AppID, AccountLogin: i.Account.Login, AccountType: i.Account.Type,
				RepositorySelection: i.RepositorySelection, Suspended: i.SuspendedAt != nil,
			})
		}
		next = nextLink(resp, apiBase)
	}
	return all, nil
}

// ListUserInstallationRepositoryIDs returns the ids of the installation's repositories that the user can access,
// following pagination (GitHub decides access: org base permission, teams, collaborator grants).
func ListUserInstallationRepositoryIDs(ctx context.Context, apiBase, userToken string, installationID int64) ([]int64, error) {
	ids := []int64{}
	next := fmt.Sprintf("%s/user/installations/%d/repositories?per_page=100", apiBase, installationID)
	for page := 0; next != ""; page++ {
		if page == 100 {
			return nil, fmt.Errorf("installation %d lists more than 10000 repositories for this user", installationID)
		}
		var out struct {
			Repositories []struct {
				ID int64 `json:"id"`
			} `json:"repositories"`
		}
		resp, err := doJSON(ctx, http.MethodGet, next, "Bearer "+userToken, nil, http.StatusOK, &out)
		if err != nil {
			return nil, err
		}
		for _, r := range out.Repositories {
			ids = append(ids, r.ID)
		}
		next = nextLink(resp, apiBase)
	}
	return ids, nil
}

type InstallationRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
}

// ListInstallationRepositories returns one page of the repositories an installation token can see. Pass the
// previous call's next cursor back in; an empty cursor fetches the first page.
func ListInstallationRepositories(ctx context.Context, apiBase, installationToken, cursor string) ([]InstallationRepo, string, error) {
	endpoint := cursor
	if endpoint == "" {
		endpoint = apiBase + "/installation/repositories?per_page=100"
	}
	var out struct {
		Repositories []InstallationRepo `json:"repositories"`
	}
	resp, err := doJSON(ctx, http.MethodGet, endpoint, "Bearer "+installationToken, nil, http.StatusOK, &out)
	if err != nil {
		return nil, "", err
	}
	return out.Repositories, nextLink(resp, apiBase), nil
}

type OpenPull struct {
	Number      int
	Title       string
	HTMLURL     string
	AuthorLogin string
	UpdatedAt   time.Time
}

func ListOpenPulls(ctx context.Context, apiBase, installationToken, repoFullName string, limit int) ([]OpenPull, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	var out []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		HTMLURL   string    `json:"html_url"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	endpoint := fmt.Sprintf("%s/repos/%s/pulls?state=open&sort=updated&direction=desc&per_page=%d", apiBase, repoFullName, limit)
	if _, err := doJSON(ctx, http.MethodGet, endpoint, "Bearer "+installationToken, nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	pulls := make([]OpenPull, 0, len(out))
	for _, p := range out {
		pulls = append(pulls, OpenPull{Number: p.Number, Title: p.Title, HTMLURL: p.HTMLURL, AuthorLogin: p.User.Login, UpdatedAt: p.UpdatedAt})
	}
	return pulls, nil
}

type PublicUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// GetUserByLogin looks up a public account; the App's bot account (<slug>[bot]) is resolved this way.
func GetUserByLogin(ctx context.Context, apiBase, token, login string) (*PublicUser, error) {
	var out PublicUser
	if _, err := doJSON(ctx, http.MethodGet, apiBase+"/users/"+url.PathEscape(login), "Bearer "+token, nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteInstallation uninstalls the App from the account or organization (what "Uninstall" does on GitHub). It
// authenticates as the App with its JWT. An installation that is already gone (404) counts as success.
func DeleteInstallation(ctx context.Context, apiBase, appJWT string, installationID int64) error {
	endpoint := fmt.Sprintf("%s/app/installations/%d", apiBase, installationID)
	_, err := doJSON(ctx, http.MethodDelete, endpoint, "Bearer "+appJWT, nil, http.StatusNoContent, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}
