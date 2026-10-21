package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/githubapp"
	"github.com/livereview/network/hexmos"
	networkgithub "github.com/livereview/network/providers/github"
	storagegithub "github.com/livereview/storage/providers/github"
	"github.com/livereview/storage/providers/pullrequests"
	storageusers "github.com/livereview/storage/users"
)

// "Continue with GitHub" for LiveReview Cloud: one GitHub screen installs the LiveReview GitHub App and signs
// the user in. Routes only exist when the App is configured (GITHUB_APP_ID and friends); see internal/githubapp.

// registerGitHubAppRoutes wires the GitHub App endpoints. It is only called when s.githubApp is set.
func (s *Server) registerGitHubAppRoutes(public, v1 *echo.Group, authMiddleware *auth.AuthMiddleware) {
	public.GET("/auth/github-app/start", s.GitHubAppStart)
	public.GET("/auth/github-app/callback", s.GitHubAppCallback)
	v1.POST("/github-app/webhook", s.gitHubAppWebhookHandler(authMiddleware))
	v1.GET("/github-app/demo", s.GitHubAppDemo,
		authMiddleware.RequireAuth(),
		authMiddleware.BuildOrgContextFromHeader(),
		authMiddleware.ValidateOrgAccess(),
		authMiddleware.BuildPermissionContext(),
	)
	v1.GET("/github-app/install-check", s.GitHubAppInstallCheck,
		authMiddleware.RequireAuth(),
		authMiddleware.BuildOrgContextFromHeader(),
		authMiddleware.ValidateOrgAccess(),
		authMiddleware.BuildPermissionContext(),
	)
	// Session-only (RequireAuth, not API keys): it cuts a connection, so org owners only.
	v1.DELETE("/github-app/installations/:connector_id", s.GitHubAppDisconnect,
		authMiddleware.RequireAuth(),
		authMiddleware.BuildOrgContextFromHeader(),
		authMiddleware.ValidateOrgAccess(),
		authMiddleware.BuildPermissionContext(),
	)
}

type githubAppInstallCheckResponse struct {
	// Applicable is false when the input is not a pull request (or repository) on the App's GitHub host.
	Applicable bool `json:"applicable"`
	// Installed is true when one of the org's GitHub connectors already lists the repository.
	Installed  bool   `json:"installed"`
	Owner      string `json:"owner,omitempty"`
	Repository string `json:"repository,omitempty"`
	// TargetID is the owner's numeric GitHub id, used to open GitHub's install page already on that owner.
	TargetID int64 `json:"target_id,omitempty"`
}

// githubAppOwnerRepo reads "owner/repo" from ?repo=, or from a GitHub pull request URL in ?url= on the App's own
// GitHub host. ok is false for anything else.
func (s *Server) githubAppOwnerRepo(c echo.Context) (owner, repo string, ok bool) {
	full := strings.Trim(strings.TrimSpace(c.QueryParam("repo")), "/")
	if full == "" {
		raw := strings.TrimSpace(c.QueryParam("url"))
		u, err := url.Parse(raw)
		base, baseErr := url.Parse(s.githubApp.BaseURL)
		if err != nil || baseErr != nil || !strings.EqualFold(u.Hostname(), base.Hostname()) {
			return "", "", false
		}
		full = githubPullRepo(raw)
	}
	parts := strings.Split(full, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// GitHubAppInstallCheck tells the UI whether LiveReview can already post to a repository for the caller's org, and
// if not, which GitHub account the app should be installed on (so the install page can open on it). It only reads
// the org's own connectors (tenant-scoped) plus one public GitHub lookup of the owner's id.
func (s *Server) GitHubAppInstallCheck(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Org context not found"})
	}
	owner, repo, ok := s.githubAppOwnerRepo(c)
	if !ok {
		return c.JSON(http.StatusOK, githubAppInstallCheckResponse{})
	}
	resp := githubAppInstallCheckResponse{Applicable: true, Owner: owner, Repository: owner + "/" + repo}

	store := storagegithub.NewTokenStore(s.db)
	_, err := store.FindPreferredConnectorForRepo(orgID, resp.Repository)
	switch {
	case err == nil:
		resp.Installed = true
		return c.JSON(http.StatusOK, resp)
	case !errors.Is(err, sql.ErrNoRows):
		log.Printf("[GitHubApp] install-check: connector lookup failed for %s (org %d): %v", resp.Repository, orgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check the GitHub connection"})
	}
	// Right after an install the repositories are not synced yet, but an all-repositories install already covers it.
	if all, err := store.HasAppConnectorForOwnerAllRepos(orgID, owner); err != nil {
		log.Printf("[GitHubApp] install-check: owner lookup failed for %s (org %d): %v", owner, orgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check the GitHub connection"})
	} else if all {
		resp.Installed = true
		return c.JSON(http.StatusOK, resp)
	}

	// Not installed: look up the owner's id so GitHub can open the install page on that account. A failed lookup
	// only costs the shortcut; the install page then lists the accounts as before.
	cfg := s.githubApp
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()
	appJWT, err := networkgithub.MintAppJWT(cfg.AppID, cfg.PrivateKey, time.Now())
	if err == nil {
		var user *networkgithub.PublicUser
		if user, err = networkgithub.GetUserByLogin(ctx, cfg.APIBaseURL(), appJWT, owner); err == nil {
			resp.TargetID = user.ID
		}
	}
	if err != nil {
		log.Printf("[GitHubApp] install-check: could not resolve GitHub owner %q: %v", owner, err)
	}
	return c.JSON(http.StatusOK, resp)
}

// GitHubAppDisconnect uninstalls the GitHub App from the GitHub account or organization (as the App, via its own
// JWT) and stops LiveReview using it for the caller's org: the connector is marked removed (its cached token is
// cleared) and its repositories are dropped, exactly as the GitHub "installation deleted" webhook does (which
// GitHub also sends, harmlessly, after the uninstall). To use the App again, install it again from GitHub.
func (s *Server) GitHubAppDisconnect(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Org context not found"})
	}
	if pc := auth.GetPermissionContext(c); pc == nil || pc.RequireOrgOwner() != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Only an organization owner can disconnect a GitHub installation"})
	}
	connectorID, err := strconv.ParseInt(c.Param("connector_id"), 10, 64)
	if err != nil || connectorID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid connector id"})
	}
	store := storagegithub.NewTokenStore(s.db)
	rec, err := store.GetGitHubTokenByConnectorID(connectorID)
	// Same answer for "does not exist", "not a GitHub App connector" and "another org's": never confirm other orgs' ids.
	if err != nil || rec == nil || rec.OrgID != orgID || rec.TokenType != storagegithub.TokenTypeGitHubApp {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "GitHub installation not found"})
	}
	// Uninstall from GitHub first: if that fails nothing has changed and the owner can simply retry.
	var meta struct {
		InstallationID int64 `json:"installation_id"`
	}
	if err := json.Unmarshal(rec.Metadata, &meta); err != nil || meta.InstallationID <= 0 {
		log.Printf("[GitHubApp] disconnect connector %d: no installation id in metadata", connectorID)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	cfg := s.githubApp
	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	appJWT, err := networkgithub.MintAppJWT(cfg.AppID, cfg.PrivateKey, time.Now())
	if err == nil {
		err = networkgithub.DeleteInstallation(ctx, cfg.APIBaseURL(), appJWT, meta.InstallationID)
	}
	if err != nil {
		log.Printf("[GitHubApp] disconnect connector %d (org %d): uninstalling installation %d failed: %v", connectorID, orgID, meta.InstallationID, err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Could not uninstall the app from GitHub. Please try again."})
	}
	yes := true
	if err := store.SetAppConnectorFlags(connectorID, &yes, nil); err != nil {
		log.Printf("[GitHubApp] disconnect connector %d (org %d) failed: %v", connectorID, orgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	if err := pullrequests.NewStore(s.db).DeleteRepositoriesByConnector(connectorID); err != nil {
		log.Printf("[GitHubApp] disconnect connector %d: dropping repositories failed: %v", connectorID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "disconnected"})
}

// ---------------------------------------------------------------------------------------------------------
// start

// GitHubAppStart begins the GitHub round trip: a signed state (mirrored in a cookie) then a redirect to the
// install page, or to the OAuth authorize page for users who already installed the App.
func (s *Server) GitHubAppStart(c echo.Context) error {
	// target_id (install mode only) is the numeric GitHub id of the account or organization to install on, so
	// GitHub opens straight at that account instead of listing them; anything that is not a positive number is ignored.
	var targetID int64
	if id, err := strconv.ParseInt(c.QueryParam("target_id"), 10, 64); err == nil && id > 0 {
		targetID = id
	}
	return s.redirectToGitHub(c, sanitizeGHSrc(c.QueryParam("src")), sanitizeGHMode(c.QueryParam("mode")), targetID)
}

func (s *Server) redirectToGitHub(c echo.Context, src, mode string, targetID int64) error {
	nonce, err := newGHNonce()
	if err != nil {
		return s.githubAppErrorPage(c, http.StatusInternalServerError, "Could not start sign-in. Please try again.")
	}
	signed, err := signGHState(ghState{Nonce: nonce, Src: src, Mode: mode, Exp: time.Now().Add(ghStateTTL).Unix()}, s.githubAppSecret)
	if err != nil {
		return s.githubAppErrorPage(c, http.StatusInternalServerError, "Could not start sign-in. Please try again.")
	}

	// Lax, not Strict: the return trip from GitHub is a cross-site top-level GET, which Strict would drop.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set whenever the request is HTTPS (plain HTTP only happens in local development)
	c.SetCookie(&http.Cookie{
		Name: ghStateCookieName, Value: nonce, Path: "/api/v1/auth/github-app",
		MaxAge: int(ghStateTTL.Seconds()), HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
	})

	target := s.githubApp.InstallURL(signed)
	if mode == ghModeInstall && targetID > 0 {
		target = s.githubApp.InstallURLForTarget(signed, targetID)
	}
	if mode == ghModeSignin {
		target = s.githubApp.AuthorizeURL(signed)
	}
	return c.Redirect(http.StatusFound, target)
}

// ---------------------------------------------------------------------------------------------------------
// callback

func pickPrimaryVerifiedEmail(emails []networkgithub.UserEmail) string {
	for _, e := range emails {
		if e.Primary && e.Verified && strings.Contains(e.Email, "@") {
			return strings.ToLower(strings.TrimSpace(e.Email))
		}
	}
	return ""
}

// GitHubAppCallback finishes the round trip: verify the state, prove who the GitHub user is, prove they own the
// installation, sign them in through Hexmos (cloud), connect the installation to their org, and hand the
// browser to the normal Cloud login with the Hexmos JWT.
func (s *Server) GitHubAppCallback(c echo.Context) error {
	cfg := s.githubApp
	ctx := c.Request().Context()

	if c.QueryParam("setup_action") == "request" {
		// A member asked an owner to approve the install; there is no installation (and no code) yet.
		return s.githubAppErrorPage(c, http.StatusOK, "Installation requested. An owner of the GitHub organization needs to approve it on GitHub; you'll get an email when they do. Then start again from the sign-in page.")
	}

	// 1. state: signed, unexpired, and bound to this browser by the cookie nonce.
	state, err := verifyGHState(c.QueryParam("state"), s.githubAppSecret, time.Now())
	cookie, cookieErr := c.Cookie(ghStateCookieName)
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- expiring cookie; Secure matches how it was set (HTTPS only outside local development)
	c.SetCookie(&http.Cookie{Name: ghStateCookieName, Value: "", Path: "/api/v1/auth/github-app", MaxAge: -1, HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode})
	if err != nil || cookieErr != nil || !hmac.Equal([]byte(cookie.Value), []byte(state.Nonce)) {
		return s.githubAppErrorPage(c, http.StatusBadRequest, "This sign-in link expired or was opened in a different browser. Please start again.")
	}

	code := c.QueryParam("code")
	if code == "" {
		return s.githubAppErrorPage(c, http.StatusBadRequest, "GitHub did not return an authorization code. Please start again.")
	}

	// 2. the GitHub user.
	userTok, err := networkgithub.ExchangeUserCode(ctx, cfg.BaseURL, cfg.ClientID, cfg.ClientSecret, code)
	if err != nil {
		log.Printf("[GitHubApp] code exchange failed: %v", err)
		return s.githubAppErrorPage(c, http.StatusBadRequest, "GitHub rejected the sign-in code. Please start again.")
	}
	userToken := userTok.AccessToken

	emails, err := networkgithub.ListUserEmails(ctx, cfg.APIBaseURL(), userToken)
	if err != nil {
		log.Printf("[GitHubApp] list emails failed: %v", err)
		return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not read your GitHub email address. Please try again.")
	}
	email := pickPrimaryVerifiedEmail(emails)
	if email == "" {
		return s.githubAppErrorPage(c, http.StatusBadRequest, "Your GitHub account needs a verified primary email address to sign in.")
	}

	// 3. the installation must belong to this GitHub user; otherwise anyone could claim someone else's by editing the URL.
	installs, err := networkgithub.ListUserInstallations(ctx, cfg.APIBaseURL(), userToken)
	if err != nil {
		log.Printf("[GitHubApp] list installations failed: %v", err)
		return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not read your GitHub installations. Please try again.")
	}
	var targets []networkgithub.Installation
	explicit := c.QueryParam("installation_id") != ""
	for _, inst := range installs {
		if inst.AppID != cfg.AppID || inst.Suspended {
			continue
		}
		if explicit && strconv.FormatInt(inst.ID, 10) != c.QueryParam("installation_id") {
			continue
		}
		targets = append(targets, inst)
	}
	if explicit && len(targets) == 0 {
		return s.githubAppErrorPage(c, http.StatusForbidden, "That installation doesn't belong to your GitHub account.")
	}
	// Signing in with GitHub (no installation_id) is allowed with nothing installed yet: the account is created
	// below, then the user is sent straight on to GitHub's install page (no LiveReview session yet, so there is no
	// login screen flash in between). The callback that follows the install signs them in.
	if len(targets) == 0 && state.Mode != ghModeSignin {
		return s.githubAppErrorPage(c, http.StatusBadRequest, "The LiveReview GitHub App isn't installed on your account yet. Please start again and choose Install.")
	}

	// 4. identity: a Hexmos account (Parse user + JWT) and its LiveReview user + org.
	if !s.deploymentConfig.IsCloud {
		return s.githubAppErrorPage(c, http.StatusNotImplemented, "Sign-in with GitHub is only available on LiveReview Cloud for now.")
	}
	signup, err := hexmos.ExternalProviderSignup(ctx, "github_app", userToken, "livereview")
	if err != nil {
		log.Printf("[GitHubApp] fw-parse signup failed: %v", err)
		return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not create your Hexmos account right now. Please try again in a moment.")
	}
	if !strings.EqualFold(signup.User.Email, email) {
		log.Printf("[GitHubApp] fw-parse email %q differs from verified GitHub email %q", signup.User.Email, email)
		return s.githubAppErrorPage(c, http.StatusBadRequest, "Your GitHub email did not match your Hexmos account. Please contact support.")
	}
	first, last := signup.User.FirstName, signup.User.LastName
	provisioned, err := s.authHandlers.EnsureCloudUserCore(auth.EnsureCloudUserInput{
		JWT: signup.JWT, Email: email, FirstName: first, LastName: last, Source: "gh-login:" + state.Src,
	})
	if err != nil {
		var cerr *auth.EnsureCloudUserError
		if errors.As(err, &cerr) && cerr.Status < http.StatusInternalServerError {
			log.Printf("[GitHubApp] ensure cloud user rejected: %v", err)
			return s.githubAppErrorPage(c, http.StatusBadRequest, "Could not set up your LiveReview account. Please contact support.")
		}
		log.Printf("[GitHubApp] ensure cloud user failed: %v", err)
		return s.githubAppErrorPage(c, http.StatusInternalServerError, "Could not set up your LiveReview account. Please try again.")
	}
	if provisioned.CreatedOrg {
		if err := storageusers.NewUserStore(s.db).SetOrgSignupPath(provisioned.OrgID, "gh_app"); err != nil {
			log.Printf("[GitHubApp] could not record signup_path for org %d: %v", provisioned.OrgID, err)
		}
	}

	if len(targets) == 0 {
		return s.redirectToGitHub(c, state.Src, ghModeInstall, 0)
	}

	// 5. connect each installation to the org.
	connected := 0
	alreadyConnected := false // a connector for one of these installations existed before this sign-in
	for _, inst := range targets {
		connectorID, created, err := s.connectGitHubAppInstallation(ctx, provisioned.OrgID, inst)
		if errors.Is(err, storagegithub.ErrInstallationBoundToOtherOrg) {
			if explicit {
				return s.githubAppErrorPage(c, http.StatusConflict, "This GitHub installation is already connected to another LiveReview account.")
			}
			log.Printf("[GitHubApp] installation %d is bound to another org; skipping", inst.ID)
			continue
		}
		if err != nil {
			log.Printf("[GitHubApp] connecting installation %d failed: %v", inst.ID, err)
			return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not connect your GitHub installation. Please try again.")
		}
		connected++
		if !created {
			alreadyConnected = true
		}
		s.syncGitHubAppConnectorAsync(connectorID, created)
	}
	if connected == 0 {
		return s.githubAppErrorPage(c, http.StatusConflict, "Your GitHub installations are already connected to other LiveReview accounts.")
	}

	// 6. hand over to the normal Cloud login, using the same ?data= contract the Hexmos onelogin uses.
	data, err := json.Marshal(map[string]interface{}{
		"result": map[string]interface{}{
			"jwt": signup.JWT,
			"data": map[string]interface{}{
				"email":         email,
				"first_name":    first,
				"last_name":     last,
				"username":      signup.User.Username,
				"profilePicUrl": signup.User.ProfilePicURL,
			},
		},
	})
	if err != nil {
		return s.githubAppErrorPage(c, http.StatusInternalServerError, "Could not finish sign-in. Please try again.")
	}
	// A user can belong to several LiveReview orgs and the UI opens whichever one it last used. Tell it which org the
	// installation(s) were connected to (read and cleared by ui/src/index.tsx); otherwise get-started asks the wrong
	// org, finds no GitHub connection and sends the user back to GitHub. Not a secret, so not HttpOnly.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure, go.lang.security.audit.net.cookie-missing-httponly.cookie-missing-httponly -- the UI must read it from JS (index.tsx); holds only an org id; Secure whenever HTTPS
	c.SetCookie(&http.Cookie{
		Name: ghOrgCookieName, Value: strconv.FormatInt(provisioned.OrgID, 10), Path: "/",
		MaxAge: 300, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
	})
	// A returning user (plain GitHub sign-in, nothing new installed, their org and connection already existed) has
	// finished onboarding: send them to the GitHub App card on Git Providers, where their accounts are managed, instead of get-started.
	if state.Mode == ghModeSignin && !explicit && alreadyConnected && !provisioned.CreatedOrg {
		// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure, go.lang.security.audit.net.cookie-missing-httponly.cookie-missing-httponly -- the UI must read it from JS (index.tsx); holds only a page hint; Secure whenever HTTPS
		c.SetCookie(&http.Cookie{
			Name: ghNextCookieName, Value: ghNextGitProviders, Path: "/",
			MaxAge: 300, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
		})
	}
	return c.Redirect(http.StatusFound, "/?data="+url.QueryEscape(string(data)))
}

// connectGitHubAppInstallation mints an installation token and records the installation as a connector of the org.
func (s *Server) connectGitHubAppInstallation(ctx context.Context, orgID int64, inst networkgithub.Installation) (connectorID int64, created bool, err error) {
	cfg := s.githubApp
	appJWT, err := networkgithub.MintAppJWT(cfg.AppID, cfg.PrivateKey, time.Now())
	if err != nil {
		return 0, false, err
	}
	tok, err := networkgithub.CreateInstallationToken(ctx, cfg.APIBaseURL(), appJWT, inst.ID)
	if err != nil {
		return 0, false, err
	}
	return storagegithub.NewTokenStore(s.db).UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: cfg.BaseURL, InstallationID: inst.ID, AccountLogin: inst.AccountLogin, AccountType: inst.AccountType,
		RepositorySelection: inst.RepositorySelection, AppSlug: cfg.Slug, Token: tok.Token, ExpiresAt: tok.ExpiresAt,
	})
}

// syncGitHubAppConnectorAsync discovers the installation's repositories in the background. A sign-in by an
// existing connector only re-syncs when it has no repositories yet, so it doesn't re-queue every PR backfill.
func (s *Server) syncGitHubAppConnectorAsync(connectorID int64, created bool) {
	if s.autoWebhookInstaller == nil {
		return
	}
	if !created {
		names, err := pullrequests.NewStore(s.db).ListRepositoryFullNamesByConnector(connectorID)
		if err == nil && len(names) > 0 {
			return
		}
	}
	go func() {
		if err := s.autoWebhookInstaller.SyncAppConnector(context.Background(), int(connectorID)); err != nil {
			log.Printf("[GitHubApp] repository sync failed for connector %d: %v", connectorID, err)
		}
	}()
}

// ---------------------------------------------------------------------------------------------------------
// webhook

const githubAppWebhookMaxBody = 25 << 20 // GitHub caps webhook payloads at 25 MB

func validGitHubSignature(secret string, body []byte, header string) bool {
	if secret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

type githubAppWebhookEnvelope struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	RepositoriesRemoved []struct {
		ID int64 `json:"id"`
	} `json:"repositories_removed"`
}

func (s *Server) gitHubAppWebhookHandler(authMiddleware *auth.AuthMiddleware) echo.HandlerFunc {
	return func(c echo.Context) error {
		body, err := io.ReadAll(io.LimitReader(c.Request().Body, githubAppWebhookMaxBody))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		}
		if !validGitHubSignature(s.githubApp.WebhookSecret, body, c.Request().Header.Get("X-Hub-Signature-256")) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		}

		var env githubAppWebhookEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		}
		event := c.Request().Header.Get("X-GitHub-Event")
		ignored := func(reason string) error {
			return c.JSON(http.StatusOK, map[string]string{"status": "ignored", "reason": reason})
		}
		if event == "ping" || env.Installation.ID == 0 {
			return ignored("no_installation")
		}

		store := storagegithub.NewTokenStore(s.db)
		rec, err := store.GetAppConnectorByInstallation(s.githubApp.BaseURL, env.Installation.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return ignored("unknown_installation") // 200 so GitHub does not retry
		}
		if err != nil {
			log.Printf("[GitHubApp] webhook connector lookup failed: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		}

		switch event {
		case "installation":
			return s.handleGitHubAppInstallationEvent(c, store, rec, env)
		case "installation_repositories":
			return s.handleGitHubAppRepositoriesEvent(c, rec, env)
		}

		var meta struct {
			Removed   bool `json:"removed"`
			Suspended bool `json:"suspended"`
		}
		_ = json.Unmarshal(rec.Metadata, &meta)
		if meta.Removed || meta.Suspended {
			return ignored("installation_inactive")
		}

		// Everything else (pull_request, comments, ...) is processed exactly like a PAT connector's webhook.
		if err := authMiddleware.SetConnectorOrgContext(c, rec.ID); err != nil {
			return err
		}
		c.Request().Body = io.NopCloser(bytes.NewReader(body)) // the orchestrator reads the body again
		return s.WebhookOrchestratorV2Handler(c)
	}
}

func (s *Server) handleGitHubAppInstallationEvent(c echo.Context, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord, env githubAppWebhookEnvelope) error {
	yes, no := true, false
	var err error
	switch env.Action {
	case "deleted":
		if err = store.SetAppConnectorFlags(rec.ID, &yes, nil); err == nil {
			// Nothing can sync a removed installation; drop its repositories rather than failing every sweep.
			err = pullrequests.NewStore(s.db).DeleteRepositoriesByConnector(rec.ID)
		}
	case "suspend":
		err = store.SetAppConnectorFlags(rec.ID, nil, &yes)
	case "unsuspend":
		err = store.SetAppConnectorFlags(rec.ID, nil, &no)
	}
	if err != nil {
		log.Printf("[GitHubApp] installation %q event failed for connector %d: %v", env.Action, rec.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "event": "installation." + env.Action})
}

func (s *Server) handleGitHubAppRepositoriesEvent(c echo.Context, rec *storagegithub.IntegrationTokenRecord, env githubAppWebhookEnvelope) error {
	switch env.Action {
	case "added":
		s.syncGitHubAppConnectorAsync(rec.ID, true)
	case "removed":
		store := pullrequests.NewStore(s.db)
		for _, r := range env.RepositoriesRemoved {
			if err := store.DeleteRepositoryByProviderID(rec.ID, strconv.FormatInt(r.ID, 10)); err != nil {
				log.Printf("[GitHubApp] removing repository %d from connector %d failed: %v", r.ID, rec.ID, err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			}
		}
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "event": "installation_repositories." + env.Action})
}

// ---------------------------------------------------------------------------------------------------------
// demo (get-started page data)

type githubAppDemoInstallation struct {
	ConnectorID         int64  `json:"connector_id"`
	AccountLogin        string `json:"account_login"`
	AccountType         string `json:"account_type"`
	RepositorySelection string `json:"repository_selection"`
	SettingsURL         string `json:"settings_url"`
}

type githubAppDemoRepo struct {
	FullName string `json:"full_name"`
	WebURL   string `json:"web_url"`
	Private  bool   `json:"private"`
}

type githubAppDemoPull struct {
	RepoFullName string    `json:"repo_full_name"`
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	WebURL       string    `json:"web_url"`
	Author       string    `json:"author"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type githubAppDemoResponse struct {
	Connected        bool                        `json:"connected"`
	Installations    []githubAppDemoInstallation `json:"installations"`
	Repositories     []githubAppDemoRepo         `json:"repositories"`
	OpenPullRequests []githubAppDemoPull         `json:"open_pull_requests"`
}

const (
	githubAppDemoMaxRepos     = 5
	githubAppDemoPullsPerRepo = 5
	githubAppDemoMaxPulls     = 20
)

// GitHubAppDemo returns what the get-started page shows: the org's App installations, their repositories and
// open pull requests. It reads GitHub live because the background repository sync may still be running right
// after install. The org comes from the authenticated org context, never from the request.
func (s *Server) GitHubAppDemo(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Org context not found"})
	}
	resp := githubAppDemoResponse{
		Installations: []githubAppDemoInstallation{}, Repositories: []githubAppDemoRepo{}, OpenPullRequests: []githubAppDemoPull{},
	}

	cfg := s.githubApp
	store := storagegithub.NewTokenStore(s.db)
	records, err := store.ListAppConnectorsForOrg(orgID)
	if err != nil {
		log.Printf("[GitHubApp] demo: list connectors failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load GitHub connections"})
	}

	// ?light=1 answers only "is a GitHub App connected?" (no GitHub calls) - used for nav visibility and banners.
	light := c.QueryParam("light") == "1"

	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	reposForPulls := []string{}
	pullTokens := map[string]string{}
	for i := range records {
		rec := &records[i]
		var meta struct {
			InstallationID      int64  `json:"installation_id"`
			AccountLogin        string `json:"account_login"`
			AccountType         string `json:"account_type"`
			RepositorySelection string `json:"repository_selection"`
		}
		_ = json.Unmarshal(rec.Metadata, &meta)

		var token string
		if !light {
			var err error
			if token, err = githubapp.ConnectorTokenWith(ctx, cfg, store, rec); err != nil {
				log.Printf("[GitHubApp] demo: connector %d unusable: %v", rec.ID, err)
				continue
			}
		}
		resp.Connected = true
		resp.Installations = append(resp.Installations, githubAppDemoInstallation{
			ConnectorID: rec.ID, AccountLogin: meta.AccountLogin, AccountType: meta.AccountType,
			RepositorySelection: meta.RepositorySelection, SettingsURL: cfg.InstallationSettingsURL(meta.AccountType, meta.AccountLogin, meta.InstallationID),
		})
		if light {
			continue
		}
		repos, _, err := networkgithub.ListInstallationRepositories(ctx, cfg.APIBaseURL(), token, "")
		if err != nil {
			log.Printf("[GitHubApp] demo: listing repositories for connector %d failed: %v", rec.ID, err)
			continue
		}
		for _, r := range repos {
			resp.Repositories = append(resp.Repositories, githubAppDemoRepo{FullName: r.FullName, WebURL: r.HTMLURL, Private: r.Private})
			if len(reposForPulls) < githubAppDemoMaxRepos {
				reposForPulls = append(reposForPulls, r.FullName)
				pullTokens[r.FullName] = token
			}
		}
	}

	for _, full := range reposForPulls {
		pulls, err := networkgithub.ListOpenPulls(ctx, cfg.APIBaseURL(), pullTokens[full], full, githubAppDemoPullsPerRepo)
		if err != nil {
			log.Printf("[GitHubApp] demo: listing pulls for %s failed: %v", full, err)
			continue
		}
		for _, p := range pulls {
			resp.OpenPullRequests = append(resp.OpenPullRequests, githubAppDemoPull{
				RepoFullName: full, Number: p.Number, Title: p.Title, WebURL: p.HTMLURL, Author: p.AuthorLogin, UpdatedAt: p.UpdatedAt,
			})
		}
	}
	sort.SliceStable(resp.OpenPullRequests, func(i, j int) bool {
		return resp.OpenPullRequests[i].UpdatedAt.After(resp.OpenPullRequests[j].UpdatedAt)
	})
	if len(resp.OpenPullRequests) > githubAppDemoMaxPulls {
		resp.OpenPullRequests = resp.OpenPullRequests[:githubAppDemoMaxPulls]
	}
	return c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------------------------------------------------
// error page

var githubAppErrorTemplate = template.Must(template.New("gh-app-error").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>LiveReview sign-in</title>
<style>
:root{--bg:#f6f7f9;--fg:#111827;--muted:#4b5563;--card:#fff;--accent:#2563eb;--border:#e5e7eb}
@media (prefers-color-scheme:dark){:root{--bg:#0b1020;--fg:#e5e7eb;--muted:#9ca3af;--card:#111827;--accent:#60a5fa;--border:#1f2937}}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif;padding:16px}
main{max-width:30rem;background:var(--card);border:1px solid var(--border);border-radius:12px;padding:28px}
h1{font-size:1.25rem;margin:0 0 8px}p{color:var(--muted);margin:0 0 20px}
a.btn{display:inline-block;background:var(--accent);color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px;font-weight:600}
a.alt{margin-left:14px;color:var(--accent)}
</style></head><body><main>
<h1>LiveReview</h1><p>{{.Message}}</p>
<a class="btn" href="/#/gh-login">Try again</a><a class="alt" href="/">Other ways to sign in</a>
</main></body></html>`))

// githubAppErrorPage renders a small self-contained error page; the GitHub round trip is a browser navigation,
// so a JSON error would just strand the user.
func (s *Server) githubAppErrorPage(c echo.Context, status int, message string) error {
	var buf bytes.Buffer
	if err := githubAppErrorTemplate.Execute(&buf, map[string]string{"Message": message}); err != nil {
		return c.String(status, message)
	}
	return c.HTMLBlob(status, buf.Bytes())
}
