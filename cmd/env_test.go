package cmd

import "testing"

func TestCheckRequiredConfigCloudTestModeRequiresRazorpayTestVars(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SECRET", "jwt-secret")
	t.Setenv("CLOUD_JWT_SECRET", "cloud-secret")
	t.Setenv("RAZORPAY_MODE", "test")
	t.Setenv("RAZORPAY_WEBHOOK_SECRET", "whsec")

	result := CheckRequiredConfig(true)

	missing := make(map[string]bool, len(result.Missing))
	for _, item := range result.Missing {
		missing[item] = true
	}

	wantMissing := []string{
		"RAZORPAY_TEST_KEY",
		"RAZORPAY_TEST_SECRET",
		"RAZORPAY_TEST_MONTHLY_PLAN_ID",
		"RAZORPAY_TEST_YEARLY_PLAN_ID",
	}

	for _, key := range wantMissing {
		if !missing[key] {
			t.Fatalf("expected %s to be required and missing", key)
		}
	}
}

func TestCheckRequiredConfigCloudLiveModeRequiresLiveVars(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SECRET", "jwt-secret")
	t.Setenv("CLOUD_JWT_SECRET", "cloud-secret")
	t.Setenv("RAZORPAY_MODE", "live")
	t.Setenv("RAZORPAY_WEBHOOK_SECRET", "whsec")

	result := CheckRequiredConfig(true)

	missing := make(map[string]bool, len(result.Missing))
	for _, item := range result.Missing {
		missing[item] = true
	}

	wantMissing := []string{
		"RAZORPAY_LIVE_KEY",
		"RAZORPAY_LIVE_SECRET",
		"RAZORPAY_LIVE_MONTHLY_PLAN_ID",
		"RAZORPAY_LIVE_YEARLY_PLAN_ID",
	}

	for _, key := range wantMissing {
		if !missing[key] {
			t.Fatalf("expected %s to be required and missing", key)
		}
	}

	if missing["RAZORPAY_TEST_KEY"] {
		t.Fatalf("did not expect test-mode keys to be required when RAZORPAY_MODE=live")
	}
}
