package github

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GitHubProfile represents the user profile info fetched from GitHub.
type GitHubProfile struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Company   string `json:"company"`
	Location  string `json:"location"`
}

// FetchGitHubProfile fetches the user profile from GitHub using PAT.
func FetchGitHubProfile(pat string) (*GitHubProfile, error) {
	url := "https://api.github.com/user"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request - please check the request format")
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s", pat))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to GitHub - please verify your internet connection")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("invalid Personal Access Token - please verify your PAT is correct and has the required scopes")
		case http.StatusForbidden:
			return nil, fmt.Errorf("access forbidden - your PAT may not have sufficient permissions or rate limit exceeded")
		case http.StatusNotFound:
			return nil, fmt.Errorf("GitHub API endpoint not found - please check your token configuration")
		default:
			return nil, fmt.Errorf("GitHub connection failed (HTTP %d) - please verify your PAT is correct", resp.StatusCode)
		}
	}

	var profile GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("invalid response from GitHub - the API may be temporarily unavailable")
	}

	return &profile, nil
}
