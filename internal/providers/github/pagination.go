package github

import (
	"net/http"
	"regexp"
)

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// nextPageFromLinkHeader extracts the "next" page URL from a GitHub API
// response's Link header, per https://docs.github.com/en/rest/guides/using-pagination-in-the-rest-api.
// Returns "" if there is no next page.
func nextPageFromLinkHeader(resp *http.Response) string {
	link := resp.Header.Get("Link")
	if link == "" {
		return ""
	}
	m := linkNextRe.FindStringSubmatch(link)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}
