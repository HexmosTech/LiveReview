package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Review represents a code review record
type Review struct {
	ID          int64           `json:"id"`
	Repository  string          `json:"repository"`
	Branch      string          `json:"branch"`
	CommitHash  string          `json:"commit_hash"`
	PrMrURL     string          `json:"pr_mr_url"`
	ConnectorID *int64          `json:"connector_id"`
	Status      string          `json:"status"`
	TriggerType string          `json:"trigger_type"`
	UserEmail   string          `json:"user_email"`
	Provider    string          `json:"provider"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at"`
	Metadata    json.RawMessage `json:"metadata"`
}

// AIComment represents an AI-generated comment
type AIComment struct {
	ID         int64           `json:"id"`
	ReviewID   int64           `json:"review_id"`
	Type       string          `json:"comment_type"`
	Content    json.RawMessage `json:"content"`
	FilePath   *string         `json:"file_path"`
	LineNumber *int            `json:"line_number"`
	CreatedAt  time.Time       `json:"created_at"`
	OrgID      int64           `json:"org_id"`
}

// ReviewManager handles review operations
type ReviewManager struct {
	db *sql.DB
}

// NewReviewManager creates a new review manager
func NewReviewManager(db *sql.DB) *ReviewManager {
	return &ReviewManager{db: db}
}

// CreateReview creates a new review record
func (rm *ReviewManager) CreateReview(repository, branch, commitHash, prMrURL, triggerType, userEmail, provider string, connectorID *int64, metadata map[string]interface{}) (*Review, error) {
	var metadataJSON []byte
	var err error

	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO reviews (repository, branch, commit_hash, pr_mr_url, connector_id, trigger_type, user_email, provider, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	var review Review
	err = rm.db.QueryRow(query, repository, branch, commitHash, prMrURL, connectorID, triggerType, userEmail, provider, metadataJSON).Scan(&review.ID, &review.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create review: %w", err)
	}

	// Fill in the rest of the review data
	review.Repository = repository
	review.Branch = branch
	review.CommitHash = commitHash
	review.PrMrURL = prMrURL
	review.ConnectorID = connectorID
	review.Status = "created"
	review.TriggerType = triggerType
	review.UserEmail = userEmail
	review.Provider = provider
	review.Metadata = metadataJSON

	return &review, nil
}

// UpdateReviewStatus updates the status of a review
func (rm *ReviewManager) UpdateReviewStatus(reviewID int64, status string) error {
	var query string
	var args []interface{}

	switch status {
	case "in_progress":
		query = `UPDATE reviews SET status = $1, started_at = NOW() WHERE id = $2`
		args = []interface{}{status, reviewID}
	case "completed", "failed":
		query = `UPDATE reviews SET status = $1, completed_at = NOW() WHERE id = $2`
		args = []interface{}{status, reviewID}
	default:
		query = `UPDATE reviews SET status = $1 WHERE id = $2`
		args = []interface{}{status, reviewID}
	}

	_, err := rm.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update review status: %w", err)
	}

	return nil
}

// GetReview retrieves a review by ID
func (rm *ReviewManager) GetReview(reviewID int64) (*Review, error) {
	query := `
		SELECT id, repository, branch, commit_hash, pr_mr_url, connector_id, status, trigger_type, user_email, provider, created_at, started_at, completed_at, metadata
		FROM reviews
		WHERE id = $1
	`

	var review Review
	err := rm.db.QueryRow(query, reviewID).Scan(
		&review.ID,
		&review.Repository,
		&review.Branch,
		&review.CommitHash,
		&review.PrMrURL,
		&review.ConnectorID,
		&review.Status,
		&review.TriggerType,
		&review.UserEmail,
		&review.Provider,
		&review.CreatedAt,
		&review.StartedAt,
		&review.CompletedAt,
		&review.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get review: %w", err)
	}

	return &review, nil
}

// AddAIComment adds an AI comment to a review
func (rm *ReviewManager) AddAIComment(reviewID int64, commentType string, content map[string]interface{}, filePath *string, lineNumber *int, orgID int64) (*AIComment, error) {
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal comment content: %w", err)
	}

	query := `
		INSERT INTO ai_comments (review_id, comment_type, content, file_path, line_number, org_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	var comment AIComment
	err = rm.db.QueryRow(query, reviewID, commentType, contentJSON, filePath, lineNumber, orgID).Scan(&comment.ID, &comment.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to add AI comment: %w", err)
	}

	comment.ReviewID = reviewID
	comment.Type = commentType
	comment.Content = contentJSON
	comment.FilePath = filePath
	comment.LineNumber = lineNumber
	comment.OrgID = orgID

	return &comment, nil
}

// GetReviewComments retrieves all AI comments for a review
func (rm *ReviewManager) GetReviewComments(reviewID int64) ([]AIComment, error) {
	query := `
		SELECT id, review_id, comment_type, content, file_path, line_number, created_at, org_id
		FROM ai_comments
		WHERE review_id = $1
		ORDER BY created_at ASC
	`

	rows, err := rm.db.Query(query, reviewID)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI comments: %w", err)
	}
	defer rows.Close()

	var comments []AIComment
	for rows.Next() {
		var comment AIComment
		err := rows.Scan(
			&comment.ID,
			&comment.ReviewID,
			&comment.Type,
			&comment.Content,
			&comment.FilePath,
			&comment.LineNumber,
			&comment.CreatedAt,
			&comment.OrgID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan AI comment: %w", err)
		}
		comments = append(comments, comment)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return comments, nil
}

// GetReviewDuration calculates the duration of a review
func (rm *ReviewManager) GetReviewDuration(reviewID int64) (*time.Duration, error) {
	review, err := rm.GetReview(reviewID)
	if err != nil {
		return nil, err
	}

	if review.StartedAt == nil || review.CompletedAt == nil {
		return nil, nil // Review not yet completed
	}

	duration := review.CompletedAt.Sub(*review.StartedAt)
	return &duration, nil
}

// GetTotalAIComments returns the total count of AI comments across all reviews
func (rm *ReviewManager) GetTotalAIComments() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM ai_comments`
	err := rm.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get AI comments count: %w", err)
	}
	return count, nil
}

// TrackAICommentFromURL is a helper function to track AI comments based on MR/PR URL
// This is useful when we have the comment content but need to find the associated review
func TrackAICommentFromURL(db *sql.DB, prMrURL, commentType string, content map[string]interface{}, filePath *string, lineNumber *int, orgID int64) error {
	// Find the review by PR/MR URL
	query := `
		SELECT id FROM reviews 
		WHERE pr_mr_url = $1 
		ORDER BY created_at DESC 
		LIMIT 1
	`

	var reviewID int64
	err := db.QueryRow(query, prMrURL).Scan(&reviewID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No review found for this URL, skip tracking
			return nil
		}
		return fmt.Errorf("failed to find review for URL %s: %w", prMrURL, err)
	}

	// Add the AI comment
	reviewManager := NewReviewManager(db)
	_, err = reviewManager.AddAIComment(reviewID, commentType, content, filePath, lineNumber, orgID)
	if err != nil {
		return fmt.Errorf("failed to add AI comment: %w", err)
	}

	return nil
}
