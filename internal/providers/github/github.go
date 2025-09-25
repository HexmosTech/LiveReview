package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
	log.Printf("[DEBUG] PostComment called with prID: '%s', FilePath: '%s', Line: %d", prID, comment.FilePath, comment.Line)
	// prID format: owner/repo/number
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid GitHub PR ID format: expected 'owner/repo/number', got '%s'", prID)
	}
	owner := parts[0]
	repo := parts[1]
	number := parts[2]

	// If this is a line comment (has FilePath and Line), use the pull request review comments API
	if comment.FilePath != "" && comment.Line > 0 {
		return p.postLineComment(ctx, owner, repo, number, comment)
	}

	// Otherwise, post as a general PR comment (issue comment)
	return p.postGeneralComment(ctx, owner, repo, number, comment)
}

func (p *GitHubProvider) postGeneralComment(ctx context.Context, owner, repo, number string, comment *models.ReviewComment) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, number)
	payload := map[string]string{"body": comment.Content}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		return fmt.Errorf("GitHub general comment failed: %s", resp.Status)
	}

	log.Printf("[DEBUG] Successfully posted general comment")
	return nil
}

func (p *GitHubProvider) postLineComment(ctx context.Context, owner, repo, number string, comment *models.ReviewComment) error {
	// First get the PR details to get the head commit SHA
	prDetailsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s", owner, repo, number)
	req, _ := http.NewRequestWithContext(ctx, "GET", prDetailsURL, nil)
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get PR details for line comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub PR details failed for line comment: %s", resp.Status)
	}

	var pr struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return fmt.Errorf("failed to decode PR details: %w", err)
	}

	// Now create the line comment using the pull request comments API
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/comments", owner, repo, number)

	payload := map[string]interface{}{
		"body":      comment.Content,
		"commit_id": pr.Head.SHA,
		"path":      comment.FilePath,
		"line":      comment.Line,
	}

	// Determine side based on IsDeletedLine field
	if comment.IsDeletedLine {
		payload["side"] = "LEFT" // Comment on the old version (deleted line)
	} else {
		payload["side"] = "RIGHT" // Comment on the new version (added line)
	}

	data, _ := json.Marshal(payload)
	req, _ = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "token "+p.PAT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post line comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		// Read response body for debugging
		var responseBody bytes.Buffer
		responseBody.ReadFrom(resp.Body)
		log.Printf("[DEBUG] Line comment failed. Status: %s, Response: %s", resp.Status, responseBody.String())
		return fmt.Errorf("GitHub line comment failed: %s", resp.Status)
	}

	log.Printf("[DEBUG] Successfully posted line comment on %s:%d", comment.FilePath, comment.Line)
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
			Login     string `json:"login"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
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
	authorName := pr.User.Name
	if authorName == "" {
		authorName = pr.User.Login
	}

	return &providers.MergeRequestDetails{
		ID:             fmt.Sprintf("%d", pr.Number),
		Title:          pr.Title,
		Description:    pr.Body,
		Author:         pr.User.Login,
		AuthorName:     authorName,
		AuthorUsername: pr.User.Login,
		AuthorAvatar:   pr.User.AvatarURL,
		CreatedAt:      pr.CreatedAt,
		URL:            mrURL,
		State:          pr.State,
		WebURL:         pr.HTMLURL,
		SourceBranch:   pr.Head.Ref,
		TargetBranch:   pr.Base.Ref,
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
		SHA       string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	log.Printf("[DEBUG] GitHub API returned %d files", len(files))

	var diffs []*models.CodeDiff
	for _, f := range files {
		log.Printf("[DEBUG] Processing file: %s, status: %s, patch length: %d", f.Filename, f.Status, len(f.Patch))

		// Parse the patch into hunks
		hunks := p.parsePatchIntoHunks(f.Patch)

		diff := &models.CodeDiff{
			FilePath:  f.Filename,
			CommitID:  f.SHA,
			FileType:  p.getFileType(f.Filename),
			IsNew:     f.Status == "added",
			IsDeleted: f.Status == "removed",
			IsRenamed: f.Status == "renamed",
			Hunks:     hunks,
		}

		log.Printf("[DEBUG] Created CodeDiff for %s with %d hunks", f.Filename, len(hunks))
		diffs = append(diffs, diff)
	}

	log.Printf("[DEBUG] Returning %d diffs with actual content", len(diffs))
	return diffs, nil
}

// parsePatchIntoHunks parses a GitHub patch string into DiffHunk objects
func (p *GitHubProvider) parsePatchIntoHunks(patch string) []models.DiffHunk {
	if patch == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	var hunks []models.DiffHunk
	var currentHunk *models.DiffHunk
	var hunkContent strings.Builder

	// Regex to match hunk headers like @@ -1,3 +1,4 @@
	hunkHeaderRegex := regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

	for _, line := range lines {
		if match := hunkHeaderRegex.FindStringSubmatch(line); match != nil {
			// Save previous hunk if exists
			if currentHunk != nil {
				currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
				hunks = append(hunks, *currentHunk)
				hunkContent.Reset()
			}

			// Parse hunk header
			oldStart, _ := strconv.Atoi(match[1])
			oldCount := 1
			if match[2] != "" {
				oldCount, _ = strconv.Atoi(match[2])
			}
			newStart, _ := strconv.Atoi(match[3])
			newCount := 1
			if match[4] != "" {
				newCount, _ = strconv.Atoi(match[4])
			}

			currentHunk = &models.DiffHunk{
				OldStartLine: oldStart,
				OldLineCount: oldCount,
				NewStartLine: newStart,
				NewLineCount: newCount,
			}

			// Include the header line in the content
			hunkContent.WriteString(line + "\n")
		} else if currentHunk != nil {
			// Add content lines to current hunk
			hunkContent.WriteString(line + "\n")
		}
	}

	// Save the last hunk
	if currentHunk != nil {
		currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// getFileType determines file type based on extension
func (p *GitHubProvider) getFileType(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "unknown"
}
