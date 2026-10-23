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
	"fmt"
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

	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/githubapp"
	"github.com/livereview/internal/license"
	githubprovider "github.com/livereview/internal/provider_input/github"
	"github.com/livereview/network/hexmos"
	networkgithub "github.com/livereview/network/providers/github"
	storageaiconnectors "github.com/livereview/storage/aiconnectors"
	storagelicense "github.com/livereview/storage/license"
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
	// Session-only (RequireAuth, not API keys): it cuts a connection, so org owners only. ?uninstall=true also
	// uninstalls the App from GitHub, for every org using it.
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

// GitHubAppDisconnect stops the caller's org using a GitHub App installation. Several orgs can share one
// installation, so there are two ways:
//   - default: only this org's connector is marked removed (its cached token cleared, its repositories dropped);
//     the App stays installed on GitHub and other orgs keep working. Plain GitHub sign-ins don't reconnect it;
//     installing or configuring the App again does.
//   - ?uninstall=true: also uninstalls the App from the GitHub account or organization (as the App, via its own
//     JWT), which ends it for every org using it, so every org's connector is removed here too (GitHub's
//     "installation deleted" webhook, which follows, then finds nothing left to do).
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
	uninstall := c.QueryParam("uninstall") == "true"
	store := storagegithub.NewTokenStore(s.db)
	rec, err := store.GetGitHubTokenByConnectorID(connectorID)
	// Same answer for "does not exist", "not a GitHub App connector" and "another org's": never confirm other orgs' ids.
	if err != nil || rec == nil || rec.OrgID != orgID || rec.TokenType != storagegithub.TokenTypeGitHubApp {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "GitHub installation not found"})
	}
	var meta struct {
		InstallationID int64 `json:"installation_id"`
	}
	if err := json.Unmarshal(rec.Metadata, &meta); err != nil || meta.InstallationID <= 0 {
		log.Printf("[GitHubApp] disconnect connector %d: no installation id in metadata", connectorID)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	repos := pullrequests.NewStore(s.db)

	if !uninstall {
		if err := store.DisconnectAppConnector(connectorID); err != nil {
			log.Printf("[GitHubApp] disconnect connector %d (org %d) failed: %v", connectorID, orgID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
		}
		if err := repos.DeleteRepositoriesByConnector(connectorID); err != nil {
			log.Printf("[GitHubApp] disconnect connector %d: dropping repositories failed: %v", connectorID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "disconnected"})
	}

	// Uninstall from GitHub first: if that fails nothing has changed and the owner can simply retry.
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
	recs, err := store.ListAppConnectorsByInstallation(cfg.BaseURL, meta.InstallationID)
	if err != nil {
		log.Printf("[GitHubApp] uninstall installation %d: listing its connectors failed: %v", meta.InstallationID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	// Remove every org's connector even if one fails. Retrying is safe: GitHub answers 404 for the already
	// uninstalled App, which DeleteInstallation treats as done.
	yes, failed := true, false
	for _, r := range recs {
		err := store.SetAppConnectorFlags(r.ID, &yes, nil)
		if err == nil {
			err = repos.DeleteRepositoriesByConnector(r.ID)
		}
		if err != nil {
			log.Printf("[GitHubApp] uninstall installation %d: removing connector %d failed: %v", meta.InstallationID, r.ID, err)
			failed = true
		}
	}
	if failed {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disconnect"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "uninstalled"})
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
	mode := sanitizeGHMode(c.QueryParam("mode"))
	// Only /gh-login starts in sign-in mode; the in-app install buttons store their own return page.
	return s.redirectToGitHub(c, sanitizeGHSrc(c.QueryParam("src")), mode, targetID, mode == ghModeSignin)
}

func (s *Server) redirectToGitHub(c echo.Context, src, mode string, targetID int64, getStarted bool) error {
	nonce, err := newGHNonce()
	if err != nil {
		return s.githubAppErrorPage(c, http.StatusInternalServerError, "Could not start sign-in. Please try again.")
	}
	signed, err := signGHState(ghState{Nonce: nonce, Src: src, Mode: mode, Exp: time.Now().Add(ghStateTTL).Unix(), GetStarted: getStarted}, s.githubAppSecret)
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
		// Done here, before the "nothing installed yet" redirect, so it applies whichever way the flow continues.
		if state.Src == ghSrcLaunchTrial {
			s.grantLaunchTrialManagedAI(ctx, provisioned.OrgID)
		}
	}

	if len(targets) == 0 {
		return s.redirectToGitHub(c, state.Src, ghModeInstall, 0, state.GetStarted)
	}

	// 5. connect each installation to the org. Other orgs may use the same installation (each has its own
	// connector), and each gets only the repositories its GitHub user can access (see connectGitHubAppInstallation).
	connected, failed := 0, 0
	alreadyConnected := false // a connector for one of these installations existed before this sign-in
	for _, inst := range targets {
		connectorID, created, resync, err := s.connectGitHubAppInstallation(ctx, provisioned.OrgID, inst, userToken, explicit)
		if errors.Is(err, storagegithub.ErrAppConnectorDisconnected) {
			// The org disconnected it on purpose; installing or configuring the App again reconnects it.
			continue
		}
		if err != nil {
			log.Printf("[GitHubApp] connecting installation %d failed: %v", inst.ID, err)
			// The installation just approved must connect; on a plain sign-in one failing installation
			// shouldn't block the others (or the sign-in).
			if explicit {
				return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not connect your GitHub installation. Please try again.")
			}
			failed++
			continue
		}
		connected++
		if !created {
			alreadyConnected = true
		}
		s.syncGitHubAppConnectorAsync(connectorID, created || resync)
	}
	if connected == 0 && failed > 0 {
		return s.githubAppErrorPage(c, http.StatusBadGateway, "Could not connect your GitHub installation. Please try again.")
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
	// Everyone else who started on /gh-login lands on get-started.
	next := ""
	switch {
	case state.Mode == ghModeSignin && !explicit && (alreadyConnected || connected == 0) && !provisioned.CreatedOrg:
		next = ghNextGitProviders
	case state.GetStarted:
		next = ghNextGetStarted
	}
	if next != "" {
		// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure, go.lang.security.audit.net.cookie-missing-httponly.cookie-missing-httponly -- the UI must read it from JS (index.tsx); holds only a page hint; Secure whenever HTTPS
		c.SetCookie(&http.Cookie{
			Name: ghNextCookieName, Value: next, Path: "/",
			MaxAge: 300, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
		})
	}
	return c.Redirect(http.StatusFound, "/?data="+url.QueryEscape(string(data)))
}

// ghSrcLaunchTrial is the /gh-login?src= value of the Product Hunt launch CTA. New orgs created through it may
// run reviews on the LiveReview model while on the Free plan (within its monthly LOC limit).
const ghSrcLaunchTrial = "launch-trial"

// grantLaunchTrialManagedAI records that grant and adds the org's "LiveReview AI Model" connector. A failure only
// logs: sign-in carries on and the user can still add their own AI key.
func (s *Server) grantLaunchTrialManagedAI(ctx context.Context, orgID int64) {
	if err := storagelicense.NewPlanChangeStore(s.db).EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		log.Printf("[GitHubApp] launch-trial: billing state for org %d failed: %v", orgID, err)
		return
	}
	if err := storageaiconnectors.NewManagedAIStore(s.db).GrantFreeManagedAI(ctx, orgID, aidefault.ProviderName); err != nil {
		log.Printf("[GitHubApp] launch-trial: granting the LiveReview model to org %d failed: %v", orgID, err)
	}
}

// connectGitHubAppInstallation mints an installation token and records the installation as a connector of the org.
// The org gets every repository the installation covers only when this GitHub user can access all of them (whoever
// installed or manages it); otherwise only the ones the user can access, since anyone who merely sees the
// installation (an outside collaborator, say) must not get the rest. GitHub supplies both lists, so nothing here
// trusts the callback URL. explicit (the user just installed or configured it) also reconnects an installation the
// org had disconnected. resync is true when the connector's repositories need syncing again.
func (s *Server) connectGitHubAppInstallation(ctx context.Context, orgID int64, inst networkgithub.Installation, userToken string, explicit bool) (connectorID int64, created, resync bool, err error) {
	cfg := s.githubApp
	store := storagegithub.NewTokenStore(s.db)
	var before appConnectorScope // the existing connector's scope, if any
	if prev, err := store.GetAppConnectorByInstallation(cfg.BaseURL, inst.ID, orgID); err == nil {
		_ = json.Unmarshal(prev.Metadata, &before)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, false, err
	}

	appJWT, err := networkgithub.MintAppJWT(cfg.AppID, cfg.PrivateKey, time.Now())
	if err != nil {
		return 0, false, false, err
	}
	tok, err := networkgithub.CreateInstallationToken(ctx, cfg.APIBaseURL(), appJWT, inst.ID)
	if err != nil {
		return 0, false, false, err
	}
	userRepos, err := networkgithub.ListUserInstallationRepositoryIDs(ctx, cfg.APIBaseURL(), userToken, inst.ID)
	if err != nil {
		return 0, false, false, fmt.Errorf("listing the user's repositories: %w", err)
	}
	all, err := installationRepositoryIDs(ctx, cfg.APIBaseURL(), tok.Token)
	if err != nil {
		return 0, false, false, fmt.Errorf("listing the installation's repositories: %w", err)
	}

	in := storagegithub.AppConnectorUpsert{
		OrgID: orgID, BaseURL: cfg.BaseURL, InstallationID: inst.ID, AccountLogin: inst.AccountLogin, AccountType: inst.AccountType,
		RepositorySelection: inst.RepositorySelection, AppSlug: cfg.Slug, Token: tok.Token, ExpiresAt: tok.ExpiresAt,
		RepoScope: storagegithub.RepoScopeInstaller, KeepDisconnected: !explicit,
	}
	if !containsAllIDs(userRepos, all) {
		in.RepoScope, in.AllowedRepoIDs = storagegithub.RepoScopeUser, userRepos
	}
	connectorID, created, err = store.UpsertAppInstallationConnector(in)
	if err != nil {
		return connectorID, false, false, err
	}
	switch {
	case explicit || before.Removed:
		// Just installed or configured (the repositories it grants may have changed), or reconnected.
		return connectorID, created, true, nil
	case before.RepoScope == storagegithub.RepoScopeUser:
		// Widened to every repository, or the user's accessible ones changed.
		return connectorID, created, in.RepoScope != storagegithub.RepoScopeUser || !sameIDSet(before.AllowedRepoIDs, in.AllowedRepoIDs), nil
	default:
		// New (created already syncs) or a live installer connector, which keeps every repository.
		return connectorID, created, false, nil
	}
}

// installationRepositoryIDs lists the ids of every repository an installation covers.
func installationRepositoryIDs(ctx context.Context, apiBase, installationToken string) ([]int64, error) {
	var ids []int64
	cursor := ""
	for page := 0; ; page++ {
		if page == 100 {
			return nil, errors.New("installation lists more than 10000 repositories")
		}
		repos, next, err := networkgithub.ListInstallationRepositories(ctx, apiBase, installationToken, cursor)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			ids = append(ids, r.ID)
		}
		if next == "" {
			return ids, nil
		}
		cursor = next
	}
}

// containsAllIDs reports whether have includes every id in want.
func containsAllIDs(have, want []int64) bool {
	set := make(map[int64]bool, len(have))
	for _, id := range have {
		set[id] = true
	}
	for _, id := range want {
		if !set[id] {
			return false
		}
	}
	return true
}

// appConnectorScope reads a GitHub App connector's repository scope from its metadata.
type appConnectorScope struct {
	RepoScope      string  `json:"repo_scope"`
	AllowedRepoIDs []int64 `json:"allowed_repo_ids"`
	Removed        bool    `json:"removed"`
}

func sameIDSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int64]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
	}
	return true
}

// githubAppSyncTimeout bounds one background repository sync (listing repos, registering them, PR backfill).
const githubAppSyncTimeout = 15 * time.Minute

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
		// Bounded so a stalled GitHub call can't keep this goroutine alive forever.
		ctx, cancel := context.WithTimeout(context.Background(), githubAppSyncTimeout)
		defer cancel()
		if err := s.autoWebhookInstaller.SyncAppConnector(ctx, int(connectorID)); err != nil {
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
	Repository *struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	// The PR a comment belongs to: issue_comment carries it as the issue (whose html_url is the PR's for PR
	// comments), review events as pull_request.
	Issue *struct {
		PullRequest *struct {
			HTMLURL string `json:"html_url"`
		} `json:"pull_request"` // present only when the issue is a pull request
	} `json:"issue"`
	PullRequest *struct {
		HTMLURL string `json:"html_url"`
	} `json:"pull_request"`
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

		// Several orgs can share one installation, each with its own connector.
		store := storagegithub.NewTokenStore(s.db)
		recs, err := store.ListAppConnectorsByInstallation(s.githubApp.BaseURL, env.Installation.ID)
		if err != nil {
			log.Printf("[GitHubApp] webhook connector lookup failed: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		}
		if len(recs) == 0 {
			return ignored("unknown_installation") // 200 so GitHub does not retry
		}

		switch event {
		case "installation":
			return s.handleGitHubAppInstallationEvent(c, store, recs, env)
		case "installation_repositories":
			return s.handleGitHubAppRepositoriesEvent(c, recs, env)
		}

		// The orgs this event is for: live connectors that cover the repository (a user-scoped one only covers
		// the repositories its user can access), oldest first.
		var targets []storagegithub.IntegrationTokenRecord
		for _, rec := range recs {
			if appConnectorCovers(rec.Metadata, env.Repository) {
				targets = append(targets, rec)
			}
		}
		if len(targets) == 0 {
			return ignored("installation_inactive")
		}

		// One org handles the event through the normal pipeline: the oldest connector, or for comments the org
		// that reviewed this PR most recently (so replies come from that org and its AI provider).
		target := targets[0]
		if prURL := env.prURL(); prURL != "" && event != "pull_request" && len(targets) > 1 {
			ids := make([]int64, len(targets))
			for i, rec := range targets {
				ids[i] = rec.ID
			}
			id, err := store.LatestReviewConnector(ids, prURL)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				log.Printf("[GitHubApp] webhook: finding the reviewing org failed: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			}
			for _, rec := range targets {
				if rec.ID == id {
					target = rec
				}
			}
		}
		// The other orgs only need their copy of the PR list updated. A failure there must not block the org
		// handling the event; the periodic PR reconciliation sweep catches those copies up.
		if event == "pull_request" && len(targets) > 1 {
			if err := s.queueSharedPullRequestState(c, body, targets[1:]); err != nil {
				log.Printf("[GitHubApp] webhook: updating other orgs' PR failed: %v", err)
			}
		}

		// Everything else (pull_request, comments, ...) is processed exactly like a PAT connector's webhook.
		if err := authMiddleware.SetConnectorOrgContext(c, target.ID); err != nil {
			return err
		}
		c.Request().Body = io.NopCloser(bytes.NewReader(body)) // the orchestrator reads the body again
		return s.WebhookOrchestratorV2Handler(c)
	}
}

// prURL is the pull request a comment or review event belongs to ("" for anything else).
func (env githubAppWebhookEnvelope) prURL() string {
	if env.PullRequest != nil && env.PullRequest.HTMLURL != "" {
		return env.PullRequest.HTMLURL
	}
	if env.Issue != nil && env.Issue.PullRequest != nil {
		return env.Issue.PullRequest.HTMLURL
	}
	return ""
}

// appConnectorCovers reports whether a GitHub App connector is live and may act on the repository: installer
// connectors cover every repository, user-scoped ones only their allowed list. Events without a repository are
// covered by every live connector.
func appConnectorCovers(metadata []byte, repo *struct {
	ID int64 `json:"id"`
}) bool {
	var meta struct {
		appConnectorScope
		Suspended bool `json:"suspended"`
	}
	if json.Unmarshal(metadata, &meta) != nil || meta.Removed || meta.Suspended {
		return false
	}
	if repo == nil || meta.RepoScope != storagegithub.RepoScopeUser {
		return true
	}
	for _, id := range meta.AllowedRepoIDs {
		if id == repo.ID {
			return true
		}
	}
	return false
}

// queueSharedPullRequestState updates the PR in each of these orgs (connectors sharing the installation with the one
// that handles the event).
func (s *Server) queueSharedPullRequestState(c echo.Context, body []byte, recs []storagegithub.IntegrationTokenRecord) error {
	if s.jobQueue == nil {
		return errors.New("job queue not initialized")
	}
	ev, matched, err := (&githubprovider.GitHubV2Provider{}).ConvertPullRequestStateEvent(
		map[string]string{"X-GitHub-Event": "pull_request"}, body)
	if err != nil || !matched {
		return err // not a state change (e.g. review_requested): nothing to update
	}
	for _, rec := range recs {
		if err := s.jobQueue.QueueSharedPRStateSyncJob(c.Request().Context(), rec.OrgID, rec.ID, "github", *ev); err != nil {
			return err
		}
	}
	return nil
}

// handleGitHubAppInstallationEvent applies an installation's deletion or suspension to every org's connector for it.
func (s *Server) handleGitHubAppInstallationEvent(c echo.Context, store *storagegithub.TokenStore, recs []storagegithub.IntegrationTokenRecord, env githubAppWebhookEnvelope) error {
	yes, no := true, false
	failed := false // keep going so one connector's failure doesn't leave the others unchanged
	for _, rec := range recs {
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
			failed = true
		}
	}
	if failed {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "event": "installation." + env.Action})
}

// handleGitHubAppRepositoriesEvent applies repositories added to or removed from the installation to every org
// using it (a user-scoped org only picks up added ones its user may access).
func (s *Server) handleGitHubAppRepositoriesEvent(c echo.Context, recs []storagegithub.IntegrationTokenRecord, env githubAppWebhookEnvelope) error {
	store := pullrequests.NewStore(s.db)
	for _, rec := range recs {
		if !appConnectorCovers(rec.Metadata, nil) {
			continue
		}
		switch env.Action {
		case "added":
			s.syncGitHubAppConnectorAsync(rec.ID, true)
		case "removed":
			for _, r := range env.RepositoriesRemoved {
				if err := store.DeleteRepositoryByProviderID(rec.ID, strconv.FormatInt(r.ID, 10)); err != nil {
					log.Printf("[GitHubApp] removing repository %d from connector %d failed: %v", r.ID, rec.ID, err)
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
				}
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
	// RepoScope is "user" when this org only gets the repositories its GitHub user can access (the App was
	// installed by someone else), otherwise "installer".
	RepoScope string `json:"repo_scope"`
	// OtherOrgs counts the other LiveReview orgs using this installation.
	OtherOrgs int `json:"other_orgs"`
	// OtherAccounts lists those orgs' owners, masked ("at•••@gmail.com"): enough to recognise a teammate, not
	// to contact them. Full addresses never leave the server.
	OtherAccounts []string `json:"other_accounts"`
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
			appConnectorScope
			InstallationID      int64  `json:"installation_id"`
			AccountLogin        string `json:"account_login"`
			AccountType         string `json:"account_type"`
			RepositorySelection string `json:"repository_selection"`
			Suspended           bool   `json:"suspended"`
		}
		_ = json.Unmarshal(rec.Metadata, &meta)
		if meta.Suspended {
			// Not usable until unsuspended on GitHub; the full answer skips it too (no token can be minted).
			continue
		}
		scope := storagegithub.RepoScopeInstaller
		if meta.RepoScope == storagegithub.RepoScopeUser {
			scope = storagegithub.RepoScopeUser
		}
		others, err := store.CountOtherOrgsUsingInstallation(cfg.BaseURL, meta.InstallationID, orgID)
		var ownerEmails []string
		if err == nil {
			ownerEmails, err = store.OtherOrgOwnerEmailsForInstallation(cfg.BaseURL, meta.InstallationID, orgID)
		}
		if err != nil {
			log.Printf("[GitHubApp] demo: looking up other orgs for connector %d failed: %v", rec.ID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load GitHub connections"})
		}
		otherAccounts := make([]string, 0, len(ownerEmails))
		for _, e := range ownerEmails {
			otherAccounts = append(otherAccounts, maskEmail(e))
		}

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
			RepoScope: scope, OtherOrgs: others, OtherAccounts: otherAccounts,
		})
		if light {
			continue
		}
		repos, _, err := networkgithub.ListInstallationRepositories(ctx, cfg.APIBaseURL(), token, "")
		if err != nil {
			log.Printf("[GitHubApp] demo: listing repositories for connector %d failed: %v", rec.ID, err)
			continue
		}
		allowed := map[int64]bool{}
		for _, id := range meta.AllowedRepoIDs {
			allowed[id] = true
		}
		for _, r := range repos {
			if scope == storagegithub.RepoScopeUser && !allowed[r.ID] {
				continue
			}
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

// maskEmail keeps the first two characters of the name and the domain: "athreyac4@gmail.com" -> "at•••@gmail.com"
// (one character when the name is that short).
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return "•••"
	}
	name := []rune(email[:at])
	keep := 2
	if len(name) <= 2 {
		keep = 1
	}
	return string(name[:keep]) + "•••" + email[at:]
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
