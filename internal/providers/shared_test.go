package providers

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "typed AuthError with 401",
			err: &AuthError{
				Provider:   "github",
				StatusCode: 401,
				Message:    "Bad credentials",
			},
			expected: true,
		},
		{
			name: "wrapped typed AuthError",
			err: fmt.Errorf("request failed: %w", &AuthError{
				Provider:   "gitlab",
				StatusCode: 403,
				Message:    "403 Forbidden",
			}),
			expected: true,
		},
		{
			name:     "string with Bad credentials",
			err:      errors.New("GitHub API request failed with status 401: {\"message\":\"Bad credentials\"}"),
			expected: true,
		},
		{
			name:     "string with 401 Unauthorized",
			err:      errors.New("GitLab API error: 401 Unauthorized"),
			expected: true,
		},
		{
			name:     "string with status 401",
			err:      errors.New("request failed with status 401: unauthorized"),
			expected: true,
		},
		{
			name:     "string with invalid token",
			err:      errors.New("oauth: invalid token provided"),
			expected: true,
		},
		{
			name:     "string with token expired",
			err:      errors.New("access token expired at 2026-10-27"),
			expected: true,
		},
		{
			name:     "string with failed to resolve github app token",
			err:      errors.New("failed to resolve GitHub App token for connector 5: installation suspended"),
			expected: true,
		},
		{
			name:     "Azure DevOps invalid Personal Access Token",
			err:      errors.New("invalid Personal Access Token - verify the token and its scopes"),
			expected: true,
		},
		{
			name:     "Azure DevOps bracketed (401)",
			err:      errors.New("azure devops pull request fetch failed (401): TF400813: The user is not authorized"),
			expected: true,
		},
		{
			name:     "GitLab invalid_token json error",
			err:      errors.New("API request failed with status 401: {\"error\":\"invalid_token\",\"error_description\":\"Token is expired\"}"),
			expected: true,
		},
		{
			name:     "Bitbucket 401 Unauthorized response",
			err:      errors.New("Bitbucket PR details failed: 401 Unauthorized, response: {\"type\": \"error\"}"),
			expected: true,
		},
		{
			name:     "Gitea bracketed (401)",
			err:      errors.New("gitea pull request fetch failed (401): unauthorized"),
			expected: true,
		},
		{
			name:     "rate limit error with 403 must NOT be auth error",
			err:      errors.New("status 403: API rate limit exceeded"),
			expected: false,
		},
		{
			name:     "unrelated network error",
			err:      errors.New("connection reset by peer"),
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAuthError(tc.err)
			if got != tc.expected {
				t.Errorf("IsAuthError(%v) = %v, expected %v", tc.err, got, tc.expected)
			}
		})
	}

	// Test provider-specific calls (e.g. gitlab vs gitlab-self-hosted)
	t.Run("gitlab and gitlab-self-hosted provider checks", func(t *testing.T) {
		glErr := errors.New("API request failed with status 401: {\"error\":\"invalid_token\"}")
		if !IsAuthError(glErr, "gitlab") {
			t.Error("expected IsAuthError to return true for gitlab")
		}
		if !IsAuthError(glErr, "gitlab-self-hosted") {
			t.Error("expected IsAuthError to return true for gitlab-self-hosted")
		}
		if !IsAuthError(glErr, "gitlab-com") {
			t.Error("expected IsAuthError to return true for gitlab-com")
		}
	})
}

