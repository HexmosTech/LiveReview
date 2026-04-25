package payment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const scheduleChangeAtNowSentinel int64 = -1

var ErrNoPendingScheduledChange = errors.New("no pending update for subscription")

var razorpayHTTPClient = &http.Client{Timeout: 20 * time.Second}

type razorpayAPIErrorResponse struct {
	Error struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error"`
}

func isNoPendingScheduledChangeError(statusCode int, body []byte) bool {
	if statusCode < http.StatusBadRequest {
		return false
	}

	var apiErr razorpayAPIErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return false
	}

	description := strings.ToLower(strings.TrimSpace(apiErr.Error.Description))
	return strings.Contains(description, "no pending update")
}

func buildUpdateSubscriptionRequest(quantity int, scheduleChangeAt int64) map[string]interface{} {
	updateReq := map[string]interface{}{
		"quantity":        quantity,
		"customer_notify": 0,
	}
	if scheduleChangeAt == scheduleChangeAtNowSentinel {
		// Razorpay expects explicit "now" for immediate/prorated changes.
		updateReq["schedule_change_at"] = "now"
	} else {
		// Razorpay accepts cycle_end scheduling token for deferred quantity changes.
		updateReq["schedule_change_at"] = "cycle_end"
	}
	return updateReq
}

func buildCreateSubscriptionRequest(planID string, quantity int, notesMap map[string]string, startAt int64) map[string]interface{} {
	createReq := map[string]interface{}{
		"plan_id":         strings.TrimSpace(planID),
		"quantity":        quantity,
		"total_count":     12,
		"customer_notify": 0,
	}
	if len(notesMap) > 0 {
		createReq["notes"] = notesMap
	}
	if startAt > 0 {
		createReq["start_at"] = startAt
	}
	return createReq
}

// CreateSubscription creates a new subscription for a plan
func CreateSubscription(mode, planID string, quantity int, notesMap map[string]string) (*RazorpaySubscription, error) {
	return CreateSubscriptionAt(mode, planID, quantity, notesMap, 0)
}

// CreateSubscriptionAt creates a new subscription and optionally schedules activation at a future unix timestamp.
func CreateSubscriptionAt(mode, planID string, quantity int, notesMap map[string]string, startAt int64) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}
	reqBody := buildCreateSubscriptionRequest(planID, quantity, notesMap, startAt)

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

	resp, err := razorpayHTTPClient.Do(req)
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

	resp, err := razorpayHTTPClient.Do(req)
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

	resp, err := razorpayHTTPClient.Do(req)
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

	updateReq := buildUpdateSubscriptionRequest(quantity, scheduleChangeAt)

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

	resp, err := razorpayHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay API error (status %d): %s (request=%s)", resp.StatusCode, string(body), string(jsonData))
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
		CancelAtCycleEnd: cancelAtCycleEnd,
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

	resp, err := razorpayHTTPClient.Do(req)
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

// CancelScheduledChangesByID cancels a pending scheduled subscription update.
func CancelScheduledChangesByID(mode, subscriptionID string) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/subscriptions/%s/cancel_scheduled_changes", razorpayBaseURL, subscriptionID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := razorpayHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if isNoPendingScheduledChangeError(resp.StatusCode, body) {
			return nil, ErrNoPendingScheduledChange
		}
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}

// RetrieveScheduledChangesByID fetches pending scheduled change details for a subscription.
func RetrieveScheduledChangesByID(mode, subscriptionID string) (*RazorpaySubscription, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/subscriptions/%s/retrieve_scheduled_changes", razorpayBaseURL, subscriptionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.SetBasicAuth(accessKey, secretKey)

	resp, err := razorpayHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if isNoPendingScheduledChangeError(resp.StatusCode, body) {
			return nil, ErrNoPendingScheduledChange
		}
		return nil, fmt.Errorf("razorpay API error (status %d): %s", resp.StatusCode, string(body))
	}

	var subscription RazorpaySubscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &subscription, nil
}
