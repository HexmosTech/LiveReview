package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	networkazuredevops "github.com/livereview/network/providers/azuredevops"
)

// Profile represents the authenticated user's Azure DevOps profile, used to
// confirm a PAT is valid and to display connector confirmation info in the UI.
type Profile struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	OrgName      string `json:"orgName"`
}

// connectionDataResponse mirrors the subset of the Connection Data API we need.
// Unlike the vssps profile API, this is scoped to the organization and only
// requires whatever scope the PAT already has for that org (e.g. Code),
// rather than a separate "User Profile (Read)" scope.
type connectionDataResponse struct {
	AuthenticatedUser struct {
		ID                  string `json:"id"`
		ProviderDisplayName string `json:"providerDisplayName"`
		CustomDisplayName   string `json:"customDisplayName"`
	} `json:"authenticatedUser"`
}

// FetchAzureDevOpsProfile validates a PAT against the given organization URL
// (e.g. https://dev.azure.com/myorg) by fetching the authenticated user's
// identity via the org-scoped Connection Data API.
func FetchAzureDevOpsProfile(orgURL, pat string) (*Profile, error) {
	if pt := decodePackedToken(pat); pt.pat != "" {
		pat = pt.pat
	}

	apiBase := NormalizeOrgURL(orgURL)
	if apiBase == "" {
		return nil, fmt.Errorf("organization URL is required, e.g. https://dev.azure.com/myorg")
	}
	orgName, err := OrgNameFromURL(apiBase)
	if err != nil {
		return nil, err
	}

	// A bare context.Background() + zero-value http.Client has no deadline at
	// all - a slow/unresponsive org URL (including one a user mistypes during
	// PAT validation) would block this call, and the request handler calling
	// it, indefinitely.
	client := networkazuredevops.NewHTTPClient(15 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	apiURL := fmt.Sprintf("%s/_apis/connectionData?api-version=7.1-preview", apiBase)
	req, err := networkazuredevops.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request")
	}
	networkazuredevops.ApplyPATAuth(req, pat)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Azure DevOps - please check the organization URL and network connectivity")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("invalid Personal Access Token - verify the token and its scopes")
	case http.StatusNotFound:
		return nil, fmt.Errorf("organization not found - verify the organization URL")
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("azure devops connection failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var cd connectionDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&cd); err != nil {
		return nil, fmt.Errorf("failed to decode connection data response: %w", err)
	}

	return &Profile{
		ID:           cd.AuthenticatedUser.ID,
		DisplayName:  firstNonEmpty(cd.AuthenticatedUser.CustomDisplayName, cd.AuthenticatedUser.ProviderDisplayName),
		EmailAddress: cd.AuthenticatedUser.ProviderDisplayName,
		OrgName:      orgName,
	}, nil
}
