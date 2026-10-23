package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/database"
	"github.com/livereview/internal/githubapp"
	storagegithub "github.com/livereview/storage/providers/github"
)

const (
	ghTestAppID       = 987654
	ghTestCloudSecret = "gh-app-test-cloud-secret"
	ghTestWebhookSec  = "whsec-test"
)

// fakeGitHub plays github.com + api for one test: a GHES-style host, so the API lives under /api/v3.
type fakeGitHub struct {
	mu            sync.Mutex
	emails        string // JSON for GET /user/emails
	installations string // JSON array body of "installations"
	userRepos     string // JSON array body of "repositories" for GET /user/installations/{id}/repositories
	instRepos     string // JSON array body of "repositories" for GET /installation/repositories
	deleted       []string // installation ids the App was asked to uninstall
	failDelete    bool
	srv           *httptest.Server
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{
		emails:        `[{"email":"Gh.User@Example.test","primary":true,"verified":true}]`,
		installations: "[]",
		userRepos:     `[{"id":1}]`,
		instRepos:     `[{"id":1,"name":"api","full_name":"acme/api","html_url":"http://x/acme/api","private":true}]`,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/login/oauth/access_token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "good-code" {
				_, _ = fmt.Fprint(w, `{"error":"bad_verification_code","error_description":"bad"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"access_token":"ghu_testuser"}`)
		case r.URL.Path == "/api/v3/user/emails":
			_, _ = fmt.Fprint(w, f.emails)
		case r.URL.Path == "/api/v3/user/installations":
			_, _ = fmt.Fprintf(w, `{"installations":%s}`, f.installations)
		case strings.HasPrefix(r.URL.Path, "/api/v3/user/installations/") && strings.HasSuffix(r.URL.Path, "/repositories"):
			_, _ = fmt.Fprintf(w, `{"repositories":%s}`, f.userRepos)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v3/app/installations/"):
			f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/v3/app/installations/"))
			if f.failDelete {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/api/v3/app/installations/") && strings.HasSuffix(r.URL.Path, "/access_tokens"):
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"token":"ghs_inst","expires_at":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		case r.URL.Path == "/api/v3/users/newco":
			_, _ = fmt.Fprint(w, `{"id":777,"login":"newco"}`)
		case r.URL.Path == "/api/v3/installation/repositories":
			_, _ = fmt.Fprintf(w, `{"repositories":%s}`, f.instRepos)
		case r.URL.Path == "/api/v3/repos/acme/api/pulls":
			_, _ = fmt.Fprint(w, `[{"number":7,"title":"Fix","html_url":"http://x/acme/api/pull/7","updated_at":"2026-10-19T10:00:00Z","user":{"login":"octo"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) installation(id int64, login string) string {
	return fmt.Sprintf(`[{"id":%d,"app_id":%d,"account":{"login":%q,"type":"Organization"},"repository_selection":"selected","suspended_at":null}]`, id, ghTestAppID, login)
}

// fakeParse plays fw-parse: it "signs up" the GitHub user and returns a Hexmos JWT for the given email.
func newFakeParse(t *testing.T, email string, fail bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			_, _ = fmt.Fprint(w, `{"result":{"success":false,"message":"nope"}}`)
			return
		}
		claims := auth.JWTClaims{Email: email, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
		signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(ghTestCloudSecret))
		_, _ = fmt.Fprintf(w, `{"result":{"jwt":%q,"data":{"email":%q,"first_name":"Gh","last_name":"User","username":%q}}}`, signed, email, email)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FW_PARSE_BASE_URL", srv.URL)
}

func newGitHubAppTestServer(t *testing.T, f *fakeGitHub) (*Server, *sql.DB) {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	t.Setenv("CLOUD_JWT_SECRET", ghTestCloudSecret)
	t.Setenv("DISCORD_SIGNUP_WEBHOOK_URL", "")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:               db,
		deploymentConfig: &DeploymentConfig{IsCloud: true},
		authHandlers:     auth.NewAuthHandlers(auth.NewTokenService(db, "x"), db),
		githubAppSecret:  []byte("state-secret"),
		githubApp: &githubapp.Config{
			AppID: ghTestAppID, Slug: "lr-test", ClientID: "cid", ClientSecret: "csec", WebhookSecret: ghTestWebhookSec,
			BaseURL: f.srv.URL, PrivateKey: key,
		},
	}
	return s, db
}

// cleanupGitHubAppTenant removes the user, personal org and connectors created for an email.
func cleanupGitHubAppTenant(t *testing.T, db *sql.DB, email string) {
	t.Helper()
	t.Cleanup(func() {
		var userID, orgID sql.NullInt64
		_ = db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
		_ = db.QueryRow(`SELECT id FROM orgs WHERE name = $1`, email).Scan(&orgID)
		if orgID.Valid {
			_, _ = db.Exec(`DELETE FROM repositories WHERE org_id = $1`, orgID.Int64)
			_, _ = db.Exec(`DELETE FROM integration_tokens WHERE org_id = $1`, orgID.Int64)
		}
		if userID.Valid {
			_, _ = db.Exec(`DELETE FROM auth_tokens WHERE user_id = $1`, userID.Int64)
			_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1`, userID.Int64)
			_, _ = db.Exec(`UPDATE users SET default_org_id = NULL WHERE id = $1`, userID.Int64)
		}
		if orgID.Valid {
			_, _ = db.Exec(`DELETE FROM prompt_application_context WHERE org_id = $1`, orgID.Int64)
			_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID.Int64)
		}
		if userID.Valid {
			_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID.Int64)
		}
	})
}

type callbackParams struct {
	state        string
	cookieNonce  string
	code         string
	installation string
	setupAction  string
}

func (s *Server) doCallback(t *testing.T, p callbackParams) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{}
	if p.state != "" {
		q.Set("state", p.state)
	}
	if p.code != "" {
		q.Set("code", p.code)
	}
	if p.installation != "" {
		q.Set("installation_id", p.installation)
	}
	if p.setupAction != "" {
		q.Set("setup_action", p.setupAction)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github-app/callback?"+q.Encode(), nil)
	if p.cookieNonce != "" {
		req.AddCookie(&http.Cookie{Name: ghStateCookieName, Value: p.cookieNonce})
	}
	rec := httptest.NewRecorder()
	if err := s.GitHubAppCallback(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	return rec
}

func (s *Server) validState(t *testing.T, mode string) (state, nonce string) {
	t.Helper()
	nonce = "nonce-" + fmt.Sprint(time.Now().UnixNano())
	state, err := signGHState(ghState{Nonce: nonce, Src: "test", Mode: mode, Exp: time.Now().Add(time.Minute).Unix()}, s.githubAppSecret)
	if err != nil {
		t.Fatal(err)
	}
	return state, nonce
}

func TestGitHubAppStart_RedirectsToInstallOrAuthorize(t *testing.T) {
	f := newFakeGitHub(t)
	s, _ := newGitHubAppTestServer(t, f)

	for mode, wantPath := range map[string]string{"": "/github-apps/lr-test/installations/new", "signin": "/login/oauth/authorize"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github-app/start?src=Home&mode="+mode, nil)
		rec := httptest.NewRecorder()
		if err := s.GitHubAppStart(echo.New().NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusFound {
			t.Fatalf("mode %q: status %d", mode, rec.Code)
		}
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if loc.Path != wantPath {
			t.Errorf("mode %q: redirected to %s, want path %s", mode, loc.Path, wantPath)
		}
		st, err := verifyGHState(loc.Query().Get("state"), s.githubAppSecret, time.Now())
		if err != nil || st.Src != "home" {
			t.Fatalf("mode %q: bad state in redirect: %v %+v", mode, err, st)
		}
		var cookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == ghStateCookieName {
				cookie = c
			}
		}
		if cookie == nil || cookie.Value != st.Nonce || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("mode %q: state cookie wrong: %+v", mode, cookie)
		}
	}
}

func TestGitHubAppCallback_RejectsBadStateAndCookie(t *testing.T) {
	f := newFakeGitHub(t)
	s, _ := newGitHubAppTestServer(t, f)
	state, nonce := s.validState(t, ghModeInstall)
	expired, _ := signGHState(ghState{Nonce: "n", Exp: time.Now().Add(-time.Minute).Unix()}, s.githubAppSecret)

	cases := map[string]callbackParams{
		"no cookie":       {state: state, code: "good-code"},
		"cookie mismatch": {state: state, cookieNonce: "different", code: "good-code"},
		"expired state":   {state: expired, cookieNonce: "n", code: "good-code"},
		"no state":        {cookieNonce: nonce, code: "good-code"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			rec := s.doCallback(t, p)
			if rec.Code != http.StatusBadRequest || strings.Contains(rec.Header().Get("Location"), "data=") {
				t.Fatalf("status %d location %q, want a 400 error page", rec.Code, rec.Header().Get("Location"))
			}
		})
	}
}

func TestGitHubAppCallback_SetupActionRequestIsInformational(t *testing.T) {
	f := newFakeGitHub(t)
	s, _ := newGitHubAppTestServer(t, f)
	rec := s.doCallback(t, callbackParams{setupAction: "request"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "needs to approve") {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestGitHubAppCallback_RejectsUnverifiedEmailBadCodeAndForeignInstallation(t *testing.T) {
	f := newFakeGitHub(t)
	s, _ := newGitHubAppTestServer(t, f)

	state, nonce := s.validState(t, ghModeInstall)
	if rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "stale"}); rec.Code != http.StatusBadRequest {
		t.Errorf("bad code: status %d", rec.Code)
	}

	f.emails = `[{"email":"a@b.test","primary":true,"verified":false}]`
	state, nonce = s.validState(t, ghModeInstall)
	rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code", installation: "555"})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "verified primary email") {
		t.Errorf("unverified email: status %d body %q", rec.Code, rec.Body.String())
	}

	// The user can only see installation 111; asking to claim 222 must fail even though the email is fine.
	f.emails = `[{"email":"gh.user@example.test","primary":true,"verified":true}]`
	f.installations = f.installation(111, "mine")
	state, nonce = s.validState(t, ghModeInstall)
	rec = s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code", installation: "222"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign installation: status %d, want 403", rec.Code)
	}
}

func TestGitHubAppCallback_HappyPathCreatesAccountAndConnector(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-happy-%d@example.test", time.Now().UnixNano())
	f.emails = fmt.Sprintf(`[{"email":%q,"primary":true,"verified":true}]`, strings.ToUpper(email))
	inst := time.Now().UnixNano() % 1_000_000_000_000
	f.installations = f.installation(inst, "acme")
	newFakeParse(t, email, false)
	cleanupGitHubAppTenant(t, db, email)

	state, nonce := s.validState(t, ghModeInstall)
	rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code", installation: fmt.Sprint(inst), setupAction: "install"})
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Path != "/" {
		t.Fatalf("redirected to %q, want the app root", loc.String())
	}
	orgCookie := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == ghOrgCookieName {
			orgCookie = c.Value
		}
	}
	var data struct {
		Result struct {
			JWT  string `json:"jwt"`
			Data struct {
				Email     string `json:"email"`
				FirstName string `json:"first_name"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(loc.Query().Get("data")), &data); err != nil {
		t.Fatalf("data param is not the onelogin JSON contract: %v", err)
	}
	if data.Result.JWT == "" || data.Result.Data.Email != email || data.Result.Data.FirstName != "Gh" {
		t.Fatalf("data wrong: %+v", data)
	}

	var orgID int64
	var signupPath sql.NullString
	if err := db.QueryRow(`SELECT id, signup_path FROM orgs WHERE name = $1`, email).Scan(&orgID, &signupPath); err != nil || signupPath.String != "gh_app" {
		t.Fatalf("org not created with signup_path gh_app: %v %v", err, signupPath)
	}
	if orgCookie != fmt.Sprint(orgID) {
		t.Fatalf("org cookie %q, want the connected org %d", orgCookie, orgID)
	}
	rec2, err := storagegithub.NewTokenStore(db).GetAppConnectorByInstallation(f.srv.URL, inst, orgID)
	if err != nil {
		t.Fatalf("connector missing: %v", err)
	}
	if rec2.OrgID != orgID || rec2.TokenType != "GITHUB_APP" || rec2.PatToken != "ghs_inst" || rec2.Provider != "github" {
		t.Fatalf("connector wrong: %+v", rec2)
	}

	nextCookie := func(r *httptest.ResponseRecorder) string {
		for _, c := range r.Result().Cookies() {
			if c.Name == ghNextCookieName {
				return c.Value
			}
		}
		return ""
	}
	if nextCookie(rec) != "" {
		t.Errorf("a first install is onboarding, not a returning visit; got next=%q", nextCookie(rec))
	}

	// A second sign-in is idempotent: same org, same single connector.
	state, nonce = s.validState(t, ghModeSignin)
	second := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code"})
	if second.Code != http.StatusFound {
		t.Fatalf("signin status %d: %s", second.Code, second.Body.String())
	}
	if nextCookie(second) != ghNextGitProviders {
		t.Errorf("a returning sign-in should land on Git Providers; next cookie = %q", nextCookie(second))
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM integration_tokens WHERE org_id = $1 AND token_type = 'GITHUB_APP'`, orgID).Scan(&n)
	if n != 1 {
		t.Fatalf("got %d connectors after a repeat sign-in, want 1", n)
	}

	// The cookie is cleared so the state can't be replayed.
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == ghStateCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("state cookie was not cleared")
	}
}

func TestGitHubAppCallback_SignInWithNothingInstalledCreatesAccountThenGoesToInstall(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-noinstall-%d@example.test", time.Now().UnixNano())
	f.emails = fmt.Sprintf(`[{"email":%q,"primary":true,"verified":true}]`, email)
	f.installations = `[]`
	newFakeParse(t, email, false)
	cleanupGitHubAppTenant(t, db, email)

	state, nonce := s.validState(t, ghModeSignin)
	rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code"})
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Path != "/github-apps/lr-test/installations/new" || loc.Query().Get("data") != "" {
		t.Fatalf("redirected to %q, want GitHub's install page (no ?data= login)", loc.String())
	}

	var orgID int64
	if err := db.QueryRow(`SELECT id FROM orgs WHERE name = $1`, email).Scan(&orgID); err != nil {
		t.Fatalf("org not created: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM integration_tokens WHERE org_id = $1 AND token_type = 'GITHUB_APP'`, orgID).Scan(&n)
	if n != 0 {
		t.Fatalf("got %d connectors, want 0 (nothing is installed yet)", n)
	}

	// An install-mode callback without an installation is still rejected.
	state, nonce = s.validState(t, ghModeInstall)
	if rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code"}); rec.Code != http.StatusBadRequest {
		t.Errorf("install mode with nothing installed: status %d, want 400", rec.Code)
	}
}

// connectorMeta reads the scope fields of a GitHub App connector.
func connectorMeta(t *testing.T, rec *storagegithub.IntegrationTokenRecord) appConnectorScope {
	t.Helper()
	var sc appConnectorScope
	if err := json.Unmarshal(rec.Metadata, &sc); err != nil {
		t.Fatal(err)
	}
	return sc
}

func TestGitHubAppCallback_SharedInstallationConnectsSecondOrgWithItsUsersRepos(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	emailA := fmt.Sprintf("gh-owner-a-%d@example.test", time.Now().UnixNano())
	emailB := fmt.Sprintf("gh-owner-b-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, emailA)
	cleanupGitHubAppTenant(t, db, emailB)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	f.installations = f.installation(inst, "acme")

	callback := func(email, mode, installation string) *httptest.ResponseRecorder {
		f.emails = fmt.Sprintf(`[{"email":%q,"primary":true,"verified":true}]`, email)
		newFakeParse(t, email, false)
		state, nonce := s.validState(t, mode)
		return s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code", installation: installation})
	}
	// The installation covers repositories 1 and 2. A installs the App and can access both.
	f.instRepos = `[{"id":1,"name":"api","full_name":"acme/api"},{"id":2,"name":"web","full_name":"acme/web"}]`
	f.userRepos = `[{"id":1},{"id":2}]`
	if rec := callback(emailA, ghModeInstall, fmt.Sprint(inst)); rec.Code != http.StatusFound {
		t.Fatalf("installer: status %d: %s", rec.Code, rec.Body.String())
	}
	// B signs up on their own, sees the installation, and can only access repository 1. Adding installation_id to
	// the callback URL by hand must not get B the rest.
	f.userRepos = `[{"id":1}]`
	if rec := callback(emailB, ghModeSignin, ""); rec.Code != http.StatusFound {
		t.Fatalf("second org: status %d: %s", rec.Code, rec.Body.String())
	}
	if rec := callback(emailB, ghModeInstall, fmt.Sprint(inst)); rec.Code != http.StatusFound {
		t.Fatalf("second org, crafted installation_id: status %d: %s", rec.Code, rec.Body.String())
	}

	var orgA, orgB int64
	_ = db.QueryRow(`SELECT id FROM orgs WHERE name = $1`, emailA).Scan(&orgA)
	_ = db.QueryRow(`SELECT id FROM orgs WHERE name = $1`, emailB).Scan(&orgB)
	store := storagegithub.NewTokenStore(db)
	connA, err := store.GetAppConnectorByInstallation(f.srv.URL, inst, orgA)
	if err != nil || connectorMeta(t, connA).RepoScope != storagegithub.RepoScopeInstaller {
		t.Fatalf("installer org connector: %+v %v", connA, err)
	}
	connB, err := store.GetAppConnectorByInstallation(f.srv.URL, inst, orgB)
	if err != nil {
		t.Fatalf("second org has no connector: %v", err)
	}
	if sc := connectorMeta(t, connB); sc.RepoScope != storagegithub.RepoScopeUser || len(sc.AllowedRepoIDs) != 1 || sc.AllowedRepoIDs[0] != 1 {
		t.Fatalf("second org connector scope = %+v, want user [1]", sc)
	}

	// The installer signing in again stays installer, even if GitHub now reports fewer repositories for them.
	if rec := callback(emailA, ghModeSignin, ""); rec.Code != http.StatusFound {
		t.Fatalf("installer sign-in: status %d", rec.Code)
	}
	if connA, _ = store.GetAppConnectorByInstallation(f.srv.URL, inst, orgA); connectorMeta(t, connA).RepoScope != storagegithub.RepoScopeInstaller {
		t.Fatal("a plain sign-in narrowed the installer's connector")
	}
}

func TestGitHubAppCallback_GetStartedOnlyAfterSuccess(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-landing-%d@example.test", time.Now().UnixNano())
	f.emails = fmt.Sprintf(`[{"email":%q,"primary":true,"verified":true}]`, email)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	f.installations = f.installation(inst, "acme")
	newFakeParse(t, email, false)
	cleanupGitHubAppTenant(t, db, email)

	nonce := "nonce-" + fmt.Sprint(time.Now().UnixNano())
	state, err := signGHState(ghState{Nonce: nonce, Src: "test", Mode: ghModeSignin, Exp: time.Now().Add(time.Minute).Unix(), GetStarted: true}, s.githubAppSecret)
	if err != nil {
		t.Fatal(err)
	}
	rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code"})
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	next := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == ghNextCookieName {
			next = c.Value
		}
	}
	if next != ghNextGetStarted {
		t.Fatalf("a new /gh-login sign-in should land on get-started; next cookie = %q", next)
	}

	// Starting from /gh-login carries the flag in the signed state, not in the browser.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github-app/start?mode=signin&src=test", nil)
	startRec := httptest.NewRecorder()
	if err := s.GitHubAppStart(echo.New().NewContext(req, startRec)); err != nil {
		t.Fatal(err)
	}
	loc, _ := url.Parse(startRec.Header().Get("Location"))
	st, err := verifyGHState(loc.Query().Get("state"), s.githubAppSecret, time.Now())
	if err != nil || !st.GetStarted {
		t.Fatalf("sign-in start state = %+v, %v; want GetStarted", st, err)
	}
}

func TestGitHubAppCallback_ParseFailureStopsBeforeAnyConnector(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-parsefail-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, email)
	f.emails = fmt.Sprintf(`[{"email":%q,"primary":true,"verified":true}]`, email)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	f.installations = f.installation(inst, "acme")
	newFakeParse(t, email, true)

	state, nonce := s.validState(t, ghModeInstall)
	rec := s.doCallback(t, callbackParams{state: state, cookieNonce: nonce, code: "good-code", installation: fmt.Sprint(inst)})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
	if recs, _ := storagegithub.NewTokenStore(db).ListAppConnectorsByInstallation(f.srv.URL, inst); len(recs) != 0 {
		t.Fatal("a connector was created even though the Hexmos signup failed")
	}
}

func signedWebhook(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (s *Server) doWebhook(t *testing.T, event string, body string, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github-app/webhook", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	if sig == "valid" {
		sig = signedWebhook(s.githubApp.WebhookSecret, []byte(body))
	}
	if sig != "" {
		req.Header.Set("X-Hub-Signature-256", sig)
	}
	rec := httptest.NewRecorder()
	handler := s.gitHubAppWebhookHandler(auth.NewAuthMiddleware(nil, s.db))
	if err := handler(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestGitHubAppWebhook_SignatureUnknownInstallationAndUninstall(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)

	body := `{"action":"deleted","installation":{"id":424242}}`
	for name, sig := range map[string]string{"missing": "", "wrong": "sha256=" + strings.Repeat("0", 64), "not hex": "sha256=zz", "no prefix": "abc"} {
		if rec := s.doWebhook(t, "installation", body, sig); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s signature: status %d, want 401", name, rec.Code)
		}
	}

	if rec := s.doWebhook(t, "installation", body, "valid"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "unknown_installation") {
		t.Errorf("unknown installation: %d %s, want 200 ignored", rec.Code, rec.Body.String())
	}

	// A connected installation: uninstall flags it removed and clears the token so nothing keeps using it.
	email := fmt.Sprintf("gh-hook-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, email)
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'gh hook test') RETURNING id`, email).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	inst := time.Now().UnixNano() % 1_000_000_000_000
	connID, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "User", Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	// A second org shares the installation; the uninstall reaches it too.
	other := fmt.Sprintf("gh-hook-other-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, other)
	var otherOrg int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'gh hook test') RETURNING id`, other).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	otherConn, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: otherOrg, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "User", Token: "ghs_other", ExpiresAt: time.Now().Add(time.Hour),
		RepoScope: storagegithub.RepoScopeUser, AllowedRepoIDs: []int64{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = fmt.Sprintf(`{"action":"deleted","installation":{"id":%d}}`, inst)
	if rec := s.doWebhook(t, "installation", body, "valid"); rec.Code != http.StatusOK {
		t.Fatalf("uninstall: %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []int64{connID, otherConn} {
		got, _ := storagegithub.NewTokenStore(db).GetGitHubTokenByConnectorID(id)
		if got.PatToken != "" || !strings.Contains(string(got.Metadata), `"removed": true`) {
			t.Fatalf("connector %d not marked removed: pat=%q meta=%s", id, got.PatToken, got.Metadata)
		}
	}
	// Events for a removed installation are ignored rather than processed.
	if rec := s.doWebhook(t, "issue_comment", fmt.Sprintf(`{"action":"created","installation":{"id":%d}}`, inst), "valid"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "installation_inactive") {
		t.Fatalf("removed installation event: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAppConnectorCovers_RespectsScopeAndState(t *testing.T) {
	repo := func(id int64) *struct {
		ID int64 `json:"id"`
	} {
		return &struct {
			ID int64 `json:"id"`
		}{ID: id}
	}
	cases := []struct {
		name string
		meta string
		repo int64
		want bool
	}{
		{"installer covers any repo", `{"repo_scope":"installer"}`, 9, true},
		{"no scope means installer", `{}`, 9, true},
		{"user scope covers its repo", `{"repo_scope":"user","allowed_repo_ids":[1,2]}`, 2, true},
		{"user scope skips other repos", `{"repo_scope":"user","allowed_repo_ids":[1,2]}`, 3, false},
		{"removed covers nothing", `{"removed":true}`, 1, false},
		{"suspended covers nothing", `{"suspended":true}`, 1, false},
	}
	for _, tc := range cases {
		if got := appConnectorCovers([]byte(tc.meta), repo(tc.repo)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if !appConnectorCovers([]byte(`{"repo_scope":"user","allowed_repo_ids":[]}`), nil) {
		t.Error("an event without a repository is for every live connector")
	}
}

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{
		"athreyac4@gmail.com": "at•••@gmail.com",
		"ab@hexmos.com":       "a•••@hexmos.com",
		"x@y.io":              "x•••@y.io",
		"no-at-sign":          "•••",
	} {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUserScopedRepoIDs(t *testing.T) {
	user := &ConnectorDetails{TokenType: storagegithub.TokenTypeGitHubApp, Metadata: map[string]interface{}{
		"repo_scope": "user", "allowed_repo_ids": []interface{}{float64(101), float64(202)},
	}}
	allowed, scoped := userScopedRepoIDs(user)
	if !scoped || len(allowed) != 2 || !allowed["101"] || !allowed["202"] {
		t.Fatalf("user scope: %v %v", allowed, scoped)
	}
	installer := &ConnectorDetails{TokenType: storagegithub.TokenTypeGitHubApp, Metadata: map[string]interface{}{"repo_scope": "installer"}}
	if _, scoped := userScopedRepoIDs(installer); scoped {
		t.Error("an installer connector syncs every repository")
	}
	pat := &ConnectorDetails{TokenType: "PAT", Metadata: map[string]interface{}{"repo_scope": "user"}}
	if _, scoped := userScopedRepoIDs(pat); scoped {
		t.Error("PAT connectors are never user-scoped")
	}
}

func TestGitHubAppWebhook_RepositoryNoOrgCoversIsIgnored(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-hook-scope-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, email)
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'gh hook test') RETURNING id`, email).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	inst := time.Now().UnixNano() % 1_000_000_000_000
	if _, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "Organization", Token: "t",
		ExpiresAt: time.Now().Add(time.Hour), RepoScope: storagegithub.RepoScopeUser, AllowedRepoIDs: []int64{1},
	}); err != nil {
		t.Fatal(err)
	}
	// Repository 2 isn't one this org's user can access, so nothing is processed (and no stub repo is created).
	body := fmt.Sprintf(`{"action":"opened","installation":{"id":%d},"repository":{"id":2},"pull_request":{"html_url":"http://x/acme/b/pull/1"}}`, inst)
	if rec := s.doWebhook(t, "pull_request", body, "valid"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "installation_inactive") {
		t.Fatalf("uncovered repository: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGitHubAppDemo_ScopedToOrg(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	email := fmt.Sprintf("gh-demo-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, email)
	other := fmt.Sprintf("gh-demo-other-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, other)

	mkOrg := func(name string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'demo test') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orgID, otherID := mkOrg(email), mkOrg(other)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	if _, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
		RepositorySelection: "selected", Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	var callDemo func(org int64, query string) githubAppDemoResponse
	call := func(org int64) githubAppDemoResponse { return callDemo(org, "") }
	callDemo = func(org int64, query string) githubAppDemoResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/github-app/demo"+query, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("org_id", org)
		if err := s.GitHubAppDemo(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("demo: %v %d %s", err, rec.Code, rec.Body.String())
		}
		var out githubAppDemoResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	mine := call(orgID)
	if !mine.Connected || len(mine.Installations) != 1 || len(mine.Repositories) != 1 || mine.Repositories[0].FullName != "acme/api" ||
		len(mine.OpenPullRequests) != 1 || mine.OpenPullRequests[0].Number != 7 {
		t.Fatalf("own org response wrong: %+v", mine)
	}
	if want := f.srv.URL + "/organizations/acme/settings/installations/" + fmt.Sprint(inst); mine.Installations[0].SettingsURL != want {
		t.Errorf("settings url %q, want %q", mine.Installations[0].SettingsURL, want)
	}
	// ?light=1 answers "connected?" without calling GitHub: no repositories or pull requests.
	if light := callDemo(orgID, "?light=1"); !light.Connected || len(light.Installations) != 1 || len(light.Repositories) != 0 || len(light.OpenPullRequests) != 0 {
		t.Fatalf("light response wrong: %+v", light)
	}
	if theirs := call(otherID); theirs.Connected || len(theirs.Repositories) != 0 || theirs.Installations == nil {
		t.Fatalf("another org must see nothing (and non-null arrays): %+v", theirs)
	}
}

func TestGitHubAppDemo_ListsOtherAccountsMasked(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	store := storagegithub.NewTokenStore(db)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	mkOrg := func(name string) int64 {
		cleanupGitHubAppTenant(t, db, name)
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'demo shared test') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
			OrgID: id, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
			Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	suffix := time.Now().UnixNano()
	mine := mkOrg(fmt.Sprintf("gh-demo-mine-%d@example.test", suffix))
	theirsEmail := fmt.Sprintf("teammate-%d@example.test", suffix)
	theirs := mkOrg(theirsEmail)
	var userID int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, theirsEmail).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, org_id, role_id) VALUES ($1, $2, (SELECT id FROM roles WHERE name = 'owner'))`, userID, theirs); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github-app/demo?light=1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("org_id", mine)
	if err := s.GitHubAppDemo(c); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("demo: %v %d", err, rec.Code)
	}
	if strings.Contains(rec.Body.String(), theirsEmail) {
		t.Fatal("the other account's full email was sent")
	}
	var out githubAppDemoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Installations) != 1 || out.Installations[0].OtherOrgs != 1 ||
		len(out.Installations[0].OtherAccounts) != 1 || out.Installations[0].OtherAccounts[0] != "te•••@example.test" {
		t.Fatalf("installation = %+v, want 1 other account te•••@example.test", out.Installations)
	}
}

func TestGitHubAppDisconnect_OwnersOnlyAndOrgScoped(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	name := fmt.Sprintf("gh-disc-%d@example.test", time.Now().UnixNano())
	other := fmt.Sprintf("gh-disc-other-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, name)
	cleanupGitHubAppTenant(t, db, other)
	mkOrg := func(n string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'disconnect test') RETURNING id`, n).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orgID, otherID := mkOrg(name), mkOrg(other)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	connectorID, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
		RepositorySelection: "selected", Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	call := func(org int64, owner bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/github-app/installations/"+fmt.Sprint(connectorID)+"?uninstall=true", nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("connector_id")
		c.SetParamValues(fmt.Sprint(connectorID))
		c.Set("org_id", org)
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{IsOwner: owner, IsMember: !owner, OrgID: org})
		if err := s.GitHubAppDisconnect(c); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	removed := func() bool {
		var r string
		_ = db.QueryRow(`SELECT COALESCE(metadata->>'removed','false') FROM integration_tokens WHERE id = $1`, connectorID).Scan(&r)
		return r == "true"
	}

	if rec := call(orgID, false); rec.Code != http.StatusForbidden || removed() {
		t.Fatalf("a plain member must not disconnect: %d removed=%v", rec.Code, removed())
	}
	if rec := call(otherID, true); rec.Code != http.StatusNotFound || removed() {
		t.Fatalf("another org's owner must get 404 and change nothing: %d removed=%v", rec.Code, removed())
	}
	if len(f.deleted) != 0 {
		t.Fatalf("rejected calls must not uninstall anything, got %v", f.deleted)
	}
	f.failDelete = true
	if rec := call(orgID, true); rec.Code != http.StatusBadGateway || removed() {
		t.Fatalf("a failed GitHub uninstall must change nothing: %d removed=%v", rec.Code, removed())
	}
	f.failDelete = false
	f.deleted = nil
	if rec := call(orgID, true); rec.Code != http.StatusOK || !removed() {
		t.Fatalf("owner disconnect: %d removed=%v", rec.Code, removed())
	}
	if len(f.deleted) != 1 || f.deleted[0] != fmt.Sprint(inst) {
		t.Fatalf("expected the App to be uninstalled from GitHub installation %d, got %v", inst, f.deleted)
	}
	var token string
	_ = db.QueryRow(`SELECT COALESCE(pat_token,'') FROM integration_tokens WHERE id = $1`, connectorID).Scan(&token)
	if token != "" {
		t.Error("cached installation token was not cleared")
	}
}

func TestGitHubAppDisconnect_SharedInstallation(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	store := storagegithub.NewTokenStore(db)
	inst := time.Now().UnixNano() % 1_000_000_000_000
	mk := func(prefix string) (orgID, connectorID int64) {
		name := fmt.Sprintf("%s-%d@example.test", prefix, time.Now().UnixNano())
		cleanupGitHubAppTenant(t, db, name)
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'shared disconnect test') RETURNING id`, name).Scan(&orgID); err != nil {
			t.Fatal(err)
		}
		connectorID, _, err := store.UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
			OrgID: orgID, BaseURL: f.srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
			Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		return orgID, connectorID
	}
	orgA, connA := mk("gh-shared-a")
	orgB, connB := mk("gh-shared-b")
	orgC, connC := mk("gh-shared-c")
	_ = orgC

	call := func(org, connectorID int64, uninstall bool) *httptest.ResponseRecorder {
		target := "/api/v1/github-app/installations/" + fmt.Sprint(connectorID)
		if uninstall {
			target += "?uninstall=true"
		}
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodDelete, target, nil), rec)
		c.SetParamNames("connector_id")
		c.SetParamValues(fmt.Sprint(connectorID))
		c.Set("org_id", org)
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{IsOwner: true, OrgID: org})
		if err := s.GitHubAppDisconnect(c); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	removed := func(id int64) bool {
		var r string
		_ = db.QueryRow(`SELECT COALESCE(metadata->>'removed','false') FROM integration_tokens WHERE id = $1`, id).Scan(&r)
		return r == "true"
	}

	// Disconnect only: this org stops, GitHub and the other orgs are untouched.
	if rec := call(orgA, connA, false); rec.Code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", rec.Code, rec.Body.String())
	}
	if !removed(connA) || removed(connB) || removed(connC) || len(f.deleted) != 0 {
		t.Fatalf("disconnect: A=%v B=%v C=%v github uninstalls=%v", removed(connA), removed(connB), removed(connC), f.deleted)
	}
	// Uninstall: GitHub uninstalls it, and every org's connector goes.
	if rec := call(orgB, connB, true); rec.Code != http.StatusOK {
		t.Fatalf("uninstall: %d %s", rec.Code, rec.Body.String())
	}
	if !removed(connB) || !removed(connC) || len(f.deleted) != 1 || f.deleted[0] != fmt.Sprint(inst) {
		t.Fatalf("uninstall: B=%v C=%v github uninstalls=%v", removed(connB), removed(connC), f.deleted)
	}
}

func TestGitHubAppStart_TargetIDOpensInstallPageOnThatAccount(t *testing.T) {
	f := newFakeGitHub(t)
	s, _ := newGitHubAppTestServer(t, f)

	start := func(query string) *url.URL {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github-app/start?"+query, nil)
		rec := httptest.NewRecorder()
		if err := s.GitHubAppStart(echo.New().NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		loc, _ := url.Parse(rec.Header().Get("Location"))
		return loc
	}

	loc := start("mode=install&src=x&target_id=42")
	if loc.Path != "/github-apps/lr-test/installations/new/permissions" || loc.Query().Get("target_id") != "42" {
		t.Errorf("install with target_id went to %s", loc)
	}
	if _, err := verifyGHState(loc.Query().Get("state"), s.githubAppSecret, time.Now()); err != nil {
		t.Errorf("state missing or bad on the targeted install URL: %v", err)
	}
	for _, bad := range []string{"abc", "-5", "0", "1.5"} {
		if loc := start("mode=install&src=x&target_id=" + bad); loc.Path != "/github-apps/lr-test/installations/new" {
			t.Errorf("target_id=%s must be ignored, went to %s", bad, loc)
		}
	}
	if loc := start("mode=signin&src=x&target_id=42"); loc.Path != "/login/oauth/authorize" {
		t.Errorf("sign-in must ignore target_id, went to %s", loc)
	}
}

func TestGitHubAppInstallCheck_ScopedToOrgAndResolvesOwner(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)
	name := fmt.Sprintf("gh-check-%d@example.test", time.Now().UnixNano())
	other := fmt.Sprintf("gh-check-other-%d@example.test", time.Now().UnixNano())
	cleanupGitHubAppTenant(t, db, name)
	cleanupGitHubAppTenant(t, db, other)
	mkOrg := func(n string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'install-check test') RETURNING id`, n).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orgID, otherID := mkOrg(name), mkOrg(other)
	connectorID, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: time.Now().UnixNano() % 1_000_000_000_000, AccountLogin: "acme",
		AccountType: "Organization", RepositorySelection: "selected", Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url)
		VALUES ($1, $2, 'github', $3, 'acme/api', 'api', 'http://x')`, orgID, connectorID, fmt.Sprint(time.Now().UnixNano())); err != nil {
		t.Fatal(err)
	}

	check := func(org int64, query string) githubAppInstallCheckResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/github-app/install-check?"+query, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("org_id", org)
		if err := s.GitHubAppInstallCheck(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var out githubAppInstallCheckResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	pr := func(path string) string { return "url=" + url.QueryEscape(f.srv.URL+path) }

	if got := check(orgID, pr("/acme/api/pull/7")); !got.Applicable || !got.Installed || got.Repository != "acme/api" {
		t.Errorf("a connected repository must be reported installed: %+v", got)
	}
	// The same repository for another org is not connected there: connections are never shared across orgs.
	if got := check(otherID, pr("/acme/api/pull/7")); !got.Applicable || got.Installed {
		t.Errorf("another org must not see acme/api as installed: %+v", got)
	}
	// An all-repositories install covers a repository the background sync hasn't listed yet.
	if _, _, err := storagegithub.NewTokenStore(db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: f.srv.URL, InstallationID: time.Now().UnixNano()%1_000_000_000_000 + 1, AccountLogin: "allco",
		AccountType: "Organization", RepositorySelection: "all", Token: "ghs_live", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if got := check(orgID, pr("/allco/brand-new/pull/1")); !got.Installed {
		t.Errorf("an all-repositories install must cover a not-yet-synced repository: %+v", got)
	}
	if got := check(otherID, pr("/allco/brand-new/pull/1")); got.Installed {
		t.Errorf("another org must not be covered by this org's install: %+v", got)
	}
	got := check(orgID, pr("/newco/web/pull/3"))
	if !got.Applicable || got.Installed || got.Owner != "newco" || got.TargetID != 777 {
		t.Errorf("an unconnected repository must come back with the owner's id: %+v", got)
	}
	if got := check(orgID, "repo=newco/web"); !got.Applicable || got.Installed || got.TargetID != 777 {
		t.Errorf("repo=owner/name must work like a pull request URL: %+v", got)
	}
	// An owner GitHub does not know still reports not installed, just without the shortcut.
	if got := check(orgID, pr("/ghost/web/pull/3")); !got.Applicable || got.Installed || got.TargetID != 0 {
		t.Errorf("unknown owner: %+v", got)
	}
	for _, q := range []string{"url=" + url.QueryEscape("https://gitlab.com/a/b/-/merge_requests/1"), pr("/acme/api/issues/7"), "url=not-a-url", ""} {
		if got := check(orgID, q); got.Applicable {
			t.Errorf("%q is not a GitHub pull request, got %+v", q, got)
		}
	}
}
