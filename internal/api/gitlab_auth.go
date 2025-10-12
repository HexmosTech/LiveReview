package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	gitlabprovider "github.com/livereview/internal/provider_input/gitlab"
)

// GitLabHandleCodeExchange handles the exchange of authorization code for access token
func (s *Server) GitLabHandleCodeExchange(c echo.Context) error {
	var req gitlabprovider.GitLabTokenRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request format"})
	}

	result, err := s.gitlabAuthService.ExchangeCode(req)
	if err != nil {
		var authErr *gitlabprovider.AuthError
		if errors.As(err, &authErr) {
			return c.JSON(authErr.Status, ErrorResponse{Error: authErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":         "GitLab integration successful",
		"integration_id":  result.IntegrationID,
		"username":        result.Username,
		"connection_name": result.ConnectionName,
	})
}

// RefreshGitLabToken refreshes an expired GitLab token and updates it in the database.
func (s *Server) RefreshGitLabToken(integrationID int64, clientID, clientSecret string) gitlabprovider.GitLabRefreshTokenResult {
	return s.gitlabAuthService.RefreshToken(integrationID, clientID, clientSecret)
}

// GitLabRefreshToken refreshes an expired GitLab token via HTTP handler
func (s *Server) GitLabRefreshToken(c echo.Context) error {
	var req struct {
		IntegrationID      int64  `json:"integration_id" query:"integration_id"`
		GitlabClientID     string `json:"gitlab_client_id" query:"gitlab_client_id"`
		GitlabClientSecret string `json:"gitlab_client_secret" query:"gitlab_client_secret"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request format"})
	}

	if req.IntegrationID <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing or invalid integration_id parameter"})
	}

	result := s.RefreshGitLabToken(req.IntegrationID, req.GitlabClientID, req.GitlabClientSecret)
	if result.Error != nil {
		switch {
		case strings.Contains(result.Error.Error(), "failed to retrieve token data: sql: no rows"):
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "Token not found"})
		case strings.Contains(result.Error.Error(), "no refresh token available"):
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "No refresh token available"})
		case strings.Contains(result.Error.Error(), "missing client secret parameter"):
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Client secret not found in request or database"})
		default:
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: result.Error.Error()})
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":        "GitLab token refreshed successfully",
		"integration_id": req.IntegrationID,
		"expires_in":     result.TokenData.ExpiresIn,
	})
}
