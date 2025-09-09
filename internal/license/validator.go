package license

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// ParsedPublicKey caches a public key with kid.
type ParsedPublicKey struct {
	Kid string
	Key *rsa.PublicKey
}

// ValidateOfflineJWT verifies signature & extracts claims without server call.
func ValidateOfflineJWT(tokenStr string, pub *ParsedPublicKey) (jwt.MapClaims, error) {
	if tokenStr == "" {
		return nil, ErrLicenseMissing
	}
	parser := jwt.NewParser()
	token, err := parser.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method %s", t.Method.Alg())
		}
		return pub.Key, nil
	})
	if err != nil {
		return nil, ErrLicenseInvalid
	}
	if !token.Valid {
		return nil, ErrLicenseInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrLicenseInvalid
	}
	if expRaw, ok := claims["exp"]; ok {
		switch v := expRaw.(type) {
		case float64:
			if time.Unix(int64(v), 0).Before(time.Now()) {
				return nil, ErrLicenseExpired
			}
		}
	}
	return claims, nil
}

// FetchPublicKey retrieves the current public key JSON from server.
func FetchPublicKey(ctx context.Context, cfg *Config, hc *http.Client) (*ParsedPublicKey, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.APIBase+"/jwtLicence/publicKey", nil)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, NetworkError{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("publicKey http %d: %s", resp.StatusCode, string(b))
	}
	var envelope struct {
		Data struct {
			Kid       string `json:"kid"`
			PublicKey string `json:"publicKey"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	block, _ := pem.Decode([]byte(envelope.Data.PublicKey))
	if block == nil {
		return nil, errors.New("invalid pem public key")
	}
	pkAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkix key: %w", err)
	}
	rsaKey, ok := pkAny.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key not RSA")
	}
	return &ParsedPublicKey{Kid: envelope.Data.Kid, Key: rsaKey}, nil
}

// ValidateOnline posts token to validation endpoint. dryRun avoids counting usage server-side if supported.
func ValidateOnline(ctx context.Context, cfg *Config, hc *http.Client, token string, dryRun bool) (map[string]any, *string, error) {
	body := fmt.Sprintf(`{"token":"%s"}`, token)
	if token == "" {
		return nil, nil, ErrLicenseMissing
	}
	url := cfg.APIBase + "/jwtLicence/validate"
	if dryRun {
		url += "?dryRun=true"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, NetworkError{Err: err}
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("validate http %d: %s", resp.StatusCode, string(payload))
	}
	var envelope struct {
		Data  map[string]any `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, nil, fmt.Errorf("decode validate: %w", err)
	}
	if envelope.Error.Code != "" {
		code := envelope.Error.Code
		return nil, &code, ErrLicenseInvalid
	}
	return envelope.Data, nil, nil
}
