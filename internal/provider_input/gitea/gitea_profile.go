package gitea

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GiteaProfile represents the user profile info fetched from Gitea using a PAT.
type GiteaProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// FetchGiteaProfile validates a PAT by fetching the authenticated user's profile.
// It calls <baseURL>/api/v1/user using the provided PAT.
func FetchGiteaProfile(baseURL, pat string) (*GiteaProfile, error) {
	base := strings.TrimSuffix(baseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("base_url is required for Gitea")
	}
	apiURL := fmt.Sprintf("%s/api/v1/user", base)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request")
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s", pat))
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Gitea - please check the URL and network connectivity")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("invalid Personal Access Token - verify the token and scopes")
	default:
		return nil, fmt.Errorf("Gitea connection failed (HTTP %d) - verify base URL and PAT", resp.StatusCode)
	}

	var profile GiteaProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("invalid response from Gitea - unable to parse profile")
	}

	return &profile, nil
}
