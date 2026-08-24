package chatexport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// BookmarkEntry is one entry to register in the PDF's /Outlines sidebar -
// its display text and nesting level (0 = top).
type BookmarkEntry struct {
	Text  string
	Level int
}

var (
	blockStructureRe = regexp.MustCompile(`^(#{1,6}\s|[-*+]\s|[0-9]+[.)]\s|>|` + "```" + `|~~~|\|)`)
	blankLinesRe     = regexp.MustCompile(`\n{2,}`)
)

// normalizeSoftWraps collapses an embedded newline inside a plain prose
// paragraph into the single space CommonMark says it implies, leaving
// headings/lists/blockquotes/tables/fenced code untouched. Without this, a
// bare "\n" inside assistant text (common in LLM output) renders in the PDF
// with the space at the line-wrap point silently dropped - goldmark-pdf
// v0.4.2's default ast.KindText renderer ignores Text.SoftLineBreak().
// Normalizing here, on the raw text before it ever reaches goldmark, avoids
// needing a renderer-level workaround for it.
//
// A block is left untouched if ANY of its lines look structural, not just
// its first: LLM output very often opens a list or table with a plain
// lead-in line ("Key findings:\n- foo\n- bar"), and checking only the first
// line would still collapse that whole block into one run-on paragraph.
func normalizeSoftWraps(md string) string {
	blocks := blankLinesRe.Split(md, -1)
	for i, block := range blocks {
		if blockHasStructure(block) {
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

func blockHasStructure(block string) bool {
	if strings.Contains(block, "```") || strings.Contains(block, "~~~") {
		return true
	}
	for _, line := range strings.Split(block, "\n") {
		if blockStructureRe.MatchString(line) {
			return true
		}
	}
	return false
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

// turnKicker is the small, de-emphasized caption shown above each turn's
// content - role plus a time, never the word "Turn" or its sequence number,
// which repeat on every single turn and would otherwise be the most visually
// prominent thing on the page for no reason (the content is what changes).
func turnKicker(turn ExportTurn) string {
	if turn.CreatedAt.IsZero() {
		return roleLabel(turn.Role)
	}
	return fmt.Sprintf("%s  ·  %s", roleLabel(turn.Role), turn.CreatedAt.Format("15:04"))
}

// blockquoteWrap prefixes every line of text (including blank ones, with a
// lone ">") so a multi-paragraph message survives as a single Markdown
// blockquote instead of being split into several by CommonMark's
// blank-line-ends-blockquote rule. Used for user turns so a reader can tell
// at a glance, without reading a role label, which lines are the question
// being asked - both renderers give blockquotes a distinct, indented
// treatment already.
func blockquoteWrap(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + line
		}
	}
	return strings.Join(lines, "\n")
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

// pageBreakSentinel is recognized by pdf.go's bookmarkNodeRenderer as a
// real page break (Pdf.AddPage()), emitted between conversations in a
// compiled export so each one starts on its own page.
const pageBreakSentinel = "<!--export-pagebreak-->\n"

// writeTurn renders one turn (kicker, text, charts, files, debug artifacts)
// into b, registering a descriptively-named bookmark ("Turn N — Role", for
// the PDF outline sidebar only) at level, shared by both a
// single-conversation export and each conversation's turns inside a
// compiled one. The in-body caption is deliberately smaller and quieter
// than a heading (see turnKicker) - the outline is where "Turn N" earns its
// keep, not the page itself.
func writeTurn(b *strings.Builder, bookmarks *[]BookmarkEntry, turn ExportTurn, level int) {
	bookmarkText := fmt.Sprintf("Turn %d — %s", turn.Seq, roleLabel(turn.Role))
	writeBookmarkSentinel(b, bookmarks, bookmarkText, level)
	fmt.Fprintf(b, "###### %s\n\n", turnKicker(turn))

	if strings.TrimSpace(turn.Text) != "" {
		text := normalizeSoftWraps(turn.Text)
		if turn.Role == "user" {
			text = blockquoteWrap(text)
		}
		b.WriteString(text)
		b.WriteString("\n\n")
	}

	for _, chart := range turn.Charts {
		title := sanitizeInline(chart.Title)
		if title != "" {
			fmt.Fprintf(b, "**%s**\n\n", title)
		}
		if chart.Description != "" {
			fmt.Fprintf(b, "*%s*\n\n", sanitizeInline(chart.Description))
		}
		if len(chart.Stats) > 0 {
			// A bullet list, not trailing-space hard breaks: goldmark-pdf
			// doesn't reliably honor a "  \n" hard break within a single
			// paragraph (it rendered every stat run together on one line),
			// while a list is its own block per item in both PDF and HTML.
			for _, stat := range chart.Stats {
				fmt.Fprintf(b, "- **%s:** %s\n", sanitizeInline(stat.Label), sanitizeInline(stat.Value))
			}
			b.WriteString("\n")
		}
		dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(chart.PNG)
		// Alt text deliberately empty: goldmark-pdf's image node renderer
		// doesn't skip walking its children after placing the image (it
		// returns ast.WalkContinue, not WalkSkipChildren), so a non-empty
		// alt here would render a second time as plain body text right
		// below the image - on top of the title/description already
		// written above it.
		fmt.Fprintf(b, "![](%s)\n\n", dataURI)
	}

	if len(turn.Files) > 0 {
		b.WriteString("| File | Kind | Rows |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, f := range turn.Files {
			fmt.Fprintf(b, "| %s | %s | %d |\n", escapeTableCell(f.Filename), escapeTableCell(f.Kind), f.Rows)
		}
		b.WriteString("\n")
	}

	if len(turn.DebugArtifacts) > 0 {
		b.WriteString("**Debug artifacts**\n\n```json\n")
		b.Write(prettyJSON(turn.DebugArtifacts))
		b.WriteString("\n```\n\n")
	}
}

// ToMarkdown renders doc into a single Markdown document for RenderPDF,
// plus the ordered bookmark entries to register alongside it. The title
// (H1) and subtitle (H3, deliberately not H2 - see the Styles palette in
// pdf.go - so it reads as clearly secondary rather than a second title) sit
// on their own cover block, followed by a thin rule. A single-conversation
// export is doc.Conversations[0] with no separate conversation heading
// (doc.Title already names it - a second identical heading would just be
// noise); a multi-conversation compiled export gets an H2 heading and its
// own bookmark per conversation, with a page break between each.
func ToMarkdown(doc *CompiledDoc) (string, []BookmarkEntry) {
	var b strings.Builder
	var bookmarks []BookmarkEntry

	writeBookmarkSentinel(&b, &bookmarks, doc.Title, 0)
	fmt.Fprintf(&b, "# %s\n\n", doc.Title)
	if doc.Subtitle != "" {
		fmt.Fprintf(&b, "### %s\n\n", doc.Subtitle)
	}

	multi := len(doc.Conversations) > 1
	generated := time.Now().Format("2006-01-02 15:04")
	if multi {
		fmt.Fprintf(&b, "###### Generated %s  ·  %d conversations\n\n", generated, len(doc.Conversations))
	} else if len(doc.Conversations) == 1 {
		conv := doc.Conversations[0]
		fmt.Fprintf(&b, "###### Generated %s  ·  %d turns  ·  updated %s\n\n",
			generated, len(conv.Turns), conv.Conversation.UpdatedAt.Format("2006-01-02 15:04"))
	}
	b.WriteString("---\n\n")

	for i, conv := range doc.Conversations {
		if i > 0 {
			b.WriteString(pageBreakSentinel)
		}

		if multi {
			heading := fmt.Sprintf("Conversation %d — %s", i+1, conv.Conversation.Title)
			writeBookmarkSentinel(&b, &bookmarks, heading, 1)
			fmt.Fprintf(&b, "## %s\n\n", heading)
			fmt.Fprintf(&b, "###### %d turns  ·  updated %s\n\n",
				len(conv.Turns), conv.Conversation.UpdatedAt.Format("2006-01-02 15:04"))
		} else {
			writeBookmarkSentinel(&b, &bookmarks, conv.Conversation.Title, 1)
		}

		for _, turn := range conv.Turns {
			writeTurn(&b, &bookmarks, turn, 2)
		}
	}

	return b.String(), bookmarks
}

func escapeTableCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
