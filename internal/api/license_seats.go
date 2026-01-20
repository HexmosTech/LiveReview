package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/api/middleware"
	"github.com/livereview/pkg/models"
)

// SeatAssignment represents a license seat assignment
type SeatAssignment struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"user_id"`
	Email            string    `json:"email"`
	FirstName        *string   `json:"first_name,omitempty"`
	LastName         *string   `json:"last_name,omitempty"`
	AssignedByUserID *int64    `json:"assigned_by_user_id,omitempty"`
	AssignedByEmail  *string   `json:"assigned_by_email,omitempty"`
	AssignedAt       time.Time `json:"assigned_at"`
	IsActive         bool      `json:"is_active"`
}

// SeatAssignmentListResponse is the response for listing seat assignments
type SeatAssignmentListResponse struct {
	Assignments    []SeatAssignment `json:"assignments"`
	TotalSeats     int              `json:"total_seats"`
	AssignedSeats  int              `json:"assigned_seats"`
	AvailableSeats int              `json:"available_seats"`
	Unlimited      bool             `json:"unlimited"`
}

// UnassignedUser represents a user without a license seat
type UnassignedUser struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	IsActive  bool    `json:"is_active"`
	Role      *string `json:"role,omitempty"`
}

// attachLicenseSeatsRoutes registers license seat assignment endpoints
func (s *Server) attachLicenseSeatsRoutes(v1 *echo.Group, authMiddleware *auth.AuthMiddleware) {
	// Only for self-hosted mode - require auth
	group := v1.Group("/license/seats")
	group.Use(authMiddleware.RequireAuth())
	group.GET("", s.handleListSeatAssignments)
	group.GET("/unassigned", s.handleListUnassignedUsers)
	group.POST("/assign", s.handleAssignSeat)
	group.POST("/assign-bulk", s.handleBulkAssignSeats)
	group.DELETE("/:user_id", s.handleRevokeSeat)
	group.POST("/revoke-bulk", s.handleBulkRevokeSeats)
}

// handleListSeatAssignments lists all current seat assignments
func (s *Server) handleListSeatAssignments(c echo.Context) error {
	// Only allow in self-hosted mode
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	// Get license state for seat count info
	svc := s.licenseService()
	licState, err := svc.LoadOrInit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed_to_load_license"})
	}

	rows, err := s.db.Query(`
		SELECT 
			lsa.id,
			lsa.user_id,
			u.email,
			u.first_name,
			u.last_name,
			lsa.assigned_by_user_id,
			ab.email as assigned_by_email,
			lsa.assigned_at,
			lsa.is_active
		FROM license_seat_assignments lsa
		JOIN users u ON lsa.user_id = u.id
		LEFT JOIN users ab ON lsa.assigned_by_user_id = ab.id
		WHERE lsa.is_active = TRUE
		ORDER BY lsa.assigned_at ASC
	`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_error"})
	}
	defer rows.Close()

	var assignments []SeatAssignment
	for rows.Next() {
		var a SeatAssignment
		err := rows.Scan(
			&a.ID,
			&a.UserID,
			&a.Email,
			&a.FirstName,
			&a.LastName,
			&a.AssignedByUserID,
			&a.AssignedByEmail,
			&a.AssignedAt,
			&a.IsActive,
		)
		if err != nil {
			continue
		}
		assignments = append(assignments, a)
	}

	if assignments == nil {
		assignments = []SeatAssignment{}
	}

	totalSeats := 0
	if licState.SeatCount != nil {
		totalSeats = *licState.SeatCount
	}

	resp := SeatAssignmentListResponse{
		Assignments:    assignments,
		TotalSeats:     totalSeats,
		AssignedSeats:  len(assignments),
		AvailableSeats: totalSeats - len(assignments),
		Unlimited:      licState.Unlimited,
	}

	if resp.Unlimited {
		resp.AvailableSeats = -1 // Indicate unlimited
	}

	return c.JSON(http.StatusOK, resp)
}

// handleListUnassignedUsers lists all active users without a seat assignment
func (s *Server) handleListUnassignedUsers(c echo.Context) error {
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	// Get distinct users without seat assignments
	// A user can be in multiple orgs with different roles, so we pick the "highest" role
	rows, err := s.db.Query(`
		SELECT DISTINCT ON (u.id)
			u.id,
			u.email,
			u.first_name,
			u.last_name,
			u.is_active,
			COALESCE(r.name, 'member') as role
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles r ON ur.role_id = r.id
		WHERE u.is_active = TRUE
		AND NOT EXISTS (
			SELECT 1 FROM license_seat_assignments lsa 
			WHERE lsa.user_id = u.id AND lsa.is_active = TRUE
		)
		ORDER BY u.id, 
			CASE r.name 
				WHEN 'super_admin' THEN 1 
				WHEN 'owner' THEN 2 
				WHEN 'admin' THEN 3 
				ELSE 4 
			END ASC
	`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_error"})
	}
	defer rows.Close()

	var users []UnassignedUser
	for rows.Next() {
		var u UnassignedUser
		err := rows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.IsActive, &u.Role)
		if err != nil {
			continue
		}
		users = append(users, u)
	}

	if users == nil {
		users = []UnassignedUser{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"users": users})
}

// handleAssignSeat assigns a license seat to a user
func (s *Server) handleAssignSeat(c echo.Context) error {
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := c.Bind(&body); err != nil || body.UserID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_id_required"})
	}

	// Get current user from context (set by auth middleware)
	user, ok := c.Get("user").(*models.User)
	if !ok || user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid_auth"})
	}
	assignerUserID := user.ID

	// Check seat availability
	svc := s.licenseService()
	licState, err := svc.LoadOrInit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed_to_load_license"})
	}

	if licState.Status == "missing" || licState.Status == "invalid" || licState.Status == "expired" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_not_active"})
	}

	if !licState.Unlimited && licState.SeatCount != nil {
		var assignedCount int
		err := s.db.QueryRow("SELECT COUNT(*) FROM license_seat_assignments WHERE is_active = TRUE").Scan(&assignedCount)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_error"})
		}
		if assignedCount >= *licState.SeatCount {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "no_seats_available"})
		}
	}

	// Check if user already has an assignment
	var existingID sql.NullInt64
	err = s.db.QueryRow("SELECT id FROM license_seat_assignments WHERE user_id = $1 AND is_active = TRUE", body.UserID).Scan(&existingID)
	if err == nil && existingID.Valid {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_already_assigned"})
	}

	// Create assignment
	_, err = s.db.Exec(`
		INSERT INTO license_seat_assignments (user_id, assigned_by_user_id, is_active)
		VALUES ($1, $2, TRUE)
	`, body.UserID, assignerUserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed_to_assign"})
	}

	// Invalidate cache so enforcement middleware sees new assignment immediately
	middleware.InvalidateSeatAssignmentCache()

	return c.JSON(http.StatusOK, map[string]string{"message": "seat_assigned"})
}

// handleBulkAssignSeats assigns seats to multiple users
func (s *Server) handleBulkAssignSeats(c echo.Context) error {
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	var body struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := c.Bind(&body); err != nil || len(body.UserIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_ids_required"})
	}

	// Get current user from context (set by auth middleware)
	user, ok := c.Get("user").(*models.User)
	if !ok || user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid_auth"})
	}
	assignerUserID := user.ID

	// Check seat availability
	svc := s.licenseService()
	licState, err := svc.LoadOrInit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed_to_load_license"})
	}

	if licState.Status == "missing" || licState.Status == "invalid" || licState.Status == "expired" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_not_active"})
	}

	if !licState.Unlimited && licState.SeatCount != nil {
		var assignedCount int
		err := s.db.QueryRow("SELECT COUNT(*) FROM license_seat_assignments WHERE is_active = TRUE").Scan(&assignedCount)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_error"})
		}
		available := *licState.SeatCount - assignedCount
		if len(body.UserIDs) > available {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error":   "insufficient_seats",
				"message": "Not enough seats available",
			})
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transaction_error"})
	}
	defer tx.Rollback()

	successCount := 0
	for _, userID := range body.UserIDs {
		// Check if already assigned
		var existingID sql.NullInt64
		err = tx.QueryRow("SELECT id FROM license_seat_assignments WHERE user_id = $1 AND is_active = TRUE", userID).Scan(&existingID)
		if err == nil && existingID.Valid {
			continue // Skip already assigned
		}

		_, err = tx.Exec(`
			INSERT INTO license_seat_assignments (user_id, assigned_by_user_id, is_active)
			VALUES ($1, $2, TRUE)
		`, userID, assignerUserID)
		if err == nil {
			successCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "commit_error"})
	}

	// Invalidate cache so enforcement middleware sees new assignments immediately
	middleware.InvalidateSeatAssignmentCache()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "bulk_assign_complete",
		"assigned": successCount,
		"total":    len(body.UserIDs),
	})
}

// handleRevokeSeat revokes a license seat from a user
func (s *Server) handleRevokeSeat(c echo.Context) error {
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
	}

	result, err := s.db.Exec(`
		UPDATE license_seat_assignments 
		SET is_active = FALSE, revoked_at = NOW()
		WHERE user_id = $1 AND is_active = TRUE
	`, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_error"})
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "assignment_not_found"})
	}

	// Invalidate cache so enforcement middleware sees revocation immediately
	middleware.InvalidateSeatAssignmentCache()

	return c.JSON(http.StatusOK, map[string]string{"message": "seat_revoked"})
}

// handleBulkRevokeSeats revokes seats from multiple users
func (s *Server) handleBulkRevokeSeats(c echo.Context) error {
	if middleware.IsCloudMode() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not_available_in_cloud_mode"})
	}

	var body struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := c.Bind(&body); err != nil || len(body.UserIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_ids_required"})
	}

	tx, err := s.db.Begin()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transaction_error"})
	}
	defer tx.Rollback()

	revokedCount := 0
	for _, userID := range body.UserIDs {
		result, err := tx.Exec(`
			UPDATE license_seat_assignments 
			SET is_active = FALSE, revoked_at = NOW()
			WHERE user_id = $1 AND is_active = TRUE
		`, userID)
		if err == nil {
			affected, _ := result.RowsAffected()
			if affected > 0 {
				revokedCount++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "commit_error"})
	}

	// Invalidate cache so enforcement middleware sees revocations immediately
	middleware.InvalidateSeatAssignmentCache()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "bulk_revoke_complete",
		"revoked": revokedCount,
		"total":   len(body.UserIDs),
	})
}
