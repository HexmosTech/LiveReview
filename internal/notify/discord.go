// Package notify sends one-off, best-effort Discord notifications: new
// signups, enterprise enquiries, and org milestones (first GitHub connect,
// first review submitted). It never blocks or fails the caller's request —
// every send is fire-and-forget and errors are only logged.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// PostDiscordWebhook sends a JSON payload to a Discord webhook and reports any failure.
func PostDiscordWebhook(webhookURL string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// PostMilestone sends a plain-text message to the signup Discord webhook
// (DISCORD_SIGNUP_WEBHOOK_URL), asynchronously. A no-op if the webhook isn't
// configured. Callers never need to wrap this in their own goroutine.
func PostMilestone(message string) {
	webhookURL := os.Getenv("DISCORD_SIGNUP_WEBHOOK_URL")
	if webhookURL == "" {
		return
	}
	go func() {
		if err := PostDiscordWebhook(webhookURL, map[string]string{"content": message}); err != nil {
			log.Printf("[discord-notify] %v", err)
		}
	}()
}
