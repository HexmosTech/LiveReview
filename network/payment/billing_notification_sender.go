package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type BillingEmailMessage struct {
	ToEmail   string
	OrgID     int64
	EventType string
	Payload   json.RawMessage
}

func SendBillingNotificationEmailPlaceholder(ctx context.Context, message BillingEmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(message.ToEmail) == "" {
		return fmt.Errorf("recipient email is required")
	}
	if message.OrgID <= 0 {
		return fmt.Errorf("org id must be > 0")
	}
	if strings.TrimSpace(message.EventType) == "" {
		return fmt.Errorf("event type is required")
	}

	log.Printf("[billing-email-placeholder] org=%d to=%s event=%s payload=%s", message.OrgID, strings.TrimSpace(message.ToEmail), strings.TrimSpace(message.EventType), strings.TrimSpace(string(message.Payload)))
	return nil
}
