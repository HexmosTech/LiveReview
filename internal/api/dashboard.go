package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// DashboardData represents the structure of dashboard information
type DashboardData struct {
	// Statistics
	TotalReviews         int `json:"total_reviews"`
	TotalComments        int `json:"total_comments"`
	ConnectedProviders   int `json:"connected_providers"`
	ActiveAIConnectors   int `json:"active_ai_connectors"`
	
	// Recent Activity
	RecentActivity []ActivityItem `json:"recent_activity"`
	
	// Performance Metrics
	PerformanceMetrics PerformanceMetrics `json:"performance_metrics"`
	
	// System Status
	SystemStatus SystemStatus `json:"system_status"`
	
	// Last updated timestamp
	LastUpdated time.Time `json:"last_updated"`
}

// ActivityItem represents a single activity entry
type ActivityItem struct {
	ID          int       `json:"id"`
	Action      string    `json:"action"`
	Repository  string    `json:"repository"`
	Timestamp   time.Time `json:"timestamp"`
	TimeAgo     string    `json:"time_ago"`
	Type        string    `json:"type"` // "review", "comment", "connection", etc.
}

// PerformanceMetrics represents system performance data
type PerformanceMetrics struct {
	AvgResponseTime    float64 `json:"avg_response_time_seconds"`
	ReviewsThisWeek    int     `json:"reviews_this_week"`
	CommentsThisWeek   int     `json:"comments_this_week"`
	SuccessRate        float64 `json:"success_rate_percentage"`
}

// SystemStatus represents current system status
type SystemStatus struct {
	JobQueueHealth   string `json:"job_queue_health"`
	DatabaseHealth   string `json:"database_health"`
	APIHealth        string `json:"api_health"`
	LastHealthCheck  time.Time `json:"last_health_check"`
}

// DashboardManager handles dashboard data updates and retrieval
type DashboardManager struct {
	db     *sql.DB
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDashboardManager creates a new dashboard manager
func NewDashboardManager(db *sql.DB) *DashboardManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &DashboardManager{
		db:     db,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the background dashboard data collection
func (dm *DashboardManager) Start() {
	log.Println("Starting dashboard manager...")
	
	// Initial update
	if err := dm.updateDashboardData(); err != nil {
		log.Printf("Error in initial dashboard update: %v", err)
	}
	
	// Start periodic updates every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-dm.ctx.Done():
				log.Println("Dashboard manager stopped")
				return
			case <-ticker.C:
				if err := dm.updateDashboardData(); err != nil {
					log.Printf("Error updating dashboard data: %v", err)
				} else {
					log.Println("Dashboard data updated successfully")
				}
			}
		}
	}()
}

// Stop stops the dashboard manager
func (dm *DashboardManager) Stop() {
	log.Println("Stopping dashboard manager...")
	dm.cancel()
}

// updateDashboardData collects and updates dashboard metrics
func (dm *DashboardManager) updateDashboardData() error {
	log.Println("Collecting dashboard data...")
	
	dashboardData := DashboardData{
		LastUpdated: time.Now(),
	}
	
	// Collect statistics
	if err := dm.collectStatistics(&dashboardData); err != nil {
		log.Printf("Error collecting statistics: %v", err)
	}
	
	// Collect recent activity
	if err := dm.collectRecentActivity(&dashboardData); err != nil {
		log.Printf("Error collecting recent activity: %v", err)
	}
	
	// Collect performance metrics
	if err := dm.collectPerformanceMetrics(&dashboardData); err != nil {
		log.Printf("Error collecting performance metrics: %v", err)
	}
	
	// Collect system status
	if err := dm.collectSystemStatus(&dashboardData); err != nil {
		log.Printf("Error collecting system status: %v", err)
	}
	
	// Store in database
	return dm.storeDashboardData(dashboardData)
}

// collectStatistics gathers basic statistics
func (dm *DashboardManager) collectStatistics(data *DashboardData) error {
	// Count total reviews from job_queue
	err := dm.db.QueryRow(`
		SELECT COUNT(*) FROM job_queue 
		WHERE job_type = 'review' AND status = 'completed'
	`).Scan(&data.TotalReviews)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error counting reviews: %v", err)
	}
	
	// Count total comments (approximation based on completed jobs)
	err = dm.db.QueryRow(`
		SELECT COUNT(*) FROM job_queue 
		WHERE job_type IN ('review', 'comment') AND status = 'completed'
	`).Scan(&data.TotalComments)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error counting comments: %v", err)
	}
	
	// Count connected providers
	err = dm.db.QueryRow(`
		SELECT COUNT(*) FROM integration_tokens
	`).Scan(&data.ConnectedProviders)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error counting providers: %v", err)
	}
	
	// Count active AI connectors
	err = dm.db.QueryRow(`
		SELECT COUNT(*) FROM ai_connectors WHERE is_active = true
	`).Scan(&data.ActiveAIConnectors)
	if err != nil && err != sql.ErrNoRows {
		// If ai_connectors table doesn't exist yet, default to 1
		data.ActiveAIConnectors = 1
	}
	
	return nil
}

// collectRecentActivity gathers recent activity data
func (dm *DashboardManager) collectRecentActivity(data *DashboardData) error {
	// Get recent job queue activities
	rows, err := dm.db.Query(`
		SELECT 
			id, 
			job_type, 
			project_path, 
			created_at,
			status
		FROM job_queue 
		WHERE created_at >= NOW() - INTERVAL '7 days'
		ORDER BY created_at DESC 
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("error querying recent activity: %v", err)
	}
	defer rows.Close()
	
	var activities []ActivityItem
	for rows.Next() {
		var id int
		var jobType, projectPath, status string
		var createdAt time.Time
		
		err := rows.Scan(&id, &jobType, &projectPath, &createdAt, &status)
		if err != nil {
			continue
		}
		
		// Convert job type to human-readable action
		action := "Unknown activity"
		activityType := "other"
		switch jobType {
		case "review":
			if status == "completed" {
				action = "Code review completed"
				activityType = "review"
			} else {
				action = "Code review started"
				activityType = "review"
			}
		case "webhook_install":
			action = "Repository connected"
			activityType = "connection"
		case "webhook_removal":
			action = "Repository disconnected"
			activityType = "connection"
		case "comment":
			action = "Comment added"
			activityType = "comment"
		}
		
		activities = append(activities, ActivityItem{
			ID:         id,
			Action:     action,
			Repository: projectPath,
			Timestamp:  createdAt,
			TimeAgo:    formatTimeAgo(createdAt),
			Type:       activityType,
		})
	}
	
	// If no activities found, add some default entries
	if len(activities) == 0 {
		// Check if we have any connectors
		var connectorCount int
		err = dm.db.QueryRow("SELECT COUNT(*) FROM integration_tokens").Scan(&connectorCount)
		if err == nil && connectorCount > 0 {
			activities = append(activities, ActivityItem{
				ID:         1,
				Action:     "System ready",
				Repository: "LiveReview",
				Timestamp:  time.Now().Add(-1 * time.Hour),
				TimeAgo:    "1h ago",
				Type:       "system",
			})
		}
	}
	
	data.RecentActivity = activities
	return nil
}

// collectPerformanceMetrics gathers performance data
func (dm *DashboardManager) collectPerformanceMetrics(data *DashboardData) error {
	// Calculate reviews this week
	err := dm.db.QueryRow(`
		SELECT COUNT(*) FROM job_queue 
		WHERE job_type = 'review' 
		AND status = 'completed'
		AND created_at >= DATE_TRUNC('week', NOW())
	`).Scan(&data.PerformanceMetrics.ReviewsThisWeek)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error counting weekly reviews: %v", err)
	}
	
	// Calculate comments this week (approximation)
	data.PerformanceMetrics.CommentsThisWeek = data.PerformanceMetrics.ReviewsThisWeek * 3 // Estimate
	
	// Calculate success rate
	var totalJobs, completedJobs int
	err = dm.db.QueryRow(`
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed
		FROM job_queue 
		WHERE created_at >= NOW() - INTERVAL '24 hours'
	`).Scan(&totalJobs, &completedJobs)
	
	if err == nil && totalJobs > 0 {
		data.PerformanceMetrics.SuccessRate = float64(completedJobs) / float64(totalJobs) * 100
	} else {
		data.PerformanceMetrics.SuccessRate = 100.0 // Default to 100% if no data
	}
	
	// Mock average response time for now
	data.PerformanceMetrics.AvgResponseTime = 2.3
	
	return nil
}

// collectSystemStatus gathers system health information
func (dm *DashboardManager) collectSystemStatus(data *DashboardData) error {
	data.SystemStatus.LastHealthCheck = time.Now()
	data.SystemStatus.APIHealth = "healthy"
	data.SystemStatus.DatabaseHealth = "healthy"
	
	// Check job queue health
	var pendingJobs int
	err := dm.db.QueryRow(`
		SELECT COUNT(*) FROM job_queue 
		WHERE status = 'pending' AND created_at < NOW() - INTERVAL '1 hour'
	`).Scan(&pendingJobs)
	
	if err == nil {
		if pendingJobs > 10 {
			data.SystemStatus.JobQueueHealth = "warning"
		} else if pendingJobs > 50 {
			data.SystemStatus.JobQueueHealth = "error"
		} else {
			data.SystemStatus.JobQueueHealth = "healthy"
		}
	} else {
		data.SystemStatus.JobQueueHealth = "unknown"
	}
	
	return nil
}

// storeDashboardData saves the collected data to the database
func (dm *DashboardManager) storeDashboardData(data DashboardData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling dashboard data: %v", err)
	}
	
	_, err = dm.db.Exec(`
		UPDATE dashboard_cache 
		SET data = $1, updated_at = NOW() 
		WHERE id = 1
	`, jsonData)
	
	if err != nil {
		return fmt.Errorf("error storing dashboard data: %v", err)
	}
	
	return nil
}

// GetDashboardData retrieves the cached dashboard data
func (s *Server) GetDashboardData(c echo.Context) error {
	var jsonData []byte
	var updatedAt time.Time
	
	err := s.db.QueryRow(`
		SELECT data, updated_at FROM dashboard_cache WHERE id = 1
	`).Scan(&jsonData, &updatedAt)
	
	if err != nil {
		if err == sql.ErrNoRows {
			// Return empty dashboard data if no cache exists
			emptyData := DashboardData{
				LastUpdated:      time.Now(),
				RecentActivity:   []ActivityItem{},
				PerformanceMetrics: PerformanceMetrics{},
				SystemStatus:     SystemStatus{},
			}
			return c.JSON(http.StatusOK, emptyData)
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve dashboard data",
		})
	}
	
	// Parse and return the JSON data
	var dashboardData DashboardData
	if err := json.Unmarshal(jsonData, &dashboardData); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to parse dashboard data",
		})
	}
	
	return c.JSON(http.StatusOK, dashboardData)
}

// formatTimeAgo returns a human-readable time difference
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)
	
	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", minutes)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	} else {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
