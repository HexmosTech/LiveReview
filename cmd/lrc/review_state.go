package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/livereview/internal/naming"
)

// ReviewState holds the current state of a review for the web UI
type ReviewState struct {
	mu sync.RWMutex

	// Metadata
	ReviewID      string    `json:"reviewID"`
	FriendlyName  string    `json:"friendlyName"`
	GeneratedTime string    `json:"generatedTime"`
	StartedAt     time.Time `json:"-"`

	// Status
	Status string `json:"status"` // "in_progress", "completed", "failed"

	// Content
	Summary string                 `json:"summary"`
	Files   []diffReviewFileResult `json:"files"`

	// Counts
	TotalFiles    int `json:"totalFiles"`
	TotalComments int `json:"totalComments"`

	// UI Config
	Interactive        bool   `json:"interactive"`
	IsPostCommitReview bool   `json:"isPostCommitReview"`
	InitialMsg         string `json:"initialMsg"`

	// API Config (for frontend to know where to poll events)
	APIURL string `json:"apiURL"`

	// Error info
	ErrorSummary string `json:"errorSummary,omitempty"`
}

// NewReviewState creates a new ReviewState with initial values
func NewReviewState(reviewID string, files []diffReviewFileResult, interactive, isPostCommitReview bool, initialMsg, apiURL string) *ReviewState {
	return &ReviewState{
		ReviewID:           reviewID,
		FriendlyName:       naming.GenerateFriendlyName(),
		GeneratedTime:      time.Now().Format("2006-01-02 15:04:05 MST"),
		StartedAt:          time.Now(),
		Status:             "in_progress",
		Files:              files,
		TotalFiles:         len(files),
		Interactive:        interactive,
		IsPostCommitReview: isPostCommitReview,
		InitialMsg:         initialMsg,
		APIURL:             apiURL,
	}
}

// UpdateFromResult updates the state from a final review result
// It merges comments into existing files rather than replacing them,
// to preserve the hunk data from the initial diff parsing
func (rs *ReviewState) UpdateFromResult(result *diffReviewResponse) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.Status = result.Status
	rs.Summary = result.Summary

	// Merge comments from result into existing files (preserving hunks)
	totalComments := 0
	for i := range rs.Files {
		for _, resultFile := range result.Files {
			if rs.Files[i].FilePath == resultFile.FilePath {
				rs.Files[i].Comments = resultFile.Comments
				break
			}
		}
		totalComments += len(rs.Files[i].Comments)
	}
	rs.TotalComments = totalComments
}

// SetCompleted marks the review as completed
func (rs *ReviewState) SetCompleted(summary string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Status = "completed"
	if summary != "" {
		rs.Summary = summary
	}
}

// SetFailed marks the review as failed with an error
func (rs *ReviewState) SetFailed(errorSummary string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Status = "failed"
	rs.ErrorSummary = errorSummary
}

// AddComments adds comments to the total count
// Note: Comments are associated with files in the poll result,
// so full comment merging happens via UpdateFromResult
func (rs *ReviewState) AddComments(count int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.TotalComments += count
}

// GetJSON returns the current state as JSON
func (rs *ReviewState) GetJSON() ([]byte, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return json.Marshal(rs)
}

// ServeHTTP implements http.Handler for the /api/review endpoint
func (rs *ReviewState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	data, err := rs.GetJSON()
	if err != nil {
		http.Error(w, "Failed to serialize state", http.StatusInternalServerError)
		return
	}

	w.Write(data)
}

// PrepareHTMLData converts ReviewState to HTMLTemplateData for initial page render
func (rs *ReviewState) PrepareHTMLData() *HTMLTemplateData {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	files := make([]HTMLFileData, len(rs.Files))
	for i, file := range rs.Files {
		files[i] = prepareFileData(file)
	}

	return &HTMLTemplateData{
		GeneratedTime:      rs.GeneratedTime,
		Summary:            "", // Don't include placeholder summary
		Status:             rs.Status,
		TotalFiles:         rs.TotalFiles,
		TotalComments:      rs.TotalComments,
		Files:              files,
		HasSummary:         false, // Will be set when actual summary arrives
		FriendlyName:       rs.FriendlyName,
		Interactive:        rs.Interactive,
		IsPostCommitReview: rs.IsPostCommitReview,
		InitialMsg:         rs.InitialMsg,
		ReviewID:           rs.ReviewID,
		APIURL:             rs.APIURL,
		APIKey:             "", // Don't expose to frontend
	}
}
