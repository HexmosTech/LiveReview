package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/livereview/pkg/models"
)

// TokenService handles JWT token creation, validation, and management
type TokenService struct {
	db        *sql.DB
	secretKey []byte

	// Configurable token durations
	AccessTokenDuration  time.Duration // Default: 15 minutes
	RefreshTokenDuration time.Duration // Default: 30 days
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"` // "Bearer"
}

// JWTClaims represents the claims in our JWT tokens
type JWTClaims struct {
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	TokenHash string `json:"token_hash"` // Reference to database token
	jwt.RegisteredClaims
}

// NewTokenService creates a new token service
func NewTokenService(db *sql.DB, secretKey string) *TokenService {
	return &TokenService{
		db:                   db,
		secretKey:            []byte(secretKey),
		AccessTokenDuration:  15 * time.Minute,    // Short-lived access tokens
		RefreshTokenDuration: 30 * 24 * time.Hour, // 30 days for refresh tokens
	}
}

// generateRandomToken creates a cryptographically secure random token
func (ts *TokenService) generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashToken creates a SHA256 hash of the token for database storage
func (ts *TokenService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CreateTokenPair creates both access and refresh tokens for a user
func (ts *TokenService) CreateTokenPair(user *models.User, userAgent, ipAddress string) (*TokenPair, error) {
	// Generate refresh token
	refreshToken, err := ts.generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	refreshTokenHash := ts.hashToken(refreshToken)
	refreshExpiresAt := time.Now().Add(ts.RefreshTokenDuration)

	// Store refresh token in database
	var refreshTokenID int64
	err = ts.db.QueryRow(`
		INSERT INTO auth_tokens (user_id, token_hash, token_type, expires_at, user_agent, ip_address)
		VALUES ($1, $2, 'refresh', $3, $4, $5)
		RETURNING id
	`, user.ID, refreshTokenHash, refreshExpiresAt, userAgent, ipAddress).Scan(&refreshTokenID)

	if err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	// Generate access token
	accessToken, err := ts.generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	accessTokenHash := ts.hashToken(accessToken)
	accessExpiresAt := time.Now().Add(ts.AccessTokenDuration)

	// Store access token in database
	_, err = ts.db.Exec(`
		INSERT INTO auth_tokens (user_id, token_hash, token_type, expires_at, user_agent, ip_address)
		VALUES ($1, $2, 'session', $3, $4, $5)
	`, user.ID, accessTokenHash, accessExpiresAt, userAgent, ipAddress)

	if err != nil {
		return nil, fmt.Errorf("failed to store access token: %w", err)
	}

	// Create JWT access token
	claims := &JWTClaims{
		UserID:    user.ID,
		Email:     user.Email,
		TokenHash: accessTokenHash,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "livereview",
			Subject:   fmt.Sprintf("user_%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtString, err := token.SignedString(ts.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign JWT: %w", err)
	}

	return &TokenPair{
		AccessToken:  jwtString,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiresAt,
		TokenType:    "Bearer",
	}, nil
}

// ValidateAccessToken validates a JWT access token and returns the user
func (ts *TokenService) ValidateAccessToken(tokenString string) (*models.User, error) {
	// Parse and validate JWT
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return ts.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Check if token exists in database and is active
	var tokenExists bool
	err = ts.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM auth_tokens 
			WHERE user_id = $1 
			AND token_hash = $2 
			AND token_type = 'session'
			AND is_active = true 
			AND expires_at > NOW()
		)
	`, claims.UserID, claims.TokenHash).Scan(&tokenExists)

	if err != nil {
		return nil, fmt.Errorf("failed to check token in database: %w", err)
	}

	if !tokenExists {
		return nil, fmt.Errorf("token not found or expired")
	}

	// Update last_used_at
	_, err = ts.db.Exec(`
		UPDATE auth_tokens 
		SET last_used_at = NOW() 
		WHERE user_id = $1 AND token_hash = $2 AND token_type = 'session'
	`, claims.UserID, claims.TokenHash)

	if err != nil {
		// Log but don't fail - this is not critical
		fmt.Printf("Warning: failed to update last_used_at: %v\n", err)
	}

	// Get user details
	user := &models.User{}
	err = ts.db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at
		FROM users WHERE id = $1
	`, claims.UserID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// RefreshTokenPair creates a new token pair using a valid refresh token
func (ts *TokenService) RefreshTokenPair(refreshToken, userAgent, ipAddress string) (*TokenPair, error) {
	refreshTokenHash := ts.hashToken(refreshToken)

	// Validate refresh token and get user
	var userID int64
	err := ts.db.QueryRow(`
		SELECT user_id FROM auth_tokens 
		WHERE token_hash = $1 
		AND token_type = 'refresh'
		AND is_active = true 
		AND expires_at > NOW()
	`, refreshTokenHash).Scan(&userID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid or expired refresh token")
		}
		return nil, fmt.Errorf("failed to validate refresh token: %w", err)
	}

	// Get user details
	user := &models.User{}
	err = ts.db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Revoke the old refresh token
	_, err = ts.db.Exec(`
		UPDATE auth_tokens 
		SET is_active = false, revoked_at = NOW()
		WHERE token_hash = $1 AND token_type = 'refresh'
	`, refreshTokenHash)

	if err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

	// Create new token pair
	return ts.CreateTokenPair(user, userAgent, ipAddress)
}

// RevokeToken revokes a specific token
func (ts *TokenService) RevokeToken(tokenHash, tokenType string) error {
	_, err := ts.db.Exec(`
		UPDATE auth_tokens 
		SET is_active = false, revoked_at = NOW()
		WHERE token_hash = $1 AND token_type = $2
	`, tokenHash, tokenType)

	return err
}

// RevokeAllUserTokens revokes all tokens for a specific user (logout from all devices)
func (ts *TokenService) RevokeAllUserTokens(userID int64) error {
	_, err := ts.db.Exec(`
		UPDATE auth_tokens 
		SET is_active = false, revoked_at = NOW()
		WHERE user_id = $1 AND is_active = true
	`, userID)

	return err
}

// CleanupExpiredTokens removes expired tokens from the database
// This should be called periodically by a background job
func (ts *TokenService) CleanupExpiredTokens() error {
	// Delete expired tokens
	result, err := ts.db.Exec(`
		DELETE FROM auth_tokens 
		WHERE expires_at < NOW() - INTERVAL '7 days'
	`)

	if err != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("Cleaned up %d expired tokens\n", rowsAffected)
	}

	return nil
}

// parseTokenClaims parses a JWT token and returns the claims without full validation
// This is used internally when we need to extract the token hash for revocation
func (ts *TokenService) parseTokenClaims(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return ts.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// StartCleanupScheduler starts a background task to clean up expired tokens
// This should be called when the application starts
func (ts *TokenService) StartCleanupScheduler() {
	// Run cleanup every hour
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		// Run cleanup immediately on startup
		if err := ts.CleanupExpiredTokens(); err != nil {
			fmt.Printf("Token cleanup error: %v\n", err)
		}

		// Then run every hour
		for range ticker.C {
			if err := ts.CleanupExpiredTokens(); err != nil {
				fmt.Printf("Token cleanup error: %v\n", err)
			}
		}
	}()
}
