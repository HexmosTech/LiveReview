package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/reviewmodel"
)

const (
	bitbucketAPIBase      = "https://api.bitbucket.org/2.0"
	defaultBitbucketPRURL = "https://bitbucket.org/contorted/fb_backends/pull-requests/1"
)

type bitbucketUser struct {
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

type bitbucketCommit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Author  struct {
		Raw  string         `json:"raw"`
		User *bitbucketUser `json:"user"`
	} `json:"author"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type bitbucketInline struct {
	Path string `json:"path"`
	From *int   `json:"from"`
	To   *int   `json:"to"`
}

type bitbucketComment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User      *bitbucketUser `json:"user"`
	CreatedOn string         `json:"created_on"`
	UpdatedOn string         `json:"updated_on"`
	Parent    *struct {
		ID int `json:"id"`
	} `json:"parent"`
	Inline  *bitbucketInline `json:"inline"`
	Deleted bool             `json:"deleted"`
	Type    string           `json:"type"`
}

func runBitbucket(args []string) error {
	fs := flag.NewFlagSet("bitbucket", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "Bitbucket workspace")
	repo := fs.String("repo", "", "Bitbucket repository slug")
	prNumber := fs.Int("pr", 0, "Pull request number")
	email := fs.String("email", "", "Bitbucket account email (optional if BITBUCKET_EMAIL set or integration token available)")
	token := fs.String("token", "", "Bitbucket app password (optional if BITBUCKET_TOKEN/BITBUCKET_APP_PASSWORD set or integration token available)")
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	urlFlag := stringFlag{value: defaultBitbucketPRURL}
	fs.Var(&urlFlag, "url", "Bitbucket pull request URL (overrides --workspace/--repo/--pr)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	workspaceVal := strings.TrimSpace(*workspace)
	repoVal := strings.TrimSpace(*repo)
	prVal := *prNumber
	urlVal := strings.TrimSpace(urlFlag.value)

	var workspaceSlug, repoSlug, prID, prURL string
	useURL := urlFlag.set || (workspaceVal == "" && repoVal == "" && prVal == 0)

	if useURL {
		var err error
		workspaceSlug, repoSlug, prID, err = parseBitbucketPRURL(urlVal)
		if err != nil {
			return err
		}
		if urlVal == "" {
			prURL = defaultBitbucketPRURL
		} else {
			prURL = urlVal
		}
	} else if workspaceVal != "" && repoVal != "" && prVal > 0 {
		workspaceSlug = workspaceVal
		repoSlug = repoVal
		prID = strconv.Itoa(prVal)
		prURL = fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%s", workspaceSlug, repoSlug, prID)
	} else {
		return errors.New("must provide --workspace/--repo/--pr or a valid --url")
	}

	emailVal := strings.TrimSpace(*email)
	if emailVal == "" {
		emailVal = strings.TrimSpace(os.Getenv("BITBUCKET_EMAIL"))
	}
	tokenVal := strings.TrimSpace(*token)
	if tokenVal == "" {
		tokenVal = strings.TrimSpace(os.Getenv("BITBUCKET_TOKEN"))
	}
	if tokenVal == "" {
		tokenVal = strings.TrimSpace(os.Getenv("BITBUCKET_APP_PASSWORD"))
	}

	if emailVal == "" || tokenVal == "" {
		dbEmail, dbToken, dbErr := findBitbucketCredentialsFromDB(workspaceSlug, repoSlug)
		if dbErr != nil {
			if emailVal == "" || tokenVal == "" {
				return fmt.Errorf("bitbucket credentials not provided and lookup failed: %w", dbErr)
			}
		} else {
			if emailVal == "" {
				emailVal = dbEmail
			}
			if tokenVal == "" {
				tokenVal = dbToken
			}
		}
	}

	if emailVal == "" || tokenVal == "" {
		return errors.New("bitbucket email or token missing; supply flags/env or configure integration token")
	}

	client := &http.Client{Timeout: 15 * time.Second}

	commits, err := fetchBitbucketPRCommits(client, workspaceSlug, repoSlug, prID, emailVal, tokenVal)
	if err != nil {
		return fmt.Errorf("fetch commits: %w", err)
	}
	comments, err := fetchBitbucketPRComments(client, workspaceSlug, repoSlug, prID, emailVal, tokenVal)
	if err != nil {
		return fmt.Errorf("fetch comments: %w", err)
	}
	filteredComments := filterBitbucketComments(comments)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := writeJSONPretty(filepath.Join(*outDir, "bb_raw_commits.json"), map[string]interface{}{
		"workspace":    workspaceSlug,
		"repo":         repoSlug,
		"pull_request": prID,
		"items":        commits,
	}); err != nil {
		return fmt.Errorf("write raw commits: %w", err)
	}

	if err := writeJSONPretty(filepath.Join(*outDir, "bb_raw_comments.json"), map[string]interface{}{
		"workspace":    workspaceSlug,
		"repo":         repoSlug,
		"pull_request": prID,
		"items":        comments,
	}); err != nil {
		return fmt.Errorf("write raw comments: %w", err)
	}

	timelineItems := buildBitbucketTimeline(workspaceSlug, repoSlug, commits, filteredComments)
	sort.Slice(timelineItems, func(i, j int) bool {
		return timelineItems[i].CreatedAt.Before(timelineItems[j].CreatedAt)
	})

	prevIndex := reviewmodel.BuildPrevCommitIndex(timelineItems)
	exportTimeline := reviewmodel.BuildExportTimeline(timelineItems)
	commentTree := buildBitbucketCommentTree(filteredComments)
	exportTree := reviewmodel.BuildExportCommentTreeWithPrev(commentTree, prevIndex)

	timelinePath := filepath.Join(*outDir, "bb_timeline.json")
	if err := writeJSONPretty(timelinePath, exportTimeline); err != nil {
		return fmt.Errorf("write timeline: %w", err)
	}

	treePath := filepath.Join(*outDir, "bb_comment_tree.json")
	if err := writeJSONPretty(treePath, exportTree); err != nil {
		return fmt.Errorf("write comment tree: %w", err)
	}

	fmt.Printf("Target PR: %s\n", prURL)
	fmt.Printf("Bitbucket artifacts written to %s (bb_timeline.json, bb_comment_tree.json)\n", *outDir)
	fmt.Printf("Summary: commits=%d comments=%d (filtered from %d raw)\n", len(commits), len(filteredComments), len(comments))
	return nil
}

func parseBitbucketPRURL(raw string) (string, string, string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", "", errors.New("PR URL is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("parse PR URL: %w", err)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 4 {
		return "", "", "", fmt.Errorf("invalid PR URL path %q", parsed.Path)
	}
	// Expected: workspace/repo/pull-requests/<id>
	var idx int
	for i, part := range segments {
		if part == "pull-requests" || part == "pullrequests" {
			idx = i
			break
		}
	}
	if idx == 0 || idx+1 >= len(segments) {
		return "", "", "", fmt.Errorf("invalid PR URL path %q", parsed.Path)
	}
	workspace := segments[0]
	repository := segments[1]
	prID := segments[idx+1]
	if _, err := strconv.Atoi(prID); err != nil {
		return "", "", "", fmt.Errorf("invalid PR number %q", prID)
	}
	return workspace, repository, prID, nil
}

func findBitbucketCredentialsFromDB(workspace, repo string) (string, string, error) {
	rows, err := fetchIntegrationTokens()
	if err != nil {
		return "", "", fmt.Errorf("fetch integration_tokens: %w", err)
	}

	target := strings.ToLower(strings.TrimSpace(workspace + "/" + repo))
	var fallbackEmail, fallbackToken string

	for _, row := range rows {
		provider := strings.ToLower(strings.TrimSpace(asString(row["provider"])))
		if provider == "" || !strings.Contains(provider, "bitbucket") {
			continue
		}
		token := strings.TrimSpace(asString(row["pat_token"]))
		if token == "" {
			continue
		}

		metadataRaw := strings.TrimSpace(asString(row["metadata"]))
		var meta map[string]interface{}
		if metadataRaw != "" {
			if err := json.Unmarshal([]byte(metadataRaw), &meta); err != nil {
				meta = nil
			}
		}

		repoMatch := target == ""
		candidates := collectBitbucketRepoCandidates(meta, row)
		if target != "" {
			targetLower := target
			for _, cand := range candidates {
				norm := normalizeBitbucketRepoCandidate(cand)
				if norm != "" && norm == targetLower {
					repoMatch = true
					break
				}
			}
		}

		email := strings.TrimSpace(asString(metaValue(meta, "email")))
		if email == "" {
			email = strings.TrimSpace(asString(metaValue(meta, "username")))
		}
		if email == "" {
			email = strings.TrimSpace(asString(row["connection_email"]))
		}

		if repoMatch && email != "" {
			if name := strings.TrimSpace(asString(row["connection_name"])); name != "" {
				fmt.Printf("Using Bitbucket credentials from integration_tokens connection %s\n", name)
			} else {
				fmt.Println("Using Bitbucket credentials from integration_tokens")
			}
			return email, token, nil
		}

		if fallbackToken == "" && email != "" {
			fallbackEmail = email
			fallbackToken = token
		}
	}

	if fallbackToken != "" {
		fmt.Println("Using Bitbucket credentials from integration_tokens (fallback)")
		return fallbackEmail, fallbackToken, nil
	}

	return "", "", errors.New("no Bitbucket credentials found in integration_tokens")
}

func collectBitbucketRepoCandidates(meta map[string]interface{}, row map[string]interface{}) []string {
	candidates := []string{}
	if meta != nil {
		if v := strings.TrimSpace(asString(meta["project_full_name"])); v != "" {
			candidates = append(candidates, v)
		}
		workspace := strings.TrimSpace(asString(meta["workspace"]))
		repository := strings.TrimSpace(asString(meta["repository"]))
		if workspace != "" && repository != "" {
			candidates = append(candidates, workspace+"/"+repository)
		}
		if arr := toStringSlice(meta["projects_cache"]); len(arr) > 0 {
			candidates = append(candidates, arr...)
		}
	}
	if v := strings.TrimSpace(asString(row["provider_url"])); v != "" {
		candidates = append(candidates, v)
	}
	if arr := toStringSlice(row["projects_cache"]); len(arr) > 0 {
		candidates = append(candidates, arr...)
	}
	return candidates
}

func normalizeBitbucketRepoCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		if u, err := url.Parse(value); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 {
				return strings.ToLower(strings.TrimSpace(parts[0]) + "/" + strings.TrimSpace(parts[1]))
			}
		}
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) >= 2 {
		return strings.ToLower(strings.TrimSpace(parts[0]) + "/" + strings.TrimSpace(parts[1]))
	}
	return ""
}

func metaValue(meta map[string]interface{}, key string) interface{} {
	if meta == nil {
		return ""
	}
	return meta[key]
}

func asString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case fmt.Stringer:
		return val.String()
	case json.Number:
		return val.String()
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return ""
	}
}

func toStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case nil:
		return nil
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s := strings.TrimSpace(asString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(s), &parsed); err == nil {
				return parsed
			}
		}
		return []string{s}
	case []byte:
		return toStringSlice(string(val))
	default:
		return nil
	}
}

func fetchBitbucketPRCommits(client *http.Client, workspace, repo, prID, email, token string) ([]bitbucketCommit, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%s/commits", bitbucketAPIBase, url.PathEscape(workspace), url.PathEscape(repo), url.PathEscape(prID))
	results := make([]bitbucketCommit, 0, 32)
	next := endpoint
	for next != "" {
		req, err := http.NewRequest("GET", next, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.SetBasicAuth(email, token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "LiveReview-MRModel/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call Bitbucket commits API: %w", err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				err = fmt.Errorf("Bitbucket commits API failed: %s: %s", resp.Status, string(body))
				return
			}
			var page struct {
				Values []bitbucketCommit `json:"values"`
				Next   string            `json:"next"`
			}
			if decodeErr := json.NewDecoder(resp.Body).Decode(&page); decodeErr != nil {
				err = fmt.Errorf("decode commits response: %w", decodeErr)
				return
			}
			results = append(results, page.Values...)
			next = strings.TrimSpace(page.Next)
		}()
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func fetchBitbucketPRComments(client *http.Client, workspace, repo, prID, email, token string) ([]bitbucketComment, error) {
	endpoint := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%s/comments", bitbucketAPIBase, url.PathEscape(workspace), url.PathEscape(repo), url.PathEscape(prID))
	results := make([]bitbucketComment, 0, 64)
	next := endpoint
	for next != "" {
		req, err := http.NewRequest("GET", next, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.SetBasicAuth(email, token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "LiveReview-MRModel/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call Bitbucket comments API: %w", err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				err = fmt.Errorf("Bitbucket comments API failed: %s: %s", resp.Status, string(body))
				return
			}
			var page struct {
				Values []bitbucketComment `json:"values"`
				Next   string             `json:"next"`
			}
			if decodeErr := json.NewDecoder(resp.Body).Decode(&page); decodeErr != nil {
				err = fmt.Errorf("decode comments response: %w", decodeErr)
				return
			}
			results = append(results, page.Values...)
			next = strings.TrimSpace(page.Next)
		}()
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func filterBitbucketComments(comments []bitbucketComment) []bitbucketComment {
	out := make([]bitbucketComment, 0, len(comments))
	for _, comment := range comments {
		if comment.Deleted {
			continue
		}
		body := strings.TrimSpace(comment.Content.Raw)
		if body == "" {
			continue
		}
		out = append(out, comment)
	}
	return out
}

func buildBitbucketTimeline(workspace, repo string, commits []bitbucketCommit, comments []bitbucketComment) []reviewmodel.TimelineItem {
	items := make([]reviewmodel.TimelineItem, 0, len(commits)+len(comments))

	for _, commit := range commits {
		timestamp := parseBitbucketTime(commit.Date)
		message := strings.TrimSpace(commit.Message)
		title := message
		if idx := strings.IndexByte(title, '\n'); idx >= 0 {
			title = title[:idx]
		}
		if title == "" {
			title = commit.Hash
		}
		webURL := strings.TrimSpace(commit.Links.HTML.Href)
		if webURL == "" {
			webURL = fmt.Sprintf("https://bitbucket.org/%s/%s/commits/%s", workspace, repo, commit.Hash)
		}
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "commit",
			ID:        commit.Hash,
			CreatedAt: timestamp,
			Author:    bitbucketCommitAuthor(commit),
			Commit: &reviewmodel.TimelineCommit{
				SHA:     commit.Hash,
				Title:   title,
				Message: message,
				WebURL:  webURL,
			},
		})
	}

	commentMap := make(map[int]*bitbucketComment, len(comments))
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	for i := range comments {
		comment := &comments[i]
		idStr := strconv.Itoa(comment.ID)
		timestamp := parseBitbucketCommentTime(*comment)
		rootID := resolveBitbucketDiscussionID(comment, commentMap)
		discussion := strconv.Itoa(rootID)
		lineOld, lineNew, filePath := bitbucketLineInfo(comment.Inline)
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "comment",
			ID:        idStr,
			CreatedAt: timestamp,
			Author:    bitbucketUserToAuthor(comment.User),
			Comment: &reviewmodel.TimelineComment{
				NoteID:     idStr,
				Discussion: discussion,
				Body:       comment.Content.Raw,
				IsSystem:   comment.User == nil,
				FilePath:   filePath,
				LineOld:    lineOld,
				LineNew:    lineNew,
			},
		})
	}

	return items
}

func buildBitbucketCommentTree(comments []bitbucketComment) reviewmodel.CommentTree {
	if len(comments) == 0 {
		return reviewmodel.CommentTree{}
	}

	sort.Slice(comments, func(i, j int) bool {
		return parseBitbucketCommentTime(comments[i]).Before(parseBitbucketCommentTime(comments[j]))
	})

	commentMap := make(map[int]*bitbucketComment, len(comments))
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	nodes := make(map[int]*reviewmodel.CommentNode, len(comments))
	for i := range comments {
		comment := &comments[i]
		node := &reviewmodel.CommentNode{
			ID:           strconv.Itoa(comment.ID),
			DiscussionID: strconv.Itoa(resolveBitbucketDiscussionID(comment, commentMap)),
			Author:       bitbucketUserToAuthor(comment.User),
			Body:         comment.Content.Raw,
			CreatedAt:    parseBitbucketCommentTime(*comment),
		}
		if comment.Inline != nil {
			node.FilePath = comment.Inline.Path
			if comment.Inline.From != nil {
				node.LineOld = *comment.Inline.From
			}
			if comment.Inline.To != nil {
				node.LineNew = *comment.Inline.To
			}
		}
		nodes[comment.ID] = node
	}

	roots := make([]*reviewmodel.CommentNode, 0, len(comments))
	for _, comment := range comments {
		node := nodes[comment.ID]
		if comment.Parent != nil {
			if parent := nodes[comment.Parent.ID]; parent != nil {
				node.ParentID = parent.ID
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	sort.Slice(roots, func(i, j int) bool {
		return roots[i].CreatedAt.Before(roots[j].CreatedAt)
	})
	for _, root := range roots {
		sortCommentChildren(root)
	}

	return reviewmodel.CommentTree{Roots: roots}
}

func bitbucketLineInfo(inline *bitbucketInline) (lineOld, lineNew int, path string) {
	if inline == nil {
		return 0, 0, ""
	}
	if inline.From != nil {
		lineOld = *inline.From
	}
	if inline.To != nil {
		lineNew = *inline.To
	}
	path = inline.Path
	return
}

func parseBitbucketCommentTime(comment bitbucketComment) time.Time {
	if t := parseBitbucketTime(comment.CreatedOn); !t.IsZero() {
		return t
	}
	if t := parseBitbucketTime(comment.UpdatedOn); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func resolveBitbucketDiscussionID(comment *bitbucketComment, lookup map[int]*bitbucketComment) int {
	if comment == nil {
		return 0
	}
	current := comment
	visited := map[int]struct{}{}
	for current.Parent != nil {
		parentID := current.Parent.ID
		if _, seen := visited[parentID]; seen {
			break
		}
		visited[parentID] = struct{}{}
		parent, ok := lookup[parentID]
		if !ok {
			return parentID
		}
		current = parent
	}
	return current.ID
}

func bitbucketCommitAuthor(commit bitbucketCommit) reviewmodel.AuthorInfo {
	if commit.Author.User != nil {
		user := commit.Author.User
		display := strings.TrimSpace(user.DisplayName)
		if display == "" {
			display = user.Username
		}
		username := selectBitbucketUsername(user)
		return reviewmodel.AuthorInfo{
			Provider: "bitbucket",
			Username: username,
			Name:     display,
			WebURL:   strings.TrimSpace(user.Links.HTML.Href),
		}
	}
	name, email := parseBitbucketAuthorRaw(commit.Author.Raw)
	return reviewmodel.AuthorInfo{Provider: "bitbucket", Name: name, Email: email}
}

func bitbucketUserToAuthor(user *bitbucketUser) reviewmodel.AuthorInfo {
	if user == nil {
		return reviewmodel.AuthorInfo{Provider: "bitbucket", Name: "system"}
	}
	display := strings.TrimSpace(user.DisplayName)
	if display == "" {
		display = user.Username
	}
	username := selectBitbucketUsername(user)
	return reviewmodel.AuthorInfo{
		Provider: "bitbucket",
		Username: username,
		Name:     display,
		WebURL:   strings.TrimSpace(user.Links.HTML.Href),
	}
}

func selectBitbucketUsername(user *bitbucketUser) string {
	if user == nil {
		return ""
	}
	username := strings.TrimSpace(user.Username)
	if username != "" {
		return username
	}
	accountID := strings.TrimSpace(user.AccountID)
	if accountID != "" {
		return accountID
	}
	uuid := strings.TrimSpace(strings.Trim(user.UUID, "{}"))
	if uuid != "" {
		return uuid
	}
	return ""
}

func parseBitbucketAuthorRaw(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if idx := strings.Index(raw, "<"); idx >= 0 {
		name := strings.TrimSpace(raw[:idx])
		rest := raw[idx+1:]
		if end := strings.Index(rest, ">"); end >= 0 {
			email := strings.TrimSpace(rest[:end])
			return name, email
		}
		return name, ""
	}
	return raw, ""
}

func parseBitbucketTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z07:00",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
