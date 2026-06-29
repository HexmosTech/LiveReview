package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type glLRCTreeEntry struct {
	Type string `json:"type"` // "blob" or "tree"
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetRepoConfigFiles fetches .lrc/ from GitLab at the given ref.
// Implements lrcfetch.Provider. Returns (nil, false, nil) when .lrc/ does not exist.
func (p *GitLabProvider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	instanceURL := strings.TrimSuffix(p.config.URL, "/")
	if instanceURL == "" {
		instanceURL = "https://gitlab.com"
	}
	token := p.config.Token

	client := &http.Client{Timeout: 15 * time.Second}
	encodedProject := url.PathEscape(repoFullName)

	treeURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/tree?path=.lrc&ref=%s&recursive=true&per_page=100",
		instanceURL, encodedProject, url.QueryEscape(ref))

	entries, found, err := glLRCFetchTree(ctx, client, treeURL, token)
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
			if strings.Contains(strings.TrimPrefix(relPath, "rules/"), "/") {
				continue
			}
		default:
			continue
		}

		content, err := glLRCFetchFileRaw(ctx, client, instanceURL, encodedProject, entry.Path, ref, token)
		if err != nil {
			return nil, false, err
		}
		files[relPath] = content
	}

	return files, true, nil
}

func glLRCFetchTree(ctx context.Context, client *http.Client, treeURL, token string) ([]glLRCTreeEntry, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: create tree request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
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

	var entries []glLRCTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("gitlab lrc: decode tree: %w", err)
	}
	if len(entries) == 0 {
		return nil, false, nil
	}
	return entries, true, nil
}

func glLRCFetchFileRaw(ctx context.Context, client *http.Client, baseURL, encodedProject, filePath, ref, token string) ([]byte, error) {
	fileURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		baseURL, encodedProject, url.PathEscape(filePath), url.QueryEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab lrc: create file request for %s: %w", filePath, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
