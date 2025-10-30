package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/bitbucket"
	rm "github.com/livereview/internal/reviewmodel"
)

// Type aliases for backward compatibility
type LocalParser = lib.LocalParser

// NewLocalParser creates a new LocalParser.
func NewLocalParser() *LocalParser {
	return lib.NewLocalParser()
}

// Wrapper functions for backward compatibility
func writeJSONPretty(path string, v interface{}) error {
	return lib.WriteJSONPretty(path, v)
}

func sortCommentChildren(node *rm.CommentNode) {
	lib.SortCommentChildren(node)
}

func extractParticipants(timeline []rm.TimelineItem) []rm.AuthorInfo {
	return lib.ExtractParticipants(timeline)
}

// MrModelImpl is a struct to hold the mrmodel library implementation.
type MrModelImpl struct {
	EnableArtifactWriting bool
}

// Helpers
func atoi(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}

// Bitbucket-specific methods

func (m *MrModelImpl) fetchBitbucketData(provider interface{}, prID string, prURL string) (details *providers.MergeRequestDetails, diffs string, commits interface{}, comments interface{}, err error) {
	// Type assertion for Bitbucket provider
	bbProvider, ok := provider.(interface {
		GetMergeRequestDetails(ctx context.Context, prURL string) (*providers.MergeRequestDetails, error)
		GetPullRequestDiff(prID string) (string, error)
		GetPullRequestCommits(prID string) (interface{}, error)
		GetPullRequestComments(prID string) (interface{}, error)
	})
	if !ok {
		return nil, "", nil, nil, fmt.Errorf("invalid Bitbucket provider")
	}

	details, err = bbProvider.GetMergeRequestDetails(context.Background(), prURL)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetMergeRequestDetails failed: %w", err)
	}

	diffs, err = bbProvider.GetPullRequestDiff(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to get MR changes: %w", err)
	}

	commits, err = bbProvider.GetPullRequestCommits(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetPullRequestCommits failed: %w", err)
	}

	comments, err = bbProvider.GetPullRequestComments(prID)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("GetPullRequestComments failed: %w", err)
	}

	return details, diffs, commits, comments, nil
}

func (m *MrModelImpl) buildBitbucketArtifact(provider *bitbucket.BitbucketProvider, prID, prURL, outDir string) (*lib.UnifiedArtifact, error) {
	details, diffs, commitsIface, commentsIface, err := m.fetchBitbucketData(provider, prID, prURL)
	if err != nil {
		return nil, err
	}

	commits, ok := commitsIface.([]bitbucket.BitbucketCommit)
	if !ok {
		return nil, fmt.Errorf("invalid commits type")
	}
	comments, ok := commentsIface.([]bitbucket.BitbucketComment)
	if !ok {
		return nil, fmt.Errorf("invalid comments type")
	}

	timelineItems := m.buildBitbucketTimeline(details.RepositoryURL, commits, comments)
	commentTree := m.buildBitbucketCommentTree(comments)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(diffs)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	diffsPtrs := make([]*lib.LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := &lib.UnifiedArtifact{
		Provider:     "bitbucket",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		Participants: extractParticipants(timelineItems),
	}

	if err := m.writeBitbucketArtifacts(outDir, commits, comments, diffs, unifiedArtifact); err != nil {
		return nil, err
	}

	return unifiedArtifact, nil
}

func (m *MrModelImpl) writeBitbucketArtifacts(outDir string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment, diffs string, unifiedArtifact *lib.UnifiedArtifact) error {
	if !m.EnableArtifactWriting {
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

func (m *MrModelImpl) buildBitbucketTimeline(repoURL string, commits []bitbucket.BitbucketCommit, comments []bitbucket.BitbucketComment) []rm.TimelineItem {
	items := make([]rm.TimelineItem, 0, len(commits)+len(comments))

	for _, commit := range commits {
		timestamp := m.parseBitbucketTime(commit.Date)
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
			workspace, repo := m.parseBitbucketRepoURL(repoURL)
			webURL = fmt.Sprintf("https://bitbucket.org/%s/%s/commits/%s", workspace, repo, commit.Hash)
		}
		items = append(items, rm.TimelineItem{
			Kind:      "commit",
			ID:        commit.Hash,
			CreatedAt: timestamp,
			Author:    m.bitbucketCommitAuthor(commit),
			Commit: &rm.TimelineCommit{
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
		timestamp := m.parseBitbucketCommentTime(*comment)
		rootID := m.resolveBitbucketDiscussionID(comment, commentMap)
		discussion := fmt.Sprintf("%d", rootID)
		lineOld, lineNew, filePath := m.bitbucketLineInfo(comment.Inline)
		items = append(items, rm.TimelineItem{
			Kind:      "comment",
			ID:        idStr,
			CreatedAt: timestamp,
			Author:    m.bitbucketUserToAuthor(comment.User),
			Comment: &rm.TimelineComment{
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

func (m *MrModelImpl) buildBitbucketCommentTree(comments []bitbucket.BitbucketComment) rm.CommentTree {
	if len(comments) == 0 {
		return rm.CommentTree{}
	}

	sort.Slice(comments, func(i, j int) bool {
		return m.parseBitbucketCommentTime(comments[i]).Before(m.parseBitbucketCommentTime(comments[j]))
	})

	commentMap := make(map[int]*bitbucket.BitbucketComment, len(comments))
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	nodes := make(map[int]*rm.CommentNode, len(comments))
	for i := range comments {
		comment := &comments[i]
		node := &rm.CommentNode{
			ID:           fmt.Sprintf("%d", comment.ID),
			DiscussionID: fmt.Sprintf("%d", m.resolveBitbucketDiscussionID(comment, commentMap)),
			Author:       m.bitbucketUserToAuthor(comment.User),
			Body:         comment.Content.Raw,
			CreatedAt:    m.parseBitbucketCommentTime(*comment),
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

	roots := make([]*rm.CommentNode, 0, len(comments))
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

	return rm.CommentTree{Roots: roots}
}

func (m *MrModelImpl) bitbucketLineInfo(inline *struct {
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

func (m *MrModelImpl) parseBitbucketCommentTime(comment bitbucket.BitbucketComment) time.Time {
	if t := m.parseBitbucketTime(comment.CreatedOn); !t.IsZero() {
		return t
	}
	if t := m.parseBitbucketTime(comment.UpdatedOn); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func (m *MrModelImpl) resolveBitbucketDiscussionID(comment *bitbucket.BitbucketComment, lookup map[int]*bitbucket.BitbucketComment) int {
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

func (m *MrModelImpl) bitbucketCommitAuthor(commit bitbucket.BitbucketCommit) rm.AuthorInfo {
	if commit.Author.User != nil {
		user := commit.Author.User
		display := strings.TrimSpace(user.DisplayName)
		if display == "" {
			display = user.Username
		}
		username := m.selectBitbucketUsername(user)
		return rm.AuthorInfo{
			Provider: "bitbucket",
			Username: username,
			Name:     display,
			WebURL:   strings.TrimSpace(user.Links.HTML.Href),
		}
	}
	name, email := m.parseBitbucketAuthorRaw(commit.Author.Raw)
	return rm.AuthorInfo{Provider: "bitbucket", Name: name, Email: email}
}

func (m *MrModelImpl) bitbucketUserToAuthor(user *bitbucket.BitbucketUser) rm.AuthorInfo {
	if user == nil {
		return rm.AuthorInfo{Provider: "bitbucket", Name: "system"}
	}
	display := strings.TrimSpace(user.DisplayName)
	if display == "" {
		display = user.Username
	}
	username := m.selectBitbucketUsername(user)
	return rm.AuthorInfo{
		Provider: "bitbucket",
		Username: username,
		Name:     display,
		WebURL:   strings.TrimSpace(user.Links.HTML.Href),
	}
}

func (m *MrModelImpl) selectBitbucketUsername(user *bitbucket.BitbucketUser) string {
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

func (m *MrModelImpl) parseBitbucketAuthorRaw(raw string) (string, string) {
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

func (m *MrModelImpl) parseBitbucketTime(value string) time.Time {
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

func (m *MrModelImpl) parseBitbucketRepoURL(repoURL string) (workspace, repo string) {
	if parsed, err := url.Parse(repoURL); err == nil {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 {
			workspace = parts[0]
			repo = parts[1]
		}
	}
	return
}
