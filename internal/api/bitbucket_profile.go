package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// BitbucketProfile represents the user profile info fetched from Bitbucket
type BitbucketProfile struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	Website     string `json:"website"`
	Location    string `json:"location"`
	Links       struct {
		Avatar struct {
			Href string `json:"href"`
		} `json:"avatar"`
	} `json:"links"`
}

// FetchBitbucketProfile fetches the user profile from Bitbucket using Atlassian API token
func FetchBitbucketProfile(email, apiToken string) (*BitbucketProfile, error) {
	// First, let's try a simpler endpoint to test authentication
	url := "https://api.bitbucket.org/2.0/user"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request - please check the request format")
	}

	// Use Basic Auth with email and Atlassian API token
	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Bitbucket - please verify your internet connection")
	}
	defer resp.Body.Close()

	// Read response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body")
	}

	if resp.StatusCode != 200 {
		// Log the actual response for debugging
		fmt.Printf("Bitbucket API response status: %d\n", resp.StatusCode)
		fmt.Printf("Bitbucket API response body: %s\n", string(body))

		switch resp.StatusCode {
		case 401:
			return nil, fmt.Errorf("authentication failed - please verify your Atlassian account email and API token. Ensure the API token is not expired and has the correct permissions")
		case 403:
			return nil, fmt.Errorf("access forbidden - your API token may have insufficient permissions. The token needs read access to your account information")
		case 404:
			return nil, fmt.Errorf("API endpoint not found - this might indicate an issue with the Bitbucket API or your account")
		default:
			return nil, fmt.Errorf("Bitbucket API error (HTTP %d) - please check your credentials and try again", resp.StatusCode)
		}
	}

	var profile BitbucketProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		fmt.Printf("Failed to parse Bitbucket response: %s\n", string(body))
		return nil, fmt.Errorf("invalid response from Bitbucket API - unable to parse user profile")
	}

	return &profile, nil
}

// ValidateBitbucketToken validates a Bitbucket API token by making a simple API call
func ValidateBitbucketToken(email, apiToken string) error {
	// Use the same endpoint as profile fetching for consistency
	url := "https://api.bitbucket.org/2.0/user"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request")
	}

	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to Bitbucket API")
	}
	defer resp.Body.Close()

	// Read response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read validation response body: %v\n", err)
	} else {
		fmt.Printf("Bitbucket validation response status: %d\n", resp.StatusCode)
		fmt.Printf("Bitbucket validation response body: %s\n", string(body))
	}

	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid email or API token - please verify your Atlassian account email and API token are correct")
	} else if resp.StatusCode == 403 {
		return fmt.Errorf("API token has insufficient permissions - ensure your token has account read permissions")
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("API validation failed with status %d - please check your credentials", resp.StatusCode)
	}

	return nil
}
