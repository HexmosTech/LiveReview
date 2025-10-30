package lib

import (
	"fmt"

	"github.com/livereview/internal/reviewmodel"
)

// FilePath represents a file involved in the diff
type FilePath struct {
	Index   int
	OldPath string
	NewPath string
}

// FileCommentIndex maps file paths to their associated comments
type FileCommentIndex map[string][]*reviewmodel.CommentNode

// ListPaths returns a list of all file paths from the diffs in a UnifiedArtifact
func ListPaths(artifact *UnifiedArtifact) []FilePath {
	var paths []FilePath
	for i, diff := range artifact.Diffs {
		paths = append(paths, FilePath{
			Index:   i + 1,
			OldPath: diff.OldPath,
			NewPath: diff.NewPath,
		})
	}
	return paths
}

// BuildFileCommentIndex creates an efficient lookup from file path to comments
// by iterating through the comment tree once
func BuildFileCommentIndex(artifact *UnifiedArtifact) FileCommentIndex {
	index := make(FileCommentIndex)

	// Iterate through all comment tree roots and their children
	for _, root := range artifact.CommentTree.Roots {
		indexCommentNode(root, index)
	}

	return index
}

// indexCommentNode recursively indexes a comment node and its children
func indexCommentNode(node *reviewmodel.CommentNode, index FileCommentIndex) {
	// If this comment has a file path, add it to the index
	if node.FilePath != "" {
		index[node.FilePath] = append(index[node.FilePath], node)
	}

	// Recursively index children
	for _, child := range node.Children {
		indexCommentNode(child, index)
	}
}

// ShowCommentsPerFile shows the comment hierarchy organized by file
func ShowCommentsPerFile(artifact *UnifiedArtifact) {
	paths := ListPaths(artifact)
	commentIndex := BuildFileCommentIndex(artifact)

	fmt.Println("\n=== Files and Their Comments ===")
	for _, path := range paths {
		fmt.Printf("\n%d. File: %s\n", path.Index, path.NewPath)
		if path.OldPath != path.NewPath {
			fmt.Printf("   (renamed from: %s)\n", path.OldPath)
		}

		// Look up comments for this file from the index
		fileComments := commentIndex[path.NewPath]
		if len(fileComments) == 0 {
			fmt.Println("   No comments on this file")
		} else {
			fmt.Printf("   Comments: %d\n", len(fileComments))
			for _, comment := range fileComments {
				printComment(comment, "   ")
			}
		}
	}
}

// printComment prints a single comment with basic info
func printComment(node *reviewmodel.CommentNode, indent string) {
	author := "unknown"
	if node.Author.Username != "" {
		author = node.Author.Username
	}

	fmt.Printf("%s- [%s] %s\n", indent, author, node.CreatedAt.Format("2006-01-02 15:04:05"))
	if node.FilePath != "" {
		fmt.Printf("%s  File: %s (line %d)\n", indent, node.FilePath, node.LineNew)
	}

	// Show first 80 chars of body
	body := node.Body
	if len(body) > 80 {
		body = body[:77] + "..."
	}
	fmt.Printf("%s  %s\n", indent, body)
}
