// Command backfill-review-pr-links is a one-time maintenance script that
// links pre-existing reviews rows to their canonical pull_requests row by
// matching reviews.pr_mr_url against pull_requests.web_url.
//
// Run manually, once per environment, after the repo/PR sync feature has had
// a chance to populate pull_requests for the org's connected repositories
// (either via the initial per-connector backfill or a few periodic
// reconciliation cycles) - matching against a table that is still empty will
// simply match nothing.
//
// This is best-effort: matching is done on a normalized form of each URL
// (lowercased scheme/host, trailing slash and ".git" suffix stripped, query
// string and fragment ignored) to absorb the most common real-world
// differences between how a URL was originally pasted into pr_mr_url and how
// the provider's API reports the same PR/MR's web_url. Reviews whose URL
// still doesn't match anything (a different host alias, a genuinely deleted
// PR, etc.) are left with pull_request_id = NULL - they remain visible via
// the existing reviews list/search, just not grouped under a PR/MR's
// review-history view.
//
// Usage:
//
//	go run ./cmd/backfill-review-pr-links
package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/livereview/internal/database"
)

// normalizeURL reduces a PR/MR URL to a comparable canonical form. It is
// intentionally conservative: it only strips differences that are known to
// vary between a hand-pasted URL and a provider API's reported web_url for
// the same PR/MR, not general URL variations.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		// Not a parseable URL - fall back to a light string normalization
		// rather than dropping it from matching entirely.
		return strings.ToLower(strings.TrimSuffix(raw, "/"))
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, ".git")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func main() {
	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	prRows, err := db.Query(`SELECT id, web_url FROM pull_requests WHERE web_url IS NOT NULL AND web_url != ''`)
	if err != nil {
		log.Fatalf("failed to load pull_requests: %v", err)
	}
	byNormalizedURL := make(map[string]int64)
	for prRows.Next() {
		var id int64
		var webURL string
		if err := prRows.Scan(&id, &webURL); err != nil {
			prRows.Close()
			log.Fatalf("failed to scan pull_requests row: %v", err)
		}
		if norm := normalizeURL(webURL); norm != "" {
			// If two pull_requests rows somehow normalize to the same URL
			// (shouldn't happen given the schema's uniqueness constraints,
			// but defensive), keep the first and don't silently overwrite it.
			if _, exists := byNormalizedURL[norm]; !exists {
				byNormalizedURL[norm] = id
			}
		}
	}
	if err := prRows.Err(); err != nil {
		prRows.Close()
		log.Fatalf("error iterating pull_requests: %v", err)
	}
	prRows.Close()
	fmt.Printf("Loaded %d pull_requests rows for matching.\n", len(byNormalizedURL))

	reviewRows, err := db.Query(`SELECT id, pr_mr_url FROM reviews WHERE pull_request_id IS NULL AND pr_mr_url IS NOT NULL`)
	if err != nil {
		log.Fatalf("failed to load candidate reviews: %v", err)
	}
	type candidate struct {
		id  int64
		url string
	}
	var candidates []candidate
	for reviewRows.Next() {
		var c candidate
		if err := reviewRows.Scan(&c.id, &c.url); err != nil {
			reviewRows.Close()
			log.Fatalf("failed to scan reviews row: %v", err)
		}
		candidates = append(candidates, c)
	}
	if err := reviewRows.Err(); err != nil {
		reviewRows.Close()
		log.Fatalf("error iterating reviews: %v", err)
	}
	reviewRows.Close()
	fmt.Printf("Found %d reviews with no pull_request_id and a non-null pr_mr_url.\n", len(candidates))

	stmt, err := db.Prepare(`UPDATE reviews SET pull_request_id = $1 WHERE id = $2`)
	if err != nil {
		log.Fatalf("failed to prepare update statement: %v", err)
	}
	defer stmt.Close()

	var matched int
	for _, c := range candidates {
		prID, ok := byNormalizedURL[normalizeURL(c.url)]
		if !ok {
			continue
		}
		if _, err := stmt.Exec(prID, c.id); err != nil {
			log.Fatalf("failed to link review %d to pull_request %d: %v", c.id, prID, err)
		}
		matched++
	}

	unmatched := len(candidates) - matched
	fmt.Printf("Linked %d reviews to their pull_requests row.\n", matched)
	fmt.Printf("%d reviews remain unmatched (no pull_requests row with a matching URL yet) - this is expected and non-blocking.\n", unmatched)
}
