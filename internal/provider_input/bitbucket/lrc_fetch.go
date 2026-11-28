package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	networkbitbucket "github.com/livereview/network/providers/bitbucket"
)

// bitbucketSrcEntry is a single item from the Bitbucket source (src) API.
type bitbucketSrcEntry struct {
	Type  string `json:"type"` // "commit_file" or "commit_directory"
	Path  string `json:"path"` // path relative to repo root (e.g. ".lrc/ignore")
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

// bitbucketSrcResponse is the paginated response from the Bitbucket src API.
type bitbucketSrcResponse struct {
	Values []bitbucketSrcEntry `json:"values"`
}

// GetRepoConfigFiles fetches the .lrc/ directory from a Bitbucket repository
// at the given ref. Implements lrcfetch.Provider.
//
// repoFullName is "workspace/repo_slug". ref is the branch name (e.g. "main").
// Auth uses Basic Auth with email (from token metadata) + app password (PatToken).
// Returns (nil, false, nil) when .lrc/ does not exist on the repo.
func (p *BitbucketV2Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	token, err := p.FindIntegrationTokenForRepo(repoFullName)
	if err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: no token for %s: %w", repoFullName, err)
	}

	email := extractBitbucketEmail(token)
	if email == "" {
		return nil, false, fmt.Errorf("bitbucket lrc: token metadata missing email for %s", repoFullName)
	}

	client := networkbitbucket.NewHTTPClient(15 * time.Second)

	// Bitbucket Cloud REST API v2 /src/{commit}/{path} truncates refs containing slashes
	// (treating the first segment as the commit and returning 404). If ref has a slash and
	// is not a 40-char commit SHA, resolve it to target commit SHA first via the branches API.
	resolvedRef := resolveBitbucketRef(ctx, client, repoFullName, ref, token.PatToken, email)

	// List .lrc/ directory.
	listURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/src/%s/.lrc/?pagelen=100",
		repoFullName, resolvedRef)
	rootEntries, found, err := bitbucketListDir(ctx, client, listURL, email, token.PatToken)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	files := make(map[string][]byte)
	hasRulesDir := false
	hasPolicyDir := false

	for _, entry := range rootEntries {
		switch {
		case entry.Type == "commit_file" && entry.Path == ".lrc/ignore":
			content, err := bitbucketFetchFile(ctx, client, "https://api.bitbucket.org/2.0/repositories/"+repoFullName+"/src/"+resolvedRef+"/.lrc/ignore", email, token.PatToken)
			if err != nil {
				return nil, false, err
			}
			files["ignore"] = content
		case entry.Type == "commit_file" && entry.Path == ".lrc/tools.toml":
			content, err := bitbucketFetchFile(ctx, client, "https://api.bitbucket.org/2.0/repositories/"+repoFullName+"/src/"+resolvedRef+"/.lrc/tools.toml", email, token.PatToken)
			if err != nil {
				return nil, false, err
			}
			files["tools.toml"] = content
		case entry.Type == "commit_directory" && entry.Path == ".lrc/policy":
			hasPolicyDir = true
		case entry.Type == "commit_directory" && entry.Path == ".lrc/rules":
			hasRulesDir = true
		}
	}

	if hasPolicyDir {
		policyURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/src/%s/.lrc/policy/?pagelen=100",
			repoFullName, resolvedRef)
		policyEntries, found, err := bitbucketListDir(ctx, client, policyURL, email, token.PatToken)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range policyEntries {
				if entry.Type != "commit_file" {
					continue
				}
				name := entry.Path[strings.LastIndex(entry.Path, "/")+1:]
				if name == "tools.toml" {
					content, err := bitbucketFetchFile(ctx, client, "https://api.bitbucket.org/2.0/repositories/"+repoFullName+"/src/"+resolvedRef+"/"+entry.Path, email, token.PatToken)
					if err != nil {
						return nil, false, err
					}
					files["policy/tools.toml"] = content
				}
			}
		}
	}

	// List .lrc/rules/ directory.
	if hasRulesDir {
		rulesURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/src/%s/.lrc/rules/?pagelen=100",
			repoFullName, resolvedRef)
		rulesEntries, found, err := bitbucketListDir(ctx, client, rulesURL, email, token.PatToken)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "commit_file" {
					continue
				}
				name := entry.Path[strings.LastIndex(entry.Path, "/")+1:]
				if !strings.HasSuffix(name, ".md") {
					continue
				}
				// Direct children only — no further slash after rules/.
				relFromRules := strings.TrimPrefix(entry.Path, ".lrc/rules/")
				if strings.Contains(relFromRules, "/") {
					continue
				}

				fileURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/src/%s/.lrc/rules/%s",
					repoFullName, resolvedRef, name)
				content, err := bitbucketFetchFile(ctx, client, fileURL, email, token.PatToken)
				if err != nil {
					return nil, false, err
				}
				files["rules/"+name] = content
			}
		}
	}

	return files, true, nil
}

func extractBitbucketEmail(token *IntegrationToken) string {
	if token.Metadata == nil {
		return ""
	}
	switch v := token.Metadata["email"].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

// bitbucketListDir lists a Bitbucket src directory. Returns (nil, false, nil) on 404.
func bitbucketListDir(ctx context.Context, client *http.Client, listURL, email, pat string) ([]bitbucketSrcEntry, bool, error) {
	req, err := networkbitbucket.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: create list request: %w", err)
	}
	req.SetBasicAuth(email, pat)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkbitbucket.Do(client, req)
	if err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("bitbucket lrc: list status %d: %s", resp.StatusCode, string(body))
	}

	var result bitbucketSrcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: decode listing: %w", err)
	}
	if len(result.Values) == 0 {
		return nil, false, nil
	}
	return result.Values, true, nil
}

// bitbucketFetchFile fetches a Bitbucket file; the src API returns raw content directly.
func bitbucketFetchFile(ctx context.Context, client *http.Client, fileURL, email, pat string) ([]byte, error) {
	req, err := networkbitbucket.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket lrc: create file request: %w", err)
	}
	req.SetBasicAuth(email, pat)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	// Use 30s timeout for file content (slightly larger)
	fileClient := networkbitbucket.NewHTTPClient(30 * time.Second)
	resp, err := networkbitbucket.Do(fileClient, req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket lrc: fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket lrc: file status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bitbucket lrc: read file: %w", err)
	}
	return data, nil
}

func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func resolveBitbucketRef(ctx context.Context, client *http.Client, repoFullName, ref, token, email string) string {
	if err := ctx.Err(); err != nil {
		return ref
	}
	if !strings.Contains(ref, "/") || isHexSHA(ref) {
		return ref
	}

	filter := url.QueryEscape(fmt.Sprintf("name = %q", ref))

	// 1. Try branch lookup
	branchURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/refs/branches?q=%s", repoFullName, filter)
	if hash := queryBitbucketRefTarget(ctx, client, branchURL, token, email); hash != "" {
		return hash
	}

	// 2. Fallback to tag lookup in case ref is a tag containing slashes
	tagURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/refs/tags?q=%s", repoFullName, filter)
	if hash := queryBitbucketRefTarget(ctx, client, tagURL, token, email); hash != "" {
		return hash
	}

	return ref
}

func queryBitbucketRefTarget(ctx context.Context, client *http.Client, queryURL, token, email string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return ""
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkbitbucket.Do(client, req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var branchResp struct {
		Values []struct {
			Name   string `json:"name"`
			Target struct {
				Hash string `json:"hash"`
			} `json:"target"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branchResp); err == nil && len(branchResp.Values) > 0 {
		return branchResp.Values[0].Target.Hash
	}
	return ""
}
