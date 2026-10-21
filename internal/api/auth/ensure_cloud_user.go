package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
	storageusers "github.com/livereview/storage/users"
	"golang.org/x/crypto/bcrypt"
)

// EnsureCloudUserRequest represents the request body for ensuring a cloud user exists
type EnsureCloudUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Source    string `json:"source,omitempty"`
}

// EnsureCloudUserInput is the non-HTTP form of the request: the Hexmos JWT plus who it is for.
type EnsureCloudUserInput struct {
	JWT       string
	Email     string
	FirstName string
	LastName  string
	Source    string
}

// EnsureCloudUserResult reports what provisioning did. All Created*/Assigned* flags are false on a repeat call.
type EnsureCloudUserResult struct {
	UserID             int64
	OrgID              int64
	Email              string
	CreatedUser        bool
	CreatedOrg         bool
	AssignedSuperAdmin bool
}

// EnsureCloudUserError carries the HTTP status and message the endpoint returns for a failure.
type EnsureCloudUserError struct {
	Status int
	Msg    string
}

func (e *EnsureCloudUserError) Error() string { return e.Msg }

func cloudErr(status int, msg string) *EnsureCloudUserError {
	return &EnsureCloudUserError{Status: status, Msg: msg}
}

// validateCloudJWT checks a Hexmos JWT against CLOUD_JWT_SECRET (manual, since the routes using it are public).
func validateCloudJWT(tokenString string) (*JWTClaims, *EnsureCloudUserError) {
	secret := os.Getenv("CLOUD_JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return nil, cloudErr(http.StatusUnauthorized, "cloud jwt not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = fmt.Errorf("invalid token")
		}
		return nil, cloudErr(http.StatusUnauthorized, err.Error())
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, cloudErr(http.StatusUnauthorized, "invalid token claims")
	}
	return claims, nil
}

// EnsureCloudUser ensures that a user and a personal organization exist, assigning super_admin role.
// Idempotent: if user/org/role mapping already exist, it returns success without changes.
func (h *AuthHandlers) EnsureCloudUser(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authorization header required"})
	}
	claims, cerr := validateCloudJWT(strings.TrimPrefix(authHeader, "Bearer "))
	if cerr != nil {
		return c.JSON(cerr.Status, map[string]string{"error": cerr.Msg})
	}

	var req EnsureCloudUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	result, cerr := h.provisionCloudUser(claims, req)
	if cerr != nil {
		return c.JSON(cerr.Status, map[string]string{"error": cerr.Msg})
	}

	// Create session tokens like normal login
	userID, orgID := result.UserID, result.OrgID
	dbDefaultOrgID, _ := storageusers.NewUserStore(h.db).DefaultOrgID(userID)
	user := &models.User{
		ID:           userID,
		Email:        result.Email,
		DefaultOrgID: dbDefaultOrgID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	userAgent := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()
	tokenPair, err := h.tokenService.CreateTokenPair(user, userAgent, ipAddress)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session tokens"})
	}

	// Get user's organizations (should at least include the personal org just created/verified)
	organizations, err := h.getUserOrganizations(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get user organizations"})
	}

	// Return a response compatible with LoginResponse structure
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":               "ok",
		"user_id":              userID,
		"org_id":               orgID,
		"created_user":         result.CreatedUser,
		"created_org":          result.CreatedOrg,
		"super_admin_assigned": result.AssignedSuperAdmin,
		"email":                result.Email,
		// Add standard login response fields
		"user": &UserInfo{
			ID:           userID,
			Email:        result.Email,
			Name:         h.userName(userID),
			DefaultOrgID: dbDefaultOrgID,
			CreatedAt:    user.CreatedAt,
			UpdatedAt:    user.UpdatedAt,
		},
		"tokens":        tokenPair,
		"organizations": organizations,
	})
}

// EnsureCloudUserCore validates a Hexmos JWT and provisions the user + personal org. It is the same code path
// as the /auth/ensure-cloud-user endpoint, for callers (the GitHub App callback) that are not an HTTP request
// carrying the JWT in a header.
func (h *AuthHandlers) EnsureCloudUserCore(in EnsureCloudUserInput) (*EnsureCloudUserResult, error) {
	claims, cerr := validateCloudJWT(in.JWT)
	if cerr != nil {
		return nil, cerr
	}
	result, cerr := h.provisionCloudUser(claims, EnsureCloudUserRequest{
		Email: in.Email, FirstName: in.FirstName, LastName: in.LastName, Source: in.Source,
	})
	if cerr != nil {
		return nil, cerr
	}
	return result, nil
}

func (h *AuthHandlers) provisionCloudUser(claims *JWTClaims, req EnsureCloudUserRequest) (*EnsureCloudUserResult, *EnsureCloudUserError) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		return nil, cloudErr(http.StatusBadRequest, "valid email required")
	}

	// Optional: enforce that token email matches requested email (defense-in-depth)
	if strings.TrimSpace(claims.Email) != "" && !strings.EqualFold(claims.Email, req.Email) {
		return nil, cloudErr(http.StatusForbidden, "token-user mismatch")
	}

	store := storageusers.NewUserStore(h.db)

	// Start transaction to ensure atomicity
	tx, err := h.db.Begin()
	if err != nil {
		return nil, cloudErr(http.StatusInternalServerError, "database transaction error")
	}
	defer tx.Rollback()

	result := &EnsureCloudUserResult{Email: req.Email}

	// 1. Find or create user
	userID, _, err := store.FindUserByEmail(tx, req.Email)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, cloudErr(http.StatusInternalServerError, "failed to query user")
		}
		// Create random password (not used directly by external auth) and hash it
		randBytes := make([]byte, 24)
		if _, rerr := rand.Read(randBytes); rerr != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to generate credentials")
		}
		hashed, herr := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randBytes)), bcrypt.DefaultCost)
		if herr != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to hash password")
		}
		var firstNamePtr, lastNamePtr interface{}
		if req.FirstName != "" {
			firstNamePtr = req.FirstName
		}
		if req.LastName != "" {
			lastNamePtr = req.LastName
		}
		userID, err = store.InsertCloudUser(tx, req.Email, string(hashed), firstNamePtr, lastNamePtr)
		if err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to create user")
		}
		result.CreatedUser = true
	}
	result.UserID = userID

	// 2. Find or create personal organization (org name == email)
	orgID, err := store.FindOrgIDByName(tx, req.Email)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, cloudErr(http.StatusInternalServerError, "failed to query organization")
		}
		orgID, err = store.InsertPersonalOrg(tx, req.Email, userID)
		if err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to create organization")
		}
		// Create prompt application context entry like service does
		if err = store.InsertPromptApplicationContext(tx, orgID); err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to init org context")
		}
		result.CreatedOrg = true
	}
	result.OrgID = orgID

	// Update user's default_org_id to this organization ID if it is currently NULL
	if err = store.SetDefaultOrgIfUnset(tx, userID, orgID); err != nil {
		return nil, cloudErr(http.StatusInternalServerError, "failed to set user default organization")
	}

	// Eagerly create the onboarding API key for a brand-new user, in this same transaction, so
	// the user's first dashboard load doesn't have to do it lazily on its own request path (see
	// collectOnboardingData in internal/api/dashboard.go, which still generates one itself as a
	// fallback if this is unset or fails - non-fatal here by design).
	if result.CreatedUser && h.onboardingKeyGenerator != nil {
		if onboardingKey, genErr := h.onboardingKeyGenerator(tx, userID, orgID); genErr != nil {
			log.Printf("[EnsureCloudUser] failed to eagerly create onboarding API key for user_id=%d: %v", userID, genErr)
		} else if err = store.SetOnboardingAPIKey(tx, userID, onboardingKey); err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to set onboarding api key")
		}
	}

	// 3. Ensure super_admin role assignment for user in this org
	superAdminRoleID, err := store.OwnerRoleID(tx)
	if err != nil {
		return nil, cloudErr(http.StatusInternalServerError, "super_admin role not found")
	}
	existingRoleID, roleErr := store.UserRoleID(tx, userID, orgID)
	switch {
	case errors.Is(roleErr, sql.ErrNoRows):
		// Assign super admin role with free plan
		if err = store.InsertFreeUserRole(tx, userID, orgID, superAdminRoleID); err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to assign super_admin role")
		}
		result.AssignedSuperAdmin = true
	case roleErr != nil:
		return nil, cloudErr(http.StatusInternalServerError, "failed to query user role")
	case existingRoleID != superAdminRoleID:
		// Upgrade role to super_admin
		if err = store.UpdateUserRole(tx, userID, orgID, superAdminRoleID); err != nil {
			return nil, cloudErr(http.StatusInternalServerError, "failed to upgrade role")
		}
		result.AssignedSuperAdmin = true
	}

	if err = tx.Commit(); err != nil {
		return nil, cloudErr(http.StatusInternalServerError, "commit failed")
	}

	// Send Discord notification for new signups (async, best-effort)
	if result.CreatedUser {
		sendSignupDiscordNotification(req.Email, req.Source)
	}
	return result, nil
}
