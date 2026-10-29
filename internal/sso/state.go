package sso

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

// StateSigner produces and verifies the OAuth/SAML "state" parameter without
// needing a server-side session table: it's a signed, short-TTL token
// carrying the org ID the login was initiated for. Derived from the same
// JWT secret every deployment already configures, with a distinct HMAC key
// (never reuse the SecretBox's AES key for a MAC).
type StateSigner struct {
	key []byte
	ttl time.Duration
}

func NewStateSigner(secret string) *StateSigner {
	key := make([]byte, 32)
	// HKDF rather than a bare hash: keeps this HMAC key independent of
	// SecretBox's AES key even though both ultimately derive from the same
	// JWT_SECRET, and doesn't degrade to the secret's raw entropy.
	_, _ = io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte("livereview-sso-state-v1")), key)
	return &StateSigner{key: key, ttl: 10 * time.Minute}
}

// Encode signs orgID + a random nonce (the nonce makes the token
// single-purpose/unguessable; it is not currently re-checked on decode, but
// keeps two concurrent logins from producing identical state values).
func (s *StateSigner) Encode(orgID int64, nonce string) string {
	payload := fmt.Sprintf("%d|%s|%d", orgID, nonce, time.Now().Add(s.ttl).Unix())
	mac := s.sign(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac)
}

// Decode verifies signature + expiry and returns the org ID the login was
// initiated for.
func (s *StateSigner) Decode(token string) (orgID int64, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, errors.New("sso: malformed state token")
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errors.New("sso: malformed state payload")
	}
	sigGiven, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("sso: malformed state signature")
	}
	if subtle.ConstantTimeCompare(sigGiven, s.sign(string(payloadRaw))) != 1 {
		return 0, errors.New("sso: state signature mismatch")
	}
	fields := strings.SplitN(string(payloadRaw), "|", 3)
	if len(fields) != 3 {
		return 0, errors.New("sso: malformed state fields")
	}
	orgID, err = strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, errors.New("sso: malformed org id in state")
	}
	expiry, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return 0, errors.New("sso: malformed expiry in state")
	}
	if time.Now().Unix() > expiry {
		return 0, errors.New("sso: state token expired")
	}
	return orgID, nil
}

func (s *StateSigner) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
