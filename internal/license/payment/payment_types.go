package license

import "encoding/json"

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
