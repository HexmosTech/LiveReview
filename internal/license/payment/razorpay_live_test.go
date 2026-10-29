package payment

import (
	"os"
	"testing"
)

// skipWithoutRazorpayTestKeys skips a test that hits the real Razorpay test-mode
// API when RAZORPAY_TEST_KEY/RAZORPAY_TEST_SECRET aren't configured, rather than
// failing - these are manual verification scripts against a live sandbox
// account, not hermetic unit tests, and most environments (CI, local dev
// without sandbox credentials) have no reason to run them.
func skipWithoutRazorpayTestKeys(t *testing.T) {
	t.Helper()
	if os.Getenv("RAZORPAY_TEST_KEY") == "" || os.Getenv("RAZORPAY_TEST_SECRET") == "" {
		t.Skip("skipping: RAZORPAY_TEST_KEY/RAZORPAY_TEST_SECRET not set (live Razorpay sandbox test)")
	}
}
