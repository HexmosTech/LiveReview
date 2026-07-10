package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/aisanitize"
	"github.com/livereview/internal/providers"
	networkazuredevops "github.com/livereview/network/providers/azuredevops"
	"github.com/livereview/pkg/models"
)

// Config holds configuration for the Azure DevOps provider.
type Config struct {
	BaseURL string `koanf:"base_url"` // organization URL, e.g. https://dev.azure.com/myorg
	Token   string `koanf:"token"`
}

// Provider implements the providers.Provider interface for Azure DevOps.
type Provider struct {
	baseURL    string // organization URL, may be empty until Configure is called
	token      string
	httpClient *http.Client
}

// NewProvider creates a Provider with the supplied configuration.
func NewProvider(cfg Config) (*Provider, error) {
	token := cfg.Token
	if pt := decodePackedToken(token); pt.pat != "" {
		token = pt.pat
	}
	return &Provider{
		baseURL:    NormalizeOrgURL(cfg.BaseURL),
		token:      token,
		httpClient: networkazuredevops.NewHTTPClient(30 * time.Second),
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "azuredevops"
}

// Configure applies dynamic configuration from the factory.
func (p *Provider) Configure(config map[string]interface{}) error {
	base := p.baseURL
	if v, ok := config["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		base = v
	}
	if v, ok := config["url"].(string); ok && strings.TrimSpace(v) != "" {
		base = v
	}

	token := p.token
	if v, ok := config["pat_token"].(string); ok && strings.TrimSpace(v) != "" {
		token = v
	}
	if v, ok := config["token"].(string); ok && strings.TrimSpace(v) != "" {
		token = v
	}
	if pt := decodePackedToken(token); pt.pat != "" {
		token = pt.pat
	}

	base = NormalizeOrgURL(base)
	if base == "" {
		return fmt.Errorf("base_url is required for Azure DevOps provider")
	}
	if token == "" {
		return fmt.Errorf("token is required for Azure DevOps provider")
	}

	p.baseURL = base
	p.token = token
	if p.httpClient == nil {
		p.httpClient = networkazuredevops.NewHTTPClient(30 * time.Second)
	}
	return nil
}

func newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	return networkazuredevops.NewRequestWithContext(ctx, method, url, nil)
}

func (p *Provider) do(req *http.Request) (*http.Response, error) {
	return networkazuredevops.Do(p.httpClient, req)
}

func (p *Provider) applyAuth(req *http.Request) {
	networkazuredevops.ApplyPATAuth(req, p.token)
}

// GetMergeRequestDetails fetches PR details for an Azure DevOps pull request URL,
// e.g. https://dev.azure.com/{org}/{project}/_git/{repo}/pullrequest/{id}.
func (p *Provider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	org, project, repo, id, err := parsePullRequestURL(mrURL)
	if err != nil {
		return nil, err
	}

	apiBase := firstNonEmpty(p.baseURL, orgAPIBase(org))

	pr, err := p.fetchPullRequest(ctx, apiBase, project, repo, id)
	if err != nil {
		return nil, err
	}

	mrID := fmt.Sprintf("%s/%s/%s/%d", org, project, repo, id)

	return &providers.MergeRequestDetails{
		ID:             mrID,
		Title:          pr.Title,
		Description:    pr.Description,
		SourceBranch:   strings.TrimPrefix(pr.SourceRefName, "refs/heads/"),
		TargetBranch:   strings.TrimPrefix(pr.TargetRefName, "refs/heads/"),
		Author:         pr.CreatedBy.UniqueName,
		AuthorName:     firstNonEmpty(pr.CreatedBy.DisplayName, pr.CreatedBy.UniqueName),
		AuthorUsername: pr.CreatedBy.UniqueName,
		AuthorAvatar:   pr.CreatedBy.ImageURL,
		CreatedAt:      pr.CreationDate,
		URL:            mrURL,
		State:          pr.Status,
		MergeStatus:    pr.MergeStatus,
		DiffRefs: providers.DiffRefs{
			BaseSHA:  pr.LastMergeTargetCommit.CommitID,
			HeadSHA:  pr.LastMergeSourceCommit.CommitID,
			StartSHA: pr.LastMergeCommit.CommitID,
		},
		WebURL:        mrURL,
		ProviderType:  "azuredevops",
		RepositoryURL: fmt.Sprintf("%s/%s/_git/%s", apiBase, neturl.PathEscape(project), neturl.PathEscape(repo)),
	}, nil
}

func (p *Provider) fetchPullRequest(ctx context.Context, apiBase, project, repo string, id int) (*pullRequest, error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("azure devops pull request fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var pr pullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode azure devops response: %w", err)
	}
	return &pr, nil
}

// GetMergeRequestChanges retrieves diffs for a PR. mrID format: org/project/repo/id.
func (p *Provider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	org, project, repo, id, err := splitMergeRequestID(mrID)
	if err != nil {
		return nil, err
	}
	apiBase := firstNonEmpty(p.baseURL, orgAPIBase(org))

	iterationID, err := p.fetchLatestIterationID(ctx, apiBase, project, repo, id)
	if err != nil {
		return nil, err
	}

	entries, err := p.fetchIterationChanges(ctx, apiBase, project, repo, id, iterationID)
	if err != nil {
		return nil, err
	}

	var diffs []*models.CodeDiff
	for _, entry := range entries {
		if entry.Item.IsFolder {
			continue
		}

		oldContent, err := p.fetchBlob(ctx, apiBase, project, repo, entry.Item.OriginalObjectID)
		if err != nil {
			return nil, err
		}
		newContent, err := p.fetchBlob(ctx, apiBase, project, repo, entry.Item.ObjectID)
		if err != nil {
			return nil, err
		}

		diffs = append(diffs, buildCodeDiff(entry, oldContent, newContent))
	}

	return diffs, nil
}

func (p *Provider) fetchLatestIterationID(ctx context.Context, apiBase, project, repo string, id int) (int, error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d/iterations?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return 0, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch iterations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return 0, fmt.Errorf("azure devops iterations fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var out iterationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("failed to decode iterations response: %w", err)
	}
	if len(out.Value) == 0 {
		return 0, fmt.Errorf("pull request has no iterations")
	}

	latest := out.Value[0].ID
	for _, it := range out.Value {
		if it.ID > latest {
			latest = it.ID
		}
	}
	return latest, nil
}

func (p *Provider) fetchIterationChanges(ctx context.Context, apiBase, project, repo string, id, iterationID int) ([]changeEntry, error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d/iterations/%d/changes?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, iterationID, apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch iteration changes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("azure devops iteration changes fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var out changesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode iteration changes response: %w", err)
	}
	return out.ChangeEntries, nil
}

// formatAzureDevOpsComment creates a consistently formatted comment body
// with severity information and suggestions properly formatted.
func formatAzureDevOpsComment(ctx context.Context, comment *models.ReviewComment) string {
	safeContent, _ := aisanitize.SanitizationPostflight(ctx, comment.Content)

	safeSuggestions := make([]string, 0, len(comment.Suggestions))
	for _, suggestion := range comment.Suggestions {
		safeSuggestion, _ := aisanitize.SanitizationPostflight(ctx, suggestion)
		safeSuggestions = append(safeSuggestions, safeSuggestion)
	}

	formattedComment := safeContent
	if comment.Severity != "" {
		formattedComment = fmt.Sprintf("**Severity: %s**\n\n%s", comment.Severity, formattedComment)
	}

	if len(safeSuggestions) > 0 {
		formattedComment += "\n\n**Suggestions:**\n"
		for i, suggestion := range safeSuggestions {
			formattedComment += fmt.Sprintf("%d. %s\n", i+1, suggestion)
		}
	}

	return formattedComment
}

// PostComment posts a comment on a PR as a new thread. Supports inline
// (file/line) comments via threadContext and general (PR-level) comments.
func (p *Provider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	if comment == nil {
		return fmt.Errorf("comment is required")
	}

	org, project, repo, id, err := splitMergeRequestID(mrID)
	if err != nil {
		return err
	}
	apiBase := firstNonEmpty(p.baseURL, orgAPIBase(org))

	formattedContent := formatAzureDevOpsComment(ctx, comment)

	payload := map[string]interface{}{
		"status": 1, // active
		"comments": []map[string]interface{}{
			{
				"parentCommentId": 0,
				"content":         formattedContent,
				"commentType":     1, // text
			},
		},
	}

	if comment.FilePath != "" && comment.Line > 0 {
		path := comment.FilePath
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		lineRange := map[string]interface{}{"line": comment.Line, "offset": 1}
		threadContext := map[string]interface{}{"filePath": path}
		if comment.IsDeletedLine {
			threadContext["leftFileStart"] = lineRange
			threadContext["leftFileEnd"] = lineRange
		} else {
			threadContext["rightFileStart"] = lineRange
			threadContext["rightFileEnd"] = lineRange
		}
		payload["threadContext"] = threadContext
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode comment payload: %w", err)
	}

	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, apiVersion)

	req, err := networkazuredevops.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.do(req)
	if err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("azure devops thread creation failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// PostComments posts multiple comments sequentially.
func (p *Provider) PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error {
	for _, c := range comments {
		if err := p.PostComment(ctx, mrID, c); err != nil {
			return err
		}
	}
	return nil
}

// parsePullRequestURL parses an Azure DevOps PR URL of the form
// https://dev.azure.com/{org}/{project}/_git/{repo}/pullrequest/{id}.
func parsePullRequestURL(mrURL string) (org, project, repo string, id int, err error) {
	parsed, perr := neturl.Parse(mrURL)
	if perr != nil {
		return "", "", "", 0, fmt.Errorf("invalid Azure DevOps PR URL: %w", perr)
	}

	rawSegments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	segments := make([]string, len(rawSegments))
	for i, s := range rawSegments {
		if unescaped, uerr := neturl.PathUnescape(s); uerr == nil {
			segments[i] = unescaped
		} else {
			segments[i] = s
		}
	}

	if len(segments) < 6 {
		return "", "", "", 0, fmt.Errorf("invalid Azure DevOps PR URL: expected /{org}/{project}/_git/{repo}/pullrequest/{id}")
	}

	marker := strings.ToLower(segments[len(segments)-2])
	gitMarker := strings.ToLower(segments[len(segments)-4])
	if marker != "pullrequest" || gitMarker != "_git" {
		return "", "", "", 0, fmt.Errorf("invalid Azure DevOps PR URL: unexpected path shape, got '%s'", parsed.Path)
	}

	prID, convErr := parsePullRequestID(segments[len(segments)-1])
	if convErr != nil {
		return "", "", "", 0, convErr
	}

	org = segments[0]
	project = segments[1]
	repo = segments[len(segments)-3]
	return org, project, repo, prID, nil
}

// splitMergeRequestID splits a composite mrID of the form org/project/repo/id.
func splitMergeRequestID(mrID string) (org, project, repo string, id int, err error) {
	parts := strings.SplitN(mrID, "/", 4)
	if len(parts) != 4 {
		return "", "", "", 0, fmt.Errorf("invalid Azure DevOps PR ID format: expected 'org/project/repo/id', got '%s'", mrID)
	}
	prID, convErr := strconv.Atoi(parts[3])
	if convErr != nil {
		return "", "", "", 0, fmt.Errorf("invalid Azure DevOps PR ID format: %w", convErr)
	}
	return parts[0], parts[1], parts[2], prID, nil
}
