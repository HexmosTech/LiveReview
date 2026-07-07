package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	networkazuredevops "github.com/livereview/network/providers/azuredevops"
)

// Thread mirrors the subset of an Azure DevOps pull request comment thread
// needed to route replies and recover inline position context.
type Thread struct {
	ID            int             `json:"id"`
	Status        string          `json:"status"`
	Comments      []ThreadComment `json:"comments"`
	ThreadContext *ThreadContext  `json:"threadContext"`
}

// ThreadComment mirrors a single comment within a thread.
type ThreadComment struct {
	ID              int64    `json:"id"`
	ParentCommentID int64    `json:"parentCommentId"`
	Content         string   `json:"content"`
	CommentType     string   `json:"commentType"`
	Author          identity `json:"author"`
	PublishedDate   string   `json:"publishedDate"`
	LastUpdatedDate string   `json:"lastUpdatedDate"`
}

// ThreadContext carries the inline file/line anchor for a thread, when present.
type ThreadContext struct {
	FilePath       string   `json:"filePath"`
	RightFileStart *LinePos `json:"rightFileStart,omitempty"`
	RightFileEnd   *LinePos `json:"rightFileEnd,omitempty"`
	LeftFileStart  *LinePos `json:"leftFileStart,omitempty"`
	LeftFileEnd    *LinePos `json:"leftFileEnd,omitempty"`
}

// LinePos is a (line, offset) position within a file, as used by threadContext.
type LinePos struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

// GetThread fetches a single comment thread (with its comments and threadContext)
// for the pull request identified by mrID ("org/project/repo/id").
func (p *Provider) GetThread(ctx context.Context, mrID string, threadID int) (*Thread, error) {
	org, project, repo, id, err := splitMergeRequestID(mrID)
	if err != nil {
		return nil, err
	}
	apiBase := firstNonEmpty(p.baseURL, orgAPIBase(org))

	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, threadID, apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch thread: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("azure devops thread fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var thread Thread
	if err := json.NewDecoder(resp.Body).Decode(&thread); err != nil {
		return nil, fmt.Errorf("failed to decode thread response: %w", err)
	}
	return &thread, nil
}

// PostThreadReply appends a reply comment to an existing thread.
func (p *Provider) PostThreadReply(ctx context.Context, mrID string, threadID int, parentCommentID int64, content string) error {
	org, project, repo, id, err := splitMergeRequestID(mrID)
	if err != nil {
		return err
	}
	apiBase := firstNonEmpty(p.baseURL, orgAPIBase(org))

	payload := map[string]any{
		"content":         content,
		"parentCommentId": parentCommentID,
		"commentType":     1, // text
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode reply payload: %w", err)
	}

	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=%s",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), id, threadID, apiVersion)

	req, err := networkazuredevops.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.do(req)
	if err != nil {
		return fmt.Errorf("failed to post thread reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("azure devops thread reply failed (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetBotIdentity fetches the authenticated identity (bot/service account) for
// this provider's org/PAT, for bot-mention and self-reply detection.
func (p *Provider) GetBotIdentity(ctx context.Context) (*Profile, error) {
	return FetchAzureDevOpsProfile(p.baseURL, p.token)
}

// repositoryInfo mirrors the subset of the Get Repository API response needed
// to resolve a repository GUID (as delivered in webhook resource links,
// which carry no human-readable names) to its project/repo names.
type repositoryInfo struct {
	Name    string     `json:"name"`
	Project projectRef `json:"project"`
}

// ResolveRepositoryByID resolves a repository GUID to its (projectName,
// repoName) pair. Azure DevOps accepts a repository ID without a project
// segment in the URL, so this works from just the GUID - needed because
// PR-comment webhook events only carry a repository GUID link, not names.
func (p *Provider) ResolveRepositoryByID(ctx context.Context, repositoryID string) (projectName, repoName string, err error) {
	apiURL := fmt.Sprintf("%s/_apis/git/repositories/%s?api-version=%s", p.baseURL, neturl.PathEscape(repositoryID), apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", "", fmt.Errorf("azure devops repository fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var info repositoryInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("failed to decode repository response: %w", err)
	}
	return info.Project.Name, info.Name, nil
}
