package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type InvitationParams struct {
	AppName        string `json:"appName"`
	InvitedToName  string `json:"invitedToName"`
	InvitedToEmail string `json:"invitedToEmail"`
	InvitedByName  string `json:"invitedByName"`
	URL            string `json:"url"`
}

func getParseAppID() string {
	if id := os.Getenv("FW_PARSE_APP_ID"); id != "" {
		return id
	}
	return "impressionserver"
}

// SendInvitationEmail sends an invitation email via the Parse Cloud Function
func SendInvitationEmail(params InvitationParams) error {
	apiURL := os.Getenv("FW_PARSE_INVITATION_URL")
	if apiURL == "" {
		return nil
	}

	appID := getParseAppID()

	fmt.Printf("[Invitation] Calling Parse invitation API at: %s for %s\n", apiURL, params.InvitedToEmail)

	jsonData, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation request: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create invitation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Parse-Application-Id", appID)

	if secret := os.Getenv("FW_PARSE_ADMIN_SECRET"); secret != "" {
		req.Header.Set("X-Internal-Admin-Secret", secret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call invitation api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invitation api returned non-ok status: %d, body: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[Invitation] Successfully called Parse invitation API for: %s\n", params.InvitedToEmail)

	var result struct {
		Result struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse invitation api response: %w, body: %s", err, string(body))
	}

	if !result.Result.Success {
		return fmt.Errorf("invitation api reported failure: %s", result.Result.Message)
	}

	return nil
}
