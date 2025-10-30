package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/livereview/internal/reviewmodel"
)

func WriteJSONPretty(path string, v interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func SortCommentChildren(node *reviewmodel.CommentNode) {
	if len(node.Children) == 0 {
		return
	}
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].CreatedAt.Before(node.Children[j].CreatedAt)
	})
	for _, child := range node.Children {
		SortCommentChildren(child)
	}
}

// ExtractParticipants collects unique authors from timeline items
func ExtractParticipants(timeline []reviewmodel.TimelineItem) []reviewmodel.AuthorInfo {
	seen := make(map[string]reviewmodel.AuthorInfo)

	for _, item := range timeline {
		// Use a composite key to uniquely identify each author
		key := fmt.Sprintf("%s:%s:%s", item.Author.Provider, item.Author.Username, item.Author.Email)
		if _, exists := seen[key]; !exists {
			seen[key] = item.Author
		}
	}

	// Convert map to slice
	participants := make([]reviewmodel.AuthorInfo, 0, len(seen))
	for _, author := range seen {
		participants = append(participants, author)
	}

	// Sort by username for consistent output
	sort.Slice(participants, func(i, j int) bool {
		if participants[i].Username != participants[j].Username {
			return participants[i].Username < participants[j].Username
		}
		return participants[i].Name < participants[j].Name
	})

	return participants
}
