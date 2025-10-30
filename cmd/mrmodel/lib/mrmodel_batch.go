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

// FileCommentTree maps file paths to their comment tree roots
type FileCommentTree map[string][]*reviewmodel.CommentNode

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
// by iterating through the comment tree once. Comments without a file path are
// stored under the key "" (empty string) as "general" comments.
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
	// Add to index - use empty string for comments without a file path (general comments)
	filePath := node.FilePath
	if filePath == "" {
		filePath = "" // General comments
	}
	index[filePath] = append(index[filePath], node)

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

	// Show general comments first (comments not associated with any file)
	generalComments := commentIndex[""]
	if len(generalComments) > 0 {
		fmt.Printf("\nGeneral Comments (not associated with specific files): %d\n", len(generalComments))
		for _, comment := range generalComments {
			printComment(comment, "   ")
		}
	}

	// Show comments for each file
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

// BuildFileCommentTree creates a tree structure of comments per file
// It takes the existing comment tree from UnifiedArtifact and organizes roots by file
func BuildFileCommentTree(artifact *UnifiedArtifact) FileCommentTree {
	tree := make(FileCommentTree)

	// Simply organize existing tree roots by their file path
	for _, root := range artifact.CommentTree.Roots {
		filePath := root.FilePath
		if filePath == "" {
			filePath = "" // General comments
		}
		tree[filePath] = append(tree[filePath], root)
	}

	return tree
} // ShowCommentsPerFileTree shows the comment tree hierarchy organized by file
func ShowCommentsPerFileTree(artifact *UnifiedArtifact) {
	paths := ListPaths(artifact)
	commentTree := BuildFileCommentTree(artifact)

	fmt.Println("\n=== Files and Their Comment Trees ===")

	// Show general comments first
	generalRoots := commentTree[""]
	if len(generalRoots) > 0 {
		fmt.Printf("\nGeneral Comments (not associated with specific files): %d thread(s)\n", len(generalRoots))
		for _, root := range generalRoots {
			printCommentTree(root, "   ", 0)
		}
	}

	// Show comments for each file
	for _, path := range paths {
		fmt.Printf("\n%d. File: %s\n", path.Index, path.NewPath)
		if path.OldPath != path.NewPath {
			fmt.Printf("   (renamed from: %s)\n", path.OldPath)
		}

		fileRoots := commentTree[path.NewPath]
		if len(fileRoots) == 0 {
			fmt.Println("   No comments on this file")
		} else {
			fmt.Printf("   Comment threads: %d\n", len(fileRoots))
			for _, root := range fileRoots {
				printCommentTree(root, "   ", 0)
			}
		}
	}
}

// printCommentTree prints a comment node and its children recursively with tree structure
func printCommentTree(node *reviewmodel.CommentNode, indent string, depth int) {
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

	// Print children with more indentation
	for _, child := range node.Children {
		printCommentTree(child, indent+"  ", depth+1)
	}
}

// ValidateFileCommentTree validates that the tree structure is correct
func ValidateFileCommentTree(tree FileCommentTree, artifact *UnifiedArtifact) map[string]interface{} {
	results := make(map[string]interface{})

	// Count total comments in tree
	totalTreeComments := 0
	var countComments func(*reviewmodel.CommentNode) int
	countComments = func(node *reviewmodel.CommentNode) int {
		count := 1
		for _, child := range node.Children {
			count += countComments(child)
		}
		return count
	}

	for _, roots := range tree {
		for _, root := range roots {
			totalTreeComments += countComments(root)
		}
	}

	// Count total comments in original artifact
	totalArtifactComments := 0
	for _, root := range artifact.CommentTree.Roots {
		totalArtifactComments += countComments(root)
	}

	results["total_tree_comments"] = totalTreeComments
	results["total_artifact_comments"] = totalArtifactComments
	results["all_comments_preserved"] = (totalTreeComments == totalArtifactComments)
	results["file_count"] = len(tree)

	// Count threads per file
	threadsPerFile := make(map[string]int)
	for filePath, roots := range tree {
		displayPath := filePath
		if displayPath == "" {
			displayPath = "[General]"
		}
		threadsPerFile[displayPath] = len(roots)
	}
	results["threads_per_file"] = threadsPerFile

	return results
}
