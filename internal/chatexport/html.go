package chatexport

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// RenderHTML writes doc as a single self-contained HTML file: charts are
// embedded as base64 PNG data URIs (the same PNGs RenderPDF uses, built
// once by BuildDoc/BuildCompiledDoc) and all CSS is inlined, so the file
// has zero external dependencies and opens correctly straight off disk.
// Each conversation gets its own <section class="conversation">, with a
// two-level TOC (conversation, then indented turn links) when there's more
// than one.
//
// Message text is converted per-turn through goldmark's *default* HTML
// renderer (extension.GFM, no html.WithUnsafe), so any HTML-looking text
// an assistant reply happens to contain is escaped rather than executed -
// an export leaves the app's trust boundary, so raw HTML from LLM output
// must never run in the reader's browser.
func RenderHTML(doc *CompiledDoc, w io.Writer) error {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))

	var toc strings.Builder
	var body strings.Builder

	fmt.Fprintf(&body, "<h1 id=\"top\">%s</h1>\n", html.EscapeString(doc.Title))
	if doc.Subtitle != "" {
		fmt.Fprintf(&body, "<p class=\"subtitle\">%s</p>\n", html.EscapeString(doc.Subtitle))
	}
	fmt.Fprintf(&body, "<p class=\"meta\">Exported from LiveReview &middot; %d conversation(s)</p>\n", len(doc.Conversations))

	multi := len(doc.Conversations) > 1
	for ci, conv := range doc.Conversations {
		convAnchor := fmt.Sprintf("conversation-%d", ci+1)
		convHeading := conv.Conversation.Title
		if multi {
			convHeading = fmt.Sprintf("Conversation %d — %s", ci+1, conv.Conversation.Title)
		}
		fmt.Fprintf(&toc, "<a href=\"#%s\" class=\"toc-conversation\">%s</a>\n", convAnchor, html.EscapeString(convHeading))

		fmt.Fprintf(&body, "<section id=\"%s\" class=\"conversation\">\n<h2>%s</h2>\n", convAnchor, html.EscapeString(convHeading))
		fmt.Fprintf(&body, "<p class=\"meta\">created %s &middot; updated %s &middot; %d turns</p>\n",
			conv.Conversation.CreatedAt.Format("2006-01-02 15:04"),
			conv.Conversation.UpdatedAt.Format("2006-01-02 15:04"),
			len(conv.Turns),
		)

		for _, turn := range conv.Turns {
			anchor := fmt.Sprintf("%s-turn-%d", convAnchor, turn.Seq)
			heading := fmt.Sprintf("Turn %d — %s", turn.Seq, roleLabel(turn.Role))
			fmt.Fprintf(&toc, "<a href=\"#%s\" class=\"toc-turn\">%s</a>\n", anchor, html.EscapeString(heading))

			if err := renderTurnHTML(&body, md, anchor, heading, turn); err != nil {
				return fmt.Errorf("conversation %d: %w", ci+1, err)
			}
		}

		body.WriteString("</section>\n")
	}

	page := fmt.Sprintf(htmlPageTemplate, html.EscapeString(doc.Title), exportCSS, toc.String(), body.String())
	_, err := w.Write([]byte(page))
	return err
}

// renderTurnHTML appends one turn's section (heading, text, charts, files,
// debug artifacts) to body.
func renderTurnHTML(body *strings.Builder, md goldmark.Markdown, anchor, heading string, turn ExportTurn) error {
	fmt.Fprintf(body, "<section id=\"%s\" class=\"turn turn-%s\">\n<h3>%s</h3>\n", anchor, turn.Role, html.EscapeString(heading))

	if strings.TrimSpace(turn.Text) != "" {
		var buf bytes.Buffer
		if err := md.Convert([]byte(normalizeSoftWraps(turn.Text)), &buf); err != nil {
			return fmt.Errorf("render turn text: %w", err)
		}
		body.Write(buf.Bytes())
	}

	for _, chart := range turn.Charts {
		body.WriteString("<figure class=\"chart\">\n")
		fmt.Fprintf(body, "<img src=\"data:image/png;base64,%s\" alt=\"%s\">\n",
			base64.StdEncoding.EncodeToString(chart.PNG), html.EscapeString(chart.Title))
		if chart.Title != "" || chart.Description != "" {
			fmt.Fprintf(body, "<figcaption><strong>%s</strong> %s</figcaption>\n",
				html.EscapeString(chart.Title), html.EscapeString(chart.Description))
		}
		body.WriteString("</figure>\n")
	}

	if len(turn.Files) > 0 {
		body.WriteString("<table class=\"files\"><thead><tr><th>File</th><th>Kind</th><th>Rows</th></tr></thead><tbody>\n")
		for _, f := range turn.Files {
			fmt.Fprintf(body, "<tr><td>%s</td><td>%s</td><td>%d</td></tr>\n",
				html.EscapeString(f.Filename), html.EscapeString(f.Kind), f.Rows)
		}
		body.WriteString("</tbody></table>\n")
	}

	if len(turn.DebugArtifacts) > 0 {
		fmt.Fprintf(body, "<details class=\"debug\"><summary>Debug artifacts</summary><pre>%s</pre></details>\n",
			html.EscapeString(string(prettyJSON(turn.DebugArtifacts))))
	}

	body.WriteString("</section>\n")
	return nil
}

const htmlPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>%s</style>
</head>
<body>
<nav class="toc"><a href="#top" class="toc-title">Contents</a>
%s</nav>
<main>
%s</main>
</body>
</html>
`

const exportCSS = `
:root{color-scheme:light dark;--bg:#fff;--fg:#1a1a1a;--muted:#666;--border:#e2e2e2;--user-bg:#eff6ff;--assistant-bg:#f8fafc;}
@media (prefers-color-scheme:dark){:root{--bg:#111317;--fg:#e6e6e6;--muted:#9aa0a6;--border:#2a2d33;--user-bg:#1b2536;--assistant-bg:#1a1c20;}}
*{box-sizing:border-box;}
body{margin:0;background:var(--bg);color:var(--fg);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;min-height:100vh;}
nav.toc{width:220px;flex-shrink:0;padding:24px 16px;border-right:1px solid var(--border);position:sticky;top:0;align-self:flex-start;height:100vh;overflow-y:auto;}
nav.toc a{display:block;padding:6px 8px;border-radius:6px;color:var(--fg);text-decoration:none;font-size:14px;}
nav.toc a.toc-title{font-weight:700;margin-bottom:8px;color:var(--muted);text-transform:uppercase;font-size:11px;letter-spacing:.05em;}
nav.toc a:hover{background:var(--border);}
main{flex:1;min-width:0;max-width:820px;padding:32px 40px 80px;}
h1{font-size:26px;margin:0 0 4px;}
p.meta{color:var(--muted);font-size:13px;margin:0 0 32px;}
section.turn{border:1px solid var(--border);border-radius:10px;padding:20px 24px;margin-bottom:20px;background:var(--assistant-bg);}
section.turn-user{background:var(--user-bg);}
section.turn h2{font-size:15px;margin:0 0 12px;color:var(--muted);text-transform:uppercase;letter-spacing:.03em;}
section.turn img{max-width:100%;height:auto;border:1px solid var(--border);border-radius:6px;}
figure.chart{margin:16px 0;}
figcaption{font-size:13px;color:var(--muted);margin-top:6px;}
table.files{width:100%;border-collapse:collapse;margin:12px 0;font-size:14px;}
table.files th,table.files td{border:1px solid var(--border);padding:6px 10px;text-align:left;}
details.debug{margin-top:12px;font-size:12px;}
details.debug pre{white-space:pre-wrap;word-break:break-word;background:var(--border);padding:12px;border-radius:6px;overflow-x:auto;}
section.turn table:not(.files){border-collapse:collapse;width:100%;}
section.turn table:not(.files) th,section.turn table:not(.files) td{border:1px solid var(--border);padding:6px 10px;}
@media (max-width:720px){body{flex-direction:column;}nav.toc{width:100%;height:auto;position:relative;border-right:none;border-bottom:1px solid var(--border);}main{padding:24px;}}
`
