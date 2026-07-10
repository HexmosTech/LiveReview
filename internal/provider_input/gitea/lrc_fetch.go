package gitea

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	networkgitea "github.com/livereview/network/providers/gitea"
)

// giteaContentEntry is a single item from the Gitea contents API.
// For directory listings, this is one entry in the returned array.
// For file fetches, this is the single object returned.
type giteaContentEntry struct {
	Type     string `json:"type"`     // "file" or "dir"
	Name     string `json:"name"`
	Path     string `json:"path"`
	Content  string `json:"content"`  // base64-encoded (files only)
	Encoding string `json:"encoding"` // "base64" (files only)
}

// GetRepoConfigFiles fetches the .lrc/ directory from a Gitea repository at
// the given ref. Implements lrcfetch.Provider.
//
// repoFullName is "owner/repo". ref is the branch name (e.g. "main").
// Returns (nil, false, nil) when .lrc/ does not exist on the repo.
func (p *GiteaV2Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	token, baseURL, err := FindIntegrationTokenForGiteaRepo(p.db, repoFullName)
	if err != nil {
		return nil, false, fmt.Errorf("gitea lrc: no token for %s: %w", repoFullName, err)
	}

	pat := token.PatToken
	client := networkgitea.NewHTTPClient(15 * time.Second)

	// List .lrc/ root directory.
	rootEntries, found, err := giteaListDir(ctx, client, baseURL, repoFullName, ".lrc", ref, pat)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	files := make(map[string][]byte)
	hasRulesDir := false

	for _, entry := range rootEntries {
		switch {
		case entry.Type == "file" && entry.Name == "ignore":
			content, err := giteaFetchFileContent(ctx, client, baseURL, repoFullName, entry.Path, ref, pat)
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
		rulesEntries, found, err := giteaListDir(ctx, client, baseURL, repoFullName, ".lrc/rules", ref, pat)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".md") {
					continue
				}
				if strings.Contains(entry.Name, "/") {
					continue // skip nested paths
				}
				content, err := giteaFetchFileContent(ctx, client, baseURL, repoFullName, entry.Path, ref, pat)
				if err != nil {
					return nil, false, err
				}
				relPath := strings.TrimPrefix(entry.Path, ".lrc/")
				files[relPath] = content
			}
		}
	}

	return files, true, nil
}

// giteaListDir lists a directory via the Gitea contents API.
// Returns (nil, false, nil) on 404.
func giteaListDir(ctx context.Context, client *http.Client, baseURL, repoFullName, path, ref, pat string) ([]giteaContentEntry, bool, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/contents/%s?ref=%s", baseURL, repoFullName, path, ref)
	req, err := networkgitea.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("gitea lrc: create list request: %w", err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgitea.Do(client, req)
	if err != nil {
		return nil, false, fmt.Errorf("gitea lrc: list %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("gitea lrc: list %s status %d: %s", path, resp.StatusCode, string(body))
	}

	var entries []giteaContentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("gitea lrc: decode listing for %s: %w", path, err)
	}
	return entries, true, nil
}

// giteaFetchFileContent fetches a file and decodes its base64 content.
// Gitea returns content as base64 with embedded newlines that must be stripped.
func giteaFetchFileContent(ctx context.Context, client *http.Client, baseURL, repoFullName, filePath, ref, pat string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/contents/%s?ref=%s", baseURL, repoFullName, filePath, ref)
	req, err := networkgitea.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := networkgitea.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitea lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}

	var entry giteaContentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("gitea lrc: decode file %s: %w", filePath, err)
	}

	// Strip embedded newlines before base64-decoding (Gitea wraps at 60 chars).
	cleaned := strings.ReplaceAll(entry.Content, "\n", "")
	data, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: base64 decode %s: %w", filePath, err)
	}
	return data, nil
}
