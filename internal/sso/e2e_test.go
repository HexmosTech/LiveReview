//go:build sso_e2e

// This file only builds/runs with `-tags=sso_e2e` (see `make test-sso-e2e`),
// never in a normal `go test ./...` - it requires a real Keycloak instance
// (docker/docker-compose.sso-test.yml) and binds a real HTTP listener on
// :8888, which the checked-in realm fixture's OIDC client redirect_uri
// points at.
package sso_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/sso"
)

func testDatabaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
}

func keycloakBaseURL() string {
	if u := os.Getenv("KEYCLOAK_BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8089"
}

const (
	testListenAddr   = "localhost:8888" // must match the realm fixture's OIDC redirect_uri host:port
	testUserEmail    = "sso-test@example.com"
	testUserPass     = "sso-test-password"
	testOIDCClientID = "livereview-oidc-test"
	testOIDCSecret   = "test-client-secret"
	testRealm        = "livereview-test"
	testJWTSecret    = "sso-e2e-test-jwt-secret"
)

// setupTestServer creates a throwaway org and starts a minimal HTTP server
// exposing every SSO route (the same handlers server.go wires, just without
// the rest of the application). It seeds no provider config - each test
// seeds whichever protocol it's exercising.
func setupTestServer(t *testing.T) (db *sql.DB, orgID int64, store *sso.Store, roleOf func(userID int64) string) {
	t.Helper()
	db, err := sql.Open("postgres", testDatabaseURL())
	if err != nil {
		t.Fatalf("connecting to test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, fmt.Sprintf("sso-e2e-%d", time.Now().UnixNano())).Scan(&orgID); err != nil {
		t.Fatalf("creating test org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM user_roles WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM org_sso_configs WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM users WHERE email = $1`, testUserEmail)
	})

	box, err := sso.NewSecretBox(testJWTSecret)
	if err != nil {
		t.Fatalf("building secret box: %v", err)
	}
	store = sso.NewStore(db, box)

	tokenService := auth.NewTokenService(db, testJWTSecret)
	authHandlers := auth.NewAuthHandlers(tokenService, db)
	authHandlers.SetSSO(store, testJWTSecret)

	e := echo.New()
	e.GET("/api/v1/auth/sso/providers", authHandlers.SSOProviders)
	e.GET("/api/v1/auth/sso/oidc/:org_id/login", authHandlers.SSOOIDCLogin)
	e.GET("/api/v1/auth/sso/oidc/callback", authHandlers.SSOOIDCCallback)
	e.GET("/api/v1/auth/sso/saml/:org_id/metadata", authHandlers.SSOSAMLMetadata)
	e.GET("/api/v1/auth/sso/saml/:org_id/login", authHandlers.SSOSAMLLogin)
	e.POST("/api/v1/auth/sso/saml/:org_id/acs", authHandlers.SSOSAMLACS)

	srv := &http.Server{Addr: testListenAddr, Handler: e}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("test server on %s stopped: %v", testListenAddr, err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	waitForListener(t, testListenAddr)

	roleOf = func(userID int64) string {
		var name string
		_ = db.QueryRow(`SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 AND ur.org_id = $2`, userID, orgID).Scan(&name)
		return name
	}
	return db, orgID, store, roleOf
}

func seedOIDCConfig(t *testing.T, store *sso.Store, orgID int64) {
	t.Helper()
	if err := store.Upsert(&sso.Config{
		OrgID:            orgID,
		ProviderType:     sso.ProviderOIDC,
		DisplayName:      "Keycloak (e2e)",
		Enabled:          true,
		OIDCIssuer:       keycloakBaseURL() + "/realms/" + testRealm,
		OIDCClientID:     testOIDCClientID,
		OIDCClientSecret: testOIDCSecret,
	}, 0); err != nil {
		t.Fatalf("seeding oidc config: %v", err)
	}
}

func seedSAMLConfig(t *testing.T, store *sso.Store, orgID int64) {
	t.Helper()
	if err := store.Upsert(&sso.Config{
		OrgID:              orgID,
		ProviderType:       sso.ProviderSAML,
		DisplayName:        "Keycloak (e2e, SAML)",
		Enabled:            true,
		SAMLIDPMetadataURL: keycloakBaseURL() + "/realms/" + testRealm + "/protocol/saml/descriptor",
	}, 0); err != nil {
		t.Fatalf("seeding saml config: %v", err)
	}
}

// newKeycloakAdminToken logs in as the bootstrap admin (KC_BOOTSTRAP_ADMIN_*
// in docker-compose.sso-test.yml) against the master realm's token
// endpoint, using Keycloak's built-in admin-cli public client.
func newKeycloakAdminToken(t *testing.T) string {
	t.Helper()
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {"admin"},
		"password":   {"admin"},
	}
	resp, err := http.PostForm(keycloakBaseURL()+"/realms/master/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatalf("fetching keycloak admin token: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("keycloak admin login failed (%d): %s", resp.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parsing keycloak admin token response: %v", err)
	}
	return out.AccessToken
}

// createSAMLClient registers a SAML SP client in the test realm for this
// run's dynamically-assigned org ID (the realm fixture can't pre-bake this,
// since LiveReview's SP entity ID embeds the org ID). A dynamically-created
// client gets no default protocol mappers, so this explicitly adds one
// releasing the user's email as a SAML attribute (ExchangeSAMLResponse
// falls back to attributes when the NameID itself isn't an email, which is
// Keycloak's default "transient" NameID behavior).
func createSAMLClient(t *testing.T, adminToken, entityID, acsURL string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"clientId":     entityID,
		"protocol":     "saml",
		"enabled":      true,
		"redirectUris": []string{acsURL},
		"attributes": map[string]string{
			"saml.authnstatement":      "true",
			"saml.client.signature":    "false",
			"saml.server.signature":    "true",
			"saml.assertion.signature": "true",
			"saml.encrypt":             "false",
		},
		"protocolMappers": []map[string]interface{}{
			{
				"name":           "email",
				"protocol":       "saml",
				"protocolMapper": "saml-user-property-mapper",
				"config": map[string]string{
					"user.attribute":       "email",
					"attribute.name":       "email",
					"friendly.name":        "email",
					"attribute.nameformat": "Basic",
				},
			},
		},
	})
	req, err := http.NewRequest(http.MethodPost, keycloakBaseURL()+"/admin/realms/"+testRealm+"/clients", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("building saml client creation request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("creating saml client: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("creating saml client failed (%d): %s", resp.StatusCode, b)
	}
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := http.Get("http://" + addr + "/api/v1/auth/sso/providers")
		if err == nil {
			conn.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("test server on %s never came up", addr)
}

var formActionRe = regexp.MustCompile(`id="kc-form-login"[^>]*action="([^"]+)"`)

// followUntilPage manually walks a redirect chain (never auto-following,
// since the final hop's fragment must be read off the Location header
// itself - fragments are never sent back to a server, so letting the HTTP
// client auto-follow would silently drop the tokens we're trying to read).
func followUntilPage(t *testing.T, client *http.Client, start string) (*http.Response, string) {
	t.Helper()
	next := start
	for i := 0; i < 10; i++ {
		resp, err := client.Get(next)
		if err != nil {
			t.Fatalf("GET %s: %v", next, err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				t.Fatalf("redirect from %s had no Location header", next)
			}
			next = loc
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(body)
	}
	t.Fatalf("too many redirects starting from %s", start)
	return nil, ""
}

func TestOIDCLoginEndToEnd(t *testing.T) {
	db, orgID, store, roleOf := setupTestServer(t)
	seedOIDCConfig(t, store, orgID)

	client := &http.Client{
		Jar: must(cookiejar.New(nil)),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginURL := fmt.Sprintf("http://%s/api/v1/auth/sso/oidc/%d/login", testListenAddr, orgID)
	_, loginPageHTML := followUntilPage(t, client, loginURL)

	m := formActionRe.FindStringSubmatch(loginPageHTML)
	if m == nil {
		t.Fatalf("could not find Keycloak login form action in page:\n%s", truncate(loginPageHTML))
	}
	formAction := strings.ReplaceAll(m[1], "&amp;", "&")

	form := url.Values{"username": {testUserEmail}, "password": {testUserPass}, "credentialId": {""}}
	req, err := http.NewRequest(http.MethodPost, formAction, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("building login POST: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("posting Keycloak login form: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected a redirect back to LiveReview after login, got %d", resp.StatusCode)
	}
	callbackURL := resp.Header.Get("Location")

	// This hits our /callback handler for real: code exchange, ID-token
	// verification, JIT provisioning, token issuance - then redirects to
	// the frontend with tokens in the URL fragment.
	finalResp, err := client.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET callback %s: %v", callbackURL, err)
	}
	finalResp.Body.Close()
	if finalResp.StatusCode < 300 || finalResp.StatusCode >= 400 {
		t.Fatalf("expected the callback to redirect to the frontend, got %d", finalResp.StatusCode)
	}
	finalLocation := finalResp.Header.Get("Location")

	parsed, err := url.Parse(finalLocation)
	if err != nil {
		t.Fatalf("parsing final redirect %q: %v", finalLocation, err)
	}
	if !strings.Contains(parsed.Path, "/sso/callback") {
		t.Fatalf("expected final redirect to /sso/callback, got %q", finalLocation)
	}
	fragValues, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parsing fragment %q: %v", parsed.Fragment, err)
	}
	accessToken := fragValues.Get("access_token")
	if accessToken == "" {
		t.Fatalf("final redirect had no access_token in fragment: %q", finalLocation)
	}

	tokenService := auth.NewTokenService(db, testJWTSecret)
	user, err := tokenService.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("issued access token did not validate: %v", err)
	}
	if user.Email != testUserEmail {
		t.Fatalf("expected provisioned user %s, got %s", testUserEmail, user.Email)
	}
	if role := roleOf(user.ID); role != "member" {
		t.Fatalf("expected JIT-provisioned user to be role 'member' in org %d, got %q", orgID, role)
	}
}

// TestSAMLLoginEndToEnd mirrors TestOIDCLoginEndToEnd, but drives the
// HTTP-Redirect-request / HTTP-POST-response SAML binding Keycloak uses by
// default: the final hop back from Keycloak is a 200 HTML page with an
// auto-submitting form carrying SAMLResponse/RelayState, not a redirect -
// so this test submits that form itself rather than following a redirect.
func TestSAMLLoginEndToEnd(t *testing.T) {
	db, orgID, store, roleOf := setupTestServer(t)

	entityID := fmt.Sprintf("http://%s/api/v1/auth/sso/saml/%d/metadata", testListenAddr, orgID)
	acsURL := fmt.Sprintf("http://%s/api/v1/auth/sso/saml/%d/acs", testListenAddr, orgID)
	createSAMLClient(t, newKeycloakAdminToken(t), entityID, acsURL)
	seedSAMLConfig(t, store, orgID)

	client := &http.Client{
		Jar: must(cookiejar.New(nil)),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginURL := fmt.Sprintf("http://%s/api/v1/auth/sso/saml/%d/login", testListenAddr, orgID)
	_, loginPageHTML := followUntilPage(t, client, loginURL)

	m := formActionRe.FindStringSubmatch(loginPageHTML)
	if m == nil {
		t.Fatalf("could not find Keycloak login form action in page:\n%s", truncate(loginPageHTML))
	}
	formAction := html.UnescapeString(m[1])

	form := url.Values{"username": {testUserEmail}, "password": {testUserPass}, "credentialId": {""}}
	req, err := http.NewRequest(http.MethodPost, formAction, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("building login POST: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("posting Keycloak login form: %v", err)
	}
	var autoSubmitHTML string
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		_, autoSubmitHTML = followUntilPage(t, client, loc)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		autoSubmitHTML = string(body)
	}

	acsAction := extractFormAction(t, autoSubmitHTML)
	samlResponse := extractInputValue(t, autoSubmitHTML, "SAMLResponse")
	relayState := extractInputValue(t, autoSubmitHTML, "RelayState")
	if samlResponse == "" {
		t.Fatalf("could not find SAMLResponse in Keycloak's auto-submit page:\n%s", truncate(autoSubmitHTML))
	}

	// This is the step a real browser's JS would perform automatically:
	// submit the SAMLResponse to our ACS endpoint. It hits our real
	// handler: signature/condition validation, identity extraction, JIT
	// provisioning, token issuance, then a redirect to the frontend.
	acsForm := url.Values{"SAMLResponse": {samlResponse}, "RelayState": {relayState}}
	acsReq, err := http.NewRequest(http.MethodPost, acsAction, strings.NewReader(acsForm.Encode()))
	if err != nil {
		t.Fatalf("building ACS POST: %v", err)
	}
	acsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	finalResp, err := client.Do(acsReq)
	if err != nil {
		t.Fatalf("posting to ACS %s: %v", acsAction, err)
	}
	finalResp.Body.Close()
	if finalResp.StatusCode < 300 || finalResp.StatusCode >= 400 {
		b, _ := io.ReadAll(finalResp.Body)
		t.Fatalf("expected the ACS endpoint to redirect to the frontend, got %d: %s", finalResp.StatusCode, b)
	}
	finalLocation := finalResp.Header.Get("Location")

	parsed, err := url.Parse(finalLocation)
	if err != nil {
		t.Fatalf("parsing final redirect %q: %v", finalLocation, err)
	}
	if !strings.Contains(parsed.Path, "/sso/callback") {
		t.Fatalf("expected final redirect to /sso/callback, got %q", finalLocation)
	}
	fragValues, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parsing fragment %q: %v", parsed.Fragment, err)
	}
	accessToken := fragValues.Get("access_token")
	if accessToken == "" {
		t.Fatalf("final redirect had no access_token in fragment: %q", finalLocation)
	}

	tokenService := auth.NewTokenService(db, testJWTSecret)
	user, err := tokenService.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("issued access token did not validate: %v", err)
	}
	if user.Email != testUserEmail {
		t.Fatalf("expected provisioned user %s, got %s", testUserEmail, user.Email)
	}
	if role := roleOf(user.ID); role != "member" {
		t.Fatalf("expected JIT-provisioned user to be role 'member' in org %d, got %q", orgID, role)
	}
}

func extractFormAction(t *testing.T, pageHTML string) string {
	t.Helper()
	re := regexp.MustCompile(`<form[^>]*\baction="([^"]*)"`)
	m := re.FindStringSubmatch(pageHTML)
	if m == nil {
		t.Fatalf("could not find a <form action=...> in page:\n%s", truncate(pageHTML))
	}
	return html.UnescapeString(m[1])
}

func extractInputValue(t *testing.T, pageHTML, name string) string {
	t.Helper()
	tagRe := regexp.MustCompile(`<input[^>]*\bname="` + regexp.QuoteMeta(name) + `"[^>]*>`)
	tag := tagRe.FindString(pageHTML)
	if tag == "" {
		return ""
	}
	valRe := regexp.MustCompile(`value="([^"]*)"`)
	m := valRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

func must(cj *cookiejar.Jar, err error) *cookiejar.Jar {
	if err != nil {
		panic(err)
	}
	return cj
}

func truncate(s string) string {
	if len(s) > 2000 {
		return s[:2000] + "...(truncated)"
	}
	return s
}
