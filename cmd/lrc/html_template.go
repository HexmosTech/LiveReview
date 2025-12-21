package main

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"
)

// HTMLTemplateData contains all data needed for the HTML template
type HTMLTemplateData struct {
	GeneratedTime string
	Summary       string
	TotalFiles    int
	TotalComments int
	Files         []HTMLFileData
	HasSummary    bool
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

// htmlTemplate is the main HTML template
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LiveReview Results</title>
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
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
            font-size: 14px;
            line-height: 1.5;
            color: #24292f;
            background-color: #f6f8fa;
            display: flex;
            height: 100vh;
            overflow: hidden;
        }
        .sidebar {
            width: 300px;
            background: white;
            border-right: 1px solid #d0d7de;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        .sidebar-header {
            padding: 16px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
        }
        .sidebar-header h2 {
            font-size: 14px;
            font-weight: 600;
            color: #24292f;
            margin-bottom: 4px;
        }
        .sidebar-stats {
            font-size: 12px;
            color: #57606a;
        }
        .sidebar-content {
            flex: 1;
            overflow-y: auto;
            padding: 8px 0;
        }
        .sidebar-file {
            padding: 8px 16px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            border-left: 3px solid transparent;
        }
        .sidebar-file:hover {
            background: #f6f8fa;
        }
        .sidebar-file.active {
            background: #ddf4ff;
            border-left-color: #0969da;
        }
        .sidebar-file-name {
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
            flex: 1;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        .sidebar-file-badge {
            background: #0969da;
            color: white;
            padding: 2px 6px;
            border-radius: 10px;
            font-size: 10px;
            font-weight: 600;
        }
        .main-content {
            flex: 1;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            width: 100%;
        }
        .header {
            padding: 16px 20px;
            background: #ffffff;
            border-bottom: 1px solid #d0d7de;
            position: sticky;
            top: 0;
            z-index: 100;
            height: 60px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
        }
        .header h1 { font-size: 24px; font-weight: 600; margin-bottom: 8px; }
        .header .meta { color: #57606a; font-size: 12px; }
        .summary {
            padding: 16px 20px;
            background: #ddf4ff;
            border-bottom: 1px solid #54aeff;
        }
        .summary h1 { font-size: 18px; font-weight: 600; margin-bottom: 12px; margin-top: 16px; }
        .summary h1:first-child { margin-top: 0; }
        .summary h2 { font-size: 16px; font-weight: 600; margin-bottom: 10px; margin-top: 14px; }
        .summary h3 { font-size: 14px; font-weight: 600; margin-bottom: 8px; margin-top: 12px; }
        .summary p { margin-bottom: 8px; }
        .summary ul, .summary ol { margin-left: 20px; margin-bottom: 8px; }
        .summary code {
            background: rgba(175, 184, 193, 0.2);
            padding: 2px 6px;
            border-radius: 3px;
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
        }
        .summary pre {
            background: rgba(175, 184, 193, 0.2);
            padding: 12px;
            border-radius: 6px;
            overflow-x: auto;
            margin-bottom: 8px;
        }
        .summary pre code {
            background: none;
            padding: 0;
        }
        .summary strong { font-weight: 600; }
        .stats {
            padding: 12px 20px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
            display: flex;
            gap: 20px;
            font-size: 13px;
        }
        .stats .stat { font-weight: 600; }
        .stats .stat .count { color: #0969da; }
        .file {
            border-bottom: 1px solid #d0d7de;
        }
        .file:last-child { border-bottom: none; }
        .file-header {
            padding: 12px 20px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .file-header:hover { background: #eaeef2; }
        .file-header .filename {
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-weight: 600;
            flex: 1;
        }
        .file-header .comment-count {
            background: #0969da;
            color: white;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }
        .file-header .toggle { font-size: 12px; color: #57606a; }
        .file-content { display: none; }
        .file.expanded .file-content { display: block; }
        .file.expanded .file-header .toggle::before { content: "▼ "; }
        .file.collapsed .file-header .toggle::before { content: "▶ "; }
        .diff-table {
            width: 100%;
            border-collapse: collapse;
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
        }
        .diff-table td {
            padding: 0 8px;
            border: none;
            vertical-align: top;
        }
        .diff-line { background: #ffffff; }
        .diff-line:hover { background: #f6f8fa; }
        .line-num {
            width: 50px;
            color: #57606a;
            text-align: right;
            user-select: none;
            padding: 0 8px;
            background: #f6f8fa;
        }
        .line-content {
            white-space: pre;
            padding-left: 12px;
            width: 100%;
        }
        .diff-add { background: #dafbe1; }
        .diff-add .line-content { background: #dafbe1; }
        .diff-del { background: #ffebe9; }
        .diff-del .line-content { background: #ffebe9; }
        .diff-context .line-content { background: #ffffff; }
        .comment-row {
            background: #fff8c5;
            border-top: 1px solid #d4a72c;
            border-bottom: 1px solid #d4a72c;
        }
        .comment-container {
            padding: 12px 16px;
            margin: 8px 50px 8px 110px;
            background: white;
            border: 1px solid #d4a72c;
            border-radius: 6px;
        }
        .comment-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .comment-badge {
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }
        .badge-info { background: #ddf4ff; color: #0969da; }
        .badge-warning { background: #fff8c5; color: #9a6700; }
        .badge-error { background: #ffebe9; color: #cf222e; }
        .comment-category {
            color: #57606a;
            font-size: 12px;
            font-weight: normal;
        }
        .comment-body {
            color: #24292f;
            line-height: 1.5;
            white-space: pre-wrap;
        }
        .hunk-header {
            background: #f6f8fa;
            color: #57606a;
            padding: 4px 8px;
            font-weight: 600;
            border-top: 1px solid #d0d7de;
            border-bottom: 1px solid #d0d7de;
        }
        .footer {
            padding: 16px 20px;
            text-align: center;
            color: #57606a;
            font-size: 12px;
            background: #f6f8fa;
        }
        .expand-all {
            padding: 8px 16px;
            margin: 10px 20px;
            background: #0969da;
            color: white;
            border: none;
            border-radius: 6px;
            cursor: pointer;
            font-size: 13px;
            font-weight: 600;
        }
        .expand-all:hover { background: #0860ca; }

        /* Scrollbar styling */
        .sidebar-content::-webkit-scrollbar,
        .main-content::-webkit-scrollbar {
            width: 8px;
        }
        .sidebar-content::-webkit-scrollbar-track,
        .main-content::-webkit-scrollbar-track {
            background: #f6f8fa;
        }
        .sidebar-content::-webkit-scrollbar-thumb,
        .main-content::-webkit-scrollbar-thumb {
            background: #d0d7de;
            border-radius: 4px;
        }
        .sidebar-content::-webkit-scrollbar-thumb:hover,
        .main-content::-webkit-scrollbar-thumb:hover {
            background: #afb8c1;
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
                <h1>🔍 LiveReview Results</h1>
                <div class="meta">Generated: {{.GeneratedTime}}</div>
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
