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
	ghNextCookieName   = "lr_gh_next" // where a returning user lands after login (see index.tsx)
	ghNextGitProviders = "git"
	ghModeInstall      = "install"
	ghModeSignin       = "signin"
)

var ghSrcPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

type ghState struct {
	Nonce string `json:"n"`
	Src   string `json:"s"`
	Mode  string `json:"m"`
	Exp   int64  `json:"e"`
}

// sanitizeGHSrc keeps the campaign tag short and harmless; anything else becomes "unknown".
func sanitizeGHSrc(src string) string {
	src = strings.ToLower(strings.TrimSpace(src))
	if ghSrcPattern.MatchString(src) {
		return src
	}
	return "unknown"
}

func sanitizeGHMode(mode string) string {
	if mode == ghModeSignin {
		return ghModeSignin
	}
	return ghModeInstall
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
