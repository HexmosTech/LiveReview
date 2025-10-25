package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	githubapi "github.com/livereview/internal/provider_input/github"
	"github.com/livereview/internal/reviewmodel"
)

const defaultGitHubPRURL = "https://github.com/livereviewbot/glabmig/pull/2"

type stringFlag struct {
	value string
	set   bool
}

func (s *stringFlag) String() string { return s.value }

func (s *stringFlag) Set(v string) error {
	s.value = v
	s.set = true
	return nil
}

// runGitHub collects GitHub PR context and writes timeline + comment tree exports.
func runGitHub(args []string) error {
	fs := flag.NewFlagSet("github", flag.ContinueOnError)
	repo := fs.String("repo", "", "GitHub repository in owner/repo format")
	prNumber := fs.Int("pr", 0, "Pull request number")
	token := fs.String("token", "", "GitHub personal access token (optional if GITHUB_TOKEN or GITHUB_PAT set)")
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	urlFlag := stringFlag{value: defaultGitHubPRURL}
	fs.Var(&urlFlag, "url", "GitHub pull request URL (overrides --repo/--pr)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoVal := strings.TrimSpace(*repo)
	prVal := *prNumber
	urlVal := strings.TrimSpace(urlFlag.value)

	var owner, name, prID, prURL string
	useURL := urlFlag.set || (repoVal == "" && prVal == 0)

	if useURL {
		var err error
		owner, name, prID, err = parseGitHubPRURL(urlVal)
		if err != nil {
			return err
		}
		prURL = urlVal
	} else if repoVal != "" && prVal > 0 {
		var err error
		owner, name, err = splitRepo(repoVal)
		if err != nil {
			return err
		}
		prID = strconv.Itoa(prVal)
		prURL = fmt.Sprintf("https://github.com/%s/%s/pull/%s", owner, name, prID)
	} else {
		return errors.New("must provide both --repo and --pr or a valid --url")
	}

	pat := strings.TrimSpace(*token)
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("GITHUB_PAT"))
	}
	if pat == "" {
		var dbErr error
		pat, dbErr = findGitHubTokenFromDB()
		if dbErr != nil {
			return fmt.Errorf("GitHub token not provided via flags/env and lookup failed: %w", dbErr)
		}
	}

	commits, err := githubapi.FetchGitHubPRCommitsV2(owner, name, prID, pat)
	if err != nil {
		return fmt.Errorf("fetch commits: %w", err)
	}
	issueComments, err := githubapi.FetchGitHubPRCommentsV2(owner, name, prID, pat)
	if err != nil {
		return fmt.Errorf("fetch issue comments: %w", err)
	}
	reviewComments, err := githubapi.FetchGitHubPRReviewCommentsV2(owner, name, prID, pat)
	if err != nil {
		return fmt.Errorf("fetch review comments: %w", err)
	}

	timelineItems := buildGitHubTimeline(owner, name, commits, issueComments, reviewComments)
	sort.Slice(timelineItems, func(i, j int) bool {
		return timelineItems[i].CreatedAt.Before(timelineItems[j].CreatedAt)
	})

	prevIndex := reviewmodel.BuildPrevCommitIndex(timelineItems)
	exportTimeline := reviewmodel.BuildExportTimeline(timelineItems)
	commentTree := buildGitHubCommentTree(issueComments, reviewComments)
	exportTree := reviewmodel.BuildExportCommentTreeWithPrev(commentTree, prevIndex)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	timelinePath := filepath.Join(*outDir, "gh_timeline.json")
	if err := writeJSONPretty(timelinePath, exportTimeline); err != nil {
		return fmt.Errorf("write timeline: %w", err)
	}

	treePath := filepath.Join(*outDir, "gh_comment_tree.json")
	if err := writeJSONPretty(treePath, exportTree); err != nil {
		return fmt.Errorf("write comment tree: %w", err)
	}

	fmt.Printf("Target PR: %s\n", prURL)
	fmt.Printf("GitHub artifacts written to %s (gh_timeline.json, gh_comment_tree.json)\n", *outDir)
	fmt.Printf("Summary: commits=%d issue_comments=%d review_comments=%d\n", len(commits), len(issueComments), len(reviewComments))
	return nil
}

func parseGitHubPRURL(raw string) (string, string, string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", "", errors.New("PR URL is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("parse PR URL: %w", err)
	}
	if parsed.Host != "github.com" {
		return "", "", "", fmt.Errorf("unsupported host %q in PR URL", parsed.Host)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 4 {
		return "", "", "", fmt.Errorf("invalid PR URL path %q", parsed.Path)
	}
	owner := segments[0]
	repo := segments[1]
	kind := segments[2]
	number := segments[3]
	if kind != "pull" && kind != "pulls" {
		return "", "", "", fmt.Errorf("invalid PR URL path %q", parsed.Path)
	}
	if _, err := strconv.Atoi(number); err != nil {
		return "", "", "", fmt.Errorf("invalid PR number %q", number)
	}
	return owner, repo, number, nil
}

func findGitHubTokenFromDB() (string, error) {
	rows, err := fetchIntegrationTokens()
	if err != nil {
		return "", fmt.Errorf("fetch integration_tokens: %w", err)
	}

	for _, row := range rows {
		provider, _ := row["provider"].(string)
		if !strings.EqualFold(provider, "github") {
			continue
		}
		token, _ := row["pat_token"].(string)
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if name, _ := row["connection_name"].(string); name != "" {
			fmt.Printf("Using GitHub PAT from integration_tokens connection %s\n", name)
		} else {
			fmt.Println("Using GitHub PAT from integration_tokens")
		}
		return token, nil
	}

	return "", errors.New("no GitHub PAT found in integration_tokens")
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo %q (expected owner/repo)", repo)
	}
	owner := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("invalid repo %q (expected owner/repo)", repo)
	}
	return owner, name, nil
}

func buildGitHubTimeline(owner, repo string, commits []githubapi.GitHubV2CommitInfo, issueComments []githubapi.GitHubV2CommentInfo, reviewComments []githubapi.GitHubV2ReviewComment) []reviewmodel.TimelineItem {
	items := make([]reviewmodel.TimelineItem, 0, len(commits)+len(issueComments)+len(reviewComments))

	for _, commit := range commits {
		timestamp := parseGitHubTime(commit.Author.Date)
		title := commit.Message
		if idx := strings.IndexRune(title, '\n'); idx >= 0 {
			title = title[:idx]
		}
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "commit",
			ID:        commit.SHA,
			CreatedAt: timestamp,
			Author: reviewmodel.AuthorInfo{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
			},
			Commit: &reviewmodel.TimelineCommit{
				SHA:     commit.SHA,
				Title:   title,
				Message: commit.Message,
				WebURL:  fmt.Sprintf("https://github.com/%s/%s/commit/%s", owner, repo, commit.SHA),
			},
		})
	}

	for _, comment := range issueComments {
		id := strconv.Itoa(comment.ID)
		timestamp := parseGitHubTime(comment.CreatedAt)
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "comment",
			ID:        id,
			CreatedAt: timestamp,
			Author:    githubUserToAuthor(comment.User),
			Comment: &reviewmodel.TimelineComment{
				NoteID: id,
				Body:   comment.Body,
			},
		})
	}

	for _, comment := range reviewComments {
		id := strconv.Itoa(comment.ID)
		timestamp := parseGitHubTime(comment.CreatedAt)
		lineNew, lineOld := deriveLineNumbers(comment)
		discussionID := ""
		if comment.PullRequestReviewID != 0 {
			discussionID = strconv.Itoa(comment.PullRequestReviewID)
		}
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "comment",
			ID:        id,
			CreatedAt: timestamp,
			Author:    githubUserToAuthor(comment.User),
			Comment: &reviewmodel.TimelineComment{
				NoteID:     id,
				Discussion: discussionID,
				Body:       comment.Body,
				FilePath:   comment.Path,
				LineNew:    lineNew,
				LineOld:    lineOld,
			},
		})
	}

	return items
}

func buildGitHubCommentTree(issueComments []githubapi.GitHubV2CommentInfo, reviewComments []githubapi.GitHubV2ReviewComment) reviewmodel.CommentTree {
	nodes := make(map[string]*reviewmodel.CommentNode, len(reviewComments))
	roots := make([]*reviewmodel.CommentNode, 0, len(issueComments)+len(reviewComments))

	for _, comment := range reviewComments {
		id := strconv.Itoa(comment.ID)
		timestamp := parseGitHubTime(comment.CreatedAt)
		lineNew, lineOld := deriveLineNumbers(comment)
		discussionID := ""
		if comment.PullRequestReviewID != 0 {
			discussionID = strconv.Itoa(comment.PullRequestReviewID)
		}
		nodes[id] = &reviewmodel.CommentNode{
			ID:           id,
			DiscussionID: discussionID,
			Author:       githubUserToAuthor(comment.User),
			Body:         comment.Body,
			CreatedAt:    timestamp,
			FilePath:     comment.Path,
			LineNew:      lineNew,
			LineOld:      lineOld,
		}
	}

	for _, comment := range reviewComments {
		id := strconv.Itoa(comment.ID)
		node := nodes[id]
		if node == nil {
			continue
		}
		if comment.InReplyToID != nil {
			parentID := strconv.Itoa(*comment.InReplyToID)
			if parent, ok := nodes[parentID]; ok {
				node.ParentID = parentID
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	for _, comment := range issueComments {
		id := strconv.Itoa(comment.ID)
		timestamp := parseGitHubTime(comment.CreatedAt)
		roots = append(roots, &reviewmodel.CommentNode{
			ID:        id,
			Author:    githubUserToAuthor(comment.User),
			Body:      comment.Body,
			CreatedAt: timestamp,
		})
	}

	sort.Slice(roots, func(i, j int) bool {
		return roots[i].CreatedAt.Before(roots[j].CreatedAt)
	})
	for _, root := range roots {
		sortCommentChildren(root)
	}

	return reviewmodel.CommentTree{Roots: roots}
}

func sortCommentChildren(node *reviewmodel.CommentNode) {
	if len(node.Children) == 0 {
		return
	}
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].CreatedAt.Before(node.Children[j].CreatedAt)
	})
	for _, child := range node.Children {
		sortCommentChildren(child)
	}
}

func deriveLineNumbers(comment githubapi.GitHubV2ReviewComment) (lineNew, lineOld int) {
	lineNew = comment.Line
	lineOld = comment.OriginalLine
	if lineNew == 0 && !strings.EqualFold(comment.Side, "RIGHT") {
		lineNew = comment.OriginalLine
	}
	if lineOld == 0 && strings.EqualFold(comment.Side, "RIGHT") {
		lineOld = comment.Line
	}
	return
}

func githubUserToAuthor(user githubapi.GitHubV2User) reviewmodel.AuthorInfo {
	return reviewmodel.AuthorInfo{
		ID:        user.ID,
		Username:  user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		WebURL:    user.HTMLURL,
	}
}

func parseGitHubTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", value); err == nil {
		return t
	}
	return time.Time{}
}

func writeJSONPretty(path string, v interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
