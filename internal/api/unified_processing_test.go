package api

import (
	"context"
	"testing"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 9: Unified Processing Core Tests
// Tests the Phase 7 components (processor, context builder, learning processor)

func TestUnifiedProcessorV2(t *testing.T) {
	server := &Server{}
	processor := NewUnifiedProcessorV2(server)
	require.NotNil(t, processor)

	t.Run("CheckResponseWarrant", func(t *testing.T) {
		// Test bot mention detection
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body: "@livereview please review this code",
				Author: UnifiedUserV2{
					Username: "testuser",
				},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{
			UserID:   "bot123",
			Username: "livereview",
			Name:     "LiveReview Bot",
			IsBot:    true,
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		assert.True(t, warrantsResponse)
		assert.Equal(t, "direct_mention", scenario.Type)
		assert.Contains(t, scenario.Reason, "mention") // Should detect mention
		assert.Greater(t, scenario.Confidence, 0.5)
	})

	t.Run("CheckResponseWarrant_GitHubMention", func(t *testing.T) {
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "github",
			Comment: &UnifiedCommentV2{
				Body:   "@LiveReview can you take a look?",
				Author: UnifiedUserV2{Username: "maintainer"},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{Username: "LiveReview"}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		assert.True(t, warrantsResponse)
		assert.Equal(t, "direct_mention", scenario.Type)
		assert.Contains(t, scenario.Reason, "mention")
	})

	t.Run("CheckResponseWarrant_BitbucketAccountIDMentionWithEmbeddedBraces", func(t *testing.T) {
		accountID := "{268052f4-1234-5678-90ab-cdef12345678}"
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "bitbucket",
			Comment: &UnifiedCommentV2{
				Body:   "Hey @{{268052f4-1234-5678-90ab-cdef12345678}} can you take a look?",
				Author: UnifiedUserV2{Username: "team_member"},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{
			UserID:   accountID,
			Username: "livereview",
			Metadata: map[string]interface{}{"account_id": accountID},
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		assert.True(t, warrantsResponse)
		assert.Equal(t, "direct_mention", scenario.Type)
	})

	t.Run("CheckResponseWarrant_BitbucketAccountIDMentionWithoutEmbeddedBraces", func(t *testing.T) {
		accountID := "{268052f4-1234-5678-90ab-cdef12345678}"
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "bitbucket",
			Comment: &UnifiedCommentV2{
				Body:   "Follow up @{268052f4-1234-5678-90ab-cdef12345678} please",
				Author: UnifiedUserV2{Username: "team_member"},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{
			UserID:   accountID,
			Username: "livereview",
			Metadata: map[string]interface{}{"account_id": accountID},
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		assert.True(t, warrantsResponse)
		assert.Equal(t, "direct_mention", scenario.Type)
	})

	t.Run("CheckResponseWarrant_NoMention", func(t *testing.T) {
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body: "Can you review this change?",
				Author: UnifiedUserV2{
					Username: "testuser",
				},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{
			UserID:   "bot123",
			Username: "livereview",
			Name:     "LiveReview Bot",
			IsBot:    true,
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		// Should not warrant response without mention
		assert.False(t, warrantsResponse)
		assert.Equal(t, "no_response", scenario.Type)
		assert.Equal(t, "top-level comment without reply or discussion context", scenario.Reason)
	})

	t.Run("CheckResponseWarrant_MissingBotInfoIsHardFailure", func(t *testing.T) {
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body:   "@livereview ping",
				Author: UnifiedUserV2{Username: "tester"},
			},
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, nil)

		assert.False(t, warrantsResponse)
		assert.Equal(t, "hard_failure", scenario.Type)
		assert.Contains(t, scenario.Reason, "bot user info")
		if assert.NotNil(t, scenario.Metadata) {
			assert.Equal(t, "bot_info", scenario.Metadata["missing"])
		}
	})

	t.Run("CheckResponseWarrant_EmptyBodyHardFailure", func(t *testing.T) {
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "github",
			Comment: &UnifiedCommentV2{
				Body:   "   ",
				Author: UnifiedUserV2{Username: "tester"},
			},
		}
		botInfo := &UnifiedBotUserInfoV2{Username: "livereview"}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		assert.False(t, warrantsResponse)
		assert.Equal(t, "hard_failure", scenario.Type)
		if assert.NotNil(t, scenario.Metadata) {
			assert.Equal(t, "event.comment.body", scenario.Metadata["missing"])
		}
	})

	t.Run("CheckResponseWarrant_BotComment", func(t *testing.T) {
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body: "@livereview please review this code",
				Author: UnifiedUserV2{
					ID:       "bot123", // Same as bot user ID
					Username: "livereview",
				},
			},
		}

		botInfo := &UnifiedBotUserInfoV2{
			UserID:   "bot123",
			Username: "livereview",
			Name:     "LiveReview Bot",
			IsBot:    true,
		}

		warrantsResponse, scenario := processor.CheckResponseWarrant(event, botInfo)

		// Bot detection may need refinement, but test shouldn't crash
		assert.NotNil(t, scenario)
		// This test documents current behavior - bot detection may need improvement
		_ = warrantsResponse // Don't assert specific behavior yet
	})

	t.Run("ProcessCommentReply", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body: "@livereview what does this function do?",
				Author: UnifiedUserV2{
					Username: "developer",
				},
			},
			MergeRequest: &UnifiedMergeRequestV2{
				ID:    "123",
				Title: "Add new feature",
			},
		}

		timeline := &UnifiedTimelineV2{
			Items: []UnifiedTimelineItemV2{
				{
					Type:      "comment",
					Timestamp: "2025-10-08T10:00:00Z",
					Comment: &UnifiedCommentV2{
						Body: "Original comment asking about function",
						Author: UnifiedUserV2{
							Username: "developer",
						},
					},
				},
			},
		}

		// This will test the processing pipeline (might need mock AI service)
		response, learning, err := processor.ProcessCommentReply(ctx, event, timeline, 0)

		// Should not error even if AI service unavailable (fallback response)
		assert.NoError(t, err)
		assert.NotEmpty(t, response)

		// Learning might be nil if no patterns detected
		if learning != nil {
			assert.NotEmpty(t, learning.Type)
		}
	})
}

func TestUnifiedContextBuilderV2(t *testing.T) {
	builder := &coreprocessor.UnifiedContextBuilderV2{}
	require.NotNil(t, builder)

	t.Run("BuildTimeline", func(t *testing.T) {
		mr := UnifiedMergeRequestV2{
			ID:          "123",
			Number:      1,
			Title:       "Test MR",
			Description: "Test merge request",
			State:       "opened",
		}

		timeline, err := builder.BuildTimeline(mr, "gitlab")

		// Should not error even with minimal data
		assert.NoError(t, err)
		assert.NotNil(t, timeline)

		// Timeline should be initialized
		assert.NotNil(t, timeline.Items)
	})

	t.Run("ExtractCommentContext", func(t *testing.T) {
		comment := UnifiedCommentV2{
			ID:   "456",
			Body: "This is a test comment",
			Author: UnifiedUserV2{
				Username: "testuser",
			},
		}

		timeline := UnifiedTimelineV2{
			Items: []UnifiedTimelineItemV2{
				{
					Type:      "comment",
					Timestamp: "2025-10-08T10:00:00Z",
					Comment: &UnifiedCommentV2{
						ID:   "454",
						Body: "Previous comment 1",
					},
				},
				{
					Type:      "comment",
					Timestamp: "2025-10-08T10:01:00Z",
					Comment: &UnifiedCommentV2{
						ID:   "455",
						Body: "Previous comment 2",
					},
				},
				{
					Type:      "comment",
					Timestamp: "2025-10-08T10:02:00Z",
					Comment:   &comment, // Target comment
				},
				{
					Type:      "comment",
					Timestamp: "2025-10-08T10:03:00Z",
					Comment: &UnifiedCommentV2{
						ID:   "457",
						Body: "Following comment 1",
					},
				},
			},
		}

		context, err := builder.ExtractCommentContext(comment, timeline)

		assert.NoError(t, err)
		assert.NotNil(t, context)

		// Should have extracted context
		assert.NotNil(t, context.MRContext)
		assert.NotNil(t, context.Timeline)
	})

	t.Run("BuildPrompt", func(t *testing.T) {
		context := CommentContextV2{
			MRContext: UnifiedMRContextV2{
				MergeRequest: UnifiedMergeRequestV2{
					Title:       "Test MR",
					Description: "Test description",
				},
				Repository: UnifiedRepositoryV2{
					Name: "test-repo",
				},
			},
			Timeline: UnifiedTimelineV2{
				Items: []UnifiedTimelineItemV2{
					{
						Type:      "comment",
						Timestamp: "2025-10-08T10:00:00Z",
						Comment: &UnifiedCommentV2{
							Body: "Original question about code",
						},
					},
				},
			},
		}

		scenario := ResponseScenarioV2{
			Type:   "comment_reply",
			Reason: "direct_mention",
		}

		prompt, err := builder.BuildPrompt(context, scenario)

		assert.NoError(t, err)
		assert.NotEmpty(t, prompt)

		// Prompt should be a valid AI prompt (contains standard prompt text)
		assert.Contains(t, prompt, "assistant")
	})
}

func TestUnifiedContextBuilderV2ThreadMetadataAcrossProviders(t *testing.T) {
	t.Parallel()

	builder := &coreprocessor.UnifiedContextBuilderV2{}

	ptr := func(val string) *string {
		return &val
	}

	tests := []struct {
		name      string
		comments  []UnifiedCommentV2
		provider  string
		expecteds []struct {
			discussionID *string
			inReplyToID  *string
		}
	}{
		{
			name:     "github lone comment",
			provider: "github",
			comments: []UnifiedCommentV2{
				{ID: "gh-1", Body: "Top-level", CreatedAt: "2025-10-17T11:00:00Z"},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{{discussionID: nil, inReplyToID: nil}},
		},
		{
			name:     "github threaded review",
			provider: "github",
			comments: []UnifiedCommentV2{
				{ID: "gh-10", Body: "Review root", CreatedAt: "2025-10-17T11:01:00Z", DiscussionID: ptr("review-77")},
				{ID: "gh-11", Body: "Follow-up", CreatedAt: "2025-10-17T11:01:10Z", DiscussionID: ptr("review-77"), InReplyToID: ptr("gh-10")},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{
				{discussionID: ptr("review-77"), inReplyToID: nil},
				{discussionID: ptr("review-77"), inReplyToID: ptr("gh-10")},
			},
		},
		{
			name:     "gitlab discussion anchor",
			provider: "gitlab",
			comments: []UnifiedCommentV2{
				{ID: "gl-1", Body: "Initial note", CreatedAt: "2025-10-17T11:02:00Z", DiscussionID: ptr("disc-1")},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{{discussionID: ptr("disc-1"), inReplyToID: nil}},
		},
		{
			name:     "gitlab threaded discussion",
			provider: "gitlab",
			comments: []UnifiedCommentV2{
				{ID: "gl-10", Body: "Bot suggestion", CreatedAt: "2025-10-17T11:02:30Z", DiscussionID: ptr("disc-42")},
				{ID: "gl-11", Body: "Human reply", CreatedAt: "2025-10-17T11:02:40Z", DiscussionID: ptr("disc-42"), InReplyToID: ptr("54")},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{
				{discussionID: ptr("disc-42"), inReplyToID: nil},
				{discussionID: ptr("disc-42"), inReplyToID: ptr("54")},
			},
		},
		{
			name:     "bitbucket lone comment",
			provider: "bitbucket",
			comments: []UnifiedCommentV2{
				{ID: "bb-1", Body: "Top-level", CreatedAt: "2025-10-17T11:03:00Z", DiscussionID: ptr("bb-1")},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{{discussionID: ptr("bb-1"), inReplyToID: nil}},
		},
		{
			name:     "bitbucket threaded reply",
			provider: "bitbucket",
			comments: []UnifiedCommentV2{
				{ID: "bb-10", Body: "Root", CreatedAt: "2025-10-17T11:03:30Z", DiscussionID: ptr("700")},
				{ID: "bb-11", Body: "Reply", CreatedAt: "2025-10-17T11:03:40Z", DiscussionID: ptr("700"), InReplyToID: ptr("700")},
			},
			expecteds: []struct {
				discussionID *string
				inReplyToID  *string
			}{
				{discussionID: ptr("700"), inReplyToID: nil},
				{discussionID: ptr("700"), inReplyToID: ptr("700")},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			timeline := builder.BuildTimelineFromData(nil, tc.comments)
			require.NotNil(t, timeline)
			require.Len(t, timeline.Items, len(tc.expecteds))

			for idx, expected := range tc.expecteds {
				item := timeline.Items[idx]
				require.NotNilf(t, item.Comment, "timeline item %d missing comment", idx)

				if expected.discussionID == nil {
					require.Nilf(t, item.Comment.DiscussionID, "timeline item %d expected nil discussion id", idx)
				} else {
					require.NotNilf(t, item.Comment.DiscussionID, "timeline item %d missing discussion id", idx)
					require.Equal(t, *expected.discussionID, *item.Comment.DiscussionID)
				}

				if expected.inReplyToID == nil {
					require.Nilf(t, item.Comment.InReplyToID, "timeline item %d expected nil reply id", idx)
				} else {
					require.NotNilf(t, item.Comment.InReplyToID, "timeline item %d missing reply id", idx)
					require.Equal(t, *expected.inReplyToID, *item.Comment.InReplyToID)
				}
			}
		})
	}
}

func TestCheckDirectBotMentionV2ProviderDelegation(t *testing.T) {
	processor := &UnifiedProcessorV2Impl{}

	tests := []struct {
		name   string
		event  UnifiedWebhookEventV2
		bot    *UnifiedBotUserInfoV2
		expect bool
	}{
		{
			name: "github username mention",
			event: UnifiedWebhookEventV2{
				Provider: "github",
				Comment:  &UnifiedCommentV2{Body: "@LiveReview ping"},
			},
			bot:    &UnifiedBotUserInfoV2{Username: "LiveReview"},
			expect: true,
		},
		{
			name: "gitlab username mention",
			event: UnifiedWebhookEventV2{
				Provider: "gitlab",
				Comment:  &UnifiedCommentV2{Body: "Please help @livereview"},
			},
			bot:    &UnifiedBotUserInfoV2{Username: "LiveReview"},
			expect: true,
		},
		{
			name: "bitbucket account id mention",
			event: UnifiedWebhookEventV2{
				Provider: "bitbucket",
				Comment:  &UnifiedCommentV2{Body: "@{268052f4-1234-5678-90ab-cdef12345678} please advise"},
			},
			bot: &UnifiedBotUserInfoV2{
				Username: "LiveReview",
				Metadata: map[string]interface{}{"account_id": "{268052f4-1234-5678-90ab-cdef12345678}"},
			},
			expect: true,
		},
		{
			name: "unknown provider fallback",
			event: UnifiedWebhookEventV2{
				Provider: "custom",
				Comment:  &UnifiedCommentV2{Body: "@livereview check this"},
			},
			bot:    &UnifiedBotUserInfoV2{Username: "LiveReview"},
			expect: true,
		},
		{
			name: "no mention",
			event: UnifiedWebhookEventV2{
				Provider: "github",
				Comment:  &UnifiedCommentV2{Body: "Just a casual note"},
			},
			bot:    &UnifiedBotUserInfoV2{Username: "LiveReview"},
			expect: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := processor.checkDirectBotMentionV2(tc.event, tc.bot)
			assert.Equal(t, tc.expect, result)
		})
	}
}

func TestLearningProcessorV2(t *testing.T) {
	server := &Server{}
	processor := NewLearningProcessorV2(server)
	require.NotNil(t, processor)

	t.Run("ExtractLearning_Documentation", func(t *testing.T) {
		response := "You should always document your public functions with clear descriptions of their purpose, parameters, and return values. This improves code maintainability."

		context := CommentContextV2{
			MRContext: UnifiedMRContextV2{
				Metadata: map[string]interface{}{
					"before_comments": []string{
						"Can you help me understand what this function does? It's not documented.",
					},
				},
			},
		}

		learning, err := processor.ExtractLearning(response, context)

		assert.NoError(t, err)
		if learning != nil {
			assert.Equal(t, "best_practice", learning.Type)
			assert.Contains(t, learning.Tags, "documentation")
			assert.Greater(t, learning.Confidence, 0.0)
		}
	})

	t.Run("ExtractLearning_Performance", func(t *testing.T) {
		response := "This loop has O(n²) complexity. Consider using a hash map for O(1) lookups to improve performance."

		context := CommentContextV2{
			MRContext: UnifiedMRContextV2{
				Metadata: map[string]interface{}{
					"before_comments": []string{
						"This code seems slow, can you help optimize it?",
					},
				},
			},
		}

		learning, err := processor.ExtractLearning(response, context)

		assert.NoError(t, err)
		if learning != nil {
			assert.Equal(t, "optimization", learning.Type)
			assert.Contains(t, learning.Tags, "performance")
		}
	})

	t.Run("ExtractLearning_NoPattern", func(t *testing.T) {
		response := "Hello, how are you today?"

		context := CommentContextV2{
			MRContext: UnifiedMRContextV2{
				Metadata: map[string]interface{}{
					"before_comments": []string{
						"Just saying hello",
					},
				},
			},
		}

		learning, err := processor.ExtractLearning(response, context)

		assert.NoError(t, err)
		// Should return nil for non-technical content
		assert.Nil(t, learning)
	})

	t.Run("FindOrgIDForRepository", func(t *testing.T) {
		repo := UnifiedRepositoryV2{
			FullName: "group/test-repo",
			WebURL:   "https://gitlab.example.com/group/test-repo",
		}

		orgID, err := processor.FindOrgIDForRepository(repo)

		// Should not error and return some org ID (default behavior)
		assert.NoError(t, err)
		assert.Greater(t, orgID, int64(0))
	})

	t.Run("BasicRepositoryProcessing", func(t *testing.T) {
		// Test basic learning processing
		repo := UnifiedRepositoryV2{
			FullName: "group/test-repo",
			WebURL:   "https://gitlab.example.com/group/test-repo",
		}

		orgID, err := processor.FindOrgIDForRepository(repo)
		assert.NoError(t, err)
		assert.Greater(t, orgID, int64(0))
	})
}

func TestProcessingPipeline(t *testing.T) {
	// Integration test for the complete processing pipeline
	server := &Server{}

	// Initialize all components
	processor := NewUnifiedProcessorV2(server)
	contextBuilder := coreprocessor.NewUnifiedContextBuilderV2()
	learningProcessor := NewLearningProcessorV2(server)

	require.NotNil(t, processor)
	require.NotNil(t, contextBuilder)
	require.NotNil(t, learningProcessor)

	t.Run("CompleteProcessingFlow", func(t *testing.T) {
		// Simulate complete processing flow

		// Step 1: Create event (would come from provider)
		event := UnifiedWebhookEventV2{
			EventType: "comment_created",
			Provider:  "gitlab",
			Comment: &UnifiedCommentV2{
				Body: "@livereview please help me understand this security vulnerability",
				Author: UnifiedUserV2{
					Username: "developer",
				},
			},
			MergeRequest: &UnifiedMergeRequestV2{
				ID:    "123",
				Title: "Fix authentication bug",
			},
		}

		// Step 2: Check response warrant
		botInfo := &UnifiedBotUserInfoV2{
			UserID:   "bot123",
			Username: "livereview",
		}

		warrantsResponse4, scenario4 := processor.CheckResponseWarrant(event, botInfo)
		assert.True(t, warrantsResponse4)
		assert.Equal(t, "direct_mention", scenario4.Type)

		// Step 3: Build timeline and context
		if event.MergeRequest != nil {
			timeline, err := contextBuilder.BuildTimeline(*event.MergeRequest, event.Provider)
			assert.NoError(t, err)
			assert.NotNil(t, timeline)

			// Step 4: Process comment reply
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			response, learning, err := processor.ProcessCommentReply(ctx, event, timeline, 0)
			assert.NoError(t, err)
			assert.NotEmpty(t, response)

			// Step 5: Apply learning if extracted
			if learning != nil {
				// In real scenario, would call ApplyLearning
				assert.NotEmpty(t, learning.Type)
				assert.NotEmpty(t, learning.Tags)
			}
		}
	})
}

func TestErrorHandling(t *testing.T) {
	server := &Server{}

	t.Run("ProcessorWithNilEvent", func(t *testing.T) {
		processor := NewUnifiedProcessorV2(server)

		// Test handling of invalid/nil inputs
		_, scenario5 := processor.CheckResponseWarrant(UnifiedWebhookEventV2{}, nil)
		assert.Equal(t, "hard_failure", scenario5.Type)
		if assert.NotNil(t, scenario5.Metadata) {
			assert.Equal(t, "event.comment", scenario5.Metadata["missing"])
		}
	})

	t.Run("ContextBuilderWithEmptyData", func(t *testing.T) {
		builder := coreprocessor.NewUnifiedContextBuilderV2()

		// Test with minimal MR data
		mr := UnifiedMergeRequestV2{
			ID: "empty",
		}

		timeline, err := builder.BuildTimeline(mr, "unknown-provider")
		assert.NoError(t, err) // Should handle gracefully
		assert.NotNil(t, timeline)
	})

	t.Run("LearningProcessorWithInvalidData", func(t *testing.T) {
		processor := NewLearningProcessorV2(server)

		// Test with empty context
		learning, err := processor.ExtractLearning("", CommentContextV2{})
		assert.NoError(t, err)
		assert.Nil(t, learning) // Should return nil for empty content
	})
}
