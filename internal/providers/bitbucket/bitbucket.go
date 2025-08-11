package bitbucket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

type BitbucketProvider struct {
	APIToken string
	Email    string
}

func NewBitbucketProvider(apiToken, email string) *BitbucketProvider {
	log.Printf("[DEBUG] NewBitbucketProvider called with token length: %d, email: %s", len(apiToken), email)
	return &BitbucketProvider{
		APIToken: apiToken,
		Email:    email,
	}
}

func (p *BitbucketProvider) Name() string {
	return "bitbucket"
}

func (p *BitbucketProvider) Configure(config map[string]interface{}) error {
	log.Printf("[DEBUG] BitbucketProvider.Configure called with config keys: %v", getKeys(config))

	if token, ok := config["pat_token"].(string); ok {
		log.Printf("[DEBUG] Setting API token, length: %d", len(token))
		p.APIToken = token
	}

	if email, ok := config["email"].(string); ok {
		log.Printf("[DEBUG] Setting email: %s", email)
		p.Email = email
	}

	if p.APIToken == "" {
		return fmt.Errorf("pat_token missing in config")
	}

	return nil
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (p *BitbucketProvider) GetMergeRequestDetails(ctx context.Context, prURL string) (*providers.MergeRequestDetails, error) {
	log.Printf("[DEBUG] BitbucketProvider.GetMergeRequestDetails called with URL: %s", prURL)

	// Parse Bitbucket PR URL: https://bitbucket.org/workspace/repository/pull-requests/123
	parsed, err := url.Parse(prURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Bitbucket PR URL: %w", err)
	}

	parts := strings.Split(parsed.Path, "/")
	if len(parts) < 5 || parts[3] != "pull-requests" {
		return nil, fmt.Errorf("invalid Bitbucket PR URL: expected /workspace/repo/pull-requests/number, got %s", prURL)
	}

	workspace := parts[1]
	repo := parts[2]
	prNumber := parts[4]

	// Bitbucket API v2.0 endpoint for pull request details
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s", workspace, repo, prNumber)

	log.Printf("[DEBUG] BitbucketProvider: API URL: %s", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(p.Email + ":" + p.APIToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bitbucket PR details failed: %s, response: %s", resp.Status, string(body))
	}

	var pr struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		Author      struct {
			DisplayName string `json:"display_name"`
			Username    string `json:"username"`
		} `json:"author"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"destination"`
		CreatedOn string `json:"created_on"`
		Links     struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
		Repository struct {
			FullName string `json:"full_name"`
			Links    struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"repository"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode PR response: %w", err)
	}

	return &providers.MergeRequestDetails{
		ID:           fmt.Sprintf("%d", pr.ID),
		Title:        pr.Title,
		Description:  pr.Description,
		Author:       pr.Author.Username,
		CreatedAt:    pr.CreatedOn,
		URL:          prURL,
		State:        pr.State,
		WebURL:       pr.Links.HTML.Href,
		SourceBranch: pr.Source.Branch.Name,
		TargetBranch: pr.Destination.Branch.Name,
		DiffRefs: providers.DiffRefs{
			BaseSHA: pr.Destination.Commit.Hash,
			HeadSHA: pr.Source.Commit.Hash,
		},
		ProviderType:  "bitbucket",
		RepositoryURL: pr.Repository.Links.HTML.Href,
	}, nil
}

func (p *BitbucketProvider) GetMergeRequestChanges(ctx context.Context, prID string) ([]*models.CodeDiff, error) {
	log.Printf("[DEBUG] BitbucketProvider.GetMergeRequestChanges called with prID: %s", prID)

	// Parse prID which should be in format "workspace/repo/prNumber"
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid Bitbucket PR ID format: expected 'workspace/repo/number', got '%s'", prID)
	}

	workspace := parts[0]
	repo := parts[1]
	prNumber := parts[2]

	// Bitbucket API v2.0 endpoint for pull request diff
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/diff", workspace, repo, prNumber)

	log.Printf("[DEBUG] BitbucketProvider: Diff API URL: %s", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(p.Email + ":" + p.APIToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bitbucket PR diff failed: %s, response: %s", resp.Status, string(body))
	}

	// Read the diff content
	diffContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read diff response: %w", err)
	}

	log.Printf("[DEBUG] BitbucketProvider: Received diff content, length: %d bytes", len(diffContent))
	if len(diffContent) > 500 {
		log.Printf("[DEBUG] BitbucketProvider: Diff content preview (first 500 chars): %s", string(diffContent[:500]))
	} else {
		log.Printf("[DEBUG] BitbucketProvider: Diff content preview: %s", string(diffContent))
	}

	// Parse the diff content into CodeDiff objects
	diffs, err := p.parseDiffContent(string(diffContent))
	if err != nil {
		log.Printf("[DEBUG] BitbucketProvider: Failed to parse diff content: %v", err)
		return nil, fmt.Errorf("failed to parse diff content: %w", err)
	}

	log.Printf("[DEBUG] BitbucketProvider: Successfully parsed %d files from diff", len(diffs))
	return diffs, nil
}

func (p *BitbucketProvider) PostComment(ctx context.Context, prID string, comment *models.ReviewComment) error {
	log.Printf("[DEBUG] BitbucketProvider.PostComment called with prID: %s, comment: %s", prID, comment.Content)

	// Parse prID which should be in format "workspace/repo/prNumber"
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid Bitbucket PR ID format: expected 'workspace/repo/number', got '%s'", prID)
	}

	workspace := parts[0]
	repo := parts[1]
	prNumber := parts[2]

	// For now, post as a general PR comment
	// Bitbucket API v2.0 endpoint for pull request comments
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments", workspace, repo, prNumber)

	payload := map[string]interface{}{
		"content": map[string]string{
			"raw": comment.Content,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal comment payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(p.Email + ":" + p.APIToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Bitbucket comment post failed: %s, response: %s", resp.Status, string(body))
	}

	log.Printf("[DEBUG] Successfully posted comment to Bitbucket PR")
	return nil
}

func (p *BitbucketProvider) PostComments(ctx context.Context, prID string, comments []*models.ReviewComment) error {
	log.Printf("[DEBUG] BitbucketProvider.PostComments called with prID: %s, %d comments", prID, len(comments))

	for _, comment := range comments {
		err := p.PostComment(ctx, prID, comment)
		if err != nil {
			return fmt.Errorf("failed to post comment: %w", err)
		}
	}

	return nil
}

// parseDiffContent parses unified diff content into CodeDiff objects
func (p *BitbucketProvider) parseDiffContent(diffContent string) ([]*models.CodeDiff, error) {
	// Split the unified diff by files (look for "diff --git" headers)
	files := p.splitDiffByFiles(diffContent)

	var diffs []*models.CodeDiff
	for fileName, filePatch := range files {
		log.Printf("[DEBUG] BitbucketProvider: Processing file: %s, patch length: %d", fileName, len(filePatch))

		// Parse the patch into hunks using GitHub's algorithm
		hunks := p.parsePatchIntoHunks(filePatch)

		// Determine file status and type
		status := p.getFileStatus(filePatch)
		fileType := p.getFileType(fileName)

		diff := &models.CodeDiff{
			FilePath:  fileName,
			CommitID:  "", // Bitbucket doesn't provide individual file SHAs in diff
			FileType:  fileType,
			IsNew:     status == "added",
			IsDeleted: status == "removed",
			IsRenamed: status == "renamed",
			Hunks:     hunks,
		}

		log.Printf("[DEBUG] BitbucketProvider: Created CodeDiff for %s with %d hunks", fileName, len(hunks))
		diffs = append(diffs, diff)
	}

	return diffs, nil
}

// splitDiffByFiles splits a unified diff into individual file patches
func (p *BitbucketProvider) splitDiffByFiles(diffContent string) map[string]string {
	files := make(map[string]string)
	lines := strings.Split(diffContent, "\n")

	var currentFile string
	var currentPatch strings.Builder

	// Regex to match diff headers like "diff --git a/file.py b/file.py"
	diffHeaderRegex := regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)

	for _, line := range lines {
		if match := diffHeaderRegex.FindStringSubmatch(line); match != nil {
			// Save previous file if exists
			if currentFile != "" {
				files[currentFile] = currentPatch.String()
				currentPatch.Reset()
			}

			// Start new file (use the target file name)
			currentFile = match[2]
			currentPatch.WriteString(line + "\n")
		} else if currentFile != "" {
			// Continue building current file's patch
			currentPatch.WriteString(line + "\n")
		}
	}

	// Save the last file
	if currentFile != "" {
		files[currentFile] = currentPatch.String()
	}

	return files
}

// parsePatchIntoHunks parses a patch string into DiffHunk objects (adapted from GitHub provider)
func (p *BitbucketProvider) parsePatchIntoHunks(patch string) []models.DiffHunk {
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

			// Create new hunk
			currentHunk = &models.DiffHunk{
				OldStartLine: oldStart,
				OldLineCount: oldCount,
				NewStartLine: newStart,
				NewLineCount: newCount,
			}

			// Add the hunk header to content
			hunkContent.WriteString(line + "\n")
		} else if currentHunk != nil {
			// Add line to current hunk content
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

// getFileStatus determines the file status from the patch
func (p *BitbucketProvider) getFileStatus(patch string) string {
	lines := strings.Split(patch, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "new file mode") {
			return "added"
		} else if strings.HasPrefix(line, "deleted file mode") {
			return "removed"
		} else if strings.Contains(line, "rename from") || strings.Contains(line, "rename to") {
			return "renamed"
		}
	}
	return "modified"
}

// getFileType determines the file type based on extension
func (p *BitbucketProvider) getFileType(filename string) string {
	if strings.Contains(filename, ".") {
		parts := strings.Split(filename, ".")
		return parts[len(parts)-1]
	}
	return ""
}
