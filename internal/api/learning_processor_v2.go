package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Phase 7.3: Learning processor for provider-agnostic learning extraction and application
// Extracted from webhook_handler.go - provider-independent learning processing

// LearningProcessorV2Impl implements the LearningProcessorV2 interface
type LearningProcessorV2Impl struct {
	server *Server // For accessing database operations and learning API
}

// NewLearningProcessorV2 creates a new learning processor instance
func NewLearningProcessorV2(server *Server) LearningProcessorV2 {
	return &LearningProcessorV2Impl{
		server: server,
	}
}

// ExtractLearning extracts learning metadata from responses (provider-agnostic)
// Extracted from augmentResponseWithLearningMetadata
func (lp *LearningProcessorV2Impl) ExtractLearning(response string, context CommentContextV2) (*LearningMetadataV2, error) {
	log.Printf("[DEBUG] Extracting learning from response")

	// Get the original comment from context metadata if available
	var originalComment string
	if beforeComments, ok := context.MRContext.Metadata["before_comments"].([]string); ok && len(beforeComments) > 0 {
		// Extract the most recent comment as the original
		if len(beforeComments) > 0 {
			originalComment = beforeComments[len(beforeComments)-1]
		}
	}

	originalCommentLower := strings.ToLower(originalComment)
	responseLower := strings.ToLower(response)

	// Detect learning opportunities from the comment and response
	var learningType string = "explanation"
	var tags []string
	var title string
	var content string

	// Pattern 1: Documentation questions
	if strings.Contains(originalCommentLower, "document") || strings.Contains(originalCommentLower, "warrant") ||
		strings.Contains(responseLower, "documentation") {
		learningType = "best_practice"
		title = "Documentation Best Practices"
		content = "Always document public functions and complex logic. Include purpose, inputs, outputs, and caveats for maintainability."
		tags = []string{"documentation", "best-practices", "maintainability"}
	} else if strings.Contains(originalCommentLower, "performance") || strings.Contains(originalCommentLower, "slow") ||
		strings.Contains(originalCommentLower, "optimize") || strings.Contains(responseLower, "performance") {
		// Pattern 2: Performance questions
		learningType = "optimization"
		title = "Performance Optimization Guidelines"
		content = "Profile before optimizing. Focus on algorithmic improvements first, then micro-optimizations. Document performance-critical sections."
		tags = []string{"performance", "optimization", "profiling"}
	} else if strings.Contains(originalCommentLower, "security") || strings.Contains(originalCommentLower, "vulnerable") ||
		strings.Contains(originalCommentLower, "safe") || strings.Contains(responseLower, "security") {
		// Pattern 3: Security concerns
		learningType = "security_review"
		title = "Security Review Checklist"
		content = "Always validate inputs, use parameterized queries, apply principle of least privilege, and review for injection vulnerabilities."
		tags = []string{"security", "validation", "best-practices"}
	} else if strings.Contains(originalCommentLower, "error") || strings.Contains(originalCommentLower, "exception") ||
		strings.Contains(originalCommentLower, "handle") || strings.Contains(responseLower, "error") {
		// Pattern 4: Error handling questions
		learningType = "error_handling"
		title = "Error Handling Patterns"
		content = "Use proper error handling, fail fast, provide meaningful error messages, and log appropriately for debugging."
		tags = []string{"error-handling", "debugging", "logging"}
	} else if strings.Contains(originalCommentLower, "test") || strings.Contains(originalCommentLower, "testing") ||
		strings.Contains(responseLower, "test") {
		// Pattern 5: Testing discussions
		learningType = "testing"
		title = "Testing Best Practices"
		content = "Write unit tests for core logic, integration tests for system interactions, and maintain good test coverage for critical paths."
		tags = []string{"testing", "quality-assurance", "best-practices"}
	} else if strings.Contains(originalCommentLower, "design") || strings.Contains(originalCommentLower, "pattern") ||
		strings.Contains(originalCommentLower, "architecture") || strings.Contains(responseLower, "design") {
		// Pattern 6: Design and architecture discussions
		learningType = "design_pattern"
		title = "Design Pattern Guidelines"
		content = "Follow SOLID principles, prefer composition over inheritance, and choose patterns that solve real problems without over-engineering."
		tags = []string{"design-patterns", "architecture", "solid-principles"}
	}

	// Only create learning metadata if we found relevant patterns
	if len(tags) == 0 {
		log.Printf("[DEBUG] No learning patterns detected")
		return nil, nil
	}

	learning := &LearningMetadataV2{
		Type:       learningType,
		Content:    lp.truncateContent(content, 500),
		Context:    lp.truncateContent(response, 300),
		Confidence: lp.calculateConfidence(originalComment, response, tags),
		Tags:       tags,
		Metadata: map[string]interface{}{
			"title":             title,
			"scope_kind":        "merge_request",
			"extraction_method": "pattern_matching",
			"original_comment":  lp.truncateContent(originalComment, 200),
		},
	}

	log.Printf("[DEBUG] Extracted learning: type=%s, tags=%v, confidence=%.2f",
		learning.Type, learning.Tags, learning.Confidence)

	return learning, nil
}

// ApplyLearning applies learning metadata to the learning system (provider-agnostic)
// Extracted from applyLearningFromReply
func (lp *LearningProcessorV2Impl) ApplyLearning(learning *LearningMetadataV2) error {
	if learning == nil {
		return nil // No learning to apply
	}

	log.Printf("[INFO] Applying learning: %s", learning.Type)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build request payload for the learning API
	requestPayload := map[string]interface{}{
		"action":     "add",
		"title":      learning.Metadata["title"],
		"body":       learning.Content,
		"scope_kind": learning.Metadata["scope_kind"],
		"tags":       learning.Tags,
		"metadata":   learning.Metadata,
		"confidence": learning.Confidence,
	}

	// Add repository ID if available
	if repoID, ok := learning.Metadata["repo_id"].(string); ok && repoID != "" {
		requestPayload["repo_id"] = repoID
	}

	// Marshal request
	jsonBody, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal learning request: %w", err)
	}

	// Create HTTP request to learning API
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8888/api/v1/learnings/apply-action-from-reply", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create learning request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Note: Organization context would be set by the caller

	// Execute request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call learning API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("learning API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get learning ID
	var result struct {
		ShortID string `json:"short_id"`
		Action  string `json:"action"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode learning API response: %w", err)
	}

	log.Printf("[INFO] Learning %s: %s (LR-%s)", result.Action, learning.Metadata["title"], result.ShortID)

	// Store the short ID back in metadata for reference
	learning.Metadata["short_id"] = result.ShortID

	return nil
}

// FindOrgIDForRepository finds the organization ID for a repository (provider-agnostic)
// Generalized from findOrgIDForGitLabInstance
func (lp *LearningProcessorV2Impl) FindOrgIDForRepository(repo UnifiedRepositoryV2) (int64, error) {
	log.Printf("[DEBUG] Finding org ID for repository %s", repo.FullName)

	// The organization finding logic depends on how LiveReview maps repositories to organizations
	// This would typically query the database to find the organization associated with the repository

	// For now, return a default org ID or implement based on the repository URL/provider
	switch {
	case strings.Contains(repo.WebURL, "gitlab"):
		return lp.findOrgIDForGitLabInstance(repo.WebURL)
	case strings.Contains(repo.WebURL, "github"):
		return lp.findOrgIDForGitHubRepo(repo.FullName)
	case strings.Contains(repo.WebURL, "bitbucket"):
		return lp.findOrgIDForBitbucketRepo(repo.FullName)
	default:
		log.Printf("[WARN] Unknown repository type for %s, using default org ID", repo.WebURL)
		return 1, nil // Default organization ID
	}
}

// Provider-specific organization ID finding methods

// findOrgIDForGitLabInstance finds organization ID for GitLab instances
// Extracted from webhook_handler.go
func (lp *LearningProcessorV2Impl) findOrgIDForGitLabInstance(gitlabURL string) (int64, error) {
	// Extract base URL from GitLab instance
	baseURL := strings.TrimSuffix(gitlabURL, "/")
	if idx := strings.Index(baseURL, "/-/"); idx != -1 {
		baseURL = baseURL[:idx]
	}

	// Query database for organization mapping
	// This would typically be a database query like:
	// SELECT org_id FROM integrations WHERE provider = 'gitlab' AND instance_url = ?

	// For now, return a default based on common patterns
	if strings.Contains(baseURL, "gitlab.com") {
		return 1, nil // Default for public GitLab
	}

	// For private instances, we'd need to lookup in the database
	// Return default org ID for now
	return 1, nil
}

// findOrgIDForGitHubRepo finds organization ID for GitHub repositories
func (lp *LearningProcessorV2Impl) findOrgIDForGitHubRepo(repoFullName string) (int64, error) {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return 1, fmt.Errorf("invalid GitHub repository format: %s", repoFullName)
	}

	owner := parts[0]

	// Query database for GitHub organization mapping
	// This would typically be a database query like:
	// SELECT org_id FROM integrations WHERE provider = 'github' AND owner = ?

	log.Printf("[DEBUG] Finding org ID for GitHub owner: %s", owner)

	// Return default org ID for now
	return 1, nil
}

// findOrgIDForBitbucketRepo finds organization ID for Bitbucket repositories
func (lp *LearningProcessorV2Impl) findOrgIDForBitbucketRepo(repoFullName string) (int64, error) {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return 1, fmt.Errorf("invalid Bitbucket repository format: %s", repoFullName)
	}

	workspace := parts[0]

	// Query database for Bitbucket workspace mapping
	// This would typically be a database query like:
	// SELECT org_id FROM integrations WHERE provider = 'bitbucket' AND workspace = ?

	log.Printf("[DEBUG] Finding org ID for Bitbucket workspace: %s", workspace)

	// Return default org ID for now
	return 1, nil
}

// Helper methods for learning processing

// calculateConfidence calculates confidence score for learning extraction
func (lp *LearningProcessorV2Impl) calculateConfidence(originalComment, response string, tags []string) float64 {
	confidence := 0.5 // Base confidence

	// Increase confidence based on multiple matching patterns
	if len(tags) > 1 {
		confidence += 0.1
	}

	// Increase confidence for explicit learning keywords
	learningKeywords := []string{"learn", "understand", "explain", "how", "why", "best practice"}
	originalLower := strings.ToLower(originalComment)

	keywordMatches := 0
	for _, keyword := range learningKeywords {
		if strings.Contains(originalLower, keyword) {
			keywordMatches++
		}
	}

	confidence += float64(keywordMatches) * 0.1

	// Increase confidence for longer, more detailed responses
	if len(response) > 200 {
		confidence += 0.1
	}

	// Cap confidence at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// truncateContent truncates content to specified length with ellipsis
func (lp *LearningProcessorV2Impl) truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength-3] + "..."
}

// GenerateLearningAcknowledgment generates an acknowledgment message for applied learning
func (lp *LearningProcessorV2Impl) GenerateLearningAcknowledgment(learning *LearningMetadataV2) string {
	if learning == nil {
		return ""
	}

	shortID := ""
	if sid, ok := learning.Metadata["short_id"].(string); ok {
		shortID = sid
	}

	title := ""
	if t, ok := learning.Metadata["title"].(string); ok {
		title = t
	}

	if shortID != "" && title != "" {
		return fmt.Sprintf("💡 *Learning captured: [%s](#%s)*", title, shortID)
	} else if title != "" {
		return fmt.Sprintf("💡 *Learning captured: %s*", title)
	}

	return "💡 *Learning opportunity noted for future reference.*"
}

// AugmentResponseWithLearning augments response with learning acknowledgment
// This combines response generation with learning application
func (lp *LearningProcessorV2Impl) AugmentResponseWithLearning(ctx context.Context, response string, context CommentContextV2, repo UnifiedRepositoryV2) (string, error) {
	// Extract learning from the response and context
	learning, err := lp.ExtractLearning(response, context)
	if err != nil {
		log.Printf("[WARN] Failed to extract learning: %v", err)
		return response, nil // Return original response without learning
	}

	if learning == nil {
		return response, nil // No learning detected
	}

	// Find organization ID for the repository
	orgID, err := lp.FindOrgIDForRepository(repo)
	if err != nil {
		log.Printf("[WARN] Failed to find org ID for repository %s: %v", repo.FullName, err)
		orgID = 1 // Use default
	}

	// Set org ID in learning metadata
	learning.OrgID = orgID

	// Apply the learning
	err = lp.ApplyLearning(learning)
	if err != nil {
		log.Printf("[WARN] Failed to apply learning: %v", err)
		return response, nil // Return original response without learning acknowledgment
	}

	// Generate acknowledgment
	ack := lp.GenerateLearningAcknowledgment(learning)

	// Combine response with acknowledgment
	if ack != "" {
		return response + "\n\n" + ack, nil
	}

	return response, nil
}

// AnalyzeLearningOpportunities analyzes context for potential learning opportunities
func (lp *LearningProcessorV2Impl) AnalyzeLearningOpportunities(context CommentContextV2) map[string]interface{} {
	analysis := make(map[string]interface{})

	// Analyze comment patterns for learning indicators
	learningIndicators := []string{
		"how", "why", "what", "explain", "understand", "learn",
		"best practice", "pattern", "recommend", "suggest",
	}

	beforeComments, _ := context.MRContext.Metadata["before_comments"].([]string)
	commentText := strings.Join(beforeComments, " ")
	commentLower := strings.ToLower(commentText)

	indicatorCount := 0
	for _, indicator := range learningIndicators {
		if strings.Contains(commentLower, indicator) {
			indicatorCount++
		}
	}

	analysis["learning_indicators"] = indicatorCount
	analysis["has_learning_potential"] = indicatorCount > 0
	analysis["learning_strength"] = float64(indicatorCount) / float64(len(learningIndicators))

	// Analyze technical domains
	domains := map[string][]string{
		"security":      {"security", "vulnerable", "safe", "auth", "permission"},
		"performance":   {"performance", "slow", "fast", "optimize", "efficient"},
		"testing":       {"test", "testing", "coverage", "mock", "assert"},
		"documentation": {"document", "comment", "explain", "clarify"},
		"architecture":  {"design", "pattern", "structure", "organize"},
	}

	domainMatches := make(map[string]int)
	for domain, keywords := range domains {
		count := 0
		for _, keyword := range keywords {
			if strings.Contains(commentLower, keyword) {
				count++
			}
		}
		if count > 0 {
			domainMatches[domain] = count
		}
	}

	analysis["domain_matches"] = domainMatches
	analysis["primary_domain"] = lp.findPrimaryDomain(domainMatches)

	return analysis
}

// findPrimaryDomain finds the domain with the most keyword matches
func (lp *LearningProcessorV2Impl) findPrimaryDomain(domainMatches map[string]int) string {
	maxCount := 0
	primaryDomain := "general"

	for domain, count := range domainMatches {
		if count > maxCount {
			maxCount = count
			primaryDomain = domain
		}
	}

	return primaryDomain
}
