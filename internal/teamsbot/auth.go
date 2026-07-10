package teamsbot

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const openIDConfigURL = "https://login.botframework.com/v1/.well-known/openidconfiguration"

var ErrJWTValidationFailed = errors.New("JWT validation failed")

type openIDConfig struct {
	Issuer  string `json:"issuer"`
	JwksURI string `json:"jwks_uri"`
}

type jwksKeys struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type Authenticator struct {
	appID      string
	httpClient *http.Client
	openIDCache *openIDConfig
	jwksCache   *jwksKeys
	cacheMu     sync.RWMutex
	cacheTime   time.Time
	cacheTTL    time.Duration
}

func NewAuthenticator(appID string) *Authenticator {
	return &Authenticator{
		appID:      appID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cacheTTL:   24 * time.Hour,
	}
}

func (a *Authenticator) ValidateJWT(ctx context.Context, authHeader string) error {
	if authHeader == "" {
		return fmt.Errorf("%w: missing authorization header", ErrJWTValidationFailed)
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		return fmt.Errorf("%w: authorization header is not Bearer", ErrJWTValidationFailed)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%w: invalid JWT format", ErrJWTValidationFailed)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("%w: failed to decode JWT header", ErrJWTValidationFailed)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%w: failed to decode JWT payload", ErrJWTValidationFailed)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("%w: failed to decode JWT signature", ErrJWTValidationFailed)
	}

	var header, payload map[string]any
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Errorf("%w: failed to parse JWT header", ErrJWTValidationFailed)
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("%w: failed to parse JWT payload", ErrJWTValidationFailed)
	}

	issuer, _ := payload["iss"].(string)
	audience, _ := payload["aud"].(string)

	if issuer == "" || issuer != "https://api.botframework.com" {
		return fmt.Errorf("%w: invalid JWT issuer: %s", ErrJWTValidationFailed, issuer)
	}
	if audience != a.appID {
		return fmt.Errorf("%w: invalid JWT audience: expected %s, got %s", ErrJWTValidationFailed, a.appID, audience)
	}

	exp, _ := payload["exp"].(float64)
	if exp > 0 && time.Now().Unix() > int64(exp) {
		return fmt.Errorf("%w: JWT token expired", ErrJWTValidationFailed)
	}

	kid, _ := header["kid"].(string)
	signingInput := parts[0] + "." + parts[1]

	keys, err := a.getJWKS(ctx)
	if err != nil {
		return fmt.Errorf("%w: failed to fetch JWKS", ErrJWTValidationFailed)
	}

	var matchedKey *jwkKey
	for _, key := range keys.Keys {
		if key.Kid == kid {
			matchedKey = &key
			break
		}
	}
	if matchedKey == nil {
		return fmt.Errorf("%w: no matching JWK key found for kid: %s", ErrJWTValidationFailed, kid)
	}

	rsaKey, err := jwkToRSAPublicKey(matchedKey)
	if err != nil {
		return fmt.Errorf("%w: failed to convert JWK to RSA key", ErrJWTValidationFailed)
	}

	hashed := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hashed[:], sig); err != nil {
		return fmt.Errorf("%w: JWT signature validation failed", ErrJWTValidationFailed)
	}

	return nil
}

func (a *Authenticator) getOpenIDConfig(ctx context.Context) (*openIDConfig, error) {
	a.cacheMu.RLock()
	if a.openIDCache != nil && time.Since(a.cacheTime) < a.cacheTTL {
		defer a.cacheMu.RUnlock()
		return a.openIDCache, nil
	}
	a.cacheMu.RUnlock()

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openIDConfigURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cfg openIDConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}

	a.openIDCache = &cfg
	a.cacheTime = time.Now()
	return &cfg, nil
}

func (a *Authenticator) getJWKS(ctx context.Context) (*jwksKeys, error) {
	a.cacheMu.RLock()
	if a.jwksCache != nil && time.Since(a.cacheTime) < a.cacheTTL {
		defer a.cacheMu.RUnlock()
		return a.jwksCache, nil
	}
	a.cacheMu.RUnlock()

	cfg, err := a.getOpenIDConfig(ctx)
	if err != nil {
		return nil, err
	}

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.JwksURI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var keys jwksKeys
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, err
	}

	a.jwksCache = &keys
	a.cacheTime = time.Now()
	return &keys, nil
}

func jwkToRSAPublicKey(key *jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}

	eInt := big.NewInt(0).SetBytes(eBytes)
	nInt := big.NewInt(0).SetBytes(nBytes)

	return &rsa.PublicKey{N: nInt, E: int(eInt.Int64())}, nil
}
