package api

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// authenticateAdmin checks the X-Admin-Password header and validates it against the stored hash in the database.
func (s *Server) authenticateAdmin(c echo.Context) error {
	password := c.Request().Header.Get("X-Admin-Password")
	log.Printf("[DEBUG] authenticateAdmin: Authentication header present: %v", password != "")
	if password == "" {
		log.Printf("[DEBUG] authenticateAdmin: Authentication failed - no password provided")
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	log.Printf("[DEBUG] authenticateAdmin: Querying database for admin password")
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		log.Printf("[DEBUG] authenticateAdmin: Database error retrieving password: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}
	log.Printf("[DEBUG] authenticateAdmin: Retrieved hashed password from database")

	// Verify the provided password
	log.Printf("[DEBUG] authenticateAdmin: Verifying password")
	if !comparePasswords(hashedPassword, password) {
		log.Printf("[DEBUG] authenticateAdmin: Authentication failed - invalid password")
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}
	log.Printf("[DEBUG] authenticateAdmin: Password verification successful")
	return nil
}
