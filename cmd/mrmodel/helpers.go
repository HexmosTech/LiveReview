package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/livereview/internal/reviewmodel"
)

func writeJSONPretty(path string, v interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
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
