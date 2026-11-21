package serverscore

import "testing"

func TestNormalizeRoutePath(t *testing.T) {
	cases := []struct {
		in       string
		wantPath string
		wantVer  bool
	}{
		{"/v1/sms/send-otp", "/sms/send-otp", true},
		{"/sms/send-otp", "/sms/send-otp", false},
		{"https://svc:8080/v1/ledger/entry", "/ledger/entry", true},
		{"/v2/orders/{id}?expand=true", "/orders/{id}", true},
		{"/orders/", "/orders", false},
		{"/", "/", false},
		{"", "", false},
		{"v1/orders", "/orders", true},
		{"/v10/orders", "/orders", true},
	}
	for _, tc := range cases {
		got, ver := NormalizeRoutePath(tc.in)
		if got != tc.wantPath || ver != tc.wantVer {
			t.Errorf("NormalizeRoutePath(%q) = (%q,%v), want (%q,%v)", tc.in, got, ver, tc.wantPath, tc.wantVer)
		}
	}
}

func TestNormalizeRoutePath_MatchesVersionedCallToUnversionedRoute(t *testing.T) {
	call, callVer := NormalizeRoutePath("/v1/sms/send-otp")
	route, routeVer := NormalizeRoutePath("/sms/send-otp")
	if call != route || callVer == routeVer {
		t.Fatalf("expected a version-mismatch match, got call=(%q,%v) route=(%q,%v)", call, callVer, route, routeVer)
	}
}
