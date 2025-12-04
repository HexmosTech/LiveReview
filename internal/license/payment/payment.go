package payment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const razorpayBaseURL = "https://api.razorpay.com/v1"

// GetRazorpayKeys returns the Razorpay access and secret keys based on the mode
// mode can be "test" or "live"
func GetRazorpayKeys(mode string) (string, string, error) {

	RAZORPAY_ACCESS_KEY := "REDACTED_LIVE_KEY"
	RAZORPAY_SECRET_KEY := "REDACTED_LIVE_SECRET"
	RAZORPAY_TEST_ACCESS_KEY := "REDACTED_TEST_KEY"
	RAZORPAY_TEST_SECRET_KEY := "REDACTED_TEST_SECRET"

	switch mode {
	case "test":
		return RAZORPAY_TEST_ACCESS_KEY, RAZORPAY_TEST_SECRET_KEY, nil
	case "live":
		return RAZORPAY_ACCESS_KEY, RAZORPAY_SECRET_KEY, nil
	default:
		return "", "", fmt.Errorf("invalid mode: %s", mode)
	}
}

// createPlanWithDetails creates a subscription plan in Razorpay with the given details
func createPlanWithDetails(plan RazorpayPlan) (*RazorpayPlan, error) {
	accessKey, secretKey, err := GetRazorpayKeys(plan.Mode)
	if err != nil {
		return nil, err
	}

	// Create a temporary struct for marshaling with notes as map
	type tempPlan struct {
		Period   string            `json:"period"`
		Interval int               `json:"interval"`
		Item     RazorpayPlanItem  `json:"item"`
		Notes    map[string]string `json:"notes,omitempty"`
	}

	temp := tempPlan{
		Period:   plan.Period,
		Interval: plan.Interval,
		Item:     plan.Item,
		Notes:    plan.NotesMap,
	}

	jsonData, err := json.Marshal(temp)
	if err != nil {
		return nil, fmt.Errorf("error marshaling plan: %w", err)
	}

	url := fmt.Sprintf("%s/plans", razorpayBaseURL)
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

	var createdPlan RazorpayPlan
	if err := json.Unmarshal(body, &createdPlan); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &createdPlan, nil
}

// CreatePlan creates a subscription plan in Razorpay with predefined LiveReview pricing
func CreatePlan(mode, planType string) (*RazorpayPlan, error) {
	var plan RazorpayPlan

	switch planType {
	case "monthly":
		// Monthly plan: $6/user/month
		plan = RazorpayPlan{
			Mode:        mode,
			Period:      "monthly",
			Interval:    1,
			Description: "LiveReview Team Monthly Plan - $6 per user per month",
			Item: RazorpayPlanItem{
				Name:     "LiveReview Team - Monthly",
				Amount:   600, // $6 in USD cents (600 cents = $6)
				Currency: "USD",
			},
			NotesMap: map[string]string{
				"plan_type": "team_monthly",
				"app_name":  "LiveReview",
			},
		}
	case "yearly":
		// Yearly plan: $60/user/year ($5/month, 17% discount)
		plan = RazorpayPlan{
			Mode:        mode,
			Period:      "yearly",
			Interval:    1,
			Description: "LiveReview Team Annual Plan - $60 per user per year (17% savings)",
			Item: RazorpayPlanItem{
				Name:     "LiveReview Team - Annual",
				Amount:   6000, // $60 in USD cents (6000 cents = $60)
				Currency: "USD",
			},
			NotesMap: map[string]string{
				"plan_type": "team_yearly",
				"app_name":  "LiveReview",
				"discount":  "17%",
			},
		}
	default:
		return nil, fmt.Errorf("invalid plan type: %s (must be 'monthly' or 'yearly')", planType)
	}

	return createPlanWithDetails(plan)
}

// GetAllPlans fetches all plans from Razorpay
func GetAllPlans(mode string) (*RazorpayPlanListResponse, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/plans", razorpayBaseURL)
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

	var planList RazorpayPlanListResponse
	if err := json.Unmarshal(body, &planList); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &planList, nil
}

// GetPlanByID fetches a specific plan by ID from Razorpay
func GetPlanByID(mode, planID string) (*RazorpayPlan, error) {
	accessKey, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/plans/%s", razorpayBaseURL, planID)
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

	var plan RazorpayPlan
	if err := json.Unmarshal(body, &plan); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &plan, nil
}
