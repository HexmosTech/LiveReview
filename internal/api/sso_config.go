package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/sso"
)

// Self-hosted enterprise only, per docs/sso.md: SSO is not offered on cloud
// (yet). This checks the current org's plan rather than a hardcoded
// "self-hosted" bit, so the moment cloud ever adds "sso" to a plan's
// Features, this gate opens for that org with no code change here.
func (s *Server) orgHasSSOFeature(orgID int64) bool {
	var planCode string
	_ = s.db.QueryRow(`SELECT current_plan_code FROM org_billing_state WHERE org_id = $1`, orgID).Scan(&planCode)
	return license.PlanType(planCode).HasFeature("sso")
}

// ssoConfigRequest is the write-side payload for PUT /sso-config/:provider_type.
// OIDCClientSecret is a pointer so "omit the field" (keep existing secret)
// is distinguishable from "empty string" (clear it) on update.
type ssoConfigRequest struct {
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`

	OIDCIssuer       string  `json:"oidc_issuer,omitempty"`
	OIDCClientID     string  `json:"oidc_client_id,omitempty"`
	OIDCClientSecret *string `json:"oidc_client_secret,omitempty"`
	OIDCScopes       string  `json:"oidc_scopes,omitempty"`

	SAMLIDPMetadataURL string `json:"saml_idp_metadata_url,omitempty"`
	SAMLIDPMetadataXML string `json:"saml_idp_metadata_xml,omitempty"`
	SAMLIDPEntityID    string `json:"saml_idp_entity_id,omitempty"`

	// Reconciliation only applies to provider_type=oidc - see Phase 5 of
	// docs/sso.md's deprovisioning section. Ignored for SAML.
	ReconciliationEnabled  bool   `json:"reconciliation_enabled,omitempty"`
	ReconciliationInterval string `json:"reconciliation_interval,omitempty"`
}

// ssoConfigResponse never carries secrets - this is what GET/PUT return.
type ssoConfigResponse struct {
	ProviderType        string `json:"provider_type"`
	DisplayName         string `json:"display_name"`
	Enabled             bool   `json:"enabled"`
	OIDCIssuer          string `json:"oidc_issuer,omitempty"`
	OIDCClientID        string `json:"oidc_client_id,omitempty"`
	OIDCClientSecretSet bool   `json:"oidc_client_secret_set"`
	OIDCScopes          string `json:"oidc_scopes,omitempty"`
	SAMLIDPMetadataURL  string `json:"saml_idp_metadata_url,omitempty"`
	SAMLIDPEntityID     string `json:"saml_idp_entity_id,omitempty"`
	SAMLMetadataSet     bool   `json:"saml_metadata_set"`

	ReconciliationEnabled  bool   `json:"reconciliation_enabled,omitempty"`
	ReconciliationInterval string `json:"reconciliation_interval,omitempty"`
	LastReconciledAt       string `json:"last_reconciled_at,omitempty"`
}

func redactedSSOConfig(cfg *sso.Config) ssoConfigResponse {
	resp := ssoConfigResponse{
		ProviderType:           string(cfg.ProviderType),
		DisplayName:            cfg.DisplayName,
		Enabled:                cfg.Enabled,
		OIDCIssuer:             cfg.OIDCIssuer,
		OIDCClientID:           cfg.OIDCClientID,
		OIDCClientSecretSet:    cfg.OIDCClientSecret != "",
		OIDCScopes:             cfg.OIDCScopes,
		SAMLIDPMetadataURL:     cfg.SAMLIDPMetadataURL,
		SAMLIDPEntityID:        cfg.SAMLIDPEntityID,
		SAMLMetadataSet:        cfg.SAMLIDPMetadataXML != "",
		ReconciliationEnabled:  cfg.ReconciliationEnabled,
		ReconciliationInterval: cfg.ReconciliationInterval,
	}
	if cfg.LastReconciledAt != nil {
		resp.LastReconciledAt = cfg.LastReconciledAt.Format(time.RFC3339)
	}
	return resp
}

func parseProviderType(raw string) (sso.ProviderType, bool) {
	switch sso.ProviderType(raw) {
	case sso.ProviderOIDC, sso.ProviderSAML:
		return sso.ProviderType(raw), true
	default:
		return "", false
	}
}

// requireSSOAccess checks the "Sensitive Operations Gate" + "Session-Only
// Gating" + self-hosted-enterprise-feature rules from CLAUDE.md in one
// place, since every SSO config handler needs all three.
func (s *Server) requireSSOAccess(c echo.Context, orgID int64) error {
	if err := requireSession(c); err != nil {
		return err
	}
	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only org owners can manage SSO configuration"})
	}
	if err := pc.RequireOrgOwner(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only org owners can manage SSO configuration"})
	}
	if apimiddleware.IsCloudMode() {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SSO is available on self-hosted enterprise deployments only"})
	}
	if !s.orgHasSSOFeature(orgID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SSO requires an enterprise self-hosted license"})
	}
	if s.ssoStore == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "SSO is not available on this deployment"})
	}
	return nil
}

// ListSSOConfigs returns both provider configs for the org (whichever
// exist), with secrets redacted.
func (s *Server) ListSSOConfigs(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	if err := s.requireSSOAccess(c, orgID); err != nil {
		return err
	}
	out := []ssoConfigResponse{}
	for _, provider := range []sso.ProviderType{sso.ProviderOIDC, sso.ProviderSAML} {
		cfg, err := s.ssoStore.Get(orgID, provider)
		if err == sso.ErrNotFound {
			continue
		}
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load SSO configuration"})
		}
		out = append(out, redactedSSOConfig(cfg))
	}
	return c.JSON(http.StatusOK, out)
}

// UpsertSSOConfig is PUT /orgs/:org_id/sso-config/:provider_type. Registered
// via s.audited with ActionSettingsSSOUpdate, Sensitive: true.
func (s *Server) UpsertSSOConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	provider, ok := parseProviderType(c.Param("provider_type"))
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "provider_type must be 'oidc' or 'saml'"})
	}
	if err := s.requireSSOAccess(c, orgID); err != nil {
		return err
	}

	var req ssoConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.ReconciliationInterval != "" && req.ReconciliationInterval != "hourly" && req.ReconciliationInterval != "daily" && req.ReconciliationInterval != "weekly" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "reconciliation_interval must be 'hourly', 'daily', or 'weekly'"})
	}
	if req.DisplayName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "display_name is required"})
	}

	cfg := &sso.Config{
		OrgID:                  orgID,
		ProviderType:           provider,
		DisplayName:            req.DisplayName,
		Enabled:                req.Enabled,
		OIDCIssuer:             req.OIDCIssuer,
		OIDCClientID:           req.OIDCClientID,
		OIDCScopes:             req.OIDCScopes,
		SAMLIDPMetadataURL:     req.SAMLIDPMetadataURL,
		SAMLIDPMetadataXML:     req.SAMLIDPMetadataXML,
		SAMLIDPEntityID:        req.SAMLIDPEntityID,
		ReconciliationEnabled:  provider == sso.ProviderOIDC && req.ReconciliationEnabled,
		ReconciliationInterval: req.ReconciliationInterval,
	}

	// Preserve the existing secret/metadata unless the caller explicitly
	// sent a replacement, so the settings UI never has to round-trip a
	// decrypted secret just to leave it unchanged. A real (non-not-found)
	// lookup error must fail the request rather than be treated as "no
	// existing config" - silently swallowing it here would wipe the saved
	// client secret/metadata on a transient DB error.
	existing, gerr := s.ssoStore.Get(orgID, provider)
	if gerr != nil && gerr != sso.ErrNotFound {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load existing SSO configuration"})
	}
	if gerr == nil {
		cfg.OIDCClientSecret = existing.OIDCClientSecret
		if cfg.SAMLIDPMetadataXML == "" {
			cfg.SAMLIDPMetadataXML = existing.SAMLIDPMetadataXML
		}
	}
	if req.OIDCClientSecret != nil {
		cfg.OIDCClientSecret = *req.OIDCClientSecret
	}

	pc := auth.GetPermissionContext(c)
	var actorUserID int64
	if pc != nil && pc.User != nil {
		actorUserID = pc.User.ID
	}

	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save SSO configuration"})
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.ssoStore.UpsertTx(tx, cfg, actorUserID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save SSO configuration"})
	}
	if s.auditRecorder != nil {
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:        orgID,
			Action:       auditlog.ActionSettingsSSOUpdate,
			Category:     auditlog.CategorySettings,
			ResourceType: "org_sso_config",
			ResourceID:   string(provider),
			Outcome:      auditlog.OutcomeSuccess,
			Severity:     auditlog.SeverityCritical,
			Details:      auditlog.Details("provider_type", string(provider), "enabled", req.Enabled, "display_name", req.DisplayName),
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save SSO configuration"})
	}

	saved, err := s.ssoStore.Get(orgID, provider)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "saved, but failed to re-read configuration"})
	}
	return c.JSON(http.StatusOK, redactedSSOConfig(saved))
}

// DeleteSSOConfig is DELETE /orgs/:org_id/sso-config/:provider_type.
func (s *Server) DeleteSSOConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	provider, ok := parseProviderType(c.Param("provider_type"))
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "provider_type must be 'oidc' or 'saml'"})
	}
	if err := s.requireSSOAccess(c, orgID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete SSO configuration"})
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.ssoStore.DeleteTx(tx, orgID, provider); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete SSO configuration"})
	}
	if s.auditRecorder != nil {
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:        orgID,
			Action:       auditlog.ActionSettingsSSODelete,
			Category:     auditlog.CategorySettings,
			ResourceType: "org_sso_config",
			ResourceID:   string(provider),
			Outcome:      auditlog.OutcomeSuccess,
			Severity:     auditlog.SeverityCritical,
			Details:      auditlog.Details("provider_type", string(provider)),
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete SSO configuration"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "SSO configuration deleted"})
}

// TestSSOConfig is POST /orgs/:org_id/sso-config/:provider_type/test. It
// validates the IdP is reachable without saving anything, so the settings
// UI can give immediate feedback before an owner commits a change.
func (s *Server) TestSSOConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	provider, ok := parseProviderType(c.Param("provider_type"))
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "provider_type must be 'oidc' or 'saml'"})
	}
	if err := s.requireSSOAccess(c, orgID); err != nil {
		return err
	}

	var req ssoConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	switch provider {
	case sso.ProviderOIDC:
		ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
		defer cancel()
		if err := sso.TestOIDCDiscovery(ctx, req.OIDCIssuer); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "OIDC discovery succeeded"})
	case sso.ProviderSAML:
		ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
		defer cancel()
		testCfg := &sso.Config{
			ID:                 orgID, // only used for error messages here
			ProviderType:       sso.ProviderSAML,
			SAMLIDPMetadataURL: req.SAMLIDPMetadataURL,
			SAMLIDPMetadataXML: req.SAMLIDPMetadataXML,
		}
		if testCfg.SAMLIDPMetadataXML == "" {
			if existing, gerr := s.ssoStore.Get(orgID, sso.ProviderSAML); gerr == nil {
				testCfg.SAMLIDPMetadataXML = existing.SAMLIDPMetadataXML
			}
		}
		if err := sso.TestSAMLMetadata(ctx, testCfg); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "SAML IdP metadata parsed successfully"})
	}
	return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported provider_type"})
}
