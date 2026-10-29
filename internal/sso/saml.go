package sso

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	samllib "github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
)

// BuildServiceProvider constructs a crewjam/saml SP for this org's SAML
// config. Requests are sent unsigned (SP.Key/Certificate left nil) - most
// IdPs, Keycloak included, accept unsigned AuthnRequests by default, and
// this avoids LiveReview having to generate/rotate/store an SP signing
// keypair for an MVP. AllowIDPInitiated is set because LiveReview is
// otherwise stateless across the login round trip (same reasoning as
// OIDC's signed `state`): the signed RelayState (see state.go) already
// carries the anti-replay/short-TTL guarantee `InResponseTo` matching would
// give, so skipping that check is a deliberate, documented trade rather
// than an oversight.
func BuildServiceProvider(ctx context.Context, cfg *Config, metadataURL, acsURL string) (*samllib.ServiceProvider, error) {
	if cfg.ProviderType != ProviderSAML {
		return nil, fmt.Errorf("sso: config %d is not a SAML config", cfg.ID)
	}
	idpMetadata, err := loadIDPMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}
	metaURL, err := url.Parse(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("sso: invalid metadata URL: %w", err)
	}
	acsU, err := url.Parse(acsURL)
	if err != nil {
		return nil, fmt.Errorf("sso: invalid ACS URL: %w", err)
	}
	sp := &samllib.ServiceProvider{
		EntityID:          metadataURL,
		MetadataURL:       *metaURL,
		AcsURL:            *acsU,
		IDPMetadata:       idpMetadata,
		AllowIDPInitiated: true,
	}
	return sp, nil
}

func loadIDPMetadata(ctx context.Context, cfg *Config) (*samllib.EntityDescriptor, error) {
	if strings.TrimSpace(cfg.SAMLIDPMetadataXML) != "" {
		md, err := samlsp.ParseMetadata([]byte(cfg.SAMLIDPMetadataXML))
		if err != nil {
			return nil, fmt.Errorf("sso: parsing stored SAML IdP metadata: %w", err)
		}
		return md, nil
	}
	if strings.TrimSpace(cfg.SAMLIDPMetadataURL) == "" {
		return nil, fmt.Errorf("sso: SAML config %d has neither metadata XML nor a metadata URL", cfg.ID)
	}
	u, err := url.Parse(cfg.SAMLIDPMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("sso: invalid SAML IdP metadata URL: %w", err)
	}
	md, err := samlsp.FetchMetadata(ctx, http.DefaultClient, *u)
	if err != nil {
		return nil, fmt.Errorf("sso: fetching SAML IdP metadata from %q: %w", cfg.SAMLIDPMetadataURL, err)
	}
	return md, nil
}

// samlEmailAttributeNames covers the attribute names IdPs commonly use for
// email when it isn't carried in the NameID itself (Keycloak, Okta, Azure
// AD/Entra all use one of these under default mappings).
var samlEmailAttributeNames = []string{
	"email",
	"Email",
	"mail",
	"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
	"urn:oid:0.9.2342.19200300.100.1.3",
}

// ExchangeSAMLResponse validates the POSTed SAMLResponse against the IdP
// and extracts the identity. ParseResponse covers signature verification,
// audience/recipient/expiry validation - only identity extraction is ours.
func ExchangeSAMLResponse(sp *samllib.ServiceProvider, r *http.Request) (*Identity, error) {
	assertion, err := sp.ParseResponse(r, []string{})
	if err != nil {
		if ire, ok := err.(*samllib.InvalidResponseError); ok {
			return nil, fmt.Errorf("sso: saml response validation failed: %w (detail: %v)", err, ire.PrivateErr)
		}
		return nil, fmt.Errorf("sso: saml response validation failed: %w", err)
	}

	identity := &Identity{}
	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		v := strings.TrimSpace(assertion.Subject.NameID.Value)
		if strings.Contains(v, "@") {
			identity.Email = v
		}
	}
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			name := attr.Name
			if attr.FriendlyName != "" {
				name = attr.FriendlyName
			}
			if identity.Email == "" && containsFold(samlEmailAttributeNames, name) && len(attr.Values) > 0 {
				identity.Email = strings.TrimSpace(attr.Values[0].Value)
			}
			if identity.Name == "" && (strings.EqualFold(name, "name") || strings.EqualFold(name, "displayName")) && len(attr.Values) > 0 {
				identity.Name = strings.TrimSpace(attr.Values[0].Value)
			}
		}
	}
	if identity.Email == "" {
		var nameIDVal, nameIDFmt string
		if assertion.Subject != nil && assertion.Subject.NameID != nil {
			nameIDVal, nameIDFmt = assertion.Subject.NameID.Value, assertion.Subject.NameID.Format
		}
		var attrNames []string
		for _, stmt := range assertion.AttributeStatements {
			for _, attr := range stmt.Attributes {
				attrNames = append(attrNames, fmt.Sprintf("%s/%s=%v", attr.Name, attr.FriendlyName, attr.Values))
			}
		}
		return nil, fmt.Errorf("sso: saml assertion had no usable email (NameID=%q format=%q attrs=%v)", nameIDVal, nameIDFmt, attrNames)
	}
	return identity, nil
}

func containsFold(candidates []string, name string) bool {
	for _, c := range candidates {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

// TestSAMLMetadata is used by the config "test connection" endpoint - it
// only proves the IdP metadata (URL or pasted XML) parses, without a full
// login round trip.
func TestSAMLMetadata(ctx context.Context, cfg *Config) error {
	_, err := loadIDPMetadata(ctx, cfg)
	return err
}
