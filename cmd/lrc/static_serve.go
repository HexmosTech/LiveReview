package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

// JSONTemplateData is the data structure passed to the Preact app as JSON
// It mirrors HTMLTemplateData but is serialized to JSON for the frontend
type JSONTemplateData struct {
	GeneratedTime      string         `json:"GeneratedTime"`
	Summary            string         `json:"Summary"`
	Status             string         `json:"Status"`
	TotalFiles         int            `json:"TotalFiles"`
	TotalComments      int            `json:"TotalComments"`
	Files              []JSONFileData `json:"Files"`
	HasSummary         bool           `json:"HasSummary"`
	FriendlyName       string         `json:"FriendlyName"`
	Interactive        bool           `json:"Interactive"`
	IsPostCommitReview bool           `json:"IsPostCommitReview"`
	InitialMsg         string         `json:"InitialMsg"`
	ReviewID           string         `json:"ReviewID"`
	APIURL             string         `json:"APIURL"`
	APIKey             string         `json:"APIKey"`
}

// JSONFileData represents a file for JSON serialization
type JSONFileData struct {
	ID           string         `json:"ID"`
	FilePath     string         `json:"FilePath"`
	HasComments  bool           `json:"HasComments"`
	CommentCount int            `json:"CommentCount"`
	Hunks        []JSONHunkData `json:"Hunks"`
}

// JSONHunkData represents a hunk for JSON serialization
type JSONHunkData struct {
	Header string         `json:"Header"`
	Lines  []JSONLineData `json:"Lines"`
}

// JSONLineData represents a line in a diff for JSON serialization
type JSONLineData struct {
	OldNum    string            `json:"OldNum"`
	NewNum    string            `json:"NewNum"`
	Content   string            `json:"Content"`
	Class     string            `json:"Class"`
	IsComment bool              `json:"IsComment"`
	Comments  []JSONCommentData `json:"Comments,omitempty"`
}

// JSONCommentData represents a comment for JSON serialization
type JSONCommentData struct {
	Severity    string `json:"Severity"`
	BadgeClass  string `json:"BadgeClass"`
	Category    string `json:"Category"`
	Content     string `json:"Content"`
	HasCategory bool   `json:"HasCategory"`
	Line        int    `json:"Line"`
	FilePath    string `json:"FilePath"`
}

// convertToJSONData converts HTMLTemplateData to JSONTemplateData
func convertToJSONData(data *HTMLTemplateData) *JSONTemplateData {
	files := make([]JSONFileData, len(data.Files))
	for i, file := range data.Files {
		hunks := make([]JSONHunkData, len(file.Hunks))
		for j, hunk := range file.Hunks {
			lines := make([]JSONLineData, len(hunk.Lines))
			for k, line := range hunk.Lines {
				var comments []JSONCommentData
				if line.IsComment {
					comments = make([]JSONCommentData, len(line.Comments))
					for l, comment := range line.Comments {
						comments[l] = JSONCommentData{
							Severity:    comment.Severity,
							BadgeClass:  comment.BadgeClass,
							Category:    comment.Category,
							Content:     comment.Content,
							HasCategory: comment.HasCategory,
							Line:        comment.Line,
							FilePath:    comment.FilePath,
						}
					}
				}
				lines[k] = JSONLineData{
					OldNum:    line.OldNum,
					NewNum:    line.NewNum,
					Content:   line.Content,
					Class:     line.Class,
					IsComment: line.IsComment,
					Comments:  comments,
				}
			}
			hunks[j] = JSONHunkData{
				Header: hunk.Header,
				Lines:  lines,
			}
		}
		files[i] = JSONFileData{
			ID:           file.ID,
			FilePath:     file.FilePath,
			HasComments:  file.HasComments,
			CommentCount: file.CommentCount,
			Hunks:        hunks,
		}
	}

	return &JSONTemplateData{
		GeneratedTime:      data.GeneratedTime,
		Summary:            data.Summary,
		Status:             data.Status,
		TotalFiles:         data.TotalFiles,
		TotalComments:      data.TotalComments,
		Files:              files,
		HasSummary:         data.HasSummary,
		FriendlyName:       data.FriendlyName,
		Interactive:        data.Interactive,
		IsPostCommitReview: data.IsPostCommitReview,
		InitialMsg:         data.InitialMsg,
		ReviewID:           data.ReviewID,
		APIURL:             data.APIURL,
		APIKey:             data.APIKey,
	}
}

// renderPreactHTML renders the Preact-based HTML with embedded JSON data
func renderPreactHTML(data *HTMLTemplateData) (string, error) {
	// Convert to JSON-serializable format
	jsonData := convertToJSONData(data)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return "", err
	}

	// Read the HTML template
	htmlBytes, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		return "", err
	}

	// Replace the placeholder with actual JSON data
	html := string(htmlBytes)
	html = strings.Replace(html, "{{.JSONData}}", string(jsonBytes), 1)

	// Update title if friendly name is present
	if data.FriendlyName != "" {
		html = strings.Replace(html, "<title>LiveReview Results</title>",
			"<title>LiveReview Results — "+data.FriendlyName+"</title>", 1)
	}

	return html, nil
}

// getStaticHandler returns an HTTP handler for serving static files
func getStaticHandler() http.Handler {
	// Get the static subdirectory
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticFS))
}

// serveStaticFile serves a specific static file
func serveStaticFile(w http.ResponseWriter, r *http.Request, filename string) error {
	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		return err
	}

	// Set content type based on extension
	switch {
	case strings.HasSuffix(filename, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(filename, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(filename, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	w.Write(content)
	return nil
}
