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
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", pat)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}
	var profile GitLabProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}
