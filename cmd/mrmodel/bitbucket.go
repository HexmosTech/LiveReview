package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/livereview/internal/database"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/bitbucket"
	"github.com/livereview/internal/reviewmodel"
	"github.com/livereview/pkg/shared"
)

func buildBitbucketTimeline(repoURL string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment) []reviewmodel.TimelineItem {
	items := make([]reviewmodel.TimelineItem, 0, len(commits)+len(comments))

	parsedURL, err := url.Parse(repoURL)
	var workspace, repo string
	if err == nil {
		parts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
		if len(parts) >= 2 {
			workspace = parts[0]
			repo = parts[1]
		}
	}

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

	commentMap := make(map[int]*bitbucket.BitbucketComment, len(comments))
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	for i := range comments {
		comment := &comments[i]
		idStr := fmt.Sprintf("%d", comment.ID)
		timestamp := parseBitbucketCommentTime(*comment)
		rootID := resolveBitbucketDiscussionID(comment, commentMap)
		discussion := fmt.Sprintf("%d", rootID)
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

func buildBitbucketCommentTree(comments []bitbucket.BitbucketComment) reviewmodel.CommentTree {
	if len(comments) == 0 {
		return reviewmodel.CommentTree{}
	}

	sort.Slice(comments, func(i, j int) bool {
		return parseBitbucketCommentTime(comments[i]).Before(parseBitbucketCommentTime(comments[j]))
	})

	commentMap := make(map[int]*bitbucket.BitbucketComment, len(comments))
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	nodes := make(map[int]*reviewmodel.CommentNode, len(comments))
	for i := range comments {
		comment := &comments[i]
		node := &reviewmodel.CommentNode{
			ID:           fmt.Sprintf("%d", comment.ID),
			DiscussionID: fmt.Sprintf("%d", resolveBitbucketDiscussionID(comment, commentMap)),
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
			if parent, ok := nodes[comment.Parent.ID]; ok {
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

func bitbucketLineInfo(inline *struct {
	Path string `json:"path"`
	From *int   `json:"from"`
	To   *int   `json:"to"`
}) (lineOld, lineNew int, path string) {
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

func parseBitbucketCommentTime(comment bitbucket.BitbucketComment) time.Time {
	if t := parseBitbucketTime(comment.CreatedOn); !t.IsZero() {
		return t
	}
	if t := parseBitbucketTime(comment.UpdatedOn); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func resolveBitbucketDiscussionID(comment *bitbucket.BitbucketComment, lookup map[int]*bitbucket.BitbucketComment) int {
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

func bitbucketCommitAuthor(commit bitbucket.BitbucketCommit) reviewmodel.AuthorInfo {
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

func bitbucketUserToAuthor(user *bitbucket.BitbucketUser) reviewmodel.AuthorInfo {
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

func selectBitbucketUsername(user *bitbucket.BitbucketUser) string {
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

const (
	defaultBitbucketPRURL = "https://bitbucket.org/contorted/fb_backends/pull-requests/1"
	bbArtifactEnabled     = false
)

func getBitbucketPRIDFromURL(repoURL string) (string, error) {
	_, _, prID, err := bitbucket.ParseBitbucketURL(repoURL)
	if err != nil {
		return "", err
	}
	if prID == "" {
		return "", fmt.Errorf("pull request ID not found in URL: %s", repoURL)
	}
	return prID, nil
}

func findBitbucketCredentialsFromDB() (*shared.VCSCredentials, error) {
	db, err := database.NewDB()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	var creds shared.VCSCredentials
	// Use pat_token column which stores the Personal Access Token
	// For Bitbucket, we'll need to extract email from metadata if needed
	err = db.QueryRow("SELECT pat_token FROM integration_tokens WHERE provider = 'bitbucket' LIMIT 1").Scan(&creds.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to query bitbucket credentials: %w", err)
	}

	// For now, set a default email - in production this should come from metadata or config
	creds.Email = "livereviewbot@gmail.com"
	creds.Provider = "bitbucket"

	return &creds, nil
}

func fetchBitbucketData(provider *bitbucket.BitbucketProvider, prID string, prURL string) (details *providers.MergeRequestDetails, diffs string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment, err error) {
	details, err = provider.GetMergeRequestDetails(context.Background(), prURL)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetMergeRequestDetails failed: %w", err)
	}

	diffs, err = provider.GetPullRequestDiff(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to get MR changes: %w", err)
	}

	commits, err = provider.GetPullRequestCommits(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetPullRequestCommits failed: %w", err)
	}

	comments, err = provider.GetPullRequestComments(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetPullRequestComments failed: %w", err)
	}

	return details, diffs, commits, comments, nil
}

func writeBitbucketArtifacts(outDir string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment, diffs string, unifiedArtifact *UnifiedArtifact) error {
	if !bbArtifactEnabled {
		return nil
	}

	// 1. Create output directories
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "bitbucket")
	if err := os.MkdirAll(testDataDir, 0o755); err != nil {
		return fmt.Errorf("create testdata dir: %w", err)
	}

	// 2. Write raw API responses to testdata directory
	rawCommitsPath := filepath.Join(testDataDir, "commits.json")
	if err := writeJSONPretty(rawCommitsPath, commits); err != nil {
		return fmt.Errorf("write raw commits: %w", err)
	}

	rawCommentsPath := filepath.Join(testDataDir, "comments.json")
	if err := writeJSONPretty(rawCommentsPath, comments); err != nil {
		return fmt.Errorf("write raw comments: %w", err)
	}

	rawDiffPath := filepath.Join(testDataDir, "diff.txt")
	if err := os.WriteFile(rawDiffPath, []byte(diffs), 0644); err != nil {
		return fmt.Errorf("write raw diff: %w", err)
	}

	// 3. Write unified artifact to a single file
	unifiedPath := filepath.Join(outDir, "bb_unified.json")
	if err := writeJSONPretty(unifiedPath, unifiedArtifact); err != nil {
		return fmt.Errorf("write unified artifact: %w", err)
	}

	fmt.Printf("Bitbucket unified artifact written to %s\n", unifiedPath)
	fmt.Printf("Raw API responses for testing saved in %s\n", testDataDir)

	return nil
}

func buildBitbucketUnifiedArtifact(repoURL string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment, diffs string, outDir string) (*UnifiedArtifact, error) {
	timelineItems := buildBitbucketTimeline(repoURL, commits, comments)
	commentTree := buildBitbucketCommentTree(comments)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(diffs)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := &UnifiedArtifact{
		Provider:     "bitbucket",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		Participants: extractParticipants(timelineItems),
	}

	if err := writeBitbucketArtifacts(outDir, commits, comments, diffs, unifiedArtifact); err != nil {
		return nil, err
	}

	return unifiedArtifact, nil
}

func runBitbucket(args []string) error {
	fs := flag.NewFlagSet("bitbucket", flag.ContinueOnError)
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	urlFlag := stringFlag{value: defaultBitbucketPRURL}
	fs.Var(&urlFlag, "url", "Bitbucket pull request URL")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	prURL := strings.TrimSpace(urlFlag.value)
	if prURL == "" {
		return errors.New("must provide a valid --url")
	}

	prID, err := getBitbucketPRIDFromURL(prURL)
	if err != nil {
		return fmt.Errorf("failed to get PR ID from URL: %w", err)
	}

	var provider *bitbucket.BitbucketProvider
	var errProv error

	token := os.Getenv("BITBUCKET_TOKEN")
	email := os.Getenv("BITBUCKET_EMAIL")

	if token != "" && email != "" {
		log.Println("Using Bitbucket credentials from environment variables")
		provider, errProv = bitbucket.NewBitbucketProvider(token, email, prURL)
	} else {
		log.Println("Using Bitbucket credentials from integration_tokens (fallback)")
		creds, err := findBitbucketCredentialsFromDB()
		if err != nil {
			return fmt.Errorf("failed to find bitbucket credentials: %w", err)
		}
		provider, errProv = bitbucket.NewBitbucketProvider(creds.Token, creds.Email, prURL)
	}

	if errProv != nil {
		return fmt.Errorf("bitbucket provider creation failed: %w", errProv)
	}

	details, diffs, commits, comments, err := fetchBitbucketData(provider, prID, prURL)
	if err != nil {
		return err
	}

	unifiedArtifact, err := buildBitbucketUnifiedArtifact(details.RepositoryURL, commits, comments, diffs, *outDir)
	if err != nil {
		return err
	}

	fmt.Printf("Target PR: %s\n", prURL)

	_ = unifiedArtifact
	return nil
}
