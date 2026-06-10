package organizations

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

type OrganizationHandlers struct {
	service *OrganizationService
	logger  *log.Logger
}

func NewOrganizationHandlers(service *OrganizationService, logger *log.Logger) *OrganizationHandlers {
	return &OrganizationHandlers{
		service: service,
		logger:  logger,
	}
}

// CreateOrganization creates a new organization (available to all authenticated users)
func (h *OrganizationHandlers) CreateOrganization(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	var req struct {
		Name        string `json:"name" validate:"required,min=1,max=255"`
		Description string `json:"description" validate:"max=1000"`
	}

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Basic validation
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(req.Name) > 255 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be less than 255 characters")
	}
	if len(req.Description) > 1000 {
		return echo.NewHTTPError(http.StatusBadRequest, "description must be less than 1000 characters")
	}

	org, err := h.service.CreateOrganization(user.ID, req.Name, req.Description)
	if err != nil {
		h.logger.Printf("Error creating organization: %v", err)
		// Check if it's a duplicate name error
		if strings.Contains(err.Error(), "already exists") {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":      "Organization created successfully",
		"organization": org,
	})
}

// GetUserOrganizations returns all organizations the user has access to
func (h *OrganizationHandlers) GetUserOrganizations(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	// Check if user is super admin through permission context
	permCtx := auth.GetPermissionContext(c)
	isSuperAdmin := permCtx != nil && permCtx.IsSuperAdmin

	orgs, err := h.service.GetUserOrganizations(user.ID, isSuperAdmin)
	if err != nil {
		h.logger.Printf("Error getting user organizations: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get organizations")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"organizations": orgs,
		"total":         len(orgs),
	})
}

// GetOrganization returns details for a specific organization
func (h *OrganizationHandlers) GetOrganization(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	// Check if user is super admin through permission context
	permCtx := auth.GetPermissionContext(c)
	isSuperAdmin := permCtx != nil && permCtx.IsSuperAdmin

	org, err := h.service.GetOrganizationByID(orgID, user.ID, isSuperAdmin)
	if err != nil {
		if err.Error() == "organization not found or access denied" {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		h.logger.Printf("Error getting organization: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get organization")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"organization": org,
	})
}

// UpdateOrganization updates organization details
func (h *OrganizationHandlers) UpdateOrganization(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	// Check if user has permissions - owners and super admins can update
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if !permCtx.IsSuperAdmin {
		// Check if user is an owner of this organization
		if permCtx.OrgID != orgID || permCtx.Role != "owner" {
			return echo.NewHTTPError(http.StatusForbidden, "owner or super admin privileges required")
		}
	}

	var req struct {
		Name             *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
		Description      *string `json:"description,omitempty" validate:"omitempty,max=1000"`
		Settings         *string `json:"settings,omitempty"`
		SubscriptionPlan *string `json:"subscription_plan,omitempty"`
		MaxUsers         *int    `json:"max_users,omitempty" validate:"omitempty,min=1"`
	}

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Basic validation
	if req.Name != nil && (len(*req.Name) == 0 || len(*req.Name) > 255) {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be between 1 and 255 characters")
	}
	if req.Description != nil && len(*req.Description) > 1000 {
		return echo.NewHTTPError(http.StatusBadRequest, "description must be less than 1000 characters")
	}
	if req.MaxUsers != nil && *req.MaxUsers < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "max_users must be at least 1")
	}

	org, err := h.service.UpdateOrganization(orgID, user.ID, req.Name, req.Description, req.Settings, req.SubscriptionPlan, req.MaxUsers)
	if err != nil {
		if err.Error() == "no fields to update" {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		h.logger.Printf("Error updating organization: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update organization")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":      "Organization updated successfully",
		"organization": org,
	})
}

// DeactivateOrganization soft-deletes an organization (super admin only)
func (h *OrganizationHandlers) DeactivateOrganization(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	// Super admin check is handled by RequireSuperAdmin middleware

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	err = h.service.DeactivateOrganization(orgID, user.ID)
	if err != nil {
		if err.Error() == "organization not found or already deactivated" {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		h.logger.Printf("Error deactivating organization: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to deactivate organization")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Organization deactivated successfully",
	})
}

// GetOrganizationMembers returns members of an organization with pagination
func (h *OrganizationHandlers) GetOrganizationMembers(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	// Check if user has access to this organization
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if !permCtx.IsSuperAdmin {
		if permCtx.OrgID != orgID {
			return echo.NewHTTPError(http.StatusForbidden, "access to organization required")
		}
	}

	// Parse pagination parameters
	page := 1
	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	members, totalCount, err := h.service.GetOrganizationMembers(orgID, limit, offset)
	if err != nil {
		h.logger.Printf("Error getting organization members: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get members")
	}

	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"members":     members,
		"total":       totalCount,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// ChangeUserRole changes a user's role in an organization
func (h *OrganizationHandlers) ChangeUserRole(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user ID")
	}

	// Check if user has permissions - owners and super admins can change roles
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if !permCtx.IsSuperAdmin {
		if permCtx.OrgID != orgID || permCtx.Role != "owner" {
			return echo.NewHTTPError(http.StatusForbidden, "owner or super admin privileges required")
		}
	}

	var req struct {
		RoleID int64 `json:"role_id" validate:"required,min=1"`
	}

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Basic validation
	if req.RoleID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "role_id is required and must be greater than 0")
	}

	err = h.service.ChangeUserRole(orgID, userID, req.RoleID, user.ID)
	if err != nil {
		if err.Error() == "user not found in organization" {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if err.Error() == "user already has this role" {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		h.logger.Printf("Error changing user role: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to change user role")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "User role changed successfully",
	})
}

// GetOrganizationAnalytics returns analytics for an organization
func (h *OrganizationHandlers) GetOrganizationAnalytics(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid organization ID")
	}

	// Check if user has access to this organization
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if !permCtx.IsSuperAdmin {
		if permCtx.OrgID != orgID {
			return echo.NewHTTPError(http.StatusForbidden, "access to organization required")
		}
	}

	analytics, err := h.service.GetOrganizationAnalytics(orgID)
	if err != nil {
		h.logger.Printf("Error getting organization analytics: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get analytics")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"analytics": analytics,
	})
}
