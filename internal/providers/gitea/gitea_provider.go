package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// Config holds configuration for the Gitea provider.
type Config struct {
	BaseURL string `koanf:"base_url"`
	Token   string `koanf:"token"`
}

// Provider implements the providers.Provider interface for Gitea.
type Provider struct {
	baseURL    string
	token      string
	username   string
	password   string
	httpClient *http.Client
	session    *sessionClient
}

// NewProvider creates a Provider with the supplied configuration.
func NewProvider(cfg Config) (*Provider, error) {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	pt := decodePackedToken(cfg.Token)
	tok := cfg.Token
	user := ""
	pass := ""
	if pt.pat != "" {
		tok = pt.pat
		user = pt.username
		pass = pt.password
	}
	return &Provider{
		baseURL:    base,
		token:      tok,
		username:   user,
		password:   pass,
		httpClient: &http.Client{},
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gitea"
}

// Configure applies dynamic configuration from the factory.
func (p *Provider) Configure(config map[string]interface{}) error {
	base := p.baseURL
	if v, ok := config["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		base = strings.TrimSuffix(v, "/")
	}
	if v, ok := config["url"].(string); ok && strings.TrimSpace(v) != "" {
		base = strings.TrimSuffix(v, "/")
	}
	token := p.token
	if v, ok := config["pat_token"].(string); ok && strings.TrimSpace(v) != "" {
		token = v
	}
	if v, ok := config["token"].(string); ok && strings.TrimSpace(v) != "" {
		token = v
	}

	// If token is JSON-encoded, unpack pat/username/password to allow packed storage.
	if parsed := decodePackedToken(token); parsed.pat != "" {
		token = parsed.pat
		if p.username == "" {
			p.username = parsed.username
		}
		if p.password == "" {
			p.password = parsed.password
		}
	}

	if v, ok := config["username"].(string); ok {
		p.username = strings.TrimSpace(v)
	}
	if v, ok := config["password"].(string); ok {
		p.password = v
	}

	if base == "" {
		return fmt.Errorf("base_url is required for Gitea provider")
	}
	if token == "" {
		return fmt.Errorf("token is required for Gitea provider")
	}

	p.baseURL = base
	p.token = token
	if p.httpClient == nil {
		p.httpClient = &http.Client{}
	}
	p.session = nil // reset session on reconfigure
	return nil
}

// GetMergeRequestDetails fetches PR details for a Gitea pull request URL.
func (p *Provider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	owner, repo, index, apiBase, err := p.parsePullURL(mrURL)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", apiBase, owner, repo, index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuthHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("gitea pull request fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var pr pullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode gitea response: %w", err)
	}

	number := pr.Number
	if number == 0 {
		number = pr.Index
	}
	if number == 0 {
		number = int(pr.ID)
	}

	prID := fmt.Sprintf("%s/%s/%d", owner, repo, number)

	return &providers.MergeRequestDetails{
		ID:             prID,
		Title:          pr.Title,
		Description:    pr.Body,
		SourceBranch:   pr.Head.Ref,
		TargetBranch:   pr.Base.Ref,
		Author:         pr.User.Login,
		AuthorName:     firstNonEmpty(pr.User.FullName, pr.User.Login),
		AuthorUsername: pr.User.Login,
		AuthorEmail:    pr.User.Email,
		AuthorAvatar:   pr.User.AvatarURL,
		CreatedAt:      pr.CreatedAt,
		URL:            mrURL,
		State:          pr.State,
		MergeStatus:    pr.MergeableState,
		DiffRefs: providers.DiffRefs{
			BaseSHA:  firstNonEmpty(pr.Base.SHA, pr.MergeBase),
			HeadSHA:  pr.Head.SHA,
			StartSHA: pr.MergeBase,
		},
		WebURL:        pr.HTMLURL,
		ProviderType:  "gitea",
		RepositoryURL: fmt.Sprintf("%s/%s/%s", apiBase, owner, repo),
	}, nil
}

// GetMergeRequestChanges retrieves diffs for a PR. prID format: owner/repo/number.
func (p *Provider) GetMergeRequestChanges(ctx context.Context, prID string) ([]*models.CodeDiff, error) {
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid Gitea PR ID format: expected 'owner/repo/number', got '%s'", prID)
	}
	owner := parts[0]
	repo := parts[1]
	number := parts[2]

	apiBase := p.baseURL
	if apiBase == "" {
		return nil, fmt.Errorf("base_url is required to fetch changes")
	}

	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%s/files", apiBase, owner, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuthHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("gitea pull request files fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var files []pullRequestFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode pull request files: %w", err)
	}

	var diffs []*models.CodeDiff
	for _, f := range files {
		hunks := parsePatchIntoHunks(f.Patch)
		diff := &models.CodeDiff{
			FilePath:    f.Filename,
			CommitID:    f.SHA,
			FileType:    getFileType(f.Filename),
			IsNew:       f.Status == "added",
			IsDeleted:   f.Status == "removed",
			IsRenamed:   f.Status == "renamed",
			OldFilePath: f.PreviousFilename,
			Hunks:       hunks,
		}
		diffs = append(diffs, diff)
	}

	return diffs, nil
}

// PostComment posts a comment on a PR. Supports inline (file/line) comments and general comments.
func (p *Provider) PostComment(ctx context.Context, prID string, comment *models.ReviewComment) error {
	parts := strings.Split(prID, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid Gitea PR ID format: expected 'owner/repo/number', got '%s'", prID)
	}
	owner := parts[0]
	repo := parts[1]
	number := parts[2]

	apiBase := p.baseURL
	if apiBase == "" {
		return fmt.Errorf("base_url is required to post comments")
	}

	// Inline comment path: use session (browser-style) flow as primary mechanism.
	if comment.FilePath != "" && comment.Line > 0 {
		pr, err := p.fetchPullRequest(ctx, owner, repo, number)
		if err != nil {
			return fmt.Errorf("failed to fetch pull request for inline comment: %w", err)
		}
		if !p.hasSessionCreds() {
			return fmt.Errorf("session credentials not provided for Gitea inline comments")
		}
		if err := p.postInlineViaSession(ctx, apiBase, owner, repo, number, comment, pr.Head.SHA); err != nil {
			return err
		}
		return nil
	}

	// General (issue-level) comment
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%s/comments", apiBase, owner, repo, number)
	payload := map[string]string{"body": comment.Content}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	p.applyAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gitea comment failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// PostComments posts multiple comments sequentially.
func (p *Provider) PostComments(ctx context.Context, prID string, comments []*models.ReviewComment) error {
	for _, c := range comments {
		if err := p.PostComment(ctx, prID, c); err != nil {
			return err
		}
	}
	return nil
}

// applyAuthHeaders applies Authorization and Accept headers.
func (p *Provider) applyAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("token %s", p.token))
	req.Header.Set("Accept", "application/json")
}

// Session-based inline posting (browser-style). Re-logins automatically on auth failures.
func (p *Provider) postInlineViaSession(ctx context.Context, apiBase, owner, repo, number string, comment *models.ReviewComment, headSHA string) error {
	if err := p.ensureSession(ctx); err != nil {
		return err
	}

	side := "proposed"
	if comment.IsDeletedLine {
		side = "previous"
	}

	commentURL := fmt.Sprintf("%s/%s/%s/pulls/%s/files/reviews/comments", apiBase, owner, repo, number)
	form := neturl.Values{}
	form.Set("_csrf", p.session.csrf)
	form.Set("origin", "diff")
	form.Set("latest_commit_id", headSHA)
	form.Set("side", side)
	form.Set("line", strconv.Itoa(comment.Line))
	form.Set("path", comment.FilePath)
	form.Set("diff_start_cid", "")
	form.Set("diff_end_cid", "")
	form.Set("diff_base_cid", "")
	form.Set("content", comment.Content)
	form.Set("single_review", "true")

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, commentURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to build session inline comment request: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("X-CSRF-Token", p.session.csrf)

	resp, err := p.session.client.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to post inline via session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || needsRelogin(resp) {
		if err := p.relogin(ctx); err != nil {
			return fmt.Errorf("session relogin failed: %w", err)
		}
		resp, err = p.session.client.Do(postReq)
		if err != nil {
			return fmt.Errorf("failed to post inline after relogin: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gitea session inline comment failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (p *Provider) hasSessionCreds() bool {
	return strings.TrimSpace(p.username) != "" && p.password != ""
}

type sessionClient struct {
	client *http.Client
	csrf   string
}

func (p *Provider) ensureSession(ctx context.Context) error {
	if p.session != nil && p.session.csrf != "" {
		return nil
	}
	if !p.hasSessionCreds() {
		return fmt.Errorf("session credentials not provided for Gitea inline fallback")
	}
	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	loginURL := fmt.Sprintf("%s/user/login", p.baseURL)
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build login GET: %w", err)
	}
	resp, err := cli.Do(getReq)
	if err != nil {
		return fmt.Errorf("failed to fetch login page: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	csrf := extractCSRF(string(bodyBytes))
	if csrf == "" {
		csrf = cookieValue(cli.Jar, p.baseURL, "_csrf")
	}
	if csrf == "" {
		return fmt.Errorf("failed to extract CSRF token for Gitea session")
	}

	form := neturl.Values{}
	form.Set("_csrf", csrf)
	form.Set("user_name", p.username)
	form.Set("password", p.password)
	form.Set("remember", "on")

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to build login POST: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = cli.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to execute login POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gitea login failed (%d): %s", resp.StatusCode, string(respBody))
	}

	authCookie := cookieValue(cli.Jar, p.baseURL, "gitea_incredible")
	csrfCookie := cookieValue(cli.Jar, p.baseURL, "_csrf")
	if authCookie == "" {
		return fmt.Errorf("gitea login missing session cookie")
	}
	finalCSRF := csrf
	if csrfCookie != "" {
		finalCSRF = csrfCookie
	}

	p.session = &sessionClient{client: cli, csrf: finalCSRF}
	return nil
}

func (p *Provider) relogin(ctx context.Context) error {
	p.session = nil
	return p.ensureSession(ctx)
}

func needsRelogin(resp *http.Response) bool {
	if resp == nil {
		return true
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true
	}
	loc := strings.ToLower(resp.Header.Get("Location"))
	if strings.Contains(loc, "login") {
		return true
	}
	return false
}

func extractCSRF(html string) string {
	re := regexp.MustCompile(`name="_csrf"\s+value="([^"]+)"`)
	matches := re.FindStringSubmatch(html)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func cookieValue(jar http.CookieJar, rawURL, name string) string {
	if jar == nil {
		return ""
	}
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, c := range jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (p *Provider) fetchPullRequest(ctx context.Context, owner, repo, number string) (*pullRequest, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%s", p.baseURL, owner, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build pull request request: %w", err)
	}
	p.applyAuthHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("gitea pull request fetch failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var pr pullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	return &pr, nil
}

func (p *Provider) parsePullURL(mrURL string) (owner, repo string, index int, apiBase string, err error) {
	parsed, err := neturl.Parse(mrURL)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("invalid Gitea PR URL: %w", err)
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 4 {
		return "", "", 0, "", fmt.Errorf("invalid Gitea PR URL: expected /owner/repo/pulls/<number>")
	}

	marker := segments[len(segments)-2]
	if marker != "pulls" && marker != "pull" {
		return "", "", 0, "", fmt.Errorf("invalid Gitea PR URL: expected pull request path, got '%s'", marker)
	}

	idxStr := segments[len(segments)-1]
	idx, convErr := strconv.Atoi(idxStr)
	if convErr != nil {
		return "", "", 0, "", fmt.Errorf("invalid pull request number: %w", convErr)
	}

	owner = segments[0]
	repo = segments[1]
	apiBase = p.baseURL
	if apiBase == "" {
		apiBase = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	}

	return owner, repo, idx, apiBase, nil
}

// pullRequest mirrors the subset of fields we need from Gitea.
type pullRequest struct {
	ID             int64    `json:"id"`
	Number         int      `json:"number"`
	Index          int      `json:"index"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	State          string   `json:"state"`
	CreatedAt      string   `json:"created_at"`
	MergeableState string   `json:"mergeable_state"`
	MergeBase      string   `json:"merge_base"`
	HTMLURL        string   `json:"html_url"`
	User           userInfo `json:"user"`
	Head           pullRef  `json:"head"`
	Base           pullRef  `json:"base"`
}

type userInfo struct {
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type pullRef struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo prRepo `json:"repo"`
}

type prRepo struct {
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type packedToken struct {
	pat      string
	username string
	password string
}

type pullRequestFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Patch            string `json:"patch"`
	SHA              string `json:"sha"`
}

func parsePatchIntoHunks(patch string) []models.DiffHunk {
	if patch == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	var hunks []models.DiffHunk
	var currentHunk *models.DiffHunk
	var hunkContent strings.Builder

	hunkHeaderRegex := regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

	for _, line := range lines {
		if match := hunkHeaderRegex.FindStringSubmatch(line); match != nil {
			if currentHunk != nil {
				currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
				hunks = append(hunks, *currentHunk)
				hunkContent.Reset()
			}

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

			hunkContent.WriteString(line + "\n")
		} else if currentHunk != nil {
			hunkContent.WriteString(line + "\n")
		}
	}

	if currentHunk != nil {
		currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

func getFileType(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "unknown"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func decodePackedToken(raw string) packedToken {
	var payload struct {
		Pat      string `json:"pat"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var pt packedToken
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return pt
	}
	pt.pat = payload.Pat
	pt.username = payload.Username
	pt.password = payload.Password
	return pt
}
