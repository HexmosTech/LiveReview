package azuredevops

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
)

const apiVersion = "7.1"

// NormalizeOrgURL trims whitespace and a trailing slash from an Azure DevOps
// organization URL, e.g. "https://dev.azure.com/myorg/" -> "https://dev.azure.com/myorg".
func NormalizeOrgURL(raw string) string {
	return strings.TrimSuffix(strings.TrimSpace(raw), "/")
}

// OrgNameFromURL extracts the organization name from an org URL of the form
// https://dev.azure.com/{org}.
func OrgNameFromURL(orgURL string) (string, error) {
	parsed, err := neturl.Parse(NormalizeOrgURL(orgURL))
	if err != nil {
		return "", fmt.Errorf("invalid organization URL: %w", err)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", fmt.Errorf("organization URL must include the organization name, e.g. https://dev.azure.com/myorg")
	}
	return segments[0], nil
}

// orgAPIBase returns the API base URL for a given organization name.
func orgAPIBase(org string) string {
	return "https://dev.azure.com/" + neturl.PathEscape(org)
}

// packedToken mirrors the packed-PAT convention used by other providers
// (JSON-encoded {"pat": "..."}), allowing future extension without breaking
// storage format compatibility.
type packedToken struct {
	pat string
}

func decodePackedToken(raw string) packedToken {
	var payload struct {
		Pat string `json:"pat"`
	}
	var pt packedToken
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return pt
	}
	pt.pat = payload.Pat
	return pt
}

// isEmptyObjectID reports whether an Azure DevOps blob object ID represents
// "no content" (used for added/deleted files): empty or all-zero SHA.
func isEmptyObjectID(sha string) bool {
	if sha == "" {
		return true
	}
	return strings.Trim(sha, "0") == ""
}

// parsePullRequestID parses the trailing numeric pull request id segment.
func parsePullRequestID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid pull request id %q: %w", s, err)
	}
	return id, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
