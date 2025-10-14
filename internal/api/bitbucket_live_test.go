package api

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

type bitbucketUserResponse struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
}

// TestBitbucketFetchBotUser_Live fetches bot details from Bitbucket's /user endpoint using
// basic auth credentials provided via BITBUCKET_BOT_EMAIL and BITBUCKET_BOT_TOKEN.
// The test is skipped automatically unless both environment variables are set.
func TestBitbucketFetchBotUser_Live(t *testing.T) {
	email := os.Getenv("BITBUCKET_BOT_EMAIL")
	token := os.Getenv("BITBUCKET_BOT_TOKEN")
	if email == "" || token == "" {
		t.Skip("BITBUCKET_BOT_EMAIL/BITBUCKET_BOT_TOKEN not set; skipping live Bitbucket user fetch test")
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.bitbucket.org/2.0/user", nil)
	if err != nil {
		t.Fatalf("failed to construct request: %v", err)
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot-Integration-Test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call Bitbucket user API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Bitbucket API returned status %d", resp.StatusCode)
	}

	var user bitbucketUserResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&user); err != nil {
		t.Fatalf("failed to decode Bitbucket user response: %v", err)
	}

	t.Logf("Bitbucket bot user fetched successfully: username=%s display_name=%s account_id=%s uuid=%s", user.Username, user.DisplayName, user.AccountID, user.UUID)
}
