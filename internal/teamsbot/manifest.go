package teamsbot

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

// azureGUIDPattern matches Azure App IDs and Tenant IDs, which are always
// GUIDs - defense in depth against building a manifest from a bad
// bot_app_id (e.g. a value that reached storage before validation existed
// on the save path).
var azureGUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// teamsManifestVersion pins the Teams app manifest schema version this
// package generates against. Verified field-by-field against Microsoft's
// actual v1.19 manifest JSON schema (not just the changelog docs) - a
// stable, long-supported version, chosen deliberately over the newer 1.30
// whose exact field requirements weren't independently verified.
const teamsManifestVersion = "1.19"

type teamsManifestDeveloper struct {
	Name          string `json:"name"`
	WebsiteURL    string `json:"websiteUrl"`
	PrivacyURL    string `json:"privacyUrl"`
	TermsOfUseURL string `json:"termsOfUseUrl"`
}

type teamsManifestLocalizable struct {
	Short string `json:"short"`
	Full  string `json:"full"`
}

type teamsManifestIcons struct {
	Color   string `json:"color"`
	Outline string `json:"outline"`
}

type teamsManifestBot struct {
	BotID              string   `json:"botId"`
	Scopes             []string `json:"scopes"`
	SupportsFiles      bool     `json:"supportsFiles"`
	IsNotificationOnly bool     `json:"isNotificationOnly"`
}

type teamsManifest struct {
	Schema          string                   `json:"$schema"`
	ManifestVersion string                   `json:"manifestVersion"`
	Version         string                   `json:"version"`
	ID              string                   `json:"id"`
	Developer       teamsManifestDeveloper   `json:"developer"`
	Name            teamsManifestLocalizable `json:"name"`
	Description     teamsManifestLocalizable `json:"description"`
	Icons           teamsManifestIcons       `json:"icons"`
	AccentColor     string                   `json:"accentColor"`
	Bots            []teamsManifestBot       `json:"bots"`
	ValidDomains    []string                 `json:"validDomains"`
}

// BuildAppPackage generates the Teams app package (manifest.json + icons,
// zipped) that a Teams admin uploads via Teams Admin Center to make Livi
// discoverable/installable for their org. Azure Bot Service registration
// alone (what the rest of this package wires up) is not enough for that -
// Teams only lets users find/add/@mention a bot through an installed app
// package. baseURL is the instance's own public base URL (same value
// BaseURL() resolves for chart image links), used both to identify whose
// deployment this is (developer.websiteUrl/privacyUrl/termsOfUseUrl - there
// is no dedicated privacy/terms page on a self-hosted instance, and every
// customer's instance lives at its own domain, so these intentionally point
// at the instance root rather than a hardcoded marketing domain) and to
// declare validDomains (needed because reply chart images are served from
// this same host's /api/charts/:id - see report.go).
func BuildAppPackage(cfg *TeamsConfig, baseURL string) ([]byte, error) {
	if cfg == nil || cfg.BotAppID == "" {
		return nil, fmt.Errorf("teams bot is not configured for this org yet")
	}
	if !azureGUIDPattern.MatchString(cfg.BotAppID) {
		return nil, fmt.Errorf("bot_app_id %q is not a valid Azure App ID (GUID) - re-save the Teams config with the correct value from the Azure Bot resource before downloading the app package", cfg.BotAppID)
	}
	if !isPublicHTTPSURL(baseURL) {
		return nil, fmt.Errorf("refusing to build a Teams app package with a non-public base URL %q - configure a real public HTTPS instance URL first", baseURL)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid instance base URL %q: %w", baseURL, err)
	}
	origin := parsed.Scheme + "://" + parsed.Host
	host := parsed.Hostname()

	manifest := teamsManifest{
		Schema:          fmt.Sprintf("https://developer.microsoft.com/json-schemas/teams/v%s/MicrosoftTeams.schema.json", teamsManifestVersion),
		ManifestVersion: teamsManifestVersion,
		Version:         "1.0.0",
		ID:              cfg.BotAppID,
		Developer: teamsManifestDeveloper{
			Name:          "LiveReview",
			WebsiteURL:    origin,
			PrivacyURL:    origin,
			TermsOfUseURL: origin,
		},
		Name: teamsManifestLocalizable{
			Short: "Livi",
			Full:  "Livi - LiveReview Assistant",
		},
		Description: teamsManifestLocalizable{
			Short: "LiveReview's AI code review assistant",
			Full:  "Livi helps engineering teams trigger code reviews, check review status, and answer analytics questions directly inside Microsoft Teams.",
		},
		Icons: teamsManifestIcons{
			Color:   "color.png",
			Outline: "outline.png",
		},
		AccentColor: "#001c5e",
		Bots: []teamsManifestBot{
			{
				BotID:              cfg.BotAppID,
				Scopes:             []string{"team", "personal", "groupChat"},
				SupportsFiles:      false,
				IsNotificationOnly: false,
			},
		},
		ValidDomains: []string{host},
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, f := range []struct {
		name string
		data []byte
	}{
		{"manifest.json", manifestJSON},
		{"color.png", teamsAppColorPNG},
		{"outline.png", teamsAppOutlinePNG},
	} {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s: %w", f.name, err)
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, fmt.Errorf("write zip entry %s: %w", f.name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}

	return buf.Bytes(), nil
}
