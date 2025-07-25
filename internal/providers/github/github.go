package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"log"

	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

type GitHubProvider struct {
	PAT string
}

func NewGitHubProvider(pat string) *GitHubProvider {
	log.Printf("[DEBUG] NewGitHubProvider called with PAT length: %d", len(pat))
	return &GitHubProvider{PAT: pat}
}

func (p *GitHubProvider) Name() string {
	return "github"
}

func (p *GitHubProvider) Configure(config map[string]interface{}) error {
	log.Printf("[DEBUG] GitHubProvider.Configure called with config keys: %v", getKeys(config))
	if pat, ok := config["pat_token"].(string); ok {
		log.Printf("[DEBUG] Setting PAT token, length: %d", len(pat))
		p.PAT = pat
		return nil
	}
	log.Printf("[DEBUG] pat_token not found in config")
	return fmt.Errorf("pat_token missing in config")
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (p *GitHubProvider) PostComment(ctx context.Context, prID string, comment *models.ReviewComment) error {
	log.Printf("[DEBUG] PostComment called with prID: '%s'", prID)
	// prID format: owner/repo/number
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid GitHub PR ID format: expected 'owner/repo/number', got '%s'", prID)
	}
	owner := parts[0]
	repo := parts[1]
	number := parts[2]
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, number)
	payload := map[string]string{"body": comment.Content}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("GitHub comment failed: %s", resp.Status)
	}
	return nil
}

func (p *GitHubProvider) PostComments(ctx context.Context, prID string, comments []*models.ReviewComment) error {
	for _, comment := range comments {
		err := p.PostComment(ctx, prID, comment)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *GitHubProvider) PostLineComment(ctx context.Context, prID string, comment *models.ReviewComment, commitID, path string, position int) error {
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid GitHub PR ID format: expected 'owner/repo/number', got '%s'", prID)
	}
	owner := parts[0]
	repo := parts[1]
	number := parts[2]
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/comments", owner, repo, number)
	payload := map[string]interface{}{
		"body":      comment.Content,
		"commit_id": commitID,
		"path":      path,
		"position":  position,
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("GitHub line comment failed: %s", resp.Status)
	}
	return nil
}

func (p *GitHubProvider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	parsed, err := url.Parse(mrURL)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub PR URL: %w", err)
	}
	parts := strings.Split(parsed.Path, "/")
	if len(parts) < 5 || parts[3] != "pull" {
		return nil, fmt.Errorf("invalid GitHub PR URL: expected /owner/repo/pull/number")
	}
	owner := parts[1]
	repo := parts[2]
	number := parts[4]
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s", owner, repo, number)
	log.Printf("[DEBUG] GitHubProvider: Try this curl command to debug:")
	log.Printf("curl -H 'Authorization: token %s' -H 'Accept: application/vnd.github.v3+json' '%s'", p.PAT, apiURL)
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub PR details failed: %s", resp.Status)
	}
	var pr struct {
		ID     int    `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA  string `json:"sha"`
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			SHA  string `json:"sha"`
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		CreatedAt string `json:"created_at"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	return &providers.MergeRequestDetails{
		ID:           fmt.Sprintf("%d", pr.Number),
		Title:        pr.Title,
		Description:  pr.Body,
		Author:       pr.User.Login,
		CreatedAt:    pr.CreatedAt,
		URL:          mrURL,
		State:        pr.State,
		WebURL:       pr.HTMLURL,
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		DiffRefs: providers.DiffRefs{
			BaseSHA: pr.Base.SHA,
			HeadSHA: pr.Head.SHA,
		},
		ProviderType:  "github",
		RepositoryURL: fmt.Sprintf("https://github.com/%s/%s", owner, repo),
	}, nil
}

func (p *GitHubProvider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	log.Printf("[DEBUG] GetMergeRequestChanges called with mrID: '%s', PAT length: %d", mrID, len(p.PAT))

	// Simple string splitting instead of complex scanf
	parts := strings.Split(mrID, "/")
	if len(parts) != 3 {
		log.Printf("[DEBUG] Invalid mrID format, expected 'owner/repo/number', got '%s' with %d parts: %v", mrID, len(parts), parts)
		return nil, fmt.Errorf("invalid GitHub PR ID format: expected 'owner/repo/number', got '%s'", mrID)
	}

	owner := parts[0]
	repo := parts[1]
	number := parts[2]

	log.Printf("[DEBUG] Parsed GitHub PR: owner='%s', repo='%s', number='%s'", owner, repo, number)
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/files", owner, repo, number)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub PR files failed: %s", resp.Status)
	}
	var files []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
		Patch     string `json:"patch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	var diffs []*models.CodeDiff
	for _, f := range files {
		diffs = append(diffs, &models.CodeDiff{
			FilePath: f.Filename,
			// Patch, Status, Additions, Deletions, Changes are not present in CodeDiff
			// You may want to parse f.Patch into Hunks, or store it in a custom field if needed
		})
	}
	return diffs, nil
}
