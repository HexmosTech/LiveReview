package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// The OAuth/install "state" ties the GitHub round trip to the browser that started it: it is HMAC-signed so it
// cannot be forged, expires after ghStateTTL, and its nonce is mirrored in an HttpOnly cookie so a state copied
// to another browser is rejected (login CSRF).

const (
	ghStateTTL         = 10 * time.Minute
	ghStateCookieName  = "lr_gh_state"
	ghOrgCookieName    = "lr_gh_org"  // org id the UI should open after the GitHub callback (see index.tsx)
	ghNextCookieName   = "lr_gh_next" // where the user lands after login (see index.tsx)
	ghNextGitProviders = "git"
	ghNextGetStarted   = "get-started"
	ghModeInstall      = "install"
	ghModeSignin       = "signin"
)

var ghSrcPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

type ghState struct {
	Nonce string `json:"n"`
	Src   string `json:"s"`
	Mode  string `json:"m"`
	Exp   int64  `json:"e"`
	// GetStarted: the round trip began on /gh-login, so a successful sign-in lands on get-started. Only the
	// callback decides this (nothing is stored before leaving), so a failed attempt leaves no stale destination.
	GetStarted bool `json:"g,omitempty"`
	// OrgID and UserID bind the round trip to a signed-in LiveReview session (POST /github-app/start): the callback
	// connects to that org and keeps the session, whatever email the GitHub account uses. Zero for /gh-login.
	OrgID  int64 `json:"o,omitempty"`
	UserID int64 `json:"u,omitempty"`
	// Return is the page a bound round trip lands on, as a sanitizeGHReturn key (never a URL).
	Return string `json:"r,omitempty"`
}

var ghReturnReviewPattern = regexp.MustCompile(`^review/[1-9][0-9]{0,18}$`)

// sanitizeGHReturn keeps only the pages a bound round trip may land on; anything else becomes the GitHub App card.
func sanitizeGHReturn(ret string) string {
	if ret == ghNextGetStarted || ghReturnReviewPattern.MatchString(ret) {
		return ret
	}
	return ghNextGitProviders
}

// ghReturnHash is the UI route for a sanitizeGHReturn key.
func ghReturnHash(ret string) string {
	switch {
	case ret == ghNextGetStarted:
		return "#/get-started/github"
	case ghReturnReviewPattern.MatchString(ret):
		return "#/reviews/" + strings.TrimPrefix(ret, "review/")
	default:
		return "#/git/github-app"
	}
}

// sanitizeGHSrc keeps the campaign tag short and harmless; anything else becomes "unknown".
func sanitizeGHSrc(src string) string {
	src = strings.ToLower(strings.TrimSpace(src))
	if ghSrcPattern.MatchString(src) {
		return src
	}
	return "unknown"
}

func newGHNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ghStateMAC(payload string, secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("gh-app-state:" + payload))
	return m.Sum(nil)
}

// signGHState returns base64url(json) + "." + base64url(HMAC-SHA256(secret, "gh-app-state:" + base64url(json))).
func signGHState(s ghState, secret []byte) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(ghStateMAC(payload, secret)), nil
}

var errInvalidGHState = errors.New("invalid or expired state")

// verifyGHState checks the signature (constant time) and expiry.
func verifyGHState(raw string, secret []byte, now time.Time) (*ghState, error) {
	payload, sig, ok := strings.Cut(raw, ".")
	if !ok || payload == "" || sig == "" || len(secret) == 0 {
		return nil, errInvalidGHState
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, ghStateMAC(payload, secret)) {
		return nil, errInvalidGHState
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errInvalidGHState
	}
	var s ghState
	if err := json.Unmarshal(body, &s); err != nil || s.Nonce == "" {
		return nil, errInvalidGHState
	}
	if now.Unix() > s.Exp {
		return nil, errInvalidGHState
	}
	return &s, nil
}
