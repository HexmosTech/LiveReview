package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateSubscription creates a new subscription for a plan
func CreateSubscription(mode, planID string, quantity int, notesMap map[string]string) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	// Create subscription request
	type createSubscriptionRequest struct {
		PlanID         string            `json:"plan_id"`
		Quantity       int               `json:"quantity"`
		TotalCount     int               `json:"total_count"`     // 0 for infinite
		CustomerNotify int               `json:"customer_notify"` // 1 to notify customer
		Notes          map[string]string `json:"notes,omitempty"`
	}

	reqBody := createSubscriptionRequest{
		PlanID:         planID,
		Quantity:       quantity,
		TotalCount:     12, // Default to 12 billing cycles (1 year for monthly, 12 years for yearly)
		CustomerNotify: 0,  // Don't notify in test mode
		Notes:          notesMap,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling subscription request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions", razorpayBaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}

// GetAllSubscriptions fetches all subscriptions
func GetAllSubscriptions(mode string) (*RazorpaySubscriptionListResponse, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/subscriptions", razorpayBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subList RazorpaySubscriptionListResponse
	if err := json.Unmarshal(body, &subList); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subList, nil
}

// GetSubscriptionByID fetches a specific subscription by ID
func GetSubscriptionByID(mode, subscriptionID string) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/subscriptions/%s", razorpayBaseURL, subscriptionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}

// UpdateSubscriptionQuantity updates the quantity of a subscription
// scheduleChangeAt: Unix timestamp when to apply the change (0 for end of current cycle)
func UpdateSubscriptionQuantity(mode, subscriptionID string, quantity int, scheduleChangeAt int64) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	updateReq := SubscriptionUpdateRequest{
		Quantity:         quantity,
		ScheduleChangeAt: scheduleChangeAt,
		CustomerNotify:   0,
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("error marshaling update request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s", razorpayBaseURL, subscriptionID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}

// CancelSubscription cancels a subscription
// cancelAtCycleEnd: true to cancel at end of billing cycle, false for immediate cancellation
func CancelSubscription(mode, subscriptionID string, cancelAtCycleEnd bool) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	cancelReq := SubscriptionCancelRequest{
		CancelAtCycleEnd: 0, // Immediate by default
	}
	if cancelAtCycleEnd {
		cancelReq.CancelAtCycleEnd = 1
	}

	jsonData, err := json.Marshal(cancelReq)
	if err != nil {
		return nil, fmt.Errorf("error marshaling cancel request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s/cancel", razorpayBaseURL, subscriptionID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}
