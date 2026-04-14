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
	t.Setenv("LIVEREVIEW_PRICING_PROFILE", "actual")
	t.Setenv("RAZORPAY_WEBHOOK_SECRET", "whsec")

	result := CheckRequiredConfig(true)

	missing := make(map[string]bool, len(result.Missing))
	for _, item := range result.Missing {
		missing[item] = true
	}

	wantMissing := []string{
		"RAZORPAY_LIVE_KEY",
		"RAZORPAY_LIVE_SECRET",
		"RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID",
		"RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID",
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

func TestCheckRequiredConfigCloudLiveLowPricingProfileRequiresLowPricingPlanIDs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SECRET", "jwt-secret")
	t.Setenv("CLOUD_JWT_SECRET", "cloud-secret")
	t.Setenv("RAZORPAY_MODE", "live")
	t.Setenv("LIVEREVIEW_PRICING_PROFILE", "low_pricing_test")
	t.Setenv("RAZORPAY_WEBHOOK_SECRET", "whsec")

	result := CheckRequiredConfig(true)

	missing := make(map[string]bool, len(result.Missing))
	for _, item := range result.Missing {
		missing[item] = true
	}

	wantMissing := []string{
		"RAZORPAY_LIVE_KEY",
		"RAZORPAY_LIVE_SECRET",
		"RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID",
		"RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID",
	}

	for _, key := range wantMissing {
		if !missing[key] {
			t.Fatalf("expected %s to be required and missing", key)
		}
	}
}
