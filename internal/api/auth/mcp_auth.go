package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// MCPAuthRequest represents a pending MCP authentication request
type MCPAuthRequest struct {
	RequestID string     `json:"request_id"`
	Status    string     `json:"status"` // "pending", "completed"
	TokenPair *TokenPair `json:"token_pair,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// In-memory store for MCP auth requests (since we are skipping the DB part for now)
var (
	mcpAuthStore = make(map[string]*MCPAuthRequest)
	mcpAuthMutex sync.RWMutex
)

// InitiateMCPAuth creates a new pending auth request
func (h *AuthHandlers) InitiateMCPAuth(c echo.Context) error {
	requestID := uuid.New().String()
	
	mcpAuthMutex.Lock()
	mcpAuthStore[requestID] = &MCPAuthRequest{
		RequestID: requestID,
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	mcpAuthMutex.Unlock()

	return c.JSON(http.StatusOK, map[string]string{
		"request_id": requestID,
		"login_url":  "/auth/mcp?id=" + requestID,
	})
}

// CompleteMCPAuth is called by the UI after successful login
// This route should be protected by RequireAuth middleware
func (h *AuthHandlers) CompleteMCPAuth(c echo.Context) error {
	requestID := c.QueryParam("id")
	if requestID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing request id")
	}

	// In a real app, we'd get the tokens for the current logged-in user
	// For this bridge, the UI should have already called EnsureCloudUser and have tokens
	var tokenPair TokenPair
	if err := c.Bind(&tokenPair); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid token pair")
	}

	mcpAuthMutex.Lock()
	req, ok := mcpAuthStore[requestID]
	if !ok || time.Now().After(req.ExpiresAt) {
		mcpAuthMutex.Unlock()
		return echo.NewHTTPError(http.StatusNotFound, "Request not found or expired")
	}

	req.Status = "completed"
	req.TokenPair = &tokenPair
	mcpAuthMutex.Unlock()

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// PollMCPAuth allows the MCP server to check for completion
func (h *AuthHandlers) PollMCPAuth(c echo.Context) error {
	requestID := c.Param("request_id")
	
	mcpAuthMutex.RLock()
	req, ok := mcpAuthStore[requestID]
	mcpAuthMutex.RUnlock()

	if !ok || time.Now().After(req.ExpiresAt) {
		return echo.NewHTTPError(http.StatusNotFound, "Request not found or expired")
	}

	if req.Status != "completed" {
		return c.JSON(http.StatusAccepted, map[string]string{"status": "pending"})
	}

	// Once claimed, we could remove it from the map, but let's keep it for a bit or rely on expiration
	return c.JSON(http.StatusOK, req)
}
