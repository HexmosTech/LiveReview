package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/sso"
	"github.com/livereview/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

// errSSOAccountDeactivated is returned by jitProvisionSSOUser when the
// matched user has been deactivated. SSO login must never resurrect a
// deactivated account - that's an explicit admin action only.
var errSSOAccountDeactivated = errors.New("sso: account is deactivated")

// SetSSO wires the SSO config store and state signer into the auth
// handlers. Both the OIDC and (later) SAML login routes, and the config
// CRUD handlers that validate against a live IdP, depend on this.
func (h *AuthHandlers) SetSSO(store *sso.Store, jwtSecret string) {
	h.sso = store
	h.ssoState = sso.NewStateSigner(jwtSecret)
}

// SetSSOIdentities wires the subject->user identity store in, used to
// resolve incoming OIDC back-channel logout tokens / SAML LogoutRequests to
// a LiveReview user (Phase 4 of docs/sso.md's deprovisioning section).
func (h *AuthHandlers) SetSSOIdentities(store *sso.IdentityStore) {
	h.ssoIdentities = store
}

// baseURL derives this request's own origin, used both as the OIDC/SAML
// redirect/ACS URL registered with the IdP and as the final browser
// redirect back into the frontend. Self-hosted serves API and frontend from
// the same origin behind one reverse proxy (see getSystemInfo in
// internal/api/server.go), so this is correct for both purposes without a
// separate "public URL" setting.
func baseURL(c echo.Context) string {
	return fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ssoFailureRedirect sends the browser back to the login page with an error
// hint, rather than returning raw JSON - this leg of the flow is a full-page
// browser navigation, not an XHR call.
func ssoFailureRedirect(c echo.Context, reason string) error {
	return c.Redirect(http.StatusFound, "/login?sso_error="+url.QueryEscape(reason))
}

// SSOProviderInfo is what the (unauthenticated) login page uses to decide
// whether to show a "Continue with ..." button.
type SSOProviderInfo struct {
	OrgID        int64  `json:"org_id"`
	OrgName      string `json:"org_name"`
	ProviderType string `json:"provider_type"`
	DisplayName  string `json:"display_name"`
}

// SSOProviders is public: GET /api/v1/auth/sso/providers. In self-hosted
// deployments this normally returns at most one org's providers, since
// self-hosted is typically single-org.
func (h *AuthHandlers) SSOProviders(c echo.Context) error {
	if h.sso == nil {
		return c.JSON(http.StatusOK, []SSOProviderInfo{})
	}
	configs, err := h.sso.ListEnabled()
	if err != nil {
		log.Printf("[sso] failed to list enabled providers: %v", err)
		return c.JSON(http.StatusOK, []SSOProviderInfo{})
	}
	out := make([]SSOProviderInfo, 0, len(configs))
	for _, cfg := range configs {
		var orgName string
		_ = h.db.QueryRow(`SELECT name FROM orgs WHERE id = $1`, cfg.OrgID).Scan(&orgName)
		out = append(out, SSOProviderInfo{
			OrgID:        cfg.OrgID,
			OrgName:      orgName,
			ProviderType: string(cfg.ProviderType),
			DisplayName:  cfg.DisplayName,
		})
	}
	return c.JSON(http.StatusOK, out)
}

// SSOOIDCLogin is public: GET /api/v1/auth/sso/oidc/:org_id/login. It redirects
// the browser to the org's configured IdP.
func (h *AuthHandlers) SSOOIDCLogin(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return ssoFailureRedirect(c, "invalid_org")
	}
	if h.sso == nil {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderOIDC)
	if err != nil || !cfg.Enabled {
		return ssoFailureRedirect(c, "sso_not_configured")
	}

	nonce, err := randomNonce()
	if err != nil {
		return ssoFailureRedirect(c, "internal_error")
	}
	state := h.ssoState.Encode(orgID, nonce)

	redirectURL := baseURL(c) + "/api/v1/auth/sso/oidc/callback"
	oauthCfg, _, err := sso.BuildOIDCOAuth2Config(c.Request().Context(), cfg, redirectURL)
	if err != nil {
		log.Printf("[sso] oidc login for org %d failed: %v", orgID, err)
		return ssoFailureRedirect(c, "idp_unreachable")
	}
	return c.Redirect(http.StatusFound, oauthCfg.AuthCodeURL(state))
}

// SSOOIDCCallback is public: GET /api/v1/auth/sso/oidc/callback. The IdP
// redirects the browser here with ?code=...&state=....
func (h *AuthHandlers) SSOOIDCCallback(c echo.Context) error {
	if h.sso == nil {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	state := c.QueryParam("state")
	code := c.QueryParam("code")
	if state == "" || code == "" {
		return ssoFailureRedirect(c, "missing_code_or_state")
	}
	orgID, err := h.ssoState.Decode(state)
	if err != nil {
		return ssoFailureRedirect(c, "invalid_state")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderOIDC)
	if err != nil || !cfg.Enabled {
		return ssoFailureRedirect(c, "sso_not_configured")
	}

	redirectURL := baseURL(c) + "/api/v1/auth/sso/oidc/callback"
	oauthCfg, verifier, err := sso.BuildOIDCOAuth2Config(c.Request().Context(), cfg, redirectURL)
	if err != nil {
		log.Printf("[sso] oidc callback discovery for org %d failed: %v", orgID, err)
		return ssoFailureRedirect(c, "idp_unreachable")
	}
	identity, err := sso.ExchangeOIDCCode(c.Request().Context(), oauthCfg, verifier, code)
	if err != nil {
		log.Printf("[sso] oidc callback exchange for org %d failed: %v", orgID, err)
		h.recordAuthEvent(c, auditlog.Event{
			OrgID:     orgID,
			ActorKind: auditlog.ActorKindMember,
			Action:    auditlog.ActionLoginFailure,
			Outcome:   auditlog.OutcomeFailure,
			Severity:  auditlog.SeverityWarning,
			Details:   auditlog.Details("method", "sso_oidc", "reason", "exchange_failed"),
		})
		return ssoFailureRedirect(c, "login_failed")
	}
	return h.completeSSOLogin(c, orgID, identity, "sso_oidc")
}

// SSOOIDCBackChannelLogout is public: POST /api/v1/auth/sso/oidc/:org_id/backchannel-logout.
// Per OIDC Back-Channel Logout 1.0, the IdP calls this directly (no
// browser involved) when a user logs out or is disabled at the IdP, so
// LiveReview can revoke that user's sessions in near real time rather than
// waiting for their JWT to expire or the next SCIM/reconciliation sweep.
// Best-effort and intentionally quiet on any resolution failure (unknown
// subject, SSO not configured, etc.) - per spec, this endpoint must not
// leak account existence, so it always responds 200 once the token itself
// verifies, even if there's nothing on LiveReview's side to revoke.
func (h *AuthHandlers) SSOOIDCBackChannelLogout(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return c.String(http.StatusBadRequest, "invalid org")
	}
	if h.sso == nil {
		return c.String(http.StatusNotFound, "sso not configured")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderOIDC)
	if err != nil || !cfg.Enabled {
		return c.String(http.StatusNotFound, "sso not configured")
	}
	logoutToken := c.Request().PostFormValue("logout_token")
	if logoutToken == "" {
		return c.String(http.StatusBadRequest, "missing logout_token")
	}
	subject, err := sso.VerifyBackChannelLogoutToken(c.Request().Context(), cfg, logoutToken)
	if err != nil {
		log.Printf("[sso] back-channel logout token verification failed for org %d: %v", orgID, err)
		return c.String(http.StatusBadRequest, "invalid logout_token")
	}
	if h.ssoIdentities == nil {
		return c.NoContent(http.StatusOK)
	}
	userID, err := h.ssoIdentities.LookupUserBySubject(orgID, sso.ProviderOIDC, subject)
	if err != nil {
		return c.NoContent(http.StatusOK)
	}
	if _, err := h.db.Exec(`UPDATE auth_tokens SET is_active = false, revoked_at = NOW() WHERE user_id = $1 AND is_active = true`, userID); err != nil {
		log.Printf("[sso] back-channel logout: failed to revoke sessions for user %d: %v", userID, err)
		return c.NoContent(http.StatusOK)
	}
	h.recordAuthEvent(c, auditlog.Event{
		OrgID:       orgID,
		ActorUserID: &userID,
		// The IdP pushed this logout event, not the user themselves (there
		// is no session on this endpoint) - ActorKindWebhook maps to
		// ActorCapacitySystem (see auditlog.EnrichFromEcho), matching how
		// SCIM-driven deactivations are attributed.
		ActorKind: auditlog.ActorKindWebhook,
		Action:    auditlog.ActionSessionRevoke,
		Outcome:   auditlog.OutcomeSuccess,
		Severity:  auditlog.SeverityWarning,
		Details:   auditlog.Details("method", "sso_oidc_backchannel_logout"),
	})
	return c.NoContent(http.StatusOK)
}

// samlURLs derives this org's metadata/ACS URLs from the current request's
// own origin, same reasoning as baseURL for OIDC's redirect URL.
func samlURLs(c echo.Context, orgID int64) (metadataURL, acsURL string) {
	base := baseURL(c) + "/api/v1/auth/sso/saml/" + strconv.FormatInt(orgID, 10)
	return base + "/metadata", base + "/acs"
}

// SSOSAMLMetadata is public: GET /api/v1/auth/sso/saml/:org_id/metadata.
// IdP admins fetch this to configure LiveReview as a SAML SP.
func (h *AuthHandlers) SSOSAMLMetadata(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return c.String(http.StatusBadRequest, "invalid org")
	}
	if h.sso == nil {
		return c.String(http.StatusNotFound, "sso not configured")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderSAML)
	if err != nil {
		return c.String(http.StatusNotFound, "sso not configured")
	}
	metadataURL, acsURL := samlURLs(c, orgID)
	sp, err := sso.BuildServiceProvider(c.Request().Context(), cfg, metadataURL, acsURL)
	if err != nil {
		log.Printf("[sso] saml metadata for org %d failed: %v", orgID, err)
		return c.String(http.StatusBadGateway, "idp metadata unavailable")
	}
	buf, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		return c.String(http.StatusInternalServerError, "failed to render metadata")
	}
	return c.Blob(http.StatusOK, "application/samlmetadata+xml", buf)
}

// SSOSAMLLogin is public: GET /api/v1/auth/sso/saml/:org_id/login. It
// redirects the browser to the org's configured IdP (HTTP-Redirect
// binding).
func (h *AuthHandlers) SSOSAMLLogin(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return ssoFailureRedirect(c, "invalid_org")
	}
	if h.sso == nil {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderSAML)
	if err != nil || !cfg.Enabled {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	metadataURL, acsURL := samlURLs(c, orgID)
	sp, err := sso.BuildServiceProvider(c.Request().Context(), cfg, metadataURL, acsURL)
	if err != nil {
		log.Printf("[sso] saml login for org %d failed: %v", orgID, err)
		return ssoFailureRedirect(c, "idp_unreachable")
	}
	nonce, err := randomNonce()
	if err != nil {
		return ssoFailureRedirect(c, "internal_error")
	}
	state := h.ssoState.Encode(orgID, nonce)
	redirectURL, err := sp.MakeRedirectAuthenticationRequest(state)
	if err != nil {
		log.Printf("[sso] saml authn request for org %d failed: %v", orgID, err)
		return ssoFailureRedirect(c, "idp_unreachable")
	}
	return c.Redirect(http.StatusFound, redirectURL.String())
}

// SSOSAMLACS is public: POST /api/v1/auth/sso/saml/:org_id/acs - the
// Assertion Consumer Service the IdP POSTs the SAMLResponse to.
func (h *AuthHandlers) SSOSAMLACS(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		return ssoFailureRedirect(c, "invalid_org")
	}
	if h.sso == nil {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	// ParseForm once, up front: crewjam's ParseResponse reads
	// req.PostForm directly rather than calling FormValue itself, so it
	// must already be populated.
	if err := c.Request().ParseForm(); err != nil {
		return ssoFailureRedirect(c, "invalid_response")
	}
	if _, err := h.ssoState.Decode(c.Request().PostForm.Get("RelayState")); err != nil {
		return ssoFailureRedirect(c, "invalid_state")
	}
	cfg, err := h.sso.Get(orgID, sso.ProviderSAML)
	if err != nil || !cfg.Enabled {
		return ssoFailureRedirect(c, "sso_not_configured")
	}
	metadataURL, acsURL := samlURLs(c, orgID)
	sp, err := sso.BuildServiceProvider(c.Request().Context(), cfg, metadataURL, acsURL)
	if err != nil {
		log.Printf("[sso] saml acs discovery for org %d failed: %v", orgID, err)
		return ssoFailureRedirect(c, "idp_unreachable")
	}
	identity, err := sso.ExchangeSAMLResponse(sp, c.Request())
	if err != nil {
		log.Printf("[sso] saml acs exchange for org %d failed: %v", orgID, err)
		h.recordAuthEvent(c, auditlog.Event{
			OrgID:     orgID,
			ActorKind: auditlog.ActorKindMember,
			Action:    auditlog.ActionLoginFailure,
			Outcome:   auditlog.OutcomeFailure,
			Severity:  auditlog.SeverityWarning,
			Details:   auditlog.Details("method", "sso_saml", "reason", "exchange_failed"),
		})
		return ssoFailureRedirect(c, "login_failed")
	}
	return h.completeSSOLogin(c, orgID, identity, "sso_saml")
}

// completeSSOLogin is the single convergence point for every SSO protocol:
// look up (or JIT-provision) the user in the target org, issue the same JWT
// pair password login issues, and send the browser back to the frontend
// with it. Never creates or touches a user outside orgID.
func (h *AuthHandlers) completeSSOLogin(c echo.Context, orgID int64, identity *sso.Identity, method string) error {
	email := strings.ToLower(strings.TrimSpace(identity.Email))
	if email == "" {
		return ssoFailureRedirect(c, "no_email_claim")
	}

	user, created, err := h.jitProvisionSSOUser(orgID, email, identity.Name)
	if err != nil {
		reason := "provisioning_failed"
		if errors.Is(err, errSSOAccountDeactivated) {
			reason = "account_deactivated"
		} else {
			log.Printf("[sso] JIT provisioning failed for %s in org %d: %v", email, orgID, err)
		}
		h.recordAuthEvent(c, auditlog.Event{
			OrgID:      orgID,
			ActorEmail: email,
			ActorKind:  auditlog.ActorKindMember,
			Action:     auditlog.ActionLoginFailure,
			Outcome:    auditlog.OutcomeFailure,
			Severity:   auditlog.SeverityWarning,
			Details:    auditlog.Details("method", method, "reason", reason),
		})
		return ssoFailureRedirect(c, reason)
	}

	userAgent := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()
	tokenPair, err := h.tokenService.CreateTokenPairWithOrg(user, orgID, userAgent, ipAddress)
	if err != nil {
		log.Printf("[sso] token issuance failed for %s in org %d: %v", email, orgID, err)
		return ssoFailureRedirect(c, "internal_error")
	}

	_, _ = h.db.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = $1`, user.ID)

	if h.ssoIdentities != nil && identity.Subject != "" {
		provider := sso.ProviderOIDC
		if method == "sso_saml" {
			provider = sso.ProviderSAML
		}
		if err := h.ssoIdentities.Upsert(orgID, user.ID, provider, identity.Subject, identity.RefreshToken); err != nil {
			// Non-fatal: this only degrades back-channel logout resolution
			// and the reconciliation poll for this login, never the login
			// itself.
			log.Printf("[sso] failed to record identity mapping for org %d user %d: %v", orgID, user.ID, err)
		}
	}

	uid := user.ID
	actorKind, actorCapacity := h.actorFor(user)
	if created {
		h.recordAuthEvent(c, auditlog.Event{
			OrgID:         orgID,
			ActorUserID:   &uid,
			ActorEmail:    email,
			ActorKind:     actorKind,
			ActorCapacity: actorCapacity,
			Action:        auditlog.ActionSSOProvision,
			Outcome:       auditlog.OutcomeSuccess,
			Severity:      auditlog.SeverityInfo,
			Details:       auditlog.Details("method", method, "role", "member"),
		})
	}
	h.recordAuthEvent(c, auditlog.Event{
		OrgID:         orgID,
		ActorUserID:   &uid,
		ActorEmail:    email,
		ActorKind:     actorKind,
		ActorCapacity: actorCapacity,
		Action:        auditlog.ActionLoginSuccess,
		Outcome:       auditlog.OutcomeSuccess,
		Severity:      auditlog.SeverityInfo,
		Details:       auditlog.Details("method", method),
	})

	target := fmt.Sprintf("%s/sso/callback#access_token=%s&refresh_token=%s&expires_at=%s&token_type=%s",
		baseURL(c),
		url.QueryEscape(tokenPair.AccessToken),
		url.QueryEscape(tokenPair.RefreshToken),
		url.QueryEscape(strconv.FormatInt(tokenPair.ExpiresAt.Unix(), 10)),
		url.QueryEscape(tokenPair.TokenType),
	)
	return c.Redirect(http.StatusFound, target)
}

// jitProvisionSSOUser finds the user by email. If they already exist
// globally, they're linked into orgID as a member (if not already a
// member there) - never promoted past member and never moved out of any
// other org they belong to. If they don't exist at all, they're created
// fresh as a member of orgID with no usable password (SSO is their only
// login path until an owner sets one).
func (h *AuthHandlers) jitProvisionSSOUser(orgID int64, email, displayName string) (*models.User, bool, error) {
	var userID int64
	var isActive bool
	created := false

	err := func() error {
		tx, err := h.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		err = tx.QueryRow(`SELECT id, is_active FROM users WHERE email = $1`, email).Scan(&userID, &isActive)
		if err == sql.ErrNoRows {
			randBytes := make([]byte, 32)
			if _, rerr := rand.Read(randBytes); rerr != nil {
				return rerr
			}
			hashed, herr := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randBytes)), bcrypt.DefaultCost)
			if herr != nil {
				return herr
			}
			firstName, lastName := splitDisplayName(displayName)
			if err := tx.QueryRow(`
				INSERT INTO users (email, password_hash, first_name, last_name, default_org_id, password_reset_required, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, false, NOW(), NOW())
				RETURNING id
			`, email, string(hashed), nullableStr(firstName), nullableStr(lastName), orgID).Scan(&userID); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO user_roles (user_id, org_id, role_id)
				VALUES ($1, $2, (SELECT id FROM roles WHERE name = 'member'))
			`, userID, orgID); err != nil {
				return err
			}
			created = true
		} else if err != nil {
			return err
		} else {
			if !isActive {
				// Deactivation is an explicit admin action; SSO login must
				// never silently undo it. Reject here rather than
				// reactivating - see errSSOAccountDeactivated.
				return errSSOAccountDeactivated
			}
			var isMember bool
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id = $1 AND org_id = $2)`, userID, orgID).Scan(&isMember); err != nil {
				return err
			}
			if !isMember {
				if _, err := tx.Exec(`
					INSERT INTO user_roles (user_id, org_id, role_id)
					VALUES ($1, $2, (SELECT id FROM roles WHERE name = 'member'))
				`, userID, orgID); err != nil {
					return err
				}
				created = true
			}
			if _, err := tx.Exec(`UPDATE users SET default_org_id = COALESCE(default_org_id, $1) WHERE id = $2`, orgID, userID); err != nil {
				return err
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		return nil, false, err
	}

	user := &models.User{}
	err = h.db.QueryRow(`
		SELECT id, email, password_hash, default_org_id, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DefaultOrgID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, false, err
	}
	return user, created, nil
}

func splitDisplayName(name string) (first, last string) {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
