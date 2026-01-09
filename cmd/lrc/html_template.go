package main

import (
	"bytes"
	"fmt"
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
	Interactive   bool
	InitialMsg    string
	ReviewID      string // For polling events
	APIURL        string // For polling events
	APIKey        string // For authenticated API calls
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
	Line        int
	FilePath    string
}

// prepareHTMLData converts the API response to template data
func prepareHTMLData(result *diffReviewResponse, interactive bool, initialMsg, reviewID, apiURL, apiKey string) *HTMLTemplateData {
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
		Interactive:   interactive,
		InitialMsg:    initialMsg,
		ReviewID:      reviewID,
		APIURL:        apiURL,
		APIKey:        apiKey,
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
		hunks[i] = prepareHunkData(hunk, commentsByLine, file.FilePath)
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
func prepareHunkData(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, filePath string) HTMLHunkData {
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount)

	lines := parseHunkLines(hunk, commentsByLine, filePath)

	return HTMLHunkData{
		Header: header,
		Lines:  lines,
	}
}

// parseHunkLines parses hunk content into lines with comments
func parseHunkLines(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, filePath string) []HTMLLineData {
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
				Content: line,
				Class:   "diff-del",
			}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			lineData = HTMLLineData{
				OldNum:  "",
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: line,
				Class:   "diff-add",
			}

			// Check for comments on this line
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				lineData.IsComment = true
				lineData.Comments = prepareComments(comments, filePath)
			}

			newLine++
		} else {
			lineData = HTMLLineData{
				OldNum:  fmt.Sprintf("%d", oldLine),
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: " " + line,
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
func prepareComments(comments []diffReviewComment, filePath string) []HTMLCommentData {
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
			Content:     comment.Content,
			HasCategory: comment.Category != "",
			Line:        comment.Line,
			FilePath:    filePath,
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
        
        // Tab switching function (defined early for onclick handlers)
        function switchTab(tabName) {
            // Hide all tabs
            document.querySelectorAll('.tab-content').forEach(tab => {
                tab.classList.remove('active');
                tab.style.display = 'none';
            });
            // Remove active from all buttons
            document.querySelectorAll('.tab-btn').forEach(btn => {
                btn.classList.remove('active');
            });
            
            // Show selected tab
            const selectedTab = document.getElementById(tabName + '-tab');
            if (selectedTab) {
                selectedTab.classList.add('active');
                selectedTab.style.display = 'block';
            }
            
            // Activate button
            const selectedBtn = document.querySelector('.tab-btn[data-tab="' + tabName + '"]');
            if (selectedBtn) {
                selectedBtn.classList.add('active');
            }
            
            // Clear notification badge when switching to events tab
            if (tabName === 'events') {
                const badge = document.getElementById('event-notification-badge');
                if (badge) {
                    badge.style.display = 'none';
                    badge.textContent = '0';
                }
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
            transition: background 0.3s ease;
        }
        .comment-row.highlight {
            background: rgba(139,92,246,0.25);
            animation: highlightPulse 1.5s ease-in-out;
        }
        @keyframes highlightPulse {
            0%, 100% { background: rgba(139,92,246,0.25); }
            50% { background: rgba(139,92,246,0.35); }
        }
        
        /* Animation for newly added progressive comments */
        @keyframes slideInFade {
            0% {
                opacity: 0;
                transform: translateY(-10px);
            }
            100% {
                opacity: 1;
                transform: translateY(0);
            }
        }
        
        .comment-row.new-comment {
            animation: slideInFade 0.5s ease-out, highlightPulse 2s ease-in-out;
        }
        
        /* Smooth transitions for count badges */
        .comment-count, .sidebar-file-badge {
            transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
        }
        
        .comment-count.updated, .sidebar-file-badge.updated {
            animation: countPulse 0.6s ease-out;
        }
        
        @keyframes countPulse {
            0%, 100% { transform: scale(1); }
            50% { transform: scale(1.2); background: rgba(139,92,246,0.4); }
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
            position: relative;
        }
        .comment-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
            font-weight: 700;
            color: #e5e7eb;
        }
        .comment-copy-btn {
            position: absolute;
            top: 8px;
            right: 8px;
            background: rgba(139,92,246,0.15);
            border: 1px solid rgba(139,92,246,0.3);
            color: #c4b5fd;
            padding: 4px 10px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.15s ease;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        .comment-copy-btn:hover {
            background: rgba(139,92,246,0.25);
            border-color: rgba(139,92,246,0.5);
            transform: scale(1.05);
        }
        .comment-copy-btn::before {
            content: "📋";
            font-size: 12px;
        }
        .comment-copy-btn.copied {
            background: rgba(34,197,94,0.15);
            border-color: rgba(34,197,94,0.3);
            color: #bbf7d0;
        }
        .comment-copy-btn.copied::before {
            content: "✓";
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

        /* Precommit action bar */
        .precommit-bar {
            margin: 12px 0 20px;
            padding: 14px 16px;
            border: 1px solid #334155;
            border-radius: 10px;
            background: linear-gradient(90deg, rgba(51,65,85,0.35), rgba(30,41,59,0.35));
            display: flex;
            align-items: flex-start;
            gap: 16px;
            flex-wrap: wrap;
        }

        .precommit-bar-left {
            display: flex;
            flex-direction: column;
            gap: 8px;
            min-width: 220px;
        }

        .precommit-bar-title {
            font-weight: 600;
            color: #e2e8f0;
            margin-right: 6px;
        }

        .precommit-actions {
            display: flex;
            gap: 10px;
            align-items: center;
        }

        .precommit-message {
            flex: 1;
            min-width: 260px;
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .precommit-message label {
            font-size: 12px;
            font-weight: 600;
            color: #cbd5e1;
        }

        .precommit-message textarea {
            width: 100%;
            min-height: 90px;
            resize: vertical;
            background: rgba(15,23,42,0.4);
            border: 1px solid #475569;
            color: #e2e8f0;
            border-radius: 10px;
            padding: 10px 12px;
            font-family: "JetBrains Mono", "SFMono-Regular", ui-monospace, monospace;
            font-size: 13px;
            line-height: 1.45;
        }

        .precommit-message textarea:focus {
            outline: 1px solid #60a5fa;
            box-shadow: 0 0 0 3px rgba(96,165,250,0.25);
        }

        .precommit-message-hint {
            color: #94a3b8;
            font-size: 12px;
        }

        .btn-primary {
            background: linear-gradient(135deg, #22c55e, #16a34a);
            color: #0b0f1a;
            border: none;
            padding: 10px 14px;
            border-radius: 8px;
            font-weight: 700;
            cursor: pointer;
            box-shadow: 0 6px 20px rgba(34,197,94,0.25);
        }

        .btn-ghost {
            background: transparent;
            color: #e2e8f0;
            border: 1px solid #475569;
            padding: 10px 14px;
            border-radius: 8px;
            font-weight: 600;
            cursor: pointer;
        }

        .btn-primary:disabled,
        .btn-ghost:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }

        .precommit-status {
            color: #94a3b8;
            font-size: 13px;
            min-height: 18px;
        }
        .expand-all {
            padding: var(--space-sm) var(--space-lg);
            margin: 0;
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

        .toolbar-row {
            display: flex;
            gap: 12px;
            align-items: center;
            padding: var(--space-sm) 0;
            margin-bottom: var(--space-md);
            border-bottom: 1px solid rgba(255,255,255,0.08);
        }
        
        /* Tab styles */
        .view-tabs {
            display: flex;
            gap: 8px;
            margin-right: auto;
        }
        .tab-btn {
            padding: 8px 16px;
            background: rgba(15,23,42,0.6);
            color: #94a3b8;
            border: 1px solid rgba(255,255,255,0.08);
            border-radius: 8px;
            cursor: pointer;
            font-size: 13px;
            font-weight: 600;
            transition: all 0.2s ease;
        }
        .tab-btn:hover {
            background: rgba(59,130,246,0.15);
            color: #e2e8f0;
            border-color: rgba(59,130,246,0.3);
        }
        .tab-btn.active {
            background: linear-gradient(135deg, #3b82f6, #2563eb);
            color: white;
            border-color: transparent;
            box-shadow: 0 4px 12px rgba(59,130,246,0.3);
        }
        .tab-content {
            display: none;
        }
        .tab-content.active {
            display: block;
        }
        
        /* Events tab styles */
        .events-container {
            background: rgba(15,23,42,0.6);
            border-radius: 12px;
            padding: 24px;
            margin: 16px 0;
        }
        .events-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 20px;
            padding-bottom: 16px;
            border-bottom: 1px solid rgba(255,255,255,0.08);
        }
        .events-header h3 {
            margin: 0 0 8px 0;
            font-size: 18px;
            font-weight: 600;
            color: #e6edf3;
        }
        .events-status {
            color: #8b949e;
            font-size: 13px;
        }
        .events-controls {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .auto-scroll-label {
            display: flex;
            align-items: center;
            gap: 6px;
            font-size: 13px;
            color: #8b949e;
            cursor: pointer;
        }
        .auto-scroll-label input[type="checkbox"] {
            cursor: pointer;
        }
        .copy-logs-btn {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 6px 12px;
            background: rgba(59,130,246,0.15);
            border: 1px solid rgba(59,130,246,0.3);
            border-radius: 6px;
            color: #60a5fa;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .copy-logs-btn:hover {
            background: rgba(59,130,246,0.25);
            border-color: rgba(59,130,246,0.5);
        }
        .copy-logs-btn svg {
            width: 16px;
            height: 16px;
        }
        .events-list {
            /* Use main page scroll instead of nested scroll */
            font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
            font-size: 13px;
            line-height: 1.6;
        }
        .event-item {
            padding: 4px 0;
            border-bottom: 1px solid rgba(255,255,255,0.03);
            display: flex;
            gap: 12px;
            color: #8b949e;
        }
        .event-item:hover {
            background: rgba(255,255,255,0.02);
        }
        .event-time {
            color: #6e7681;
            font-size: 12px;
            min-width: 80px;
            flex-shrink: 0;
        }
        .event-message {
            color: #c9d1d9;
            flex: 1;
        }
        .event-type {
            display: inline-block;
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            margin-right: 8px;
        }
        .event-type.batch {
            background: rgba(59,130,246,0.15);
            color: #60a5fa;
        }
        .event-type.completion {
            background: rgba(168,85,247,0.15);
            color: #c084fc;
        }
        .event-type.error {
            background: rgba(239,68,68,0.15);
            color: #f87171;
        }
        
        .toolbar-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: var(--space-md);
            margin: 0;
        }

        .copy-issues-btn {
            padding: var(--space-sm) var(--space-lg);
            background: linear-gradient(135deg, #8b5cf6, #6366f1);
            color: #fff;
            border: none;
            border-radius: 10px;
            cursor: pointer;
            font-size: 13px;
            font-weight: 800;
            box-shadow: 0 10px 25px rgba(139,92,246,0.35);
            transition: transform 0.1s ease, box-shadow 0.1s ease;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .copy-issues-btn:hover { transform: translateY(-1px); box-shadow: 0 14px 30px rgba(139,92,246,0.4); }
        .copy-issues-btn::before {
            content: "📋";
            font-size: 16px;
        }

        .issues-toolbar { margin: 10px 0 0 0; }
        .issues-panel {
            margin-top: 8px;
            background: #0f1724;
            border: 1px solid #1f2a3a;
            border-radius: 8px;
            padding: 10px;
        }
        .issues-panel.hidden { display: none; }
        .issues-actions {
            display: flex;
            gap: 10px;
            align-items: center;
            flex-wrap: wrap;
            margin-bottom: 8px;
        }
        .severity-filters {
            display: flex;
            gap: 6px;
            padding: 4px;
            background: rgba(255,255,255,0.02);
            border-radius: 6px;
            border: 1px solid rgba(255,255,255,0.05);
        }
        .severity-filter-btn {
            padding: 4px 10px;
            background: transparent;
            color: #9ca3af;
            border: 1px solid transparent;
            border-radius: 4px;
            cursor: pointer;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            transition: all 0.15s ease;
        }
        .severity-filter-btn:hover {
            background: rgba(255,255,255,0.03);
        }
        .severity-filter-btn.active {
            border-color: currentColor;
        }
        .severity-filter-btn.all { color: #60a5fa; }
        .severity-filter-btn.all.active { background: rgba(96,165,250,0.15); }
        .severity-filter-btn.error { color: #ef4444; }
        .severity-filter-btn.error.active { background: rgba(239,68,68,0.15); }
        .severity-filter-btn.warning { color: #eab308; }
        .severity-filter-btn.warning.active { background: rgba(234,179,8,0.15); }
        .severity-filter-btn.info { color: #22d3ee; }
        .severity-filter-btn.info.active { background: rgba(34,211,238,0.15); }
        .issues-list {
            max-height: 220px;
            overflow: auto;
            border-top: 1px solid #1f2a3a;
            padding-top: 8px;
            display: grid;
            gap: 6px;
        }
        .issue-item {
            display: grid;
            grid-template-columns: auto 1fr auto;
            gap: 8px;
            padding: 8px;
            background: #111827;
            border: 1px solid #1f2a3a;
            border-radius: 6px;
        }
        .issue-item.hidden { display: none; }
        .issue-nav-link {
            display: flex;
            align-items: center;
            justify-content: center;
            width: 28px;
            height: 28px;
            background: rgba(139,92,246,0.15);
            border: 1px solid rgba(139,92,246,0.3);
            border-radius: 6px;
            color: #a78bfa;
            cursor: pointer;
            font-size: 16px;
            transition: all 0.15s ease;
            text-decoration: none;
        }
        .issue-nav-link:hover {
            background: rgba(139,92,246,0.25);
            border-color: rgba(139,92,246,0.5);
            transform: scale(1.1);
        }
        .issue-path {
            font-family: ui-monospace, SFMono-Regular, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
            color: #c9d1d9;
            font-size: 12px;
        }
        .issue-message { color: #e6edf3; font-size: 13px; }
        .issues-status { color: #8b949e; font-size: 12px; }

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
{{if .Interactive}}        <div class="precommit-bar">
            <div class="precommit-bar-left">
                <div class="precommit-bar-title">Pre-commit action</div>
                <div class="precommit-actions">
                    <button id="btn-commit" class="btn-primary">Commit</button>
                    <button id="btn-commit-push" class="btn-primary">Commit and Push</button>
                    <button id="btn-skip" class="btn-ghost">Skip Commit</button>
                </div>
                <div id="precommit-status" class="precommit-status"></div>
            </div>
            <div class="precommit-message">
                <label for="commit-message">Commit message</label>
                <textarea id="commit-message" placeholder="Enter your commit message (required)">{{.InitialMsg}}</textarea>
                <div class="precommit-message-hint">Required for commit actions; ignored on Skip.</div>
            </div>
        </div>
{{end}}        <div class="toolbar-row">
            <div class="view-tabs">
                <button class="tab-btn active" data-tab="files" onclick="switchTab('files')">📁 Files & Comments</button>
                <button id="events-tab-btn" class="tab-btn" data-tab="events" onclick="switchTab('events')">
                    📊 Event Log
                    <span id="event-notification-badge" class="notification-badge" style="display: none;">0</span>
                </button>
            </div>
            <button class="expand-all" onclick="toggleAll()">Expand All Files</button>
            <button id="issues-toggle" class="copy-issues-btn">Copy Issues</button>
        </div>
        <div class="issues-toolbar">
            <div id="issues-panel" class="issues-panel hidden">
                <div class="issues-actions">
                    <div class="severity-filters">
                        <button class="severity-filter-btn all" data-severity="all">All</button>
                        <button class="severity-filter-btn error active" data-severity="error">Error</button>
                        <button class="severity-filter-btn warning active" data-severity="warning">Warning</button>
                        <button class="severity-filter-btn info" data-severity="info">Info</button>
                    </div>
                    <button id="issues-select-all" class="btn-ghost">Select All</button>
                    <button id="issues-deselect-all" class="btn-ghost">Deselect All</button>
                    <button id="issues-copy" class="btn-primary">Copy Selected</button>
                    <span id="issues-status" class="issues-status"></span>
                </div>
                <div id="issues-list" class="issues-list"></div>
            </div>
        </div>
        
        <!-- Files Tab Content -->
        <div id="files-tab" class="tab-content active">
{{if .Files}}{{range .Files}}        <div class="file collapsed" id="file_{{.ID}}" data-has-comments="{{.HasComments}}" data-filepath="{{.FilePath}}">
            <div class="file-header" onclick="toggleFile('file_{{.ID}}')">
                <span class="toggle"></span>
                <span class="filename">{{.FilePath}}</span>
{{if .HasComments}}                <span class="comment-count">{{.CommentCount}}</span>
{{end}}            </div>
            <div class="file-content">
{{if .Hunks}}                <table class="diff-table">
{{range .Hunks}}                    <tr>
                        <td colspan="3" class="hunk-header">{{.Header}}</td>
                    </tr>
{{range .Lines}}                    <tr class="diff-line {{.Class}}">
                        <td class="line-num">{{.OldNum}}</td>
                        <td class="line-num">{{.NewNum}}</td>
                        <td class="line-content">{{.Content}}</td>
                    </tr>
{{if .IsComment}}{{range .Comments}}                    <tr class="comment-row" data-line="{{.Line}}">
                        <td colspan="3">
                            <div class="comment-container" data-filepath="{{.FilePath}}" data-line="{{.Line}}" data-comment="{{.Content}}">
                                <button class="comment-copy-btn" title="Copy issue details">Copy</button>
                                <div class="comment-header">
                                    <span class="comment-badge {{.BadgeClass}}">{{.Severity}}</span>
{{if .HasCategory}}                                    <span class="comment-category">{{.Category}}</span>
{{end}}                                </div>
                                <div class="comment-body">{{.Content}}</div>
                            </div>
                        </td>
                    </tr>
{{end}}{{end}}{{end}}{{end}}                </table>
{{else}}                <div style="padding: 20px; text-align: center; color: #57606a;">
                    No diff content available.
                </div>
{{end}}            </div>
        </div>
{{end}}{{else}}        <div style="padding: 40px 20px; text-align: center; color: #57606a;">
            No files reviewed or no comments generated.
        </div>
{{end}}        </div>
        
        <!-- Events Tab Content -->
        <div id="events-tab" class="tab-content" style="display: none;">
            <div class="events-container">
                <div class="events-header">
                    <div>
                        <h3>Review Progress</h3>
                        <div class="events-status" id="events-status">Waiting for events...</div>
                    </div>
                    <div class="events-controls">
                        <label class="auto-scroll-label">
                            <input type="checkbox" id="auto-scroll-checkbox" checked>
                            <span>Auto-scroll</span>
                        </label>
                        <button id="copy-logs-btn" class="copy-logs-btn" title="Copy all logs to clipboard">
                            <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                            </svg>
                            Copy Logs
                        </button>
                    </div>
                </div>
                <div class="events-list" id="events-list">
                    <!-- Events will be dynamically added here -->
                </div>
            </div>
        </div>
        
        <div class="footer">
            Review complete: {{.TotalComments}} total comment(s)
        </div>
        </div>
    </div>

    <script>
        // Render event in the events list
        function renderEvent(event) {
            const eventsList = document.getElementById('events-list');
            if (!eventsList) return;
            
            const eventItem = document.createElement('div');
            eventItem.className = 'event-item';
            eventItem.dataset.eventId = event.id;
            eventItem.dataset.eventType = event.type || 'log'; // Store type for later reference
            
            // Update notification badge on Event Log tab if not currently viewing it
            const eventsTab = document.getElementById('events-tab');
            const eventsBadge = document.getElementById('event-notification-badge');
            if (eventsTab && !eventsTab.classList.contains('active') && eventsBadge) {
                const currentCount = parseInt(eventsBadge.textContent) || 0;
                eventsBadge.textContent = currentCount + 1;
                eventsBadge.style.display = 'inline-block';
            }
            
            // Show badges only for important event types (not regular logs)
            let badgeHTML = '';
            if (event.type === 'batch') {
                badgeHTML = '<span class="event-type batch">BATCH</span>';
            } else if (event.type === 'completion') {
                badgeHTML = '<span class="event-type completion">COMPLETE</span>';
            } else if (event.level === 'error') {
                badgeHTML = '<span class="event-type error">ERROR</span>';
            }
            
            // Format timestamp
            const timestamp = new Date(event.time).toLocaleTimeString();
            
            // Build message
            let message = '';
            const eventData = event.data || {};
            
            if (event.type === 'log') {
                message = (eventData.message || '').replace(/\\\\n/g, '\\n').replace(/\\\\t/g, '  ').replace(/\\\\"/g, '"');
            } else if (event.type === 'batch') {
                const batchId = event.batchId || 'unknown';
                if (eventData.status === 'processing') {
                    const fileCount = eventData.fileCount || 0;
                    message = 'Batch ' + batchId + ' started: processing ' + fileCount + ' file' + (fileCount !== 1 ? 's' : '');
                } else if (eventData.status === 'completed') {
                    const commentCount = eventData.fileCount || 0;
                    message = 'Batch ' + batchId + ' completed: generated ' + commentCount + ' comment' + (commentCount !== 1 ? 's' : '');
                } else {
                    message = 'Batch ' + batchId + ': ' + (eventData.status || 'unknown status');
                }
            } else if (event.type === 'completion') {
                const commentCount = eventData.commentCount || 0;
                message = eventData.resultSummary || ('Process completed with ' + commentCount + ' comment' + (commentCount !== 1 ? 's' : ''));
            } else {
                message = eventData.message || JSON.stringify(eventData);
            }
            
            // Simple single-line format: [time] [badge?] message
            eventItem.innerHTML = 
                '<span class="event-time">' + timestamp + '</span>' +
                '<span class="event-message">' + badgeHTML + message + '</span>';
            
            eventsList.appendChild(eventItem);
            
            // Auto-scroll to bottom if enabled and on events tab
            const autoScrollCheckbox = document.getElementById('auto-scroll-checkbox');
            if (autoScrollCheckbox && autoScrollCheckbox.checked) {
                const eventsTabElement = document.getElementById('events-tab');
                if (eventsTabElement && eventsTabElement.classList.contains('active')) {
                    // Scroll to bottom of page
                    setTimeout(() => {
                        window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
                    }, 100);
                }
            }
        }
        
        // Initialize on page load
        document.addEventListener('DOMContentLoaded', function() {
            const isPrecommit = {{if .Interactive}}true{{else}}false{{end}};

            // Render markdown in summary
            const summaryMarkdown = document.getElementById('summary-markdown');
            const summaryEl = document.getElementById('summary-content');
            if (summaryMarkdown && summaryEl && typeof marked !== 'undefined') {
                const markdownText = summaryMarkdown.textContent;
                summaryEl.innerHTML = marked.parse(markdownText);
            }

            if (isPrecommit) {
                const commitBtn = document.getElementById('btn-commit');
                const commitPushBtn = document.getElementById('btn-commit-push');
                const skipBtn = document.getElementById('btn-skip');
                const statusEl = document.getElementById('precommit-status');
                const messageInput = document.getElementById('commit-message');

                const setStatus = (text) => { statusEl.textContent = text; };
                const disableAll = () => {
                    commitBtn.disabled = true;
                    commitPushBtn.disabled = true;
                    skipBtn.disabled = true;
                    if (messageInput) {
                        messageInput.disabled = true;
                    }
                };

                const enableActions = () => {
                    commitBtn.disabled = false;
                    commitPushBtn.disabled = false;
                    skipBtn.disabled = false;
                    if (messageInput) {
                        messageInput.disabled = false;
                    }
                };

                const postDecision = async (path, successText, requireMessage) => {
                    if (requireMessage) {
                        const msg = messageInput ? messageInput.value.trim() : '';
                        if (!msg) {
                            setStatus('Commit message is required');
                            if (messageInput) messageInput.focus();
                            return;
                        }
                    }
                    disableAll();
                    setStatus('Sending decision...');
                    try {
                        const res = await fetch(path, {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ message: messageInput ? messageInput.value : '' })
                        });
                        if (!res.ok) throw new Error('Request failed: ' + res.status);
                        setStatus(successText + ' — you can now return to the terminal.');
                    } catch (err) {
                        setStatus('Failed: ' + err.message);
                        enableActions();
                    }
                };

                commitBtn.addEventListener('click', () => postDecision('/commit', 'Commit requested', true));
                commitPushBtn.addEventListener('click', () => postDecision('/commit-push', 'Commit and push requested', true));
                skipBtn.addEventListener('click', () => postDecision('/skip', 'Skip requested', false));
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

            // Issues copy UI
            const issuesToggle = document.getElementById('issues-toggle');
            const issuesPanel = document.getElementById('issues-panel');
            const issuesList = document.getElementById('issues-list');
            const issuesStatus = document.getElementById('issues-status');
            const issuesSelectAll = document.getElementById('issues-select-all');
            const issuesDeselectAll = document.getElementById('issues-deselect-all');
            const issuesCopy = document.getElementById('issues-copy');

            const collectIssues = () => {
                const collected = [];
                document.querySelectorAll('.file[data-has-comments="true"]').forEach(file => {
                    const filepath = file.dataset.filepath || (file.querySelector('.filename')?.textContent || '');
                    const fileId = file.id;
                    const comments = file.querySelectorAll('.comment-row');
                    comments.forEach((row, idx) => {
                        const line = row.dataset.line || '';
                        if (!row.id) {
                            row.id = 'comment-' + fileId + '-' + line + '-' + idx;
                        }
                        const commentId = row.id;
                        const body = (row.querySelector('.comment-body')?.innerText || '').trim();
                        if (!body) return;
                        const severity = (row.querySelector('.comment-badge')?.innerText || '').trim();
                        const category = (row.querySelector('.comment-category')?.innerText || '').trim();
                        collected.push({ filepath, line, body, severity, category, commentId, fileId });
                    });
                });
                return collected;
            };

            const renderIssues = (issues) => {
                issuesList.innerHTML = '';
                issues.forEach((issue, idx) => {
                    const item = document.createElement('div');
                    item.className = 'issue-item';
                    item.dataset.severity = issue.severity.toLowerCase();

                    const checkbox = document.createElement('input');
                    checkbox.type = 'checkbox';
                    // Only check ERROR and WARNING by default
                    const sevLower = issue.severity.toLowerCase();
                    checkbox.checked = (sevLower === 'error' || sevLower === 'warning' || sevLower === 'critical');
                    checkbox.dataset.idx = idx;

                    const content = document.createElement('div');
                    const path = document.createElement('div');
                    path.className = 'issue-path';
                    const lineSuffix = issue.line ? ':' + issue.line : '';
                    path.textContent = issue.filepath + lineSuffix;

                    const msg = document.createElement('div');
                    msg.className = 'issue-message';
                    const sev = issue.severity ? ' (' + issue.severity + (issue.category ? ', ' + issue.category : '') + ')' : '';
                    msg.textContent = issue.body + sev;

                    content.appendChild(path);
                    content.appendChild(msg);

                    const navLink = document.createElement('a');
                    navLink.className = 'issue-nav-link';
                    navLink.href = '#' + issue.commentId;
                    navLink.textContent = '→';
                    navLink.title = 'Navigate to comment';
                    navLink.addEventListener('click', (e) => {
                        e.preventDefault();
                        navigateToComment(issue.commentId, issue.fileId);
                    });

                    item.appendChild(checkbox);
                    item.appendChild(content);
                    item.appendChild(navLink);
                    issuesList.appendChild(item);
                });
            };

            const setIssuesStatus = (text) => { issuesStatus.textContent = text; };

            const getSelectedIssues = () => {
                const selected = [];
                issuesList.querySelectorAll('input[type="checkbox"]').forEach(cb => {
                    if (cb.checked) {
                        const idx = parseInt(cb.dataset.idx, 10);
                        if (!isNaN(idx) && currentIssues[idx]) {
                            selected.push(currentIssues[idx]);
                        }
                    }
                });
                return selected;
            };

            const copyIssues = async (issues) => {
                const lines = issues.map(issue => {
                    const lineSuffix = issue.line ? ':' + issue.line : '';
                    const sev = issue.severity ? ' (' + issue.severity + (issue.category ? ', ' + issue.category : '') + ')' : '';
                    return issue.filepath + lineSuffix + ' — ' + issue.body + sev;
                });
                const text = lines.join('\n');
                await navigator.clipboard.writeText(text);
            };

            let currentIssues = [];
            let currentSeverityFilter = 'all';

            const navigateToComment = (commentId, fileId) => {
                const file = document.getElementById(fileId);
                const comment = document.getElementById(commentId);
                if (!file || !comment) return;

                if (file.classList.contains('collapsed')) {
                    toggleFile(fileId);
                }

                setTimeout(() => {
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const commentRect = comment.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + commentRect.top - mainContentRect.top - headerHeight - 20;

                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });

                    document.querySelectorAll('.comment-row.highlight').forEach(c => c.classList.remove('highlight'));
                    comment.classList.add('highlight');
                    setTimeout(() => comment.classList.remove('highlight'), 1500);
                }, 100);
            };

            const filterBySeverity = (severity) => {
                currentSeverityFilter = severity;
                document.querySelectorAll('.severity-filter-btn').forEach(btn => {
                    btn.classList.toggle('active', btn.dataset.severity === severity);
                });
                
                issuesList.querySelectorAll('.issue-item').forEach(item => {
                    const itemSeverity = item.dataset.severity;
                    if (severity === 'all' || itemSeverity === severity) {
                        item.classList.remove('hidden');
                    } else {
                        item.classList.add('hidden');
                    }
                });
            };

            if (issuesToggle) {
                issuesToggle.addEventListener('click', () => {
                    const opening = issuesPanel.classList.contains('hidden');
                    if (opening) {
                        currentIssues = collectIssues();
                        renderIssues(currentIssues);
                        // Default to showing error and warning
                        currentSeverityFilter = 'error,warning';
                        document.querySelectorAll('.severity-filter-btn').forEach(btn => {
                            const sev = btn.dataset.severity;
                            btn.classList.toggle('active', sev === 'error' || sev === 'warning');
                        });
                        // Show only error and warning items
                        issuesList.querySelectorAll('.issue-item').forEach(item => {
                            const itemSeverity = item.dataset.severity;
                            if (itemSeverity === 'error' || itemSeverity === 'warning' || itemSeverity === 'critical') {
                                item.classList.remove('hidden');
                            } else {
                                item.classList.add('hidden');
                            }
                        });
                    }
                    issuesPanel.classList.toggle('hidden');
                    setIssuesStatus('');
                });
            }

            document.querySelectorAll('.severity-filter-btn').forEach(btn => {
                btn.addEventListener('click', () => {
                    filterBySeverity(btn.dataset.severity);
                });
            });

            if (issuesSelectAll) {
                issuesSelectAll.addEventListener('click', () => {
                    issuesList.querySelectorAll('input[type="checkbox"]').forEach(cb => {
                        if (!cb.closest('.issue-item').classList.contains('hidden')) {
                            cb.checked = true;
                        }
                    });
                });
            }

            if (issuesDeselectAll) {
                issuesDeselectAll.addEventListener('click', () => {
                    issuesList.querySelectorAll('input[type="checkbox"]').forEach(cb => {
                        if (!cb.closest('.issue-item').classList.contains('hidden')) {
                            cb.checked = false;
                        }
                    });
                });
            }

            if (issuesCopy) {
                issuesCopy.addEventListener('click', async () => {
                    const selected = getSelectedIssues();
                    if (selected.length === 0) {
                        setIssuesStatus('Nothing selected to copy');
                        return;
                    }
                    try {
                        await copyIssues(selected);
                        setIssuesStatus('Copied ' + selected.length + ' issue(s)');
                    } catch (err) {
                        setIssuesStatus('Copy failed: ' + err.message);
                    }
                });
            }

            // Individual comment copy buttons
            document.querySelectorAll('.comment-copy-btn').forEach(btn => {
                btn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    
                    const container = btn.closest('.comment-container');
                    if (!container) return;
                    
                    const filepath = container.dataset.filepath || '';
                    const line = container.dataset.line || '';
                    const comment = container.dataset.comment || '';
                    
                    // Find the corresponding diff line to get code excerpt
                    const commentRow = container.closest('.comment-row');
                    let codeExcerpt = '';
                    if (commentRow) {
                        const prevRow = commentRow.previousElementSibling;
                        if (prevRow && prevRow.classList.contains('diff-line')) {
                            const lineContent = prevRow.querySelector('.line-content');
                            if (lineContent) {
                                codeExcerpt = lineContent.textContent.trim();
                            }
                        }
                    }
                    
                    // Build the copy text
                    let copyText = '';
                    if (filepath) {
                        copyText += filepath;
                        if (line) {
                            copyText += ':' + line;
                        }
                        copyText += '\n\n';
                    }
                    
                    if (codeExcerpt) {
                        copyText += 'Code excerpt:\n  ' + codeExcerpt + '\n\n';
                    }
                    
                    if (comment) {
                        copyText += 'Issue:\n' + comment;
                    }
                    
                    try {
                        await navigator.clipboard.writeText(copyText);
                        
                        const originalText = btn.textContent;
                        btn.classList.add('copied');
                        btn.textContent = 'Copied!';
                        
                        setTimeout(() => {
                            btn.classList.remove('copied');
                            btn.textContent = originalText;
                        }, 2000);
                    } catch (err) {
                        console.error('Copy failed:', err);
                        btn.textContent = 'Failed';
                        setTimeout(() => {
                            btn.textContent = 'Copy';
                        }, 2000);
                    }
                });
            });

            // Progressive comment loading via event polling
            {{if and .ReviewID .APIURL .APIKey}}
            console.log('Initializing event polling with reviewID:', '{{.ReviewID}}');
            const reviewID = '{{.ReviewID}}';
            // Use local proxy to avoid CORS issues
            const apiURL = '';
            
            // State management (matches ReviewEventsPage.tsx lines 27-28)
            let allEvents = [];  // Complete event list (like useState)
            let pollingInterval = null;
            const displayedComments = new Set(); // Track which comments we've already shown

            // Function to create a comment element
            function createCommentElement(comment, batchId) {
                const severity = comment.Severity || 'info';
                const badgeClass = severity === 'critical' ? 'badge-critical' :
                                  severity === 'warning' ? 'badge-warning' : 'badge-info';
                
                const commentDiv = document.createElement('tr');
                commentDiv.className = 'comment-row';
                commentDiv.dataset.progressive = 'true';
                commentDiv.dataset.batchId = batchId;
                
                // Use full content hash for uniqueness to avoid false positives
                const commentKey = comment.FilePath + ':' + comment.Line + ':' + comment.Content;
                if (displayedComments.has(commentKey)) {
                    return null; // Already displayed
                }
                displayedComments.add(commentKey);
                
                commentDiv.innerHTML = '<td colspan="3" class="comment-cell">' +
                    '<div class="comment-container" data-filepath="' + (comment.FilePath || '') + '" ' +
                    'data-line="' + (comment.Line || '') + '" data-comment="' + (comment.Content || '').replace(/"/g, '&quot;') + '">' +
                    '<div class="comment-header">' +
                    '<span class="severity-badge ' + badgeClass + '">' + severity.toUpperCase() + '</span>' +
                    (comment.Category ? '<span class="category-badge">' + comment.Category + '</span>' : '') +
                    '<span class="line-info">Line ' + comment.Line + '</span>' +
                    '<button class="comment-copy-btn" title="Copy this comment">Copy</button>' +
                    '</div>' +
                    '<div class="comment-content">' + comment.Content.replace(/\n/g, '<br>') + '</div>' +
                    '</div></td>';
                
                return commentDiv;
            }

            // Function to insert comment into the appropriate file/line
            function insertComment(comment, batchId) {
                const filePath = comment.FilePath;
                const lineNum = comment.Line;
                
                // Find the file container - encode path to valid ID
                const fileId = filePath.replace(/[^a-zA-Z0-9]/g, '_');
                const fileDiv = document.getElementById(fileId);
                if (!fileDiv) {
                    console.log('File not found:', filePath);
                    return;
                }
                
                // Find the diff table
                const table = fileDiv.querySelector('.diff-table');
                if (!table) {
                    console.log('Table not found for file:', filePath);
                    return;
                }
                
                // Find the line with this new line number
                const rows = table.querySelectorAll('tr.diff-line');
                let targetRow = null;
                for (const row of rows) {
                    const newNumCell = row.querySelector('.new-num');
                    if (newNumCell && newNumCell.textContent.trim() === String(lineNum)) {
                        targetRow = row;
                        break;
                    }
                }
                
                if (!targetRow) {
                    console.log('Line not found:', lineNum, 'in file:', filePath);
                    return;
                }
                
                // Check if this exact comment already exists for this line
                let checkRow = targetRow.nextElementSibling;
                while (checkRow && checkRow.classList.contains('comment-row')) {
                    const existingComment = checkRow.querySelector('.comment-container');
                    if (existingComment) {
                        const existingContent = existingComment.dataset.comment || '';
                        if (existingContent === comment.Content) {
                            return; // Exact duplicate already exists
                        }
                    }
                    checkRow = checkRow.nextElementSibling;
                }
                
                // Create and insert comment
                const commentEl = createCommentElement(comment, batchId);
                if (commentEl) {
                    // Add new-comment class for animation
                    commentEl.classList.add('new-comment');
                    targetRow.insertAdjacentElement('afterend', commentEl);
                    
                    // Remove animation class after it completes
                    setTimeout(() => {
                        commentEl.classList.remove('new-comment');
                    }, 2500);
                    
                    // Update file comment count with animation
                    const countEl = fileDiv.querySelector('.comment-count');
                    if (countEl) {
                        const currentCount = parseInt(countEl.textContent) || 0;
                        countEl.textContent = currentCount + 1;
                        countEl.classList.add('updated');
                        setTimeout(() => countEl.classList.remove('updated'), 600);
                    }
                    
                    // Update sidebar badge with animation
                    const sidebarItem = document.querySelector('.sidebar-file[data-file-id="' + fileId + '"]');
                    if (sidebarItem) {
                        let badge = sidebarItem.querySelector('.sidebar-file-badge');
                        if (!badge) {
                            badge = document.createElement('span');
                            badge.className = 'sidebar-file-badge';
                            sidebarItem.appendChild(badge);
                        }
                        const currentCount = parseInt(badge.textContent) || 0;
                        badge.textContent = currentCount + 1;
                        badge.classList.add('updated');
                        setTimeout(() => badge.classList.remove('updated'), 600);
                    }
                    
                    // Expand the file if collapsed with smooth scroll
                    if (fileDiv.classList.contains('collapsed')) {
                        fileDiv.classList.remove('collapsed');
                        fileDiv.classList.add('expanded');
                        // Smooth scroll to show the new comment
                        setTimeout(() => {
                            commentEl.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                        }, 100);
                    }
                    
                    // Add copy handler to the new button
                    const copyBtn = commentEl.querySelector('.comment-copy-btn');
                    if (copyBtn) {
                        copyBtn.addEventListener('click', async (e) => {
                            e.stopPropagation();
                            const container = commentEl.querySelector('.comment-container');
                            const filepath = container.dataset.filepath || '';
                            const line = container.dataset.line || '';
                            const commentText = container.dataset.comment || '';
                            
                            let copyText = '';
                            if (filepath) {
                                copyText += filepath + ':' + line + '\\n\\n';
                            }
                            copyText += 'Issue:\\n' + commentText;
                            
                            try {
                                await navigator.clipboard.writeText(copyText);
                                copyBtn.textContent = 'Copied!';
                                setTimeout(() => { copyBtn.textContent = 'Copy'; }, 2000);
                            } catch (err) {
                                copyBtn.textContent = 'Failed';
                                setTimeout(() => { copyBtn.textContent = 'Copy'; }, 2000);
                            }
                        });
                    }
                    
                    console.log('Added progressive comment:', filePath + ':' + lineNum);
                }
            }

            // Transform backend event to display format (matches ReviewEventsPage.tsx lines 92-126)
            // NOTE: This intentionally duplicates ReviewEventsPage.tsx logic using vanilla JS (not a maintenance issue)
            function transformEvent(event) {
                let message = '';
                const eventData = event.data || {};
                
                switch (event.type) {
                    case 'log':
                        // Decode escaped characters
                        message = (eventData.message || '').replace(/\\n/g, '\n').replace(/\\t/g, '  ').replace(/\\"/g, '"');
                        break;
                    case 'batch':
                        const batchId = event.batchId || 'unknown';
                        if (eventData.status === 'processing') {
                            const fileCount = eventData.fileCount || 0;
                            message = 'Batch ' + batchId + ' started: processing ' + fileCount + ' file' + (fileCount !== 1 ? 's' : '');
                        } else if (eventData.status === 'completed') {
                            const commentCount = eventData.commentCount || 0;
                            message = 'Batch ' + batchId + ' completed: generated ' + commentCount + ' comment' + (commentCount !== 1 ? 's' : '');
                        } else {
                            message = 'Batch ' + batchId + ': ' + (eventData.status || 'unknown status');
                        }
                        break;
                    case 'status':
                        message = 'Status: ' + (eventData.status || 'unknown');
                        break;
                    case 'artifact':
                        message = eventData.url ? 'Generated: ' + (eventData.kind || 'artifact') : 'Artifact: ' + (eventData.kind || 'unknown');
                        break;
                    case 'completion':
                        const commentCount = eventData.commentCount || 0;
                        message = eventData.resultSummary || ('Process completed with ' + commentCount + ' comment' + (commentCount !== 1 ? 's' : ''));
                        break;
                    default:
                        message = JSON.stringify(eventData);
                }
                
                return {
                    id: event.id,
                    type: event.type,
                    time: event.time,
                    level: event.level || 'info',
                    batchId: event.batchId,
                    data: eventData,
                    message: message
                };
            }

            // Append new events intelligently (matches ReviewEventsPage.tsx lines 50-76)
            // NOTE: This intentionally duplicates ReviewEventsPage.tsx logic using vanilla JS (not a maintenance issue)
            function appendNewEvents(newEvents) {
                if (newEvents.length > allEvents.length) {
                    // Only process the NEW events (key: slice from current length)
                    const addedEvents = newEvents.slice(allEvents.length);
                    
                    console.log('Appending', addedEvents.length, 'new events (total:', newEvents.length, ')');
                    
                    for (const event of addedEvents) {
                        // Render event in events tab
                        renderEvent(event);
                        
                        // Handle batch completion with comments
                        if (event.type === 'batch' && event.data.status === 'completed' && event.data.comments) {
                            const comments = event.data.comments;
                            console.log('Processing batch', event.batchId, 'with', comments.length, 'comments');
                            
                            for (const comment of comments) {
                                insertComment(comment, event.batchId);
                            }
                            
                            // Update top-level comment count in stats section (real-time update)
                            const statsCommentCount = document.querySelector('.stats .stat:nth-child(2) .count');
                            if (statsCommentCount) {
                                const currentCount = parseInt(statsCommentCount.textContent) || 0;
                                statsCommentCount.textContent = currentCount + comments.length;
                            }
                            
                            // Update total comment count (surgical update to preserve other footer content)
                            const totalEl = document.querySelector('.footer');
                            if (totalEl) {
                                const currentTotal = parseInt(totalEl.textContent.match(/\d+/)?.[0] || '0');
                                const newTotal = currentTotal + comments.length;
                                totalEl.textContent = totalEl.textContent.replace(/\d+ total comment\(s\)/, newTotal + ' total comment(s)');
                                if (!totalEl.textContent.includes('total comment')) {
                                    totalEl.textContent = 'Review in progress: ' + newTotal + ' total comment(s)';
                                }
                            }
                        }
                        
                        // Handle completion event
                        if (event.type === 'completion') {
                            console.log('🎉 Review completion event detected!');
                            if (pollingInterval) {
                                clearInterval(pollingInterval);
                                pollingInterval = null;
                                console.log('Stopped polling after completion');
                            }
                            
                            // Update or create status indicator above summary (don't overwrite summary content)
                            let statusEl = document.getElementById('review-status');
                            if (!statusEl) {
                                // Create status element if it doesn't exist
                                statusEl = document.createElement('div');
                                statusEl.id = 'review-status';
                                statusEl.style.cssText = 'padding: 12px; margin-bottom: 16px; background: #238636; color: white; border-radius: 6px; font-weight: 500;';
                                const summaryEl = document.getElementById('summary-content');
                                if (summaryEl && summaryEl.parentNode) {
                                    summaryEl.parentNode.insertBefore(statusEl, summaryEl);
                                }
                            }
                            statusEl.textContent = '✅ Review completed';
                            
                            // Update top-level comment count with final count
                            if (event.data.commentCount !== undefined) {
                                const statsCommentCount = document.querySelector('.stats .stat:nth-child(2) .count');
                                if (statsCommentCount) {
                                    statsCommentCount.textContent = event.data.commentCount;
                                }
                            }
                            
                            // Update footer with final count (surgical update to preserve other footer content)
                            const totalEl = document.querySelector('.footer');
                            if (totalEl && event.data.commentCount !== undefined) {
                                const finalCount = event.data.commentCount;
                                totalEl.textContent = totalEl.textContent.replace(/Review in progress:/, 'Review complete:').replace(/\d+ total comment\(s\)/, finalCount + ' total comment(s)');
                                if (!totalEl.textContent.includes('Review complete')) {
                                    totalEl.textContent = 'Review complete: ' + finalCount + ' total comment(s)';
                                }
                            }
                            
                            // Update events status
                            const eventsStatus = document.getElementById('events-status');
                            if (eventsStatus) {
                                eventsStatus.textContent = '✅ Review completed';
                            }
                        }
                    }
                    
                    // Update state (critical: replace entire array)
                    allEvents = newEvents;
                }
            }

            // Poll for events (matches ReviewEventsPage.tsx lines 78-157)
            async function pollEvents() {
                try {
                    // Fetch ALL events (matches line 90: limit=1000)
                    // NOTE: 1000 limit matches ReviewEventsPage.tsx and is sufficient for typical reviews
                    const url = '/api/v1/diff-review/' + reviewID + '/events?limit=1000';
                    const response = await fetch(url);
                    
                    if (!response.ok) {
                        console.warn('Failed to fetch events (will retry):', response.status);
                        return;
                    }
                    
                    const data = await response.json();
                    const backendEvents = data.events || [];
                    
                    // Transform ALL events (matches lines 92-126)
                    const transformedEvents = backendEvents.map(transformEvent);
                    
                    console.log('[pollEvents] Fetched', transformedEvents.length, 'total events');
                    
                    // Update events status display
                    const eventsStatus = document.getElementById('events-status');
                    if (eventsStatus && transformedEvents.length > 0) {
                        eventsStatus.textContent = transformedEvents.length + ' events received';
                    }
                    
                    // Smart append only NEW events (matches line 136)
                    appendNewEvents(transformedEvents);
                    
                } catch (err) {
                    console.error('[pollEvents] Error:', err);
                }
            }
            
            // Start polling
            console.log('[Event Polling] Starting for review', reviewID);
            pollEvents(); // Initial poll
            pollingInterval = setInterval(pollEvents, 2000);

            // Stop polling after 10 minutes
            setTimeout(() => {
                if (pollingInterval) {
                    clearInterval(pollingInterval);
                    console.log('[Event Polling] Stopped after timeout');
                }
            }, 10 * 60 * 1000);
            {{end}}

            // Copy logs button handler
            const copyLogsBtn = document.getElementById('copy-logs-btn');
            if (copyLogsBtn) {
                copyLogsBtn.addEventListener('click', async function() {
                    const eventsList = document.getElementById('events-list');
                    if (!eventsList) return;
                    
                    const events = eventsList.querySelectorAll('.event-item');
                    const logsText = Array.from(events).map((eventItem, index) => {
                        const typeEl = eventItem.querySelector('.event-type');
                        const timeEl = eventItem.querySelector('.event-time');
                        const messageEl = eventItem.querySelector('.event-message');
                        
                        const type = typeEl ? typeEl.textContent : 'UNKNOWN';
                        const time = timeEl ? timeEl.textContent : '';
                        const message = messageEl ? messageEl.textContent : '';
                        
                        return '[' + (index + 1) + '] ' + time + ' - ' + type + '\\n  ' + message;
                    }).join('\\n\\n');
                    
                    try {
                        await navigator.clipboard.writeText(logsText);
                        // Show temporary success feedback
                        const originalText = copyLogsBtn.innerHTML;
                        copyLogsBtn.innerHTML = '<svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>Copied!';
                        setTimeout(() => {
                            copyLogsBtn.innerHTML = originalText;
                        }, 2000);
                    } catch (err) {
                        console.error('Failed to copy logs:', err);
                    }
                });
            }
        });
    </script>
</body>
</html>
`
