package chatexport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// BookmarkEntry is one entry to register in the PDF's /Outlines sidebar -
// its display text and nesting level (0 = top).
type BookmarkEntry struct {
	Text  string
	Level int
}

var (
	blockStructureRe = regexp.MustCompile("^(#{1,6}\\s|[-*+]\\s|[0-9]+[.)]\\s|>|```|~~~)")
	blankLinesRe      = regexp.MustCompile(`\n{2,}`)
)

// normalizeSoftWraps collapses an embedded newline inside a plain prose
// paragraph into the single space CommonMark says it implies, leaving
// headings/lists/blockquotes/fenced code untouched. Without this, a bare
// "\n" inside assistant text (common in LLM output) renders in the PDF
// with the space at the line-wrap point silently dropped - goldmark-pdf
// v0.4.2's default ast.KindText renderer ignores Text.SoftLineBreak().
// Normalizing here, on the raw text before it ever reaches goldmark,
// avoids needing a renderer-level workaround for it.
func normalizeSoftWraps(md string) string {
	blocks := blankLinesRe.Split(md, -1)
	for i, block := range blocks {
		if blockStructureRe.MatchString(block) || strings.Contains(block, "```") || strings.Contains(block, "~~~") {
			continue
		}
		lines := strings.Split(block, "\n")
		for j, line := range lines {
			lines[j] = strings.TrimRight(line, " \t")
		}
		blocks[i] = strings.Join(lines, " ")
	}
	return strings.Join(blocks, "\n\n")
}

// sanitizeInline neutralizes Markdown link/image bracket syntax in
// arbitrary text (chart titles, file names) so it can't prematurely close
// a `![alt](src)` or `[text](href)` it's interpolated into.
func sanitizeInline(s string) string {
	return strings.NewReplacer("[", "(", "]", ")").Replace(s)
}

func roleLabel(role string) string {
	if role == "user" {
		return "You"
	}
	return "Livi"
}

// prettyJSON reformats raw for readability; on failure it returns raw
// unchanged rather than erroring the whole export over cosmetics.
func prettyJSON(raw []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return raw
	}
	return buf.Bytes()
}

// writeBookmarkSentinel registers text as a bookmark entry at level and
// writes a <!--export-bookmark:N--> sentinel referencing it into b, so
// pdf.go's bookmarkNodeRenderer can call the underlying PDF's Bookmark at
// the right position once this Markdown is actually rendered (the sentinel
// parses as a standalone-line HTML block, which is otherwise a no-op in
// goldmark-pdf).
func writeBookmarkSentinel(b *strings.Builder, bookmarks *[]BookmarkEntry, text string, level int) {
	idx := len(*bookmarks)
	*bookmarks = append(*bookmarks, BookmarkEntry{Text: text, Level: level})
	fmt.Fprintf(b, "<!--export-bookmark:%d-->\n", idx)
}

// ToMarkdown renders doc into a single Markdown document for RenderPDF,
// plus the ordered bookmark entries to register alongside it.
func ToMarkdown(doc *ExportDoc) (string, []BookmarkEntry) {
	var b strings.Builder
	var bookmarks []BookmarkEntry

	writeBookmarkSentinel(&b, &bookmarks, doc.Conversation.Title, 0)
	fmt.Fprintf(&b, "# %s\n\n", doc.Conversation.Title)
	fmt.Fprintf(&b, "*Exported from LiveReview - created %s, updated %s, %d turns*\n\n",
		doc.Conversation.CreatedAt.Format("2006-01-02 15:04"),
		doc.Conversation.UpdatedAt.Format("2006-01-02 15:04"),
		len(doc.Turns),
	)

	for _, turn := range doc.Turns {
		heading := fmt.Sprintf("Turn %d — %s", turn.Seq, roleLabel(turn.Role))
		writeBookmarkSentinel(&b, &bookmarks, heading, 1)
		fmt.Fprintf(&b, "## %s\n\n", heading)

		if strings.TrimSpace(turn.Text) != "" {
			b.WriteString(normalizeSoftWraps(turn.Text))
			b.WriteString("\n\n")
		}

		for _, chart := range turn.Charts {
			title := sanitizeInline(chart.Title)
			if title != "" {
				fmt.Fprintf(&b, "**%s**\n\n", title)
			}
			if chart.Description != "" {
				fmt.Fprintf(&b, "*%s*\n\n", sanitizeInline(chart.Description))
			}
			dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(chart.PNG)
			fmt.Fprintf(&b, "![%s](%s)\n\n", title, dataURI)
		}

		if len(turn.Files) > 0 {
			b.WriteString("| File | Kind | Rows |\n")
			b.WriteString("| --- | --- | --- |\n")
			for _, f := range turn.Files {
				fmt.Fprintf(&b, "| %s | %s | %d |\n", escapeTableCell(f.Filename), escapeTableCell(f.Kind), f.Rows)
			}
			b.WriteString("\n")
		}

		if len(turn.DebugArtifacts) > 0 {
			b.WriteString("**Debug artifacts**\n\n```json\n")
			b.Write(prettyJSON(turn.DebugArtifacts))
			b.WriteString("\n```\n\n")
		}
	}

	return b.String(), bookmarks
}

func escapeTableCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
