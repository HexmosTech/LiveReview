package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	storageusers "github.com/livereview/storage/users"
	"golang.org/x/crypto/bcrypt"
)

// ProfileService handles user profile management operations
type ProfileService struct {
	store *storageusers.ProfileStore
}

// NewProfileService creates a new profile service
func NewProfileService(db *sql.DB) *ProfileService {
	return &ProfileService{
		store: storageusers.NewProfileStore(db),
	}
}

// UserProfile represents user profile information
type UserProfile struct {
	ID               int64      `json:"id"`
	Email            string     `json:"email"`
	FirstName        *string    `json:"first_name"`
	LastName         *string    `json:"last_name"`
	IsActive         bool       `json:"is_active"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	PlanType         *string    `json:"plan_type"`
	LicenseExpiresAt *time.Time `json:"license_expires_at"`
}

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Email     *string `json:"email"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

// GetUserProfile retrieves the user's profile information
func (ps *ProfileService) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	rec, err := ps.store.GetUserProfile(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	return &UserProfile{
		ID:               rec.ID,
		Email:            rec.Email,
		FirstName:        rec.FirstName,
		LastName:         rec.LastName,
		IsActive:         rec.IsActive,
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
		LastLoginAt:      rec.LastLoginAt,
		PlanType:         rec.PlanType,
		LicenseExpiresAt: rec.LicenseExpiresAt,
	}, nil
}

// UpdateUserProfile updates the user's profile information
func (ps *ProfileService) UpdateUserProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*UserProfile, error) {
	if err := validateUpdateProfileRequest(req); err != nil {
		return nil, err
	}

	err := ps.store.UpdateUserProfile(ctx, storageusers.UpdateProfileInput{
		UserID:    userID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
	})
	if err != nil {
		if errors.Is(err, storageusers.ErrEmailInUse) {
			return nil, storageusers.ErrEmailInUse
		}
		return nil, fmt.Errorf("failed to update user profile: %w", err)
	}

	// Get updated profile
	return ps.GetUserProfile(ctx, userID)
}

// ChangePassword changes the user's password
func (ps *ProfileService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	if strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		return fmt.Errorf("current and new password are required")
	}

	currentHash, err := ps.store.GetCurrentPasswordHash(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to get current password: %w", err)
	}

	// Verify current password
	err = bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword))
	if err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	updated, err := ps.store.UpdatePasswordIfCurrentHash(ctx, userID, currentHash, string(hashedPassword))
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	if !updated {
		return fmt.Errorf("current password is incorrect")
	}

	return nil
}

// GetUserOrganizations gets all organizations the user belongs to with their roles
func (ps *ProfileService) GetUserOrganizations(ctx context.Context, userID int64) ([]UserOrgRole, error) {
	recs, err := ps.store.ListUserOrganizations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}

	organizations := make([]UserOrgRole, 0, len(recs))
	for _, rec := range recs {
		organizations = append(organizations, UserOrgRole{
			OrgID:   rec.OrgID,
			OrgName: rec.OrgName,
			Role:    rec.Role,
			RoleID:  rec.RoleID,
		})
	}

	return organizations, nil
}

func validateUpdateProfileRequest(req UpdateProfileRequest) error {
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			return fmt.Errorf("email cannot be empty")
		}
		if _, err := mail.ParseAddress(email); err != nil {
			return fmt.Errorf("email is invalid")
		}
	}

	if req.FirstName != nil {
		first := strings.TrimSpace(*req.FirstName)
		if len(first) > 120 {
			return fmt.Errorf("first_name is too long")
		}
	}

	if req.LastName != nil {
		last := strings.TrimSpace(*req.LastName)
		if len(last) > 120 {
			return fmt.Errorf("last_name is too long")
		}
	}

	return nil
}

// UserOrgRole represents a user's role in an organization
type UserOrgRole struct {
	OrgID   int64  `json:"org_id"`
	OrgName string `json:"org_name"`
	Role    string `json:"role"`
	RoleID  int64  `json:"role_id"`
}
