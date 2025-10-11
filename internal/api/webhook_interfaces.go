package api

import (
	"context"
)

// Phase 1.3: Interfaces with V2 naming for conflict-free migration
// All interfaces use V2 suffix to prevent conflicts during migration

// WebhookProviderV2 - Main provider interface for platform-specific operations
type WebhookProviderV2 interface {
	// Provider identification
	ProviderName() string
	CanHandleWebhook(headers map[string]string, body []byte) bool

	// Convert provider payload to unified structure
	ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error)
	ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error)

	// Fetch additional context data (commits, discussions, etc.)
	FetchMergeRequestData(event *UnifiedWebhookEventV2) error

	// Post responses back to platform
	PostCommentReply(event *UnifiedWebhookEventV2, content string) error
	PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error
	PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error
}

// UnifiedProcessorV2 - Main unified processing interface (provider-agnostic)
type UnifiedProcessorV2 interface {
	// Check if event warrants a response
	CheckResponseWarrant(event UnifiedWebhookEventV2, botInfo *UnifiedBotUserInfoV2) (bool, ResponseScenarioV2)

	// Process comment reply flow
	ProcessCommentReply(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2, orgID int64) (string, *LearningMetadataV2, error)

	// Process full review flow
	ProcessFullReview(ctx context.Context, event UnifiedWebhookEventV2, timeline *UnifiedTimelineV2) ([]UnifiedReviewCommentV2, *LearningMetadataV2, error)
}

// LearningProcessorV2 - Learning extraction and application interface
type LearningProcessorV2 interface {
	// Extract learning metadata from responses
	ExtractLearning(response string, context CommentContextV2) (*LearningMetadataV2, error)

	// Apply learning from extracted metadata
	ApplyLearning(learning *LearningMetadataV2) error

	// Find organization ID for repository (provider-agnostic)
	FindOrgIDForRepository(repo UnifiedRepositoryV2) (int64, error)
}

// WebhookOrchestratorV2 - Main orchestrator interface for coordinating flows
type WebhookOrchestratorV2Interface interface {
	// Process comment-based events
	ProcessCommentEvent(provider string, payload interface{}) error

	// Process reviewer assignment events
	ProcessReviewerEvent(provider string, payload interface{}) error

	// Get provider by name
	GetProvider(name string) (WebhookProviderV2, error)

	// Initialize all providers
	InitializeProviders() error
}
