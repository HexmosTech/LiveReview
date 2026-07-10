package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ghLRCEntry struct {
	Type string `json:"type"` // "file" or "dir"
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetRepoConfigFiles fetches .lrc/ from GitHub at the given ref.
// Implements lrcfetch.Provider. Returns (nil, false, nil) when .lrc/ does not exist.
func (p *GitHubProvider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	if p.PAT == "" {
		return nil, false, nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	const apiBase = "https://api.github.com"

	rootEntries, found, err := ghLRCListDir(ctx, client, apiBase, repoFullName, ".lrc", ref, p.PAT)
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
			data, err := ghLRCFetchRaw(ctx, client, apiBase, repoFullName, entry.Path, ref, p.PAT)
			if err != nil {
				return nil, false, err
			}
			files["ignore"] = data
		case entry.Type == "dir" && entry.Name == "rules":
			hasRulesDir = true
		}
	}

	if hasRulesDir {
		rulesEntries, found, err := ghLRCListDir(ctx, client, apiBase, repoFullName, ".lrc/rules", ref, p.PAT)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".md") || strings.Contains(entry.Name, "/") {
					continue
				}
				data, err := ghLRCFetchRaw(ctx, client, apiBase, repoFullName, entry.Path, ref, p.PAT)
				if err != nil {
					return nil, false, err
				}
				files[strings.TrimPrefix(entry.Path, ".lrc/")] = data
			}
		}
	}

	return files, true, nil
}

func ghLRCListDir(ctx context.Context, client *http.Client, apiBase, repoFullName, path, ref, pat string) ([]ghLRCEntry, bool, error) {
	reqURL := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", apiBase, repoFullName, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("github lrc: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
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

	var entries []ghLRCEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("github lrc: decode %s: %w", path, err)
	}
	return entries, true, nil
}

func ghLRCFetchRaw(ctx context.Context, client *http.Client, apiBase, repoFullName, filePath, ref, pat string) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", apiBase, repoFullName, filePath, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("github lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
