package api

import (
	"strings"
	"testing"
	"time"
)

func TestGHState_RoundTrip(t *testing.T) {
	secret := []byte("s3cret")
	now := time.Now()
	raw, err := signGHState(ghState{Nonce: "abc", Src: "home", Mode: ghModeInstall, Exp: now.Add(ghStateTTL).Unix()}, secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := verifyGHState(raw, secret, now)
	if err != nil || got.Nonce != "abc" || got.Src != "home" || got.Mode != ghModeInstall {
		t.Fatalf("got (%+v, %v)", got, err)
	}
}

func TestGHState_Rejections(t *testing.T) {
	secret := []byte("s3cret")
	now := time.Now()
	good, _ := signGHState(ghState{Nonce: "abc", Exp: now.Add(time.Minute).Unix()}, secret)
	expired, _ := signGHState(ghState{Nonce: "abc", Exp: now.Add(-time.Second).Unix()}, secret)
	noNonce, _ := signGHState(ghState{Exp: now.Add(time.Minute).Unix()}, secret)
	payload, sig, _ := strings.Cut(good, ".")
	otherPayload, _, _ := strings.Cut(func() string {
		s, _ := signGHState(ghState{Nonce: "evil", Exp: now.Add(time.Minute).Unix()}, secret)
		return s
	}(), ".")

	cases := map[string]struct {
		raw    string
		secret []byte
	}{
		"wrong secret":     {good, []byte("other")},
		"expired":          {expired, secret},
		"missing nonce":    {noNonce, secret},
		"tampered payload": {otherPayload + "." + sig, secret},
		"tampered sig":     {payload + ".AAAA", secret},
		"no separator":     {payload, secret},
		"empty":            {"", secret},
		"empty secret":     {good, nil},
		"garbage":          {"%%%.%%%", secret},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyGHState(tc.raw, tc.secret, now); err == nil {
				t.Fatal("expected the state to be rejected")
			}
		})
	}
}

func TestSanitizeGHSrcAndMode(t *testing.T) {
	for in, want := range map[string]string{"Home": "home", "start-free_1": "start-free_1", "": "unknown", "a b": "unknown", "<script>": "unknown", strings.Repeat("a", 33): "unknown"} {
		if got := sanitizeGHSrc(in); got != want {
			t.Errorf("sanitizeGHSrc(%q) = %q, want %q", in, got, want)
		}
	}
	if sanitizeGHMode("signin") != ghModeSignin || sanitizeGHMode("anything") != ghModeInstall || sanitizeGHMode("") != ghModeInstall {
		t.Error("sanitizeGHMode wrong")
	}
}
