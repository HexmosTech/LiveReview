package payment

import "encoding/json"

// RazorpaySubscription represents a Razorpay subscription
type RazorpaySubscription struct {
	ID                  string          `json:"id,omitempty"`
	PlanID              string          `json:"plan_id"`
	Status              string          `json:"status,omitempty"`          // created, authenticated, active, pending, halted, cancelled, completed, expired, paused
	Quantity            int             `json:"quantity,omitempty"`        // Number of subscriptions (e.g., number of users)
	TotalCount          int             `json:"total_count,omitempty"`     // Number of billing cycles, 0 for infinite
	CustomerNotify      bool            `json:"customer_notify,omitempty"` // Whether to notify customer
	StartAt             int64           `json:"start_at,omitempty"`        // Unix timestamp
	EndAt               int64           `json:"end_at,omitempty"`          // Unix timestamp
	CreatedAt           int64           `json:"created_at,omitempty"`      // Unix timestamp
	ChargeAt            int64           `json:"charge_at,omitempty"`       // Unix timestamp
	AuthAttempts        int             `json:"auth_attempts,omitempty"`   // Number of authorization attempts
	PaidCount           int             `json:"paid_count,omitempty"`      // Number of successful payments
	RemainingCount      int             `json:"remaining_count,omitempty"` // Remaining billing cycles
	CurrentStart        int64           `json:"current_start,omitempty"`   // Current billing cycle start
	CurrentEnd          int64           `json:"current_end,omitempty"`     // Current billing cycle end
	ShortURL            string          `json:"short_url,omitempty"`       // Payment link
	HasScheduledChanges bool            `json:"has_scheduled_changes,omitempty"`
	ChangeScheduledAt   int64           `json:"change_scheduled_at,omitempty"`
	Notes               json.RawMessage `json:"notes,omitempty"` // Can be {} or []
	OfferID             string          `json:"offer_id,omitempty"`
	Entity              string          `json:"entity,omitempty"`
	// Internal fields
	Mode     string            `json:"-"` // "test" or "live"
	NotesMap map[string]string `json:"-"` // For sending as object when creating
}

// GetNotesMap parses the Notes field and returns it as a map
func (s *RazorpaySubscription) GetNotesMap() map[string]string {
	if len(s.Notes) == 0 {
		return nil
	}

	var notesMap map[string]string
	if err := json.Unmarshal(s.Notes, &notesMap); err != nil {
		return nil
	}
	return notesMap
}

// RazorpaySubscriptionListResponse represents the response when fetching all subscriptions
type RazorpaySubscriptionListResponse struct {
	Entity string                 `json:"entity"`
	Count  int                    `json:"count"`
	Items  []RazorpaySubscription `json:"items"`
}

// SubscriptionUpdateRequest represents a request to update subscription quantity
type SubscriptionUpdateRequest struct {
	Quantity         int   `json:"quantity,omitempty"`
	ScheduleChangeAt int64 `json:"schedule_change_at,omitempty"` // Unix timestamp, defaults to cycle_end
	CustomerNotify   int   `json:"customer_notify,omitempty"`
}

// SubscriptionCancelRequest represents a request to cancel a subscription
type SubscriptionCancelRequest struct {
	CancelAtCycleEnd int `json:"cancel_at_cycle_end"` // 1 for end of cycle, 0 for immediate
}
