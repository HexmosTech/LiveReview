package license

import "fmt"

// GetRazorpayKeys returns the Razorpay access and secret keys based on the mode
// mode can be "test" or "live"
func GetRazorpayKeys(mode string) (string, string, error) {

	RAZORPAY_ACCESS_KEY := "REDACTED_LIVE_KEY"
	RAZORPAY_SECRET_KEY := "REDACTED_LIVE_SECRET"
	RAZORPAY_TEST_ACCESS_KEY := "REDACTED_TEST_KEY"
	RAZORPAY_TEST_SECRET_KEY := "REDACTED_TEST_SECRET"

	if mode == "test" {
		return RAZORPAY_TEST_ACCESS_KEY, RAZORPAY_TEST_SECRET_KEY, nil
	} else if mode == "live" {
		return RAZORPAY_ACCESS_KEY, RAZORPAY_SECRET_KEY, nil
	} else {
		return "", "", fmt.Errorf("invalid mode: %s", mode)
	}
}
