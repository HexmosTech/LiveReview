# LiveReview Webhook Handler Refactoring Plan

## Overview
Refactor webhook_handler.go (4714 lines) into provider-specific files with unified reply/learning generation core.

**Goal**: 
- **Provider files**: Handle platform-specific fetch (webhook parsing, API calls) and post (response delivery)
- **Unified core**: Provider-agnostic reply generation (LLM prompt building, AI responses) and learning generation (metadata extraction, learning API calls)

## Phase 1: Extract Common Types & Interfaces

### 1.1 Create unified types file
- **File**: `internal/api/webhook_types.go`
- **Move**: All `Unified*` types from webhook_handler.go
  - `UnifiedMRContext`
  - `UnifiedComment` 
  - `UnifiedUser`
  - `UnifiedRepository`
  - `UnifiedPosition`
  - `UnifiedBotUserInfo`
  - `ResponseScenario`
- **Move**: Common types
  - `AIConnector`
  - `LearningMetadata`
- **Verify**: Types compile independently

### 1.2 Create base interfaces
- **File**: `internal/api/webhook_interfaces.go`
- **Define**: Provider interface (fetch/post only)
```go
type WebhookProvider interface {
    HandleWebhook(c echo.Context) error
    ConvertToUnified(payload interface{}) (UnifiedComment, error)
    GetBotUserInfo(repoContext interface{}) (*UnifiedBotUserInfo, error)
    PostResponse(comment UnifiedComment, response string) error
    FetchMRContext(comment UnifiedComment) (UnifiedMRContext, error)
}
```
- **Define**: Reply generator interface (provider-agnostic)
```go
type ReplyGenerator interface {
    CheckResponseWarrant(comment UnifiedComment, botInfo *UnifiedBotUserInfo) (bool, ResponseScenario)
    GenerateResponse(ctx context.Context, comment UnifiedComment, scenario ResponseScenario, mrContext UnifiedMRContext) (string, error)
}
```
- **Define**: Learning generator interface (provider-agnostic)  
```go
type LearningGenerator interface {
    ExtractLearning(comment UnifiedComment, response string) (*LearningMetadata, error)
    ApplyLearning(ctx context.Context, learning *LearningMetadata, orgID int64) (string, error)
}
```
- **Verify**: Interfaces separate concerns correctly

## Phase 2: Extract GitLab Provider

### 2.1 Create GitLab webhook file
- **File**: `internal/api/gitlab_webhook.go`
- **Move**: All GitLab-specific types
  - `GitLabWebhookPayload` through `GitLabBotUserInfo`
  - `GitLabHTTPClient` and methods
  - `GitLabDiscussion`, `GitLabNote`, etc.
- **Move**: GitLab handler functions
  - `GitLabWebhookHandler`
  - `GitLabCommentWebhookHandler` 
  - `processReviewerChange`
  - `triggerReviewForMR`
  - `findIntegrationTokenForProject`
- **Verify**: GitLab types compile in separate file

### 2.2 Extract GitLab comment processing
- **Move**: GitLab comment functions to `gitlab_webhook.go`
  - `processGitLabNoteEvent`
  - `checkAIResponseWarrant`
  - `getFreshBotUserInfo`
  - `checkIfReplyingToBotComment`
  - `checkDirectBotMention`
  - `extractGitLabInstanceURL`
- **Move**: GitLab API helpers
  - `getGitLabAccessToken`
  - `postEmojiToGitLabNote`
  - `postReplyToGitLabDiscussion`
  - `postGeneralCommentToGitLabMR`
  - `postToGitLabAPI`
- **Verify**: All GitLab webhook flows work unchanged

### 2.3 Extract GitLab post operations
- **Move**: GitLab posting functions (fetch/post only)
  - `generateAndPostGitLabResponse` (posting part only)
  - `postGitLabEmojiReaction`
  - `postGitLabTextResponse`
  - `postReplyToGitLabDiscussion`
  - `postGeneralCommentToGitLabMR`
  - `postToGitLabAPI`
- **Move**: GitLab data fetching
  - `findTargetComment` (GitLab-specific)
  - `getCodeContext` (GitLab API calls)
  - GitLab API client methods
- **Remove**: Prompt building logic (moves to unified core)
- **Verify**: GitLab can fetch data and post responses

## Phase 3: Extract GitHub Provider

### 3.1 Create GitHub webhook file
- **File**: `internal/api/github_webhook.go`
- **Move**: All GitHub-specific types
  - `GitHubWebhookPayload` through `GitHubBotUserInfo`
  - Comment payload types
  - Issue/PR/Review types
- **Move**: GitHub handler functions
  - `GitHubWebhookHandler`
  - `handleGitHubPullRequestEvent`
  - `handleGitHubIssueCommentEvent`
  - `handleGitHubPullRequestReviewCommentEvent`
- **Verify**: GitHub types compile in separate file

### 3.2 Extract GitHub comment processing
- **Move**: GitHub processing functions
  - `processGitHubReviewerChange`
  - `triggerReviewForGitHubPR`
  - `findIntegrationTokenForGitHubRepo`
  - `processGitHubCommentForAIResponse`
  - `convertGitHubReviewCommentToUnified`
  - `getFreshGitHubBotUserInfo`
- **Move**: GitHub helper functions
  - `convertGitHubUserToUnified`
  - `convertGitHubRepoToUnified`
  - `convertGitHubInReplyToIDPtr`
- **Verify**: GitHub webhook flows work unchanged

### 3.3 Extract GitHub post operations  
- **Move**: GitHub posting functions (fetch/post only)
  - `generateAndPostGitHubResponse` (posting part only)
  - `postGitHubCommentReaction`
  - `postGitHubCommentReply`
- **Move**: GitHub data fetching
  - `buildGitHubTimeline` (data fetching only)
  - `extractGitHubCommentContext` (GitHub API calls)
  - `fetchGitHubPRCommits`
  - `fetchGitHubPRComments`
  - `checkIfGitHubCommentIsByBot`
- **Remove**: Prompt building logic (moves to unified core)
- **Verify**: GitHub can fetch data and post responses

## Phase 4: Extract Bitbucket Provider

### 4.1 Create Bitbucket webhook file
- **File**: `internal/api/bitbucket_webhook.go`
- **Move**: All Bitbucket-specific types
  - `BitbucketWebhookPayload` through `BitbucketUserInfo`
  - Comment/PR/Repository types
- **Move**: Bitbucket handler functions
  - `BitbucketWebhookHandler`
  - `processBitbucketReviewerChange`
  - `triggerReviewForBitbucketPR`
  - `findIntegrationTokenForBitbucketRepo`
- **Verify**: Bitbucket types compile in separate file

### 4.2 Extract Bitbucket post operations
- **Move**: Bitbucket processing functions
  - `processBitbucketCommentForAIResponse`
  - `convertBitbucketToUnifiedComment`
  - `getFreshBitbucketBotUserInfo`
  - `checkIfBitbucketCommentIsByBot`
- **Move**: Bitbucket posting functions (fetch/post only)
  - `generateAndPostBitbucketResponse` (posting part only)
  - `postBitbucketCommentReply`
- **Move**: Bitbucket data fetching
  - `buildBitbucketTimeline` (data fetching only)
  - `extractBitbucketCommentContext` (Bitbucket API calls)
  - `fetchBitbucketPRCommits`
  - `fetchBitbucketPRComments`
- **Remove**: Prompt building logic (moves to unified core)
- **Verify**: Bitbucket can fetch data and post responses

## Phase 5: Create Unified Reply/Learning Core

### 5.1 Create reply generator (provider-agnostic)
- **File**: `internal/api/reply_generator.go`
- **Move**: Response warrant checking (PRESERVE EXACTLY)
  - `checkUnifiedAIResponseWarrant`
  - `classifyContentType`
  - `classifyReplyContentType` 
  - `determineResponseType`
  - `determineReplyResponseType`
- **Move**: AI response generation (PRESERVE EXACTLY)
  - `generateAIResponseFromPrompt`
  - `generateLLMResponse`
  - `generateStructuredFallbackResponse`
  - `buildContextualAIResponse` logic (make provider-agnostic)
  - `buildGeminiPromptEnhanced` (unified version)
- **Move**: Response synthesis (PRESERVE EXACTLY)
  - `synthesizeContextualResponse`
  - All `generate*Response` template functions
- **Verify**: Reply generation completely provider-agnostic

### 5.2 Create learning generator (provider-agnostic)
- **File**: `internal/api/learning_generator.go`
- **Move**: Learning detection (PRESERVE EXACTLY)
  - `augmentResponseWithLearningMetadata`
  - Learning pattern detection logic
  - Metadata extraction from comments/responses
- **Move**: Learning application (PRESERVE EXACTLY)
  - `applyLearningFromReply`
  - `findOrgIDForGitLabInstance` (make provider-agnostic)
  - Learning API integration
- **Create**: Provider-agnostic org resolution
- **Verify**: Learning extraction works across all providers

### 5.3 Create unified context builder
- **File**: `internal/api/context_builder.go`
- **Move**: Context building (PRESERVE EXACTLY)
  - `buildTimeline` (unified across providers)
  - `extractCommentContext`
  - `findTargetComment` logic
  - Timeline/context analysis
- **Move**: Prompt building (PRESERVE EXACTLY)
  - `buildGeminiPromptEnhanced` (provider-agnostic version)
  - All prompt construction logic
  - Context analysis functions
- **Move**: Helper functions
  - `parseTimeBestEffort`
  - `shortSHA`
  - `firstNonEmpty`
  - `min` function
- **Verify**: Context building works with any provider's data

## Phase 6: Refactor Main Handler

### 6.1 Simplify webhook_handler.go  
- **Keep**: Main `Server` struct and methods
- **Replace**: Provider handlers with interface calls
- **Keep**: Database operations and review tracking
- **Create**: Unified processing orchestrator
```go
type WebhookOrchestrator struct {
    providers       map[string]WebhookProvider
    replyGenerator  ReplyGenerator  
    learningGenerator LearningGenerator
}
```
- **Create**: Flow: Provider.Fetch → ReplyGenerator.Generate → LearningGenerator.Extract → Provider.Post
- **Verify**: Same webhook behavior with cleaner architecture

### 6.2 Update handler routing
- **Modify**: Each handler to use provider interface
- **Keep**: Exact same webhook endpoints
- **Keep**: Same response formats
- **Add**: Provider initialization in server setup
- **Verify**: All webhooks route correctly

### 6.3 Provider integration
- **Create**: Provider constructors with Server dependency injection
- **Ensure**: Each provider has access to database, LLM processor
- **Maintain**: Same async processing patterns
- **Keep**: Same error handling and logging
- **Verify**: Provider switching works seamlessly

## Phase 7: Testing & Validation

### 7.1 Unit test preservation
- **Verify**: Existing webhook tests still pass
- **Check**: Provider-specific logic isolated
- **Test**: Unified LLM processor independently
- **Validate**: Learning extraction still works
- **Confirm**: Response templates unchanged

### 7.2 Integration verification
- **Test**: All three provider webhooks end-to-end
- **Verify**: LLM prompt building identical
- **Check**: Response posting to each provider
- **Validate**: Learning API calls preserved
- **Confirm**: Database operations unchanged

### 7.3 Performance validation
- **Check**: No performance regression in webhook handling
- **Verify**: Memory usage not increased significantly
- **Test**: Concurrent webhook processing
- **Validate**: Error recovery mechanisms
- **Confirm**: Logging/debugging info preserved

## Success Criteria

### Code Organization
- [ ] webhook_handler.go reduced to <500 lines
- [ ] Provider files each <1500 lines
- [ ] Types clearly separated by concern
- [ ] No circular dependencies

### Functionality Preservation
- [ ] All webhook endpoints respond identically
- [ ] Reply generation (LLM prompts, AI responses) completely provider-agnostic
- [ ] Learning generation (extraction, application) works across all providers
- [ ] Provider fetch/post operations preserve platform-specific behavior
- [ ] Database operations identical

### Architecture Improvements
- [ ] Clear separation: Provider (fetch/post) vs Unified (reply/learning generation)
- [ ] Provider interface enables future platform additions
- [ ] Reply generation completely provider-agnostic and reusable
- [ ] Learning generation works universally across platforms
- [ ] Context building and prompt generation centralized
- [ ] Testing dramatically easier with clear separation of concerns

## Rollback Plan
- Keep original webhook_handler.go as webhook_handler_backup.go
- Each phase can be reverted individually
- Provider files can be merged back if needed
- Interface changes tracked in separate commits

## Dependencies
- No external library changes required
- Database schema unchanged
- API endpoints remain identical
- Webhook payload formats preserved
- LLM integration points unchanged
