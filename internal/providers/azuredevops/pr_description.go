package azuredevops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

// Helpers for the PR review links section and the auto-approve policy result.
// projectRepo is "project/repo", the repositories.full_name LiveReview stores for Azure DevOps.

func (p *Provider) prAPIPath(projectRepo string, number int) (string, error) {
	project, repo, ok := strings.Cut(projectRepo, "/")
	if !ok {
		return "", fmt.Errorf("invalid Azure DevOps repository name %q, expected project/repo", projectRepo)
	}
	return fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullRequests/%d",
		p.baseURL, neturl.PathEscape(project), neturl.PathEscape(repo), number), nil
}

// GetPRDescription returns the PR description.
func (p *Provider) GetPRDescription(ctx context.Context, projectRepo string, number int) (string, error) {
	path, err := p.prAPIPath(projectRepo, number)
	if err != nil {
		return "", err
	}
	var pr struct {
		Description string `json:"description"`
	}
	err = p.doJSON(ctx, http.MethodGet, path, nil, &pr)
	return pr.Description, err
}

// SetPRDescription replaces the PR description (Azure DevOps caps it at 4000 characters).
func (p *Provider) SetPRDescription(ctx context.Context, projectRepo string, number int, description string) error {
	path, err := p.prAPIPath(projectRepo, number)
	if err != nil {
		return err
	}
	return p.doJSON(ctx, http.MethodPatch, path, map[string]string{"description": description}, nil)
}

// PostPRThread posts a general (non-file) comment thread on the PR.
func (p *Provider) PostPRThread(ctx context.Context, projectRepo string, number int, body string) error {
	path, err := p.prAPIPath(projectRepo, number)
	if err != nil {
		return err
	}
	payload := map[string]interface{}{
		"status":   1,
		"comments": []map[string]interface{}{{"parentCommentId": 0, "content": body, "commentType": 1}},
	}
	return p.doJSON(ctx, http.MethodPost, path+"/threads", payload, nil)
}

// SetPRVote sets the bot's reviewer vote: approved (10), or "waiting for author" (-5) when approve is false.
func (p *Provider) SetPRVote(ctx context.Context, projectRepo string, number int, approve bool) error {
	path, err := p.prAPIPath(projectRepo, number)
	if err != nil {
		return err
	}
	bot, err := p.GetBotIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve bot identity: %w", err)
	}
	vote := -5
	if approve {
		vote = 10
	}
	return p.doJSON(ctx, http.MethodPut, path+"/reviewers/"+neturl.PathEscape(bot.ID), map[string]int{"vote": vote}, nil)
}

func (p *Provider) doJSON(ctx context.Context, method, apiURL string, payload, out interface{}) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	sep := "?"
	if strings.Contains(apiURL, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL+sep+"api-version="+apiVersion, reqBody)
	if err != nil {
		return err
	}
	p.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure devops %s failed with status %d: %s", method, resp.StatusCode, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
