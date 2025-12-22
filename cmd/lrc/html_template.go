package main

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"

	"github.com/livereview/internal/naming"
)

// HTMLTemplateData contains all data needed for the HTML template
type HTMLTemplateData struct {
	GeneratedTime string
	Summary       string
	TotalFiles    int
	TotalComments int
	Files         []HTMLFileData
	HasSummary    bool
	FriendlyName  string
}

// HTMLFileData represents a file for HTML rendering
type HTMLFileData struct {
	ID           string
	FilePath     string
	HasComments  bool
	CommentCount int
	Hunks        []HTMLHunkData
}

// HTMLHunkData represents a hunk for HTML rendering
type HTMLHunkData struct {
	Header string
	Lines  []HTMLLineData
}

// HTMLLineData represents a line in a diff
type HTMLLineData struct {
	OldNum    string
	NewNum    string
	Content   string
	Class     string
	IsComment bool
	Comments  []HTMLCommentData
}

// HTMLCommentData represents a comment for HTML rendering
type HTMLCommentData struct {
	Severity    string
	BadgeClass  string
	Category    string
	Content     string
	HasCategory bool
}

// prepareHTMLData converts the API response to template data
func prepareHTMLData(result *diffReviewResponse) *HTMLTemplateData {
	totalComments := countTotalComments(result.Files)

	files := make([]HTMLFileData, len(result.Files))
	for i, file := range result.Files {
		files[i] = prepareFileData(file)
	}

	return &HTMLTemplateData{
		GeneratedTime: time.Now().Format("2006-01-02 15:04:05 MST"),
		Summary:       result.Summary,
		TotalFiles:    len(result.Files),
		TotalComments: totalComments,
		Files:         files,
		HasSummary:    result.Summary != "",
		FriendlyName:  naming.GenerateFriendlyName(),
	}
}

// prepareFileData converts a file result to HTML file data
func prepareFileData(file diffReviewFileResult) HTMLFileData {
	fileID := strings.ReplaceAll(file.FilePath, "/", "_")
	hasComments := len(file.Comments) > 0

	// Create comment lookup map
	commentsByLine := make(map[int][]diffReviewComment)
	for _, comment := range file.Comments {
		commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
	}

	// Process hunks
	hunks := make([]HTMLHunkData, len(file.Hunks))
	for i, hunk := range file.Hunks {
		hunks[i] = prepareHunkData(hunk, commentsByLine)
	}

	return HTMLFileData{
		ID:           fileID,
		FilePath:     file.FilePath,
		HasComments:  hasComments,
		CommentCount: len(file.Comments),
		Hunks:        hunks,
	}
}

// prepareHunkData converts a hunk to HTML hunk data
func prepareHunkData(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment) HTMLHunkData {
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount)

	lines := parseHunkLines(hunk, commentsByLine)

	return HTMLHunkData{
		Header: header,
		Lines:  lines,
	}
}

// parseHunkLines parses hunk content into lines with comments
func parseHunkLines(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment) []HTMLLineData {
	contentLines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	var result []HTMLLineData

	for _, line := range contentLines {
		if len(line) == 0 || strings.HasPrefix(line, "@@") {
			continue
		}

		var lineData HTMLLineData

		if strings.HasPrefix(line, "-") {
			lineData = HTMLLineData{
				OldNum:  fmt.Sprintf("%d", oldLine),
				NewNum:  "",
				Content: html.EscapeString(line),
				Class:   "diff-del",
			}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			lineData = HTMLLineData{
				OldNum:  "",
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: html.EscapeString(line),
				Class:   "diff-add",
			}

			// Check for comments on this line
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				lineData.IsComment = true
				lineData.Comments = prepareComments(comments)
			}

			newLine++
		} else {
			lineData = HTMLLineData{
				OldNum:  fmt.Sprintf("%d", oldLine),
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: html.EscapeString(" " + line),
				Class:   "diff-context",
			}
			oldLine++
			newLine++
		}

		result = append(result, lineData)
	}

	return result
}

// prepareComments converts comments to HTML comment data
func prepareComments(comments []diffReviewComment) []HTMLCommentData {
	result := make([]HTMLCommentData, len(comments))

	for i, comment := range comments {
		severity := strings.ToLower(comment.Severity)
		if severity == "" {
			severity = "info"
		}

		badgeClass := "badge-" + severity
		if severity != "info" && severity != "warning" && severity != "error" {
			badgeClass = "badge-info"
		}

		result[i] = HTMLCommentData{
			Severity:    strings.ToUpper(severity),
			BadgeClass:  badgeClass,
			Category:    comment.Category,
			Content:     html.EscapeString(comment.Content),
			HasCategory: comment.Category != "",
		}
	}

	return result
}

// renderHTMLTemplate renders the HTML using templates
func renderHTMLTemplate(data *HTMLTemplateData) (string, error) {
	tmpl := template.Must(template.New("html").Parse(htmlTemplate))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

var friendlyAdjectives = []string{
	"aurora",
	"bold",
	"calm",
	"daring",
	"elegant",
	"fearless",
	"golden",
	"luminous",
	"magnetic",
	"nova",
	"radiant",
	"serene",
	"stellar",
	"swift",
	"vivid",
}

var friendlyNouns = []string{
	"atlas",
	"cascade",
	"comet",
	"harbor",
	"horizon",
	"lantern",
	"mesa",
	"orchid",
	"peak",
	"pulse",
	"quartz",
	"signal",
	"sparrow",
	"summit",
	"voyage",
}

// htmlTemplate is the main HTML template
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LiveReview Results{{if .FriendlyName}} — {{.FriendlyName}}{{end}}</title>
    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <script>
        // Define functions early so onclick handlers can reference them
        function toggleFile(fileId) {
            const file = document.getElementById(fileId);
            if (file.classList.contains('expanded')) {
                file.classList.remove('expanded');
                file.classList.add('collapsed');
            } else {
                file.classList.remove('collapsed');
                file.classList.add('expanded');
            }
        }

        let allExpanded = false;
        function toggleAll() {
            const files = document.querySelectorAll('.file');
            const button = document.querySelector('.expand-all');
            if (allExpanded) {
                files.forEach(f => {
                    f.classList.remove('expanded');
                    f.classList.add('collapsed');
                });
                button.textContent = 'Expand All Files';
                allExpanded = false;
            } else {
                files.forEach(f => {
                    f.classList.remove('collapsed');
                    f.classList.add('expanded');
                });
                button.textContent = 'Collapse All Files';
                allExpanded = true;
            }
        }
    </script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        :root {
            --space-xs: 6px;
            --space-sm: 10px;
            --space-md: 16px;
            --space-lg: 24px;
            --space-xl: 32px;
        }
        body {
            font-family: "Inter", "Segoe UI", sans-serif;
            font-size: 14px;
            line-height: 1.5;
            color: #e5e7eb;
            background: radial-gradient(circle at 20% 20%, rgba(59,130,246,0.08), transparent 35%),
                        radial-gradient(circle at 80% 0%, rgba(94,234,212,0.07), transparent 40%),
                        #0b1220;
            display: flex;
            gap: var(--space-lg);
            height: 100vh;
            padding: var(--space-lg);
            overflow: hidden;
        }
        .sidebar {
            width: 300px;
            background: #0f172a;
            border-right: 1px solid rgba(255,255,255,0.06);
            display: flex;
            flex-direction: column;
            overflow: hidden;
            box-shadow: 4px 0 12px rgba(0,0,0,0.35);
            border-radius: 18px;
        }
        .sidebar-header {
            padding: 16px;
            background: linear-gradient(90deg, rgba(59,130,246,0.15), rgba(59,130,246,0));
            border-bottom: 1px solid rgba(255,255,255,0.08);
        }
        .sidebar-header h2 {
            font-size: 14px;
            font-weight: 700;
            color: #e5e7eb;
            margin-bottom: 4px;
        }
        .sidebar-stats {
            font-size: 12px;
            color: #9ca3af;
        }
        .sidebar-content {
            flex: 1;
            overflow-y: auto;
            padding: 8px 0;
        }
        .sidebar-file {
            padding: 10px 16px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            border-left: 3px solid transparent;
            transition: background 0.15s ease, border-color 0.15s ease;
            color: #cbd5e1;
        }
        .sidebar-file:hover {
            background: rgba(255,255,255,0.03);
        }
        .sidebar-file.active {
            background: rgba(59,130,246,0.12);
            border-left-color: #60a5fa;
            color: #f8fafc;
        }
        .sidebar-file-name {
            font-family: "JetBrains Mono", "SFMono-Regular", ui-monospace, monospace;
            font-size: 12px;
            flex: 1;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        .sidebar-file-badge {
            background: linear-gradient(135deg, #60a5fa, #3b82f6);
            color: #0b1220;
            padding: 3px 7px;
            border-radius: 12px;
            font-size: 11px;
            font-weight: 700;
        }
        .main-content {
            flex: 1;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
            background: #0b1220;
            border-radius: 20px;
            padding-top: var(--space-lg);
            padding-bottom: var(--space-xl);
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            width: 100%;
            padding: 0 var(--space-xl);
            display: flex;
            flex-direction: column;
            gap: var(--space-lg);
        }
        .header {
            padding: var(--space-md) var(--space-lg);
            background: rgba(15,23,42,0.9);
            border-bottom: 1px solid rgba(255,255,255,0.08);
            border-radius: 18px;
            margin-bottom: var(--space-lg);
            box-shadow: 0 10px 30px rgba(0,0,0,0.4);
            backdrop-filter: blur(6px);
        }
        .brand {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .logo-wrap {
            width: 42px;
            height: 42px;
            border-radius: 12px;
            background: rgba(59,130,246,0.15);
            display: grid;
            place-items: center;
            border: 1px solid rgba(96,165,250,0.5);
            box-shadow: 0 10px 25px rgba(59,130,246,0.2);
            overflow: hidden;
        }
        .logo-wrap img {
            width: 32px;
            height: 32px;
            display: block;
        }
        .brand-text h1 { font-size: 22px; font-weight: 700; margin-bottom: 4px; color: #f8fafc; }
        .brand-text .meta { color: #9ca3af; font-size: 12px; margin-bottom: 6px; }
        .run-name-pill {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 4px 12px;
            border-radius: 999px;
            background: rgba(59,130,246,0.15);
            color: #bfdbfe;
            font-size: 12px;
            font-weight: 700;
            border: 1px solid rgba(96,165,250,0.35);
            width: fit-content;
        }
        .run-name-pill .dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: #60a5fa;
            display: inline-block;
            box-shadow: 0 0 6px rgba(96,165,250,0.6);
        }
        .summary {
            padding: var(--space-lg);
            background: rgba(255,255,255,0.02);
            border: 1px solid rgba(255,255,255,0.08);
            border-radius: 18px;
            backdrop-filter: blur(4px);
        }
        .summary h1 { font-size: 18px; font-weight: 700; margin-bottom: 12px; margin-top: 16px; color: #f8fafc; }
        .summary h1:first-child { margin-top: 0; }
        .summary h2 { font-size: 16px; font-weight: 700; margin-bottom: 10px; margin-top: 14px; color: #e5e7eb; }
        .summary h3 { font-size: 14px; font-weight: 700; margin-bottom: 8px; margin-top: 12px; color: #cbd5e1; }
        .summary p { margin-bottom: 8px; color: #e5e7eb; }
        .summary ul, .summary ol { margin-left: 20px; margin-bottom: 8px; color: #e5e7eb; }
        .summary code {
            background: rgba(96,165,250,0.15);
            padding: 2px 6px;
            border-radius: 4px;
            font-family: "JetBrains Mono", "SFMono-Regular", ui-monospace, monospace;
            font-size: 12px;
            color: #bfdbfe;
        }
        .summary pre {
            background: rgba(15,23,42,0.8);
            padding: 12px;
            border-radius: 8px;
            overflow-x: auto;
            margin-bottom: 8px;
            border: 1px solid rgba(255,255,255,0.05);
        }
        .summary pre code {
            background: none;
            padding: 0;
        }
        .summary strong { font-weight: 700; color: #f8fafc; }
        .stats {
            padding: var(--space-md) var(--space-lg);
            background: rgba(255,255,255,0.02);
            border: 1px solid rgba(255,255,255,0.06);
            border-radius: 16px;
            display: flex;
            gap: var(--space-lg);
            font-size: 13px;
            color: #cbd5e1;
        }
        .stats .stat { font-weight: 700; }
        .stats .stat .count { color: #60a5fa; }
        .file {
            border: 1px solid rgba(255,255,255,0.06);
            background: rgba(17,24,39,0.75);
            border-radius: 16px;
            overflow: hidden;
        }
        .file + .file { margin-top: var(--space-md); }
        .file-header {
            padding: 12px 20px;
            background: rgba(255,255,255,0.02);
            border-bottom: 1px solid rgba(255,255,255,0.05);
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            transition: background 0.15s ease;
        }
        .file-header:hover { background: rgba(96,165,250,0.08); }
        .file-header .filename {
            font-family: "JetBrains Mono", "SFMono-Regular", ui-monospace, monospace;
            font-weight: 700;
            flex: 1;
            color: #e5e7eb;
        }
        .file-header .comment-count {
            background: linear-gradient(135deg, #22d3ee, #60a5fa);
            color: #0b1220;
            padding: 3px 9px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 800;
            box-shadow: 0 4px 12px rgba(34,211,238,0.25);
        }
        .file-header .toggle { font-size: 12px; color: #94a3b8; }
        .file-content { display: none; }
        .file.expanded .file-content { display: block; }
        .file.expanded .file-header .toggle::before { content: "▼ "; }
        .file.collapsed .file-header .toggle::before { content: "▶ "; }
        .diff-table {
            width: 100%;
            border-collapse: collapse;
            font-family: "JetBrains Mono", "SFMono-Regular", ui-monospace, monospace;
            font-size: 12px;
            table-layout: auto;
        }
        .diff-table td {
            padding: 0 3px;
            border: none;
            vertical-align: top;
            overflow: hidden;
        }
        .diff-line { background: rgba(15,23,42,0.6); }
        .diff-line:hover { background: rgba(96,165,250,0.06); }
        .line-num {
            width: 45px;
            min-width: 45px;
            max-width: 45px;
            color: #94a3b8;
            text-align: right;
            user-select: none;
            padding: 2px 6px;
            background: rgba(255,255,255,0.02);
            white-space: nowrap;
            font-size: 11px;
        }
        .line-content {
            white-space: pre-wrap;
            word-break: break-word;
            overflow-wrap: anywhere;
            padding-left: 6px;
            width: auto;
            color: #e5e7eb;
        }
        .diff-add { background: rgba(34,197,94,0.12); }
        .diff-add .line-content { background: rgba(34,197,94,0.12); color: #bbf7d0; }
        .diff-del { background: rgba(239,68,68,0.12); }
        .diff-del .line-content { background: rgba(239,68,68,0.12); color: #fecdd3; }
        .diff-context .line-content { background: rgba(15,23,42,0.6); }
        .comment-row {
            background: rgba(59,130,246,0.05);
            border-top: 1px solid rgba(59,130,246,0.25);
            border-bottom: 1px solid rgba(59,130,246,0.25);
        }
        .comment-row td {
            padding-left: calc(45px * 2 + 12px);
            padding-right: 0;
        }
        .comment-container {
            padding: 12px 14px;
            margin: 10px 12px 10px 0;
            max-width: calc(100% - 24px);
            background: rgba(15,23,42,0.9);
            border: 1px solid rgba(59,130,246,0.3);
            border-radius: 8px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.35);
        }
        .comment-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
            font-weight: 700;
            color: #e5e7eb;
        }
        .comment-badge {
            padding: 3px 10px;
            border-radius: 999px;
            font-size: 11px;
            font-weight: 800;
            text-transform: uppercase;
        }
        .badge-info { background: rgba(96,165,250,0.2); color: #bfdbfe; border: 1px solid rgba(96,165,250,0.35); }
        .badge-warning { background: rgba(234,179,8,0.2); color: #fef08a; border: 1px solid rgba(234,179,8,0.35); }
        .badge-error { background: rgba(239,68,68,0.2); color: #fecaca; border: 1px solid rgba(239,68,68,0.35); }
        .comment-category {
            color: #cbd5e1;
            font-size: 12px;
            font-weight: 500;
        }
        .comment-body {
            color: #e5e7eb;
            line-height: 1.6;
            white-space: pre-wrap;
        }
        .hunk-header {
            background: rgba(255,255,255,0.03);
            color: #94a3b8;
            padding: 6px 10px;
            font-weight: 700;
            border-top: 1px solid rgba(255,255,255,0.06);
            border-bottom: 1px solid rgba(255,255,255,0.06);
        }
        .footer {
            padding: var(--space-md) var(--space-lg);
            text-align: center;
            color: #9ca3af;
            font-size: 12px;
            background: rgba(255,255,255,0.02);
            border: 1px solid rgba(255,255,255,0.06);
            border-radius: 16px;
        }
        .expand-all {
            padding: var(--space-sm) var(--space-lg);
            margin: 0;
            align-self: flex-start;
            background: linear-gradient(135deg, #22d3ee, #60a5fa);
            color: #0b1220;
            border: none;
            border-radius: 10px;
            cursor: pointer;
            font-size: 13px;
            font-weight: 800;
            box-shadow: 0 10px 25px rgba(34,211,238,0.35);
            transition: transform 0.1s ease, box-shadow 0.1s ease;
        }
        .expand-all:hover { transform: translateY(-1px); box-shadow: 0 14px 30px rgba(34,211,238,0.4); }

        /* Scrollbar styling */
        .sidebar-content::-webkit-scrollbar,
        .main-content::-webkit-scrollbar {
            width: 8px;
        }
        .sidebar-content::-webkit-scrollbar-track,
        .main-content::-webkit-scrollbar-track {
            background: rgba(255,255,255,0.04);
        }
        .sidebar-content::-webkit-scrollbar-thumb,
        .main-content::-webkit-scrollbar-thumb {
            background: rgba(148,163,184,0.4);
            border-radius: 4px;
        }
        .sidebar-content::-webkit-scrollbar-thumb:hover,
        .main-content::-webkit-scrollbar-thumb:hover {
            background: rgba(148,163,184,0.6);
        }
    </style>
</head>
<body>
    <div class="sidebar">
        <div class="sidebar-header">
            <h2>📂 Files</h2>
            <div class="sidebar-stats" id="sidebar-stats"></div>
        </div>
        <div class="sidebar-content" id="sidebar-files">
        </div>
    </div>
    <div class="main-content">
        <div class="container">
            <div class="header">
                <div class="brand">
                    <div class="logo-wrap">
                        <img alt="LiveReview" src="data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiPz4KPHN2ZyB3aWR0aD0iNTEyIiBoZWlnaHQ9IjUxMiIgdmlld0JveD0iMCAwIDUxMiA1MTIiIGZpbGw9Im5vbmUiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyI+CiAgPCEtLSBCYWNrZ3JvdW5kIGdsb3cgZWZmZWN0IC0tPgogIDxjaXJjbGUgY3g9IjI1NiIgY3k9IjI1NiIgcj0iMjQwIiBmaWxsPSIjMUU0MjlGIiBvcGFjaXR5PSIwLjIiIC8+CiAgCiAgPCEtLSBNYWluIGV5ZSBzaGFwZSAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIwMCIgZmlsbD0iIzExMTgyNyIgLz4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIwMCIgc3Ryb2tlPSIjM0I4MkY2IiBzdHJva2Utd2lkdGg9IjE2IiAvPgogIAogIDwhLS0gSXJpcyAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjEwMCIgZmlsbD0iIzYwQTVGQSIgLz4KICAKICA8IS0tIFB1cGlsIC0tPgogIDxjaXJjbGUgY3g9IjI1NiIgY3k9IjI1NiIgcj0iNTAiIGZpbGw9IiMxRTQwQUYiIC8+CiAgCiAgPCEtLSBTaW5nbGUgbGlnaHQgcmVmbGVjdGlvbiAobW9yZSBzdWJ0bGUpIC0tPgogIDxwYXRoIGQ9Ik0yMzUgMjIwQzIzNSAyMjguMjg0IDIyOC4yODQgMjM1IDIyMCAyMzVDMjExLjcxNiAyMzUgMjA1IDIyOC4yODQgMjA1IDIyMEMyMDUgMjExLjcxNiAyMTEuNzE2IDIwNSAyMjAgMjA1QzIyOC4yODQgMjA1IDIzNSAyMTEuNzE2IDIzNSAyMjBaIiBmaWxsPSJ3aGl0ZSIgb3BhY2l0eT0iMC44IiAvPgogIAogIDwhLS0gT3V0ZXIgZ2xvdyAtLT4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjIyMCIgc3Ryb2tleT0iIzkzQzVGRCIgc3Ryb2tlLXdpZHRoPSI0IiBvcGFjaXR5PSIwLjYiIC8+Cjwvc3ZnPgo=" />
                    </div>
                    <div class="brand-text">
                        <h1>LiveReview Results</h1>
                        <div class="meta">Generated: {{.GeneratedTime}}</div>
{{if .FriendlyName}}                        <div class="run-name-pill"><span class="dot"></span>Run: {{.FriendlyName}}</div>
{{end}}                    </div>
                    </div>
                </div>
{{if .HasSummary}}        <script type="text/markdown" id="summary-markdown">{{.Summary}}</script>
        <div class="summary" id="summary-content"></div>
{{end}}        <div class="stats">
            <div class="stat">Files: <span class="count">{{.TotalFiles}}</span></div>
            <div class="stat">Comments: <span class="count">{{.TotalComments}}</span></div>
        </div>
        <button class="expand-all" onclick="toggleAll()">Expand All Files</button>
{{if .Files}}{{range .Files}}        <div class="file collapsed" id="file_{{.ID}}" data-has-comments="{{.HasComments}}">
            <div class="file-header" onclick="toggleFile('file_{{.ID}}')">
                <span class="toggle"></span>
                <span class="filename">{{.FilePath}}</span>
{{if .HasComments}}                <span class="comment-count">{{.CommentCount}}</span>
{{end}}            </div>
            <div class="file-content">
{{if not .HasComments}}                <div style="padding: 20px; text-align: center; color: #57606a;">
                    No comments for this file.
                </div>
{{else}}                <table class="diff-table">
{{range .Hunks}}                    <tr>
                        <td colspan="3" class="hunk-header">{{.Header}}</td>
                    </tr>
{{range .Lines}}                    <tr class="diff-line {{.Class}}">
                        <td class="line-num">{{.OldNum}}</td>
                        <td class="line-num">{{.NewNum}}</td>
                        <td class="line-content">{{.Content}}</td>
                    </tr>
{{if .IsComment}}{{range .Comments}}                    <tr class="comment-row">
                        <td colspan="3">
                            <div class="comment-container">
                                <div class="comment-header">
                                    <span class="comment-badge {{.BadgeClass}}">{{.Severity}}</span>
{{if .HasCategory}}                                    <span class="comment-category">{{.Category}}</span>
{{end}}                                </div>
                                <div class="comment-body">{{.Content}}</div>
                            </div>
                        </td>
                    </tr>
{{end}}{{end}}{{end}}{{end}}                </table>
{{end}}            </div>
        </div>
{{end}}{{else}}        <div style="padding: 40px 20px; text-align: center; color: #57606a;">
            No files reviewed or no comments generated.
        </div>
{{end}}        <div class="footer">
            Review complete: {{.TotalComments}} total comment(s)
        </div>
        </div>
    </div>

    <script>
        // Initialize on page load
        document.addEventListener('DOMContentLoaded', function() {
            // Render markdown in summary
            const summaryMarkdown = document.getElementById('summary-markdown');
            const summaryEl = document.getElementById('summary-content');
            if (summaryMarkdown && summaryEl && typeof marked !== 'undefined') {
                const markdownText = summaryMarkdown.textContent;
                summaryEl.innerHTML = marked.parse(markdownText);
            }

            // Build sidebar file list
            const sidebarFiles = document.getElementById('sidebar-files');
            const files = document.querySelectorAll('.file');
            
            files.forEach((file, index) => {
                const fileName = file.querySelector('.filename').textContent;
                const commentCount = file.querySelector('.comment-count');
                const hasComments = commentCount !== null;
                const count = hasComments ? commentCount.textContent : '0';
                
                const sidebarItem = document.createElement('div');
                sidebarItem.className = 'sidebar-file';
                sidebarItem.dataset.fileId = file.id;
                
                const nameSpan = document.createElement('span');
                nameSpan.className = 'sidebar-file-name';
                nameSpan.textContent = fileName;
                nameSpan.title = fileName;
                sidebarItem.appendChild(nameSpan);
                
                if (hasComments) {
                    const badge = document.createElement('span');
                    badge.className = 'sidebar-file-badge';
                    badge.textContent = count;
                    sidebarItem.appendChild(badge);
                }
                
                sidebarItem.addEventListener('click', function() {
                    // Remove active from all
                    document.querySelectorAll('.sidebar-file').forEach(f => f.classList.remove('active'));
                    // Add active to clicked
                    sidebarItem.classList.add('active');
                    
                    // Expand the file if collapsed BEFORE scrolling
                    if (file.classList.contains('collapsed')) {
                        toggleFile(file.id);
                    }
                    
                    // Scroll to file accounting for fixed header
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const fileRect = file.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + fileRect.top - mainContentRect.top - headerHeight - 10;
                    
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                });
                
                sidebarFiles.appendChild(sidebarItem);
            });

            // Update sidebar stats
            const stats = document.getElementById('sidebar-stats');
            const totalFiles = files.length;
            const totalComments = {{.TotalComments}};
            stats.textContent = totalFiles + ' files • ' + totalComments + ' comments';

            // Auto-expand files with comments on load
            const filesWithComments = document.querySelectorAll('.file[data-has-comments="true"]');
            filesWithComments.forEach(f => f.classList.add('expanded'));

            // Highlight active file in sidebar on scroll
            const mainContent = document.querySelector('.main-content');
            mainContent.addEventListener('scroll', function() {
                let currentFile = null;
                files.forEach(file => {
                    const rect = file.getBoundingClientRect();
                    if (rect.top >= 0 && rect.top < window.innerHeight / 2) {
                        currentFile = file;
                    }
                });
                
                if (currentFile) {
                    document.querySelectorAll('.sidebar-file').forEach(f => f.classList.remove('active'));
                    const sidebarItem = document.querySelector('.sidebar-file[data-file-id="' + currentFile.id + '"]');
                    if (sidebarItem) {
                        sidebarItem.classList.add('active');
                    }
                }
            });
        });
    </script>
</body>
</html>
`
