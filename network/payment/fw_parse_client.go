package payment

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultSelfHostedJWTIssueURL = "https://parse.apps.hexmos.com/jwtLicence/issue"

func getSelfHostedJWTIssueURL() string {
	if configured := os.Getenv("FW_PARSE_ISSUE_URL"); configured != "" {
		return configured
	}
	return defaultSelfHostedJWTIssueURL
}

func IssueSelfHostedJWTRequest(secret string, payload []byte) (int, []byte, error) {
	if secret == "" {
		return 0, nil, fmt.Errorf("fw-parse admin secret is empty")
	}

	req, err := http.NewRequest(http.MethodPost, getSelfHostedJWTIssueURL(), bytes.NewBuffer(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Admin-Secret", secret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to call fw-parse: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return resp.StatusCode, respBody, nil
}
