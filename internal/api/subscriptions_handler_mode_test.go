package api

import "testing"

func TestResolveRazorpayModeDefaultsToLive(t *testing.T) {
	t.Setenv("RAZORPAY_MODE", "")
	if got := resolveRazorpayMode(); got != "live" {
		t.Fatalf("resolveRazorpayMode() = %q, want live", got)
	}
}

func TestResolveRazorpayModeUsesEnvironmentValue(t *testing.T) {
	t.Setenv("RAZORPAY_MODE", "live")
	if got := resolveRazorpayMode(); got != "live" {
		t.Fatalf("resolveRazorpayMode() = %q, want live", got)
	}
}
