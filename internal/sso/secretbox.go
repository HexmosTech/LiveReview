// Package sso implements per-organization OIDC and SAML single sign-on for
// self-hosted enterprise deployments (see docs/sso.md).
package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// SecretBox encrypts small secrets (OIDC client secrets, etc.) at rest using
// AES-256-GCM. The key is derived from the server's existing JWT_SECRET via
// HKDF (not used bare, so a lower-entropy JWT_SECRET doesn't translate
// one-to-one into a weak AES key) rather than a new dedicated env var, so
// deployments don't need an extra secret to manage.
type SecretBox struct {
	gcm cipher.AEAD
}

// NewSecretBox derives an AES-GCM cipher from an arbitrary-length secret
// (typically the JWT signing secret already required by every deployment).
func NewSecretBox(secret string) (*SecretBox, error) {
	if secret == "" {
		return nil, errors.New("sso: secret box key material is empty")
	}
	var key [32]byte
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte("livereview-sso-secretbox-v1")), key[:]); err != nil {
		return nil, fmt.Errorf("sso: deriving key: %w", err)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("sso: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sso: creating gcm: %w", err)
	}
	return &SecretBox{gcm: gcm}, nil
}

// Encrypt returns a base64 string safe for storing in a TEXT column.
// Empty plaintext encrypts to an empty string (so clearing a secret is a
// no-op round trip, not a decrypt error later).
func (b *SecretBox) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("sso: generating nonce: %w", err)
	}
	ciphertext := b.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. An empty input decrypts to an empty string.
func (b *SecretBox) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("sso: decoding ciphertext: %w", err)
	}
	nonceSize := b.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("sso: ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := b.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("sso: decrypting: %w", err)
	}
	return string(plaintext), nil
}
