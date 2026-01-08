package gitea

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Credentials bundles PAT plus optional basic credentials when provided in packed JSON.
type Credentials struct {
	PAT      string
	Username string
	Password string
}

// NormalizeGiteaBaseURL trims common swagger/api suffixes and returns a clean base URL.
// Unlike the old version, this preserves sub-paths for Gitea instances like https://example.com/gitea
func NormalizeGiteaBaseURL(raw string) string {
	trimmed := strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	if trimmed == "" {
		return ""
	}

	// Strip swagger or api path suffixes
	trimmed = strings.TrimSuffix(trimmed, "/api/swagger")
	trimmed = strings.TrimSuffix(trimmed, "/swagger")
	// Also strip just /api in case someone enters that
	if strings.HasSuffix(trimmed, "/api") {
		trimmed = strings.TrimSuffix(trimmed, "/api")
	}

	// Parse and validate URL structure
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return trimmed
	}

	// Reconstruct URL preserving scheme, host (with port), and path
	// This handles sub-path deployments like https://example.com/gitea
	result := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	if u.Path != "" && u.Path != "/" {
		result += u.Path
	}

	return result
}

// UnpackGiteaPAT extracts the PAT from packed token format (JSON with pat/username/password).
// If unpacking fails or PAT is empty, returns the raw token as-is.
func UnpackGiteaPAT(raw string) string {
	var payload struct {
		Pat      string `json:"pat"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		// Not packed JSON, return as-is
		return raw
	}
	if payload.Pat != "" {
		return payload.Pat
	}
	// Fallback to raw if no pat field
	return raw
}

// UnpackGiteaCredentials returns PAT plus optional username/password if present.
// If unpacking fails, only PAT is populated with the raw token value.
func UnpackGiteaCredentials(raw string) Credentials {
	var payload struct {
		Pat      string `json:"pat"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return Credentials{PAT: raw}
	}
	if payload.Pat == "" {
		payload.Pat = raw
	}
	return Credentials{PAT: payload.Pat, Username: payload.Username, Password: payload.Password}
}
