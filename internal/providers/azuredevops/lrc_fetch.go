package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

// azureItem is a single entry from the Git Items API listing.
type azureItem struct {
	ObjectID      string `json:"objectId"`
	GitObjectType string `json:"gitObjectType"` // "blob" or "tree"
	Path          string `json:"path"`          // leading-slash-prefixed, e.g. "/.lrc/rules/design.md"
}

type azureItemsResponse struct {
	Value []azureItem `json:"value"`
}

// GetRepoConfigFiles fetches the .lrc/ directory from an Azure DevOps
// repository at the given ref. Implements lrcfetch.Provider.
//
// repoFullName is normally "{project}/{repo}" (the convention used
// elsewhere in this codebase, e.g. webhook events' Repository.FullName).
// The one-shot/CLI review path, however, derives it generically from
// GetMergeRequestDetails' RepositoryURL via a provider-agnostic helper
// (internal/review/service.go's extractRepoFullName) that just returns the
// URL path as-is - for Azure DevOps that's "{org}/{project}/_git/{repo}",
// not "{project}/{repo}". parseRepoFullName below accepts both shapes.
//
// ref is a branch name (e.g. "main"). Returns (nil, false, nil) when .lrc/
// does not exist on the repo.
//
// Unlike GitHub/GitLab/Gitea's shallow directory listing (requiring one call
// per directory level), recursionLevel=Full returns the entire .lrc/ subtree
// in a single call - filtering for "ignore" and direct children of
// "rules/*.md" is done client-side below.
func (p *Provider) GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (map[string][]byte, bool, error) {
	project, repo, err := parseRepoFullName(repoFullName)
	if err != nil {
		return nil, false, err
	}

	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/items?scopePath=.lrc&recursionLevel=Full&versionDescriptor.version=%s&api-version=%s",
		p.baseURL, neturl.PathEscape(project), neturl.PathEscape(repo), neturl.QueryEscape(ref), apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, false, fmt.Errorf("azure devops lrc: create list request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return nil, false, fmt.Errorf("azure devops lrc: list .lrc: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("azure devops lrc: list .lrc status %d: %s", resp.StatusCode, string(body))
	}

	var listing azureItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, false, fmt.Errorf("azure devops lrc: decode listing: %w", err)
	}

	files := make(map[string][]byte)
	for _, item := range listing.Value {
		if item.GitObjectType != "blob" {
			continue
		}
		relPath := strings.TrimPrefix(strings.TrimPrefix(item.Path, "/"), ".lrc/")

		switch {
		case relPath == "ignore":
			content, err := p.fetchBlob(ctx, p.baseURL, project, repo, item.ObjectID)
			if err != nil {
				return nil, false, fmt.Errorf("azure devops lrc: fetch ignore: %w", err)
			}
			files["ignore"] = []byte(content)
		case strings.HasPrefix(relPath, "rules/") && strings.HasSuffix(relPath, ".md"):
			if strings.Contains(strings.TrimPrefix(relPath, "rules/"), "/") {
				continue // nested subdirectory, skip
			}
			content, err := p.fetchBlob(ctx, p.baseURL, project, repo, item.ObjectID)
			if err != nil {
				return nil, false, fmt.Errorf("azure devops lrc: fetch %s: %w", relPath, err)
			}
			files[relPath] = []byte(content)
		}
	}

	return files, true, nil
}

// parseRepoFullName extracts (project, repo) from either of the two shapes
// GetRepoConfigFiles is called with:
//   - "{project}/{repo}" - the webhook-path convention (2 segments)
//   - "{org}/{project}/_git/{repo}" - the raw RepositoryURL path used by the
//     one-shot review path (extractRepoFullName in internal/review/service.go
//     is provider-agnostic and doesn't strip Azure's org prefix/"_git" marker)
func parseRepoFullName(repoFullName string) (project, repo string, err error) {
	if idx := strings.Index(repoFullName, "/_git/"); idx != -1 {
		left := repoFullName[:idx]
		repo = repoFullName[idx+len("/_git/"):]
		if segs := strings.Split(left, "/"); len(segs) > 0 {
			project = segs[len(segs)-1]
		}
		if project == "" || repo == "" {
			return "", "", fmt.Errorf("azure devops lrc: invalid repoFullName %q", repoFullName)
		}
		return project, repo, nil
	}

	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("azure devops lrc: invalid repoFullName %q", repoFullName)
	}
	return parts[0], parts[1], nil
}
