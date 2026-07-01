package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	networkgithub "github.com/livereview/network/providers/github"
)

// githubContentEntry is the shape of items returned by the GitHub Contents API.
type githubContentEntry struct {
	Type string `json:"type"` // "file" or "dir"
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetRepoConfigFiles fetches the .lrc/ directory from a GitHub repository at
// the given ref. Implements lrcfetch.Provider.
//
// repoFullName is "owner/repo". ref is the branch name (e.g. "main").
// Returns (nil, false, nil) when .lrc/ does not exist on the repo.
func (p *GitHubV2Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	token, err := FindIntegrationTokenForGitHubRepo(p.db, repoFullName)
	if err != nil {
		return nil, false, fmt.Errorf("github lrc: no token for %s: %w", repoFullName, err)
	}

	pat := token.PatToken
	apiBase := "https://api.github.com"

	client := networkgithub.NewHTTPClient(15 * time.Second)

	// List the .lrc/ root directory.
	rootEntries, found, err := githubListDir(ctx, client, apiBase, repoFullName, ".lrc", ref, pat)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	files := make(map[string][]byte)

	// Collect .lrc/ignore and discover whether rules/ dir exists.
	hasRulesDir := false
	for _, entry := range rootEntries {
		switch {
		case entry.Type == "file" && entry.Name == "ignore":
			content, err := githubFetchFileRaw(ctx, client, apiBase, repoFullName, entry.Path, ref, pat)
			if err != nil {
				return nil, false, err
			}
			files["ignore"] = content
		case entry.Type == "dir" && entry.Name == "rules":
			hasRulesDir = true
		}
	}

	// List .lrc/rules/ and fetch direct-child .md files.
	if hasRulesDir {
		rulesEntries, found, err := githubListDir(ctx, client, apiBase, repoFullName, ".lrc/rules", ref, pat)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".md") {
					continue
				}
				// Skip nested paths: direct children of rules/ have no "/" in Name.
				if strings.Contains(entry.Name, "/") {
					continue
				}
				content, err := githubFetchFileRaw(ctx, client, apiBase, repoFullName, entry.Path, ref, pat)
				if err != nil {
					return nil, false, err
				}
				// Key relative to .lrc/: e.g. "rules/design.md"
				relPath := strings.TrimPrefix(entry.Path, ".lrc/")
				files[relPath] = content
			}
		}
	}

	return files, true, nil
}

// githubListDir calls GET /repos/{repoFullName}/contents/{path}?ref={ref} and
// returns the directory entries. Returns (nil, false, nil) on 404.
func githubListDir(ctx context.Context, client *http.Client, apiBase, repoFullName, path, ref, pat string) ([]githubContentEntry, bool, error) {
	url := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", apiBase, repoFullName, path, ref)
	req, err := networkgithub.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("github lrc: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgithub.Do(client, req)
	if err != nil {
		return nil, false, fmt.Errorf("github lrc: list %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("github lrc: list %s status %d: %s", path, resp.StatusCode, string(body))
	}

	var entries []githubContentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("github lrc: decode listing for %s: %w", path, err)
	}
	return entries, true, nil
}

// githubFetchFileRaw fetches raw file content using Accept: application/vnd.github.raw+json.
func githubFetchFileRaw(ctx context.Context, client *http.Client, apiBase, repoFullName, filePath, ref, pat string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", apiBase, repoFullName, filePath, ref)
	req, err := networkgithub.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("github lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgithub.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("github lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github lrc: read %s: %w", filePath, err)
	}
	return data, nil
}
