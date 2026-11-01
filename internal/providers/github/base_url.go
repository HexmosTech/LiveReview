package github

import "strings"

// apiBaseURL returns the GitHub REST API base URL for a given connector base
// URL: github.com uses api.github.com, while GitHub Enterprise instances serve
// their API under /api/v3 of the same host.
func apiBaseURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" || u == "https://github.com" || u == "http://github.com" || u == "github.com" || u == "https://api.github.com" {
		return "https://api.github.com"
	}
	if strings.HasSuffix(u, "/api/v3") {
		return u
	}
	return u + "/api/v3"
}

// APIBaseURL is the exported form of apiBaseURL for callers outside this package.
func APIBaseURL(baseURL string) string { return apiBaseURL(baseURL) }
