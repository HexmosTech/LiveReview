package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/providers"
	networkbitbucket "github.com/livereview/network/providers/bitbucket"
	"github.com/livereview/pkg/models"
	"golang.org/x/time/rate"
)

type BitbucketCommit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Author  struct {
		Raw  string         `json:"raw"`
		User *BitbucketUser `json:"user"`
	} `json:"author"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type BitbucketComment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User      *BitbucketUser `json:"user"`
	CreatedOn string         `json:"created_on"`
	UpdatedOn string         `json:"updated_on"`
	Parent    *struct {
		ID int `json:"id"`
	} `json:"parent"`
	Inline *struct {
		Path string `json:"path"`
		From *int   `json:"from"`
		To   *int   `json:"to"`
	} `json:"inline"`
	Deleted bool   `json:"deleted"`
	Type    string `json:"type"`
}

type BitbucketUser struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
	Links       struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type BitbucketProvider struct {
	token       string
	email       string
	repoURL     string // Add this field
	workspace   string
	repoSlug    string
	httpClient  *http.Client
	RateLimiter *rate.Limiter
}

// NewBitbucketProvider creates a new Bitbucket provider
func NewBitbucketProvider(token, email, repoURL string) (*BitbucketProvider, error) {
	log.Printf("[DEBUG] NewBitbucketProvider called with token length: %d, email: %s", len(token), email)

	workspace, repoSlug, _, err := ParseBitbucketURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository URL: %w", err)
	}

	return &BitbucketProvider{
		token:       token,
		email:       email,
		repoURL:     repoURL,
		workspace:   workspace,
		repoSlug:    repoSlug,
		httpClient:  networkbitbucket.NewHTTPClient(10 * time.Second),
		RateLimiter: rate.NewLimiter(rate.Every(1*time.Second), 5), // 5 requests per second
	}, nil
}

var mrIDRegex = regexp.MustCompile(`^(?:https?://[^/]+/)?([^/]+)/([^/]+)/(?:pull-requests/)?(\d+)(?:/.*)?$`)

// extractMRIDComponents explicitly parses an mrID/prID using a regex
// to reliably extract the workspace, repository, and pull request number.
func extractMRIDComponents(id string) (workspace, repo, prNum string, err error) {
	id = strings.TrimSpace(id)
	matches := mrIDRegex.FindStringSubmatch(id)
	if len(matches) != 4 {
		return "", "", "", fmt.Errorf("invalid Bitbucket PR identifier format: expected 'workspace/repo/number' or a valid URL, got '%s'", id)
	}
	return matches[1], matches[2], matches[3], nil
}

func ParseBitbucketURL(urlStr string) (string, string, string, error) {
	if urlStr == "" {
		return "", "", "", fmt.Errorf("repository URL is empty")
	}
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 2 {
		return "", "", "", fmt.Errorf("invalid Bitbucket URL format: %s", urlStr)
	}

	workspace := pathParts[0]
	repoSlug := pathParts[1]
	prID := ""
	if len(pathParts) > 3 && pathParts[2] == "pull-requests" {
		prID = pathParts[3]
	}

	return workspace, repoSlug, prID, nil
}

// GetMergeRequestDetails fetches the details of a pull request.
func (p *BitbucketProvider) GetMergeRequestDetails(ctx context.Context, prURL string) (*providers.MergeRequestDetails, error) {
	log.Printf("[DEBUG] BitbucketProvider.GetMergeRequestDetails called with URL: %s", prURL)

	_, _, prID, err := ParseBitbucketURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PR URL: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s", p.workspace, p.repoSlug, prID)
	log.Printf("[DEBUG] BitbucketProvider: API URL: %s", apiURL)

	req, err := networkbitbucket.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	req.SetBasicAuth(p.email, p.token)
	req.Header.Set("Accept", "application/json")

	resp, err := networkbitbucket.Do(p.httpClient, req)
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
		ID:             fmt.Sprintf("%d", pr.ID),
		Title:          pr.Title,
		Description:    pr.Description,
		Author:         pr.Author.Username,
		AuthorName:     pr.Author.DisplayName,
		AuthorUsername: pr.Author.Username,
		CreatedAt:      pr.CreatedOn,
		URL:            p.repoURL,
		State:          pr.State,
		WebURL:         pr.Links.HTML.Href,
		SourceBranch:   pr.Source.Branch.Name,
		TargetBranch:   pr.Destination.Branch.Name,
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

	// Use regex-based parser for robustness
	workspace, repo, prNumber, err := extractMRIDComponents(prID)
	if err != nil {
		return nil, err
	}

	// Bitbucket API v2.0 endpoint for pull request diff
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/diff", workspace, repo, prNumber)

	log.Printf("[DEBUG] BitbucketProvider: Diff API URL: %s", apiURL)

	req, err := networkbitbucket.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	req.SetBasicAuth(p.email, p.token)
	req.Header.Set("Accept", "text/plain")

	resp, err := networkbitbucket.Do(http.DefaultClient, req)
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

func (p *BitbucketProvider) GetMergeRequestChangesAsText(ctx context.Context, prID string) (string, error) {
	log.Printf("[DEBUG] BitbucketProvider.GetMergeRequestChangesAsText called with prID: %s", prID)

	// Use regex-based parser for robustness
	workspace, repo, prNumber, err := extractMRIDComponents(prID)
	if err != nil {
		return "", err
	}

	// Bitbucket API v2.0 endpoint for pull request diff
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/diff", workspace, repo, prNumber)

	log.Printf("[DEBUG] BitbucketProvider: Diff API URL: %s", apiURL)

	req, err := networkbitbucket.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	req.SetBasicAuth(p.email, p.token)
	req.Header.Set("Accept", "text/plain")

	resp, err := networkbitbucket.Do(http.DefaultClient, req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Bitbucket PR diff failed: %s, response: %s", resp.Status, string(body))
	}

	// Read the diff content
	diffContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read diff response: %w", err)
	}

	return string(diffContent), nil
}

func (p *BitbucketProvider) GetPullRequestCommits(ctx context.Context, prID string) ([]BitbucketCommit, error) {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/commits", p.workspace, p.repoSlug, prID)
	body, err := p.doRequest(ctx, apiURL, "GET", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR commits: %w", err)
	}

	// Bitbucket API returns paginated responses with a "values" array
	var response struct {
		Values []BitbucketCommit `json:"values"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal commits: %w", err)
	}

	return response.Values, nil
}

func (p *BitbucketProvider) GetPullRequestComments(ctx context.Context, prID string) ([]BitbucketComment, error) {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments?sort=-created_on", p.workspace, p.repoSlug, prID)
	body, err := p.doRequest(ctx, apiURL, "GET", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments: %w", err)
	}

	// Bitbucket API returns paginated responses with a "values" array
	var response struct {
		Values []BitbucketComment `json:"values"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal comments: %w", err)
	}

	return response.Values, nil
}

func (p *BitbucketProvider) GetPullRequestDiff(ctx context.Context, prID string) (string, error) {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/diff", p.workspace, p.repoSlug, prID)
	body, err := p.doRequest(ctx, apiURL, "GET", nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff: %w", err)
	}

	return string(body), nil
}

func (p *BitbucketProvider) doRequest(ctx context.Context, apiURL, method string, payload interface{}) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := networkbitbucket.NewRequestWithContext(ctx, method, apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	req.SetBasicAuth(p.email, p.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := networkbitbucket.Do(p.httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %s: %s", resp.Status, string(body))
	}

	return io.ReadAll(resp.Body)
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

func (p *BitbucketProvider) Name() string {
	return "bitbucket"
}

func (p *BitbucketProvider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	log.Printf("[DEBUG] BitbucketProvider.PostComment called with mrID: '%s', FilePath: '%s', Line: %d", mrID, comment.FilePath, comment.Line)

	workspace, repo, prNumber, err := extractMRIDComponents(mrID)
	if err != nil {
		return err
	}

	if comment.FilePath != "" && comment.Line > 0 {
		return p.postLineComment(ctx, workspace, repo, prNumber, comment)
	}
	return p.postGeneralComment(ctx, workspace, repo, prNumber, comment)
}

// formatBitbucketComment formats a comment for Bitbucket, including severity and suggestions.
func formatBitbucketComment(comment *models.ReviewComment) string {
	body := comment.Content
	if comment.Severity != "" {
		body = fmt.Sprintf("**Severity: %s**\n\n%s", comment.Severity, body)
	}
	if len(comment.Suggestions) > 0 {
		body += "\n\n**Suggestions:**\n"
		for i, s := range comment.Suggestions {
			body += fmt.Sprintf("%d. %s\n", i+1, s)
		}
	}
	return body
}

func (p *BitbucketProvider) postGeneralComment(ctx context.Context, workspace, repo, prNumber string, comment *models.ReviewComment) error {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments", workspace, repo, prNumber)

	payload := map[string]interface{}{
		"content": map[string]string{
			"raw": formatBitbucketComment(comment),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal comment payload: %w", err)
	}

	resp, err := networkbitbucket.PostCommentAPI(ctx, p.httpClient, apiURL, p.email, p.token, data)
	if err != nil {
		return fmt.Errorf("failed to post general comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Bitbucket general comment failed: %s, response: %s", resp.Status, string(body))
	}

	log.Printf("[DEBUG] BitbucketProvider: Successfully posted general comment on PR %s/%s/%s", workspace, repo, prNumber)
	return nil
}

func (p *BitbucketProvider) postLineComment(ctx context.Context, workspace, repo, prNumber string, comment *models.ReviewComment) error {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments", workspace, repo, prNumber)

	// Bitbucket inline comments use the "inline" object with "path" and "to" (new line)
	// or "from" (old line) for deleted lines.
	inlinePayload := map[string]interface{}{
		"path": comment.FilePath,
		"to":   comment.Line,
	}
	if comment.IsDeletedLine {
		// For deleted lines, use "from" instead of "to"
		inlinePayload = map[string]interface{}{
			"path": comment.FilePath,
			"from": comment.Line,
		}
	}

	payload := map[string]interface{}{
		"content": map[string]string{
			"raw": formatBitbucketComment(comment),
		},
		"inline": inlinePayload,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal inline comment payload: %w", err)
	}

	resp, err := networkbitbucket.PostCommentAPI(ctx, p.httpClient, apiURL, p.email, p.token, data)
	if err != nil {
		return fmt.Errorf("failed to post inline comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		// If the line is not part of the diff, Bitbucket returns 400/422.
		// Fall back to a general comment rather than failing the whole review.
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
			log.Printf("[WARN] BitbucketProvider: Inline comment rejected for %s:%d (%s) — falling back to general comment. Response: %s",
				comment.FilePath, comment.Line, resp.Status, string(body))
			return p.postGeneralComment(ctx, workspace, repo, prNumber, comment)
		}
		return fmt.Errorf("Bitbucket inline comment failed: %s, response: %s", resp.Status, string(body))
	}

	log.Printf("[DEBUG] BitbucketProvider: Successfully posted inline comment on %s:%d", comment.FilePath, comment.Line)
	return nil
}

func (p *BitbucketProvider) PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error {
	for _, comment := range comments {
		if err := p.PostComment(ctx, mrID, comment); err != nil {
			return err
		}
	}
	return nil
}

func (p *BitbucketProvider) Configure(config map[string]interface{}) error {
	// In this new design, the configuration (repoURL) is passed during initialization.
	// This function can be used for any additional, dynamic configuration if needed.
	// For now, it can remain empty or log the configuration attempt.
	log.Printf("[DEBUG] BitbucketProvider.Configure called with config: %v", config)
	return nil
}
