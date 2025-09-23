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
			Author: AuthorInfo{
				Name:  firstNonEmpty(c.CommitterName, c.AuthorName),
				Email: firstNonEmpty(c.CommitterEmail, c.AuthorEmail),
			},
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
				Author:    AuthorInfo{ID: n.Author.ID, Username: n.Author.Username, Name: n.Author.Name},
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
				Author:       AuthorInfo{ID: n.Author.ID, Username: n.Author.Username, Name: n.Author.Name},
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

// Export helpers: deduplicate authors and produce compact JSON payloads

// BuildExportTimeline converts TimelineItem slice to ExportTimeline with participants.
func BuildExportTimeline(items []TimelineItem) ExportTimeline {
	// Collect participants map
	m := map[string]Participant{}
	getRef := func(a AuthorInfo) string {
		if a.ID != 0 {
			return fmt.Sprintf("u:gitlab:%d", a.ID)
		}
		if a.Username != "" {
			return "u:" + a.Username
		}
		if a.Email != "" {
			return "e:" + a.Email
		}
		if a.Name != "" {
			return "n:" + a.Name
		}
		return "u:unknown"
	}

	out := make([]ExportTimelineItem, 0, len(items))
	for _, it := range items {
		ref := getRef(it.Author)
		if _, ok := m[ref]; !ok {
			m[ref] = Participant{
				Ref:       ref,
				ID:        it.Author.ID,
				Username:  it.Author.Username,
				Name:      it.Author.Name,
				Email:     it.Author.Email,
				AvatarURL: it.Author.AvatarURL,
				WebURL:    it.Author.WebURL,
			}
		}
		out = append(out, ExportTimelineItem{
			Kind:      it.Kind,
			ID:        it.ID,
			CreatedAt: it.CreatedAt,
			AuthorRef: ref,
			Commit:    it.Commit,
			Comment:   it.Comment,
		})
	}
	// Flatten participants
	parts := make([]Participant, 0, len(m))
	for _, p := range m {
		parts = append(parts, p)
	}
	// Stable order is optional; skip sorting for now
	return ExportTimeline{Participants: parts, Items: out}
}

// BuildExportCommentTree converts CommentTree to ExportCommentTree with participants and author_refs.
func BuildExportCommentTree(tree CommentTree) ExportCommentTree {
	m := map[string]Participant{}
	getRef := func(a AuthorInfo) string {
		if a.ID != 0 {
			return fmt.Sprintf("u:gitlab:%d", a.ID)
		}
		if a.Username != "" {
			return "u:" + a.Username
		}
		if a.Email != "" {
			return "e:" + a.Email
		}
		if a.Name != "" {
			return "n:" + a.Name
		}
		return "u:unknown"
	}

	var convert func(n *CommentNode) *ExportCommentNode
	convert = func(n *CommentNode) *ExportCommentNode {
		ref := getRef(n.Author)
		if _, ok := m[ref]; !ok {
			m[ref] = Participant{
				Ref:       ref,
				ID:        n.Author.ID,
				Username:  n.Author.Username,
				Name:      n.Author.Name,
				Email:     n.Author.Email,
				AvatarURL: n.Author.AvatarURL,
				WebURL:    n.Author.WebURL,
			}
		}
		out := &ExportCommentNode{
			ID:           n.ID,
			DiscussionID: n.DiscussionID,
			ParentID:     n.ParentID,
			AuthorRef:    ref,
			Body:         n.Body,
			CreatedAt:    n.CreatedAt,
			FilePath:     n.FilePath,
			LineOld:      n.LineOld,
			LineNew:      n.LineNew,
		}
		if len(n.Children) > 0 {
			out.Children = make([]*ExportCommentNode, 0, len(n.Children))
			for _, ch := range n.Children {
				out.Children = append(out.Children, convert(ch))
			}
		}
		return out
	}

	roots := make([]*ExportCommentNode, 0, len(tree.Roots))
	for _, r := range tree.Roots {
		roots = append(roots, convert(r))
	}

	parts := make([]Participant, 0, len(m))
	for _, p := range m {
		parts = append(parts, p)
	}
	return ExportCommentTree{Participants: parts, Roots: roots}
}
