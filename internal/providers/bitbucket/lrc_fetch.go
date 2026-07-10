package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type bbLRCSrcEntry struct {
	Type  string `json:"type"` // "commit_file" or "commit_directory"
	Path  string `json:"path"`
}

type bbLRCSrcResponse struct {
	Values []bbLRCSrcEntry `json:"values"`
}

// GetRepoConfigFiles fetches .lrc/ from Bitbucket at the given ref.
// Implements lrcfetch.Provider. Returns (nil, false, nil) when .lrc/ does not exist.
func (p *BitbucketProvider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	// repoFullName is "workspace/repo" — split it
	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		return nil, false, fmt.Errorf("bitbucket lrc: invalid repoFullName %q", repoFullName)
	}
	workspace, repoSlug := parts[0], parts[1]

	rootEntries, found, err := bbLRCListDir(ctx, p.httpClient, workspace, repoSlug, ref, ".lrc", p.token, p.email)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	files := make(map[string][]byte)
	hasRulesDir := false

	for _, entry := range rootEntries {
		name := entry.Path[strings.LastIndex(entry.Path, "/")+1:]
		switch {
		case entry.Type == "commit_file" && name == "ignore":
			data, err := bbLRCFetchRaw(ctx, p.httpClient, workspace, repoSlug, ref, ".lrc/ignore", p.token, p.email)
			if err != nil {
				return nil, false, err
			}
			files["ignore"] = data
		case entry.Type == "commit_directory" && name == "rules":
			hasRulesDir = true
		}
	}

	if hasRulesDir {
		rulesEntries, found, err := bbLRCListDir(ctx, p.httpClient, workspace, repoSlug, ref, ".lrc/rules", p.token, p.email)
		if err != nil {
			return nil, false, err
		}
		if found {
			for _, entry := range rulesEntries {
				if entry.Type != "commit_file" {
					continue
				}
				name := entry.Path[strings.LastIndex(entry.Path, "/")+1:]
				if !strings.HasSuffix(name, ".md") || strings.Contains(strings.TrimPrefix(entry.Path, ".lrc/rules/"), "/") {
					continue
				}
				data, err := bbLRCFetchRaw(ctx, p.httpClient, workspace, repoSlug, ref, entry.Path, p.token, p.email)
				if err != nil {
					return nil, false, err
				}
				files[strings.TrimPrefix(entry.Path, ".lrc/")] = data
			}
		}
	}

	return files, true, nil
}

func bbLRCListDir(ctx context.Context, client *http.Client, workspace, repo, ref, path, token, email string) ([]bbLRCSrcEntry, bool, error) {
	reqURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/src/%s/%s/?pagelen=100",
		workspace, repo, ref, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: create request: %w", err)
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: list %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("bitbucket lrc: list %s status %d: %s", path, resp.StatusCode, string(body))
	}

	var result bbLRCSrcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false, fmt.Errorf("bitbucket lrc: decode %s: %w", path, err)
	}
	if len(result.Values) == 0 {
		return nil, false, nil
	}
	return result.Values, true, nil
}

func bbLRCFetchRaw(ctx context.Context, client *http.Client, workspace, repo, ref, filePath, token, email string) ([]byte, error) {
	reqURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/src/%s/%s",
		workspace, repo, ref, filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket lrc: create file request for %s: %w", filePath, err)
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket lrc: fetch %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket lrc: fetch %s status %d: %s", filePath, resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
