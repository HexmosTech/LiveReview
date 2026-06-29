package gitlab

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	networkgitlabin "github.com/livereview/network/providers/gitlab"
)

type gitlabInstanceURLKey struct{}

// ExtractGitLabInstanceURL extracts the scheme+host from a GitLab project or MR
// web URL so the caller can look up the right integration token.
// Example: "https://gitlab.example.com/group/project" → "https://gitlab.example.com"
func ExtractGitLabInstanceURL(webURL string) string {
	return extractGitLabInstanceURLV2(webURL)
}

// WithInstanceURL stores a GitLab instance base URL in ctx so that
// GetRepoConfigFiles can look up the right integration token. Call this before
// GetRepoConfigFiles when you have the event's Repository.WebURL:
//
//	ctx = gitlab.WithInstanceURL(ctx, extractGitLabInstanceURLV2(event.Repository.WebURL))
func WithInstanceURL(ctx context.Context, instanceURL string) context.Context {
	return context.WithValue(ctx, gitlabInstanceURLKey{}, instanceURL)
}

func instanceURLFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(gitlabInstanceURLKey{}).(string); ok {
		return v
	}
	return ""
}

// gitlabTreeEntry is a single item returned by the GitLab repository tree API.
type gitlabTreeEntry struct {
	Type string `json:"type"` // "blob" (file) or "tree" (directory)
	Name string `json:"name"`
	Path string `json:"path"` // path relative to repo root
}

// GetRepoConfigFiles fetches the .lrc/ directory from a GitLab project at
// the given ref. Implements lrcfetch.Provider.
//
// repoFullName is "namespace/project" (e.g. "myorg/myrepo"). ref is the
// branch name (e.g. "main").
//
// The GitLab instance URL is read from ctx (set via WithInstanceURL). If not
// present, "https://gitlab.com" is used as a fallback — correct for gitlab.com
// but may fail for self-hosted instances that don't have a gitlab.com token.
//
// Returns (nil, false, nil) when .lrc/ does not exist on the project.
func (p *GitLabV2Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	instanceURL := instanceURLFromContext(ctx)
	if instanceURL == "" {
		instanceURL = "https://gitlab.com"
	}

	accessToken, err := p.getGitLabAccessTokenForLRC(instanceURL)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: no token for %s: %w", instanceURL, err)
	}

	client := networkgitlabin.NewHTTPClient(15 * time.Second)

	// URL-encode the project path for use in the API URL.
	encodedProject := url.PathEscape(repoFullName)

	// Fetch the .lrc/ tree with recursive=true — one call gets all blobs.
	treeURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/tree?path=.lrc&ref=%s&recursive=true&per_page=100",
		instanceURL, encodedProject, url.QueryEscape(ref))

	entries, found, err := gitlabFetchTree(ctx, client, treeURL, accessToken)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	files := make(map[string][]byte)

	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		relPath := strings.TrimPrefix(entry.Path, ".lrc/")
		switch {
		case relPath == "ignore":
			// ok
		case strings.HasPrefix(relPath, "rules/") && strings.HasSuffix(entry.Name, ".md"):
			// Direct child of rules/ only — no further slash after "rules/"
			if strings.Contains(strings.TrimPrefix(relPath, "rules/"), "/") {
				continue
			}
		default:
			continue
		}

		content, err := gitlabFetchFileRaw(ctx, client, instanceURL, encodedProject, entry.Path, ref, accessToken)
		if err != nil {
			return nil, false, err
		}
		files[relPath] = content
	}

	return files, true, nil
}

// getGitLabAccessTokenForLRC is a thin wrapper around the existing
// getGitLabAccessTokenV2 that returns the error in a lrc-specific form.
func (p *GitLabV2Provider) getGitLabAccessTokenForLRC(instanceURL string) (string, error) {
	query := `
		SELECT pat_token FROM integration_tokens
		WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted')
		AND RTRIM(provider_url, '/') = RTRIM($1, '/')
		LIMIT 1
	`
	var token string
	err := p.db.QueryRow(query, instanceURL).Scan(&token)
	if err == nil {
		return token, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("token query error: %w", err)
	}
	// Fallback: return any GitLab token (useful when instance URL is generic).
	fallback := `SELECT pat_token FROM integration_tokens WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') LIMIT 1`
	err = p.db.QueryRow(fallback).Scan(&token)
	if err != nil {
		return "", fmt.Errorf("no GitLab token found: %w", err)
	}
	return token, nil
}

// gitlabFetchTree calls the GitLab repository tree API and returns all entries.
// Returns (nil, false, nil) when the path does not exist (HTTP 404 or empty array).
func gitlabFetchTree(ctx context.Context, client *http.Client, treeURL, token string) ([]gitlabTreeEntry, bool, error) {
	req, err := networkgitlabin.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: create tree request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgitlabin.Do(client, req)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: tree request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("gitlab lrc: tree status %d: %s", resp.StatusCode, string(body))
	}

	var entries []gitlabTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: decode tree: %w", err)
	}
	// GitLab <17.7 returns 200 + empty array for non-existent paths.
	if len(entries) == 0 {
		return nil, false, nil
	}
	return entries, true, nil
}

// gitlabFetchFileRaw fetches the raw content of a repository file via the
// GitLab files API (returns raw bytes, no encoding).
func gitlabFetchFileRaw(ctx context.Context, client *http.Client, baseURL, encodedProject, filePath, ref, token string) ([]byte, error) {
	encodedPath := url.PathEscape(filePath)
	fileURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		baseURL, encodedProject, encodedPath, url.QueryEscape(ref))

	req, err := networkgitlabin.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgitlabin.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("gitlab lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gitlab lrc: read %s: %w", filePath, err)
	}
	return data, nil
}
