package gitea

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type gtLRCEntry struct {
	Type    string `json:"type"`    // "file" or "dir"
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded, present only for single-file responses
}

// GetRepoConfigFiles fetches .lrc/ from Gitea at the given ref.
// Implements lrcfetch.Provider. Returns (nil, false, nil) when .lrc/ does not exist.
func (p *Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	if p.baseURL == "" || p.token == "" {
		return nil, false, nil
	}

	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		return nil, false, fmt.Errorf("gitea lrc: invalid repoFullName %q", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	rootEntries, found, err := gtLRCListDir(ctx, p.httpClient, p.baseURL, owner, repo, ".lrc", ref, p.token)
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
			data, err := gtLRCFetchFile(ctx, p.httpClient, p.baseURL, owner, repo, ".lrc/ignore", ref, p.token)
			if err != nil {
				return nil, false, err
			}
			files["ignore"] = data
		case entry.Type == "dir" && entry.Name == "rules":
			hasRulesDir = true
		}
	}

	if hasRulesDir {
		rulesEntries, found, err := gtLRCListDir(ctx, p.httpClient, p.baseURL, owner, repo, ".lrc/rules", ref, p.token)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".md") || strings.Contains(entry.Name, "/") {
					continue
				}
				data, err := gtLRCFetchFile(ctx, p.httpClient, p.baseURL, owner, repo, entry.Path, ref, p.token)
				if err != nil {
					return nil, false, err
				}
				files[strings.TrimPrefix(entry.Path, ".lrc/")] = data
			}
		}
	}

	return files, true, nil
}

func gtLRCListDir(ctx context.Context, client *http.Client, baseURL, owner, repo, path, ref, token string) ([]gtLRCEntry, bool, error) {
	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s?ref=%s", baseURL, owner, repo, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("gitea lrc: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
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

	var entries []gtLRCEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("gitea lrc: decode %s: %w", path, err)
	}
	return entries, true, nil
}

// gtLRCFetchFile fetches a single file. Gitea returns base64-encoded content
// with embedded newlines — strip them before decoding.
func gtLRCFetchFile(ctx context.Context, client *http.Client, baseURL, owner, repo, filePath, ref, token string) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s?ref=%s", baseURL, owner, repo, filePath, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitea lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}

	var entry gtLRCEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("gitea lrc: decode file %s: %w", filePath, err)
	}

	cleaned := strings.ReplaceAll(entry.Content, "\n", "")
	data, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("gitea lrc: base64 decode %s: %w", filePath, err)
	}
	return data, nil
}
