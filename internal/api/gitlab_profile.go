package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GitLabProfile represents the user profile info fetched from GitLab
type GitLabProfile struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// FetchGitLabProfile fetches the user profile from GitLab using PAT and base URL
func FetchGitLabProfile(baseURL, pat string) (*GitLabProfile, error) {
	url := fmt.Sprintf("%s/api/v4/user", baseURL)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request - please check the GitLab URL format")
	}
	req.Header.Set("PRIVATE-TOKEN", pat)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cannot connect to GitLab instance - please verify the URL is correct and accessible")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		switch resp.StatusCode {
		case 401:
			return nil, fmt.Errorf("Invalid Personal Access Token - please verify your PAT is correct and has api scope")
		case 403:
			return nil, fmt.Errorf("Access forbidden - your PAT may not have sufficient permissions or may be expired")
		case 404:
			return nil, fmt.Errorf("GitLab instance not found - please check the URL (should end with no trailing slash)")
		default:
			return nil, fmt.Errorf("GitLab connection failed (HTTP %d) - please verify both URL and PAT are correct", resp.StatusCode)
		}
	}
	var profile GitLabProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("Invalid response from GitLab - the URL may not point to a valid GitLab instance")
	}
	return &profile, nil
}
