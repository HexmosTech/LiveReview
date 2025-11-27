package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ActivityEntry represents a single activity record
type ActivityEntry struct {
	ID           int             `json:"id"`
	ActivityType string          `json:"activity_type"`
	EventData    json.RawMessage `json:"event_data"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ActivityTracker handles recording activities to the database
type ActivityTracker struct {
	db    *sql.DB
	orgID int64
}

// NewActivityTracker creates a new activity tracker
func NewActivityTracker(db *sql.DB, orgID int64) *ActivityTracker {
	return &ActivityTracker{db: db, orgID: orgID}
}

// TrackActivity records a new activity in the database
func (at *ActivityTracker) TrackActivity(activityType string, eventData map[string]interface{}) error {
	return at.TrackActivityWithReview(activityType, eventData, nil)
}

// TrackActivityWithReview records a new activity in the database with optional review reference
func (at *ActivityTracker) TrackActivityWithReview(activityType string, eventData map[string]interface{}, reviewID *int64) error {
	eventDataJSON, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	query := `
		INSERT INTO recent_activity (activity_type, event_data, review_id, org_id)
		VALUES ($1, $2, $3, $4)
	`
	_, err = at.db.Exec(query, activityType, eventDataJSON, reviewID, at.orgID)
	if err != nil {
		return fmt.Errorf("failed to insert activity: %w", err)
	}

	return nil
}

// GetRecentActivities retrieves recent activities with pagination
func (at *ActivityTracker) GetRecentActivities(limit, offset int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	query := `
		SELECT ra.id, ra.activity_type, ra.event_data, ra.created_at
		FROM recent_activity ra
		WHERE ra.org_id = $1
		   OR (ra.review_id IS NOT NULL AND EXISTS (
			SELECT 1 FROM reviews r WHERE r.id = ra.review_id AND r.org_id = $1
		   ))
		   OR (
			ra.activity_type IN ('connector_created', 'webhook_installed')
			AND EXISTS (
				SELECT 1
				FROM integration_tokens it
				WHERE it.id = CASE
					WHEN (ra.event_data->>'connector_id') ~ '^[0-9]+$' THEN (ra.event_data->>'connector_id')::bigint
					ELSE NULL
				END
				AND it.org_id = $1
			)
		   )
		ORDER BY ra.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := at.db.Query(query, at.orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query activities: %w", err)
	}
	defer rows.Close()

	var activities []ActivityEntry
	for rows.Next() {
		var activity ActivityEntry
		err := rows.Scan(
			&activity.ID,
			&activity.ActivityType,
			&activity.EventData,
			&activity.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, activity)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return activities, nil
}

// GetActivityCount returns the total count of activities
func (at *ActivityTracker) GetActivityCount() (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM recent_activity ra
		WHERE ra.org_id = $1
		   OR (ra.review_id IS NOT NULL AND EXISTS (
			SELECT 1 FROM reviews r WHERE r.id = ra.review_id AND r.org_id = $1
		   ))
		   OR (
			ra.activity_type IN ('connector_created', 'webhook_installed')
			AND EXISTS (
				SELECT 1
				FROM integration_tokens it
				WHERE it.id = CASE
					WHEN (ra.event_data->>'connector_id') ~ '^[0-9]+$' THEN (ra.event_data->>'connector_id')::bigint
					ELSE NULL
				END
				AND it.org_id = $1
			)
		   )
	`
	err := at.db.QueryRow(query, at.orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get activity count: %w", err)
	}
	return count, nil
}

// GetRecentActivities handles the API endpoint for fetching recent activities
func (s *Server) GetRecentActivities(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing organization context",
		})
	}
	// Parse query parameters
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	limit := 20 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	offset := 0 // default
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	// Create activity tracker
	tracker := NewActivityTracker(s.db, orgID)

	// Get activities
	activities, err := tracker.GetRecentActivities(limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch activities",
		})
	}

	// Get total count for pagination
	totalCount, err := tracker.GetActivityCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch activity count",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"activities":  activities,
		"total_count": totalCount,
		"limit":       limit,
		"offset":      offset,
		"has_more":    offset+len(activities) < totalCount,
	})
}

// TrackReviewTriggered is a helper function to track review triggered activities
func TrackReviewTriggered(db *sql.DB, orgID int64, repository, branch, commitHash, triggerType, provider string, connectorID *int64, userEmail, originalURL string) (int64, error) {
	// First, create a review record
	reviewManager := NewReviewManager(db)
	metadata := map[string]interface{}{
		"original_url": originalURL,
	}

	review, err := reviewManager.CreateReviewWithOrg(repository, branch, commitHash, originalURL, triggerType, userEmail, provider, connectorID, metadata, orgID)
	if err != nil {
		return 0, fmt.Errorf("failed to create review record: %w", err)
	}

	// Then track the activity with review_id
	tracker := NewActivityTracker(db, orgID)
	eventData := map[string]interface{}{
		"repository":   repository,
		"branch":       branch,
		"commit_hash":  commitHash,
		"trigger_type": triggerType,
		"provider":     provider,
		"user_email":   userEmail,
		"original_url": originalURL,
		"review_id":    review.ID,
	}

	if connectorID != nil {
		eventData["connector_id"] = *connectorID
	}

	err = tracker.TrackActivityWithReview("review_triggered", eventData, &review.ID)
	if err != nil {
		// Log error but don't fail the main operation
		fmt.Printf("Failed to track review triggered activity: %v\n", err)
	}

	return review.ID, nil
}

// URL extraction helper functions
func extractRepositoryFromURL(urlStr string) string {
	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// Extract repository path from URL path
	// Examples:
	// https://gitlab.com/owner/repo/-/merge_requests/123 -> owner/repo
	// https://github.com/owner/repo/pull/123 -> owner/repo

	path := strings.TrimPrefix(parsedURL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) >= 2 {
		return fmt.Sprintf("%s/%s", parts[0], parts[1])
	}

	return ""
}

func extractBranchFromURL(urlStr string) string {
	// For now, we'll extract from query parameters or return empty
	// This would need more sophisticated parsing based on the MR/PR API responses
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// Try to get from source branch query param if present
	if sourceBranch := parsedURL.Query().Get("source_branch"); sourceBranch != "" {
		return sourceBranch
	}

	// For MR/PR URLs, we should ideally fetch the actual branch info from the API
	// For now, we'll just return empty so the UI doesn't show "unknown"
	return ""
}

func extractCommitFromURL(urlStr string) string {
	// Extract commit hash from URL if present, otherwise return empty
	// This would typically be fetched from the MR/PR API
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// Try to get from commit query param if present
	if commit := parsedURL.Query().Get("commit"); commit != "" {
		return commit
	}

	// Try to extract from path using regex for commit-like patterns
	commitRegex := regexp.MustCompile(`/commit/([a-f0-9]{7,40})`)
	if matches := commitRegex.FindStringSubmatch(parsedURL.Path); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// TrackConnectorCreated is a helper function to track connector creation activities
func TrackConnectorCreated(db *sql.DB, orgID int64, provider, providerURL string, connectorID int, repositoryCount int) {
	tracker := NewActivityTracker(db, orgID)

	eventData := map[string]interface{}{
		"provider":         provider,
		"provider_url":     providerURL,
		"connector_id":     connectorID,
		"repository_count": repositoryCount,
		"org_id":           orgID,
	}

	err := tracker.TrackActivity("connector_created", eventData)
	if err != nil {
		// Log error but don't fail the main operation
		fmt.Printf("Failed to track connector created activity: %v\n", err)
	}
}

// TrackWebhookInstalled is a helper function to track webhook installation activities
func TrackWebhookInstalled(db *sql.DB, orgID int64, repository string, connectorID int, provider string, success bool) {
	tracker := NewActivityTracker(db, orgID)

	eventData := map[string]interface{}{
		"repository":   repository,
		"connector_id": connectorID,
		"provider":     provider,
		"success":      success,
		"org_id":       orgID,
	}

	err := tracker.TrackActivity("webhook_installed", eventData)
	if err != nil {
		// Log error but don't fail the main operation
		fmt.Printf("Failed to track webhook installation activity: %v\n", err)
	}
}

// BackfillRecentActivityOrgIDs updates existing recent_activity rows to ensure org_id matches source records.
func BackfillRecentActivityOrgIDs(db *sql.DB) error {
	queries := []string{
		`UPDATE recent_activity ra
			SET org_id = r.org_id
			FROM reviews r
			WHERE ra.review_id = r.id
			  AND ra.org_id IS DISTINCT FROM r.org_id`,
		`UPDATE recent_activity ra
			SET org_id = it.org_id
			FROM integration_tokens it,
				LATERAL (
					SELECT (ra.event_data->>'connector_id')::bigint AS connector_id
					WHERE (ra.event_data->>'connector_id') ~ '^[0-9]+$'
				) conn
			WHERE ra.activity_type = 'connector_created'
			  AND conn.connector_id = it.id
			  AND ra.org_id IS DISTINCT FROM it.org_id`,
		`UPDATE recent_activity ra
			SET org_id = it.org_id
			FROM integration_tokens it,
				LATERAL (
					SELECT (ra.event_data->>'connector_id')::bigint AS connector_id
					WHERE (ra.event_data->>'connector_id') ~ '^[0-9]+$'
				) conn
			WHERE ra.activity_type = 'webhook_installed'
			  AND conn.connector_id = it.id
			  AND ra.org_id IS DISTINCT FROM it.org_id`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}
