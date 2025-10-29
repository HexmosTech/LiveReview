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

const (
	defaultGitHubPRURL          = "https://github.com/livereviewbot/glabmig/pull/2"
	githubEnableArtifactWriting = false
)

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

func mustMarshal(v interface{}) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return data
}

func buildGitHubArtifact(owner, name, prID, pat string) (*UnifiedArtifact, error) {
	commits, err := githubapi.FetchGitHubPRCommitsV2(owner, name, prID, pat)
	if err != nil {
		return nil, fmt.Errorf("fetch commits: %w", err)
	}
	issueComments, err := githubapi.FetchGitHubPRCommentsV2(owner, name, prID, pat)
	if err != nil {
		return nil, fmt.Errorf("fetch issue comments: %w", err)
	}
	reviewComments, err := githubapi.FetchGitHubPRReviewCommentsV2(owner, name, prID, pat)
	if err != nil {
		return nil, fmt.Errorf("fetch review comments: %w", err)
	}
	reviews, err := githubapi.FetchGitHubPRReviewsV2(owner, name, prID, pat)
	if err != nil {
		return nil, fmt.Errorf("fetch reviews: %w", err)
	}

	diffText, err := githubapi.FetchGitHubPRDiff(owner, name, prID, pat)
	if err != nil {
		return nil, fmt.Errorf("fetch diff: %w", err)
	}

	// 3. Process data and build unified artifact
	timelineItems := buildGitHubTimeline(owner, name, commits, issueComments, reviewComments, reviews)
	sort.Slice(timelineItems, func(i, j int) bool {
		return timelineItems[i].CreatedAt.Before(timelineItems[j].CreatedAt)
	})

	commentTree := buildGitHubCommentTree(issueComments, reviewComments, reviews)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(string(diffText))
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	// Convert []LocalCodeDiff to []*LocalCodeDiff for the unified artifact
	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := &UnifiedArtifact{
		Provider:     "github",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		Participants: extractParticipants(timelineItems),
	}

	// This is a bit of a hack to pass the raw data back to the caller for writing, without changing the artifact struct
	if githubEnableArtifactWriting {
		unifiedArtifact.RawDataPaths = map[string]string{
			"commits":         string(mustMarshal(commits)),
			"issue_comments":  string(mustMarshal(issueComments)),
			"review_comments": string(mustMarshal(reviewComments)),
			"reviews":         string(mustMarshal(reviews)),
			"diff":            diffText,
		}
	}

	return unifiedArtifact, nil
}

func buildGitHubTimeline(owner, repo string, commits []githubapi.GitHubV2CommitInfo, issueComments []githubapi.GitHubV2CommentInfo, reviewComments []githubapi.GitHubV2ReviewComment, reviews []githubapi.GitHubV2ReviewInfo) []reviewmodel.TimelineItem {
	items := make([]reviewmodel.TimelineItem, 0, len(commits)+len(issueComments)+len(reviewComments)+len(reviews))

	for _, commit := range commits {
		timestamp := selectCommitTimestamp(commit)
		message := strings.TrimSpace(commit.Commit.Message)
		title := message
		if idx := strings.IndexRune(title, '\n'); idx >= 0 {
			title = title[:idx]
		}
		if title == "" {
			title = commit.SHA
		}
		webURL := commit.HTMLURL
		if webURL == "" {
			webURL = fmt.Sprintf("https://github.com/%s/%s/commit/%s", owner, repo, commit.SHA)
		}
		authorInfo := selectCommitAuthorInfo(commit)
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "commit",
			ID:        commit.SHA,
			CreatedAt: timestamp,
			Author:    authorInfo,
			Commit: &reviewmodel.TimelineCommit{
				SHA:     commit.SHA,
				Title:   title,
				Message: message,
				WebURL:  webURL,
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

	for _, review := range reviews {
		body := strings.TrimSpace(review.Body)
		if body == "" {
			continue
		}
		id := fmt.Sprintf("review-%d", review.ID)
		timestamp := parseGitHubTime(review.SubmittedAt)
		items = append(items, reviewmodel.TimelineItem{
			Kind:      "comment",
			ID:        id,
			CreatedAt: timestamp,
			Author:    githubUserToAuthor(review.User),
			Comment: &reviewmodel.TimelineComment{
				NoteID: id,
				Body:   body,
			},
		})
	}

	return items
}

func buildGitHubCommentTree(issueComments []githubapi.GitHubV2CommentInfo, reviewComments []githubapi.GitHubV2ReviewComment, reviews []githubapi.GitHubV2ReviewInfo) reviewmodel.CommentTree {
	nodes := make(map[string]*reviewmodel.CommentNode, len(reviewComments))
	roots := make([]*reviewmodel.CommentNode, 0, len(issueComments)+len(reviewComments)+len(reviews))

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

	for _, review := range reviews {
		body := strings.TrimSpace(review.Body)
		if body == "" {
			continue
		}
		id := fmt.Sprintf("review-%d", review.ID)
		timestamp := parseGitHubTime(review.SubmittedAt)
		roots = append(roots, &reviewmodel.CommentNode{
			ID:        id,
			Author:    githubUserToAuthor(review.User),
			Body:      body,
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
	displayName := strings.TrimSpace(user.Name)
	if displayName == "" {
		displayName = user.Login
	}
	return reviewmodel.AuthorInfo{
		Provider:  "github",
		ID:        user.ID,
		Username:  user.Login,
		Name:      displayName,
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

func selectCommitTimestamp(commit githubapi.GitHubV2CommitInfo) time.Time {
	if ts := parseGitHubTime(commit.Commit.Author.Date); !ts.IsZero() {
		return ts
	}
	if ts := parseGitHubTime(commit.Commit.Committer.Date); !ts.IsZero() {
		return ts
	}
	return time.Time{}
}

func selectCommitAuthorInfo(commit githubapi.GitHubV2CommitInfo) reviewmodel.AuthorInfo {
	if commit.Author != nil {
		name := strings.TrimSpace(commit.Author.Name)
		if name == "" {
			name = commit.Author.Login
		}
		return reviewmodel.AuthorInfo{
			Provider:  "github",
			ID:        commit.Author.ID,
			Username:  commit.Author.Login,
			Name:      name,
			AvatarURL: commit.Author.AvatarURL,
			WebURL:    commit.Author.HTMLURL,
		}
	}
	payload := commit.Commit
	name := firstNonEmptyString(payload.Author.Name, payload.Committer.Name)
	email := firstNonEmptyString(payload.Author.Email, payload.Committer.Email)
	return reviewmodel.AuthorInfo{Provider: "github", Name: name, Email: email}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// =====================================================================================
//
//	CLI-specific functions
//
// =====================================================================================
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

	unifiedArtifact, err := buildGitHubArtifact(owner, name, prID, pat)
	if err != nil {
		return err
	}

	// --- Start of new implementation ---
	if githubEnableArtifactWriting {
		// 1. Create output directories
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "github")
		if err := os.MkdirAll(testDataDir, 0o755); err != nil {
			return fmt.Errorf("create testdata dir: %w", err)
		}

		// 2. Write raw API responses to testdata directory
		rawCommitsPath := filepath.Join(testDataDir, "commits.json")
		if err := os.WriteFile(rawCommitsPath, []byte(unifiedArtifact.RawDataPaths["commits"]), 0644); err != nil {
			return fmt.Errorf("write raw commits: %w", err)
		}

		rawIssueCommentsPath := filepath.Join(testDataDir, "issue_comments.json")
		if err := os.WriteFile(rawIssueCommentsPath, []byte(unifiedArtifact.RawDataPaths["issue_comments"]), 0644); err != nil {
			return fmt.Errorf("write raw issue comments: %w", err)
		}

		rawReviewCommentsPath := filepath.Join(testDataDir, "review_comments.json")
		if err := os.WriteFile(rawReviewCommentsPath, []byte(unifiedArtifact.RawDataPaths["review_comments"]), 0644); err != nil {
			return fmt.Errorf("write raw review comments: %w", err)
		}

		rawReviewsPath := filepath.Join(testDataDir, "reviews.json")
		if err := os.WriteFile(rawReviewsPath, []byte(unifiedArtifact.RawDataPaths["reviews"]), 0644); err != nil {
			return fmt.Errorf("write raw reviews: %w", err)
		}

		rawDiffPath := filepath.Join(testDataDir, "diff.txt")
		if err := os.WriteFile(rawDiffPath, []byte(unifiedArtifact.RawDataPaths["diff"]), 0644); err != nil {
			return fmt.Errorf("write raw diff: %w", err)
		}
		// clear the raw data from the main artifact before writing the final unified file
		unifiedArtifact.RawDataPaths = nil
	}

	if githubEnableArtifactWriting {
		// 4. Write unified artifact to a single file
		unifiedPath := filepath.Join(*outDir, "gh_unified.json")
		if err := writeJSONPretty(unifiedPath, unifiedArtifact); err != nil {
			return fmt.Errorf("write unified artifact: %w", err)
		}

		fmt.Printf("Target PR: %s\n", prURL)
		fmt.Printf("GitHub unified artifact written to %s\n", unifiedPath)
		testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "github")
		fmt.Printf("Raw API responses for testing saved in %s\n", testDataDir)
	}

	fmt.Printf("Summary: timeline_items=%d participants=%d diff_files=%d\n", len(unifiedArtifact.Timeline), len(unifiedArtifact.Participants), len(unifiedArtifact.Diffs))
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
