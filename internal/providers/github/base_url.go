package github

import "strings"

// apiBaseURL returns the GitHub REST API base URL for a given connector base
// URL: github.com uses api.github.com, while GitHub Enterprise instances serve
// their API under /api/v3 of the same host.
func apiBaseURL(baseURL string) string {
	if baseURL == "" || baseURL == "https://github.com" {
		return "https://api.github.com"
	}
	return strings.TrimSuffix(baseURL, "/") + "/api/v3"
}
