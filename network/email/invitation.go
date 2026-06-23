package email

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type InvitationParams struct {
	AppName        string `json:"appName"`
	InvitedToName  string `json:"invitedToName"`
	InvitedToEmail string `json:"invitedToEmail"`
	InvitedByName  string `json:"invitedByName"`
	URL            string `json:"url"`
	InstallCommandLinux   string `json:"installCommandLinux,omitempty"`
	InstallCommandWindows string `json:"installCommandWindows,omitempty"`
}

func getParseAppID() string {
	if id := os.Getenv("FW_PARSE_APP_ID"); id != "" {
		return id
	}
	return "impressionserver"
}

// SendInvitationEmail sends an invitation email. It uses SMTP for self-hosted/enterprise deployments
func SendInvitationEmail(db *sql.DB, params InvitationParams) error {
	isCloud := strings.ToLower(os.Getenv("LIVEREVIEW_IS_CLOUD")) == "true"
	if !isCloud {
		// First try fetching from database system_settings
		var data []byte
		
		err := db.QueryRow("SELECT data FROM system_settings WHERE name = 'smtp'").Scan(&data)
		if err == nil {
			var settings struct {
				Host       string `json:"host"`
				Port       int    `json:"port"`
				Username   string `json:"username"`
				Password   string `json:"password"`
				Sender     string `json:"sender"`
				SenderName string `json:"sender_name"`
				SkipTLS    bool   `json:"skip_tls"`
			}
			if err := json.Unmarshal(data, &settings); err == nil && settings.Host != "" {
				return SendInvitationEmailSMTP(
					settings.Host,
					settings.Port,
					settings.Username,
					settings.Password,
					settings.Sender,
					settings.SenderName,
					settings.SkipTLS,
					params,
				)
			}
		}

		fmt.Println("[Invitation] Selfhosted/Enterprise mode: SMTP settings not found in database, skipping invitation email")
		return nil
	}

	baseURL := os.Getenv("FW_PARSE_BASE_URL")
	if baseURL == "" {
		fmt.Println("[Invitation] Cloud mode: FW_PARSE_BASE_URL not set, skipping invitation")
		return nil
	}
	apiURL := fmt.Sprintf("%s/parse/functions/userInvitation", baseURL)

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
