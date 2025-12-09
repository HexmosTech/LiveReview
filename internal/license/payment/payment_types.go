package payment

import (
	"encoding/json"
)

// FlexBool handles both boolean and string "1"/"0" values from Razorpay
type FlexBool bool

// UnmarshalJSON implements custom unmarshaling for FlexBool
func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as bool first
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*fb = FlexBool(b)
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		// Convert "1" to true, anything else to false
		*fb = FlexBool(s == "1" || s == "true")
		return nil
	}

	// Try as number
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*fb = FlexBool(n != 0)
		return nil
	}

	// Default to false
	*fb = false
	return nil
}

// Bool returns the bool value
func (fb FlexBool) Bool() bool {
	return bool(fb)
}

// RazorpayPlan represents a Razorpay subscription plan
type RazorpayPlan struct {
	ID       string           `json:"id,omitempty"`
	Period   string           `json:"period"`
	Interval int              `json:"interval"`
	Item     RazorpayPlanItem `json:"item"`
	Notes    json.RawMessage  `json:"notes,omitempty"` // Can be {} or []
	// Read-only fields returned by API
	CreatedAt int64  `json:"created_at,omitempty"`
	Entity    string `json:"entity,omitempty"`
	// Internal fields not sent to Razorpay API
	Mode        string            `json:"-"` // "test" or "live"
	Description string            `json:"-"` // For internal use only
	NotesMap    map[string]string `json:"-"` // For sending as object when creating
}

// GetNotesMap parses the Notes field and returns it as a map
func (p *RazorpayPlan) GetNotesMap() map[string]string {
	if len(p.Notes) == 0 {
		return nil
	}

	var notesMap map[string]string
	if err := json.Unmarshal(p.Notes, &notesMap); err != nil {
		// If it's an array (empty notes), return nil
		return nil
	}
	return notesMap
}

// RazorpayPlanItem represents the item in a plan
type RazorpayPlanItem struct {
	Name     string `json:"name"`
	Amount   int    `json:"amount"`   // Amount in smallest currency unit (cents for USD, paise for INR)
	Currency string `json:"currency"` // INR, USD, etc.
}

// RazorpayPlanListResponse represents the response when fetching all plans
type RazorpayPlanListResponse struct {
	Entity string         `json:"entity"`
	Count  int            `json:"count"`
	Items  []RazorpayPlan `json:"items"`
}

// RazorpayPayment represents a Razorpay payment entity
type RazorpayPayment struct {
	ID               string          `json:"id"`
	Entity           string          `json:"entity"`
	Amount           int64           `json:"amount"`            // in paise
	Currency         string          `json:"currency"`          // e.g., "INR"
	Status           string          `json:"status"`            // created, authorized, captured, refunded, failed
	OrderID          string          `json:"order_id"`          // Associated order ID
	InvoiceID        string          `json:"invoice_id"`        // Associated invoice ID
	International    bool            `json:"international"`     // Whether payment is international
	Method           string          `json:"method"`            // card, netbanking, wallet, upi, etc.
	AmountRefunded   int64           `json:"amount_refunded"`   // Amount refunded in paise
	RefundStatus     string          `json:"refund_status"`     // null, partial, full
	Captured         FlexBool        `json:"captured"`          // Whether payment is captured (handles bool or "1"/"0")
	Description      string          `json:"description"`       // Payment description
	CardID           string          `json:"card_id"`           // Card used for payment
	Bank             string          `json:"bank"`              // Bank used for payment
	Wallet           string          `json:"wallet"`            // Wallet used for payment
	VPA              string          `json:"vpa"`               // UPI VPA
	Email            string          `json:"email"`             // Customer email
	Contact          string          `json:"contact"`           // Customer contact
	CustomerID       string          `json:"customer_id"`       // Customer ID
	TokenID          string          `json:"token_id"`          // Token ID if saved
	Notes            json.RawMessage `json:"notes"`             // Custom notes
	Fee              int64           `json:"fee"`               // Payment gateway fee
	Tax              int64           `json:"tax"`               // Tax on fee
	ErrorCode        string          `json:"error_code"`        // Error code if failed
	ErrorDescription string          `json:"error_description"` // Error description
	ErrorSource      string          `json:"error_source"`      // Error source
	ErrorStep        string          `json:"error_step"`        // Error step
	ErrorReason      string          `json:"error_reason"`      // Error reason
	CreatedAt        int64           `json:"created_at"`        // Unix timestamp
}

// GetPaymentNotesMap parses the Notes field and returns it as a map
func (p *RazorpayPayment) GetPaymentNotesMap() map[string]string {
	if len(p.Notes) == 0 {
		return nil
	}

	var notesMap map[string]string
	if err := json.Unmarshal(p.Notes, &notesMap); err != nil {
		return nil
	}
	return notesMap
}

// SubscriptionPayment represents a payment record in our database
type SubscriptionPayment struct {
	ID                int64  `json:"id"`
	SubscriptionID    int64  `json:"subscription_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayInvoiceID string `json:"razorpay_invoice_id"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	Status            string `json:"status"`
	Method            string `json:"method"`
	AuthorizedAt      *int64 `json:"authorized_at"`
	CapturedAt        *int64 `json:"captured_at"`
	FailedAt          *int64 `json:"failed_at"`
	RefundedAt        *int64 `json:"refunded_at"`
	ErrorCode         string `json:"error_code"`
	ErrorDescription  string `json:"error_description"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}
