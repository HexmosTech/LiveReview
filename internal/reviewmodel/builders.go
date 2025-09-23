package reviewmodel

import (
	"fmt"
	"sort"
	"time"

	gl "github.com/livereview/internal/providers/gitlab"
)

// BuildTimeline merges commits and discussions into a single chronological sequence.
// Inputs are raw GitLab API structs; output is provider-agnostic TimelineItem slice.
func BuildTimeline(commits []gl.GitLabCommit, discussions []gl.GitLabDiscussion) []TimelineItem {
	items := make([]TimelineItem, 0, len(commits)+len(discussions)*2)

	// Map commits
	for _, c := range commits {
		t := parseTimeOrZero(c.CommittedDate, c.AuthoredDate)
		items = append(items, TimelineItem{
			Kind:      "commit",
			ID:        c.ID,
			CreatedAt: t,
			Author:    firstNonEmpty(c.CommitterName, c.AuthorName),
			Commit: &TimelineCommit{
				SHA:     c.ID,
				Title:   c.Title,
				Message: c.Message,
				WebURL:  c.WebURL,
			},
		})
	}

	// Map notes (each note is a separate timeline item; system notes included)
	for _, d := range discussions {
		for _, n := range d.Notes {
			t := parseTimeOrZero(n.CreatedAt, n.UpdatedAt)
			// Optional file context
			var fp string
			var oldL, newL int
			if n.Position != nil {
				fp = firstNonEmpty(n.Position.NewPath, n.Position.OldPath)
				oldL = n.Position.OldLine
				newL = n.Position.NewLine
			}
			items = append(items, TimelineItem{
				Kind:      "comment",
				ID:        toID(n.ID),
				CreatedAt: t,
				Author:    n.Author.Name,
				Comment: &TimelineComment{
					NoteID:       toID(n.ID),
					Discussion:   d.ID,
					Body:         n.Body,
					IsSystem:     n.System,
					IsResolvable: n.Resolvable,
					Resolved:     n.Resolved,
					FilePath:     fp,
					LineOld:      oldL,
					LineNew:      newL,
				},
			})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items
}

// BuildCommentTree constructs a nested thread tree from discussions/notes.
// GitLab discussions provide ordered notes; we create a root per discussion and chain replies.
func BuildCommentTree(discussions []gl.GitLabDiscussion) CommentTree {
	roots := make([]*CommentNode, 0, len(discussions))
	for _, d := range discussions {
		var root *CommentNode
		var last *CommentNode
		for idx, n := range d.Notes {
			t := parseTimeOrZero(n.CreatedAt, n.UpdatedAt)
			var fp string
			var oldL, newL int
			if n.Position != nil {
				fp = firstNonEmpty(n.Position.NewPath, n.Position.OldPath)
				oldL = n.Position.OldLine
				newL = n.Position.NewLine
			}
			node := &CommentNode{
				ID:           toID(n.ID),
				DiscussionID: d.ID,
				Author:       n.Author.Name,
				Body:         n.Body,
				CreatedAt:    t,
				FilePath:     fp,
				LineOld:      oldL,
				LineNew:      newL,
			}
			if idx == 0 || d.IndividualNote {
				root = node
				last = node
			} else {
				node.ParentID = last.ID
				last.Children = append(last.Children, node)
				last = node
			}
		}
		if root != nil {
			roots = append(roots, root)
		}
	}
	return CommentTree{Roots: roots}
}

func parseTimeOrZero(primary string, fallback string) time.Time {
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05Z07:00"}
	for _, s := range []string{primary, fallback} {
		if s == "" {
			continue
		}
		for _, l := range layouts {
			if t, err := time.Parse(l, s); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func toID(i int) string { return fmtInt(i) }

func fmtInt(i int) string {
	// small helper to avoid pulling strconv all over
	// use simple conversion
	return fmt.Sprintf("%d", i)
}
