package api

import "testing"

func TestResolveRazorpayModeDefaultsToTest(t *testing.T) {
	t.Setenv("RAZORPAY_MODE", "")
	if got := resolveRazorpayMode(); got != "test" {
		t.Fatalf("resolveRazorpayMode() = %q, want test", got)
	}
}

func TestResolveRazorpayModeUsesEnvironmentValue(t *testing.T) {
	t.Setenv("RAZORPAY_MODE", "live")
	if got := resolveRazorpayMode(); got != "live" {
		t.Fatalf("resolveRazorpayMode() = %q, want live", got)
	}
}
