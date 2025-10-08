# LiveReview Webhook Handler Refactoring Plan

## Overview
Refactor webhook_handler.go (4714 lines) into provider-specific files with unified processing core using layered architecture.

**Architecture**: 
- **Provider Layer**: Platform-specific fetch (webhook parsing, API calls) → convert to unified structures → post responses
- **Unified Processing Layer**: Provider-agnostic reply generation & learning extraction using unified data structures
- **Two Flow Types**: 
  1. **Comment Reply Flow**: User comments → AI replies to comments
  2. **Full Review Flow**: Bot assigned as reviewer → Complete MR/PR review with multiple comments

**Data Flow**: `Provider.Fetch` → `Provider.ConvertToUnified` → `UnifiedProcessor.Process` → `Provider.PostResponse`

## Phase 1: Analyze Current Data Structures & Create Unified Types

### 1.1 Analyze existing unified conversion patterns
**From webhook_handler.go analysis**:
- `convertGitHubReviewCommentToUnified()` - GitHub→Unified comment conversion
- `convertBitbucketToUnifiedComment()` - Bitbucket→Unified comment conversion  
- `convertGitHubUserToUnified()`, `convertGitHubRepoToUnified()` - Helper conversions
- **Missing**: GitLab→Unified conversion (currently processes GitLab directly)

### 1.2 Create comprehensive unified types
- **File**: `internal/api/unified_types.go`
- **Expand**: Existing `Unified*` types to cover ALL data accessed in webhook_handler.go:

**UnifiedWebhookEvent** (new - top level):
```go
type UnifiedWebhookEvent struct {
    EventType    string                 // "comment_created", "reviewer_assigned", "mr_updated"
    Provider     string                 // "gitlab", "github", "bitbucket"  
    Timestamp    string
    Repository   UnifiedRepository
    MergeRequest *UnifiedMergeRequest   // For MR events
    Comment      *UnifiedComment        // For comment events  
    ReviewerChange *UnifiedReviewerChange // For reviewer assignment events
    Actor        UnifiedUser            // User who triggered the event
}
```

**UnifiedMergeRequest** (enhanced):
```go
type UnifiedMergeRequest struct {
    ID           string
    Number       int                    // For display (IID/Number)
    Title        string
    Description  string
    State        string
    Author       UnifiedUser
    SourceBranch string
    TargetBranch string
    WebURL       string
    CreatedAt    string
    UpdatedAt    string
    Reviewers    []UnifiedUser
    Assignees    []UnifiedUser
    Labels       []string              
    Metadata     map[string]interface{} // Provider-specific data
}
```

**UnifiedReviewerChange** (new):
```go
type UnifiedReviewerChange struct {
    Action           string              // "added", "removed"
    CurrentReviewers []UnifiedUser
    PreviousReviewers []UnifiedUser
    BotAssigned      bool
    BotRemoved       bool
    ChangedBy        UnifiedUser
}
```

**UnifiedComment** (enhanced from existing):
```go  
type UnifiedComment struct {
    ID          string
    Body        string
    Author      UnifiedUser
    CreatedAt   string
    UpdatedAt   string
    WebURL      string
    InReplyToID *string
    Position    *UnifiedPosition       // For inline code comments
    DiscussionID *string               // Thread/discussion ID
    System      bool                   // System vs user comment
    Metadata    map[string]interface{} // Provider-specific data
}
```

**UnifiedCommit** (new - for timeline building):
```go
type UnifiedCommit struct {
    SHA       string
    Message   string
    Author    UnifiedCommitAuthor
    Timestamp string
    WebURL    string
}

type UnifiedCommitAuthor struct {
    Name  string  
    Email string
}
```

**UnifiedTimeline** (new - for context building):
```go
type UnifiedTimeline struct {
    Items []UnifiedTimelineItem
}

type UnifiedTimelineItem struct {
    Type      string                  // "commit", "comment", "review_change"
    Timestamp string
    Commit    *UnifiedCommit
    Comment   *UnifiedComment  
    ReviewChange *UnifiedReviewerChange
}
```

- **Move**: Common types unchanged
  - `AIConnector`, `LearningMetadata`, `ResponseScenario`
- **Verify**: All data accessed in current webhook_handler.go can be represented

### 1.3 Create interfaces based on actual flows
- **File**: `internal/api/webhook_interfaces.go`
- **Define**: Provider interface (matches current patterns):
```go
type WebhookProvider interface {
    // Main webhook entry point
    HandleWebhook(c echo.Context) error
    
    // Convert provider payload to unified structure  
    ConvertCommentEvent(payload interface{}) (*UnifiedWebhookEvent, error)
    ConvertReviewerEvent(payload interface{}) (*UnifiedWebhookEvent, error)
    
    // Fetch additional context data (commits, discussions, etc.)
    FetchMRTimeline(mr UnifiedMergeRequest) (*UnifiedTimeline, error)
    FetchCodeContext(comment UnifiedComment) (string, error) // Diff hunks, file content
    
    // Get bot user info for warrant checking
    GetBotUserInfo(repository UnifiedRepository) (*UnifiedBotUserInfo, error)
    
    // Post responses back to platform
    PostCommentReply(mr UnifiedMergeRequest, parentComment *UnifiedComment, response string) error
    PostReviewComments(mr UnifiedMergeRequest, comments []UnifiedReviewComment) error
}
```

- **Define**: Unified processor interface:
```go
type UnifiedProcessor interface {
    // Check if event warrants a response
    CheckResponseWarrant(event UnifiedWebhookEvent, botInfo *UnifiedBotUserInfo) (bool, ResponseScenario)
    
    // Process comment reply flow
    ProcessCommentReply(ctx context.Context, event UnifiedWebhookEvent, timeline *UnifiedTimeline) (string, *LearningMetadata, error)
    
    // Process full review flow  
    ProcessFullReview(ctx context.Context, event UnifiedWebhookEvent, timeline *UnifiedTimeline) ([]UnifiedReviewComment, *LearningMetadata, error)
}

type UnifiedReviewComment struct {
    FilePath    string
    LineNumber  int
    Content     string
    Severity    string
    Category    string
}
```
- **Verify**: Interfaces match both flow types and current functionality

## Phase 2: Extract GitLab Provider

### 2.1 Create GitLab provider file
- **File**: `internal/api/gitlab_provider.go`
- **Move**: All GitLab-specific types (PRESERVE EXACTLY)
  - `GitLabWebhookPayload` through `GitLabBotUserInfo`
  - `GitLabHTTPClient` and methods  
  - `GitLabDiscussion`, `GitLabNote`, `GitLabNotePosition`
  - `TimelineItem`, `CommentContext` (currently GitLab-specific)
- **Move**: GitLab webhook handlers
  - `GitLabWebhookHandler` 
  - `GitLabCommentWebhookHandler`
- **Verify**: All GitLab types compile independently

### 2.2 Create GitLab conversion methods  
- **Create**: Missing GitLab→Unified conversions (based on existing GitHub/Bitbucket patterns):
```go
func (g *GitLabProvider) ConvertCommentEvent(payload interface{}) (*UnifiedWebhookEvent, error)
func (g *GitLabProvider) ConvertReviewerEvent(payload interface{}) (*UnifiedWebhookEvent, error)  
func (g *GitLabProvider) convertGitLabNoteToUnified(note GitLabNoteWebhookPayload) UnifiedComment
func (g *GitLabProvider) convertGitLabMRToUnified(mr GitLabMergeRequest) UnifiedMergeRequest
func (g *GitLabProvider) convertGitLabUserToUnified(user GitLabUser) UnifiedUser
```
- **Extract**: Data access patterns from current functions:
  - `processReviewerChange` → identify what data is accessed for reviewer changes
  - `checkAIResponseWarrant` → understand comment warrant checking data needs
  - `buildContextualAIResponse` → identify MR context data requirements
- **Verify**: Can convert all GitLab webhook events to unified structures

### 2.3 Extract GitLab data fetching & posting
- **Move**: GitLab API operations (PRESERVE EXACTLY)
  - `getFreshBotUserInfo`, `getGitLabAccessToken`
  - `GetMergeRequestCommits`, `GetMergeRequestDiscussions`, `GetMergeRequestNotes`
  - `findTargetComment`, `getCodeContext` 
  - `buildTimeline`, `extractCommentContext` (make GitLab-specific)
- **Move**: GitLab posting functions (PRESERVE EXACTLY)
  - `postEmojiToGitLabNote`, `postReplyToGitLabDiscussion`
  - `postGeneralCommentToGitLabMR`, `postToGitLabAPI`
- **Move**: GitLab utilities  
  - `extractGitLabInstanceURL`, `normalizeGitLabURL`
  - `findIntegrationTokenForProject`
- **Implement**: Provider interface methods:
```go
func (g *GitLabProvider) FetchMRTimeline(mr UnifiedMergeRequest) (*UnifiedTimeline, error)
func (g *GitLabProvider) FetchCodeContext(comment UnifiedComment) (string, error)
func (g *GitLabProvider) PostCommentReply(mr UnifiedMergeRequest, parentComment *UnifiedComment, response string) error
```
- **Verify**: GitLab provider can fetch all data and post responses

## Phase 3: Extract GitHub Provider

### 3.1 Create GitHub provider file
- **File**: `internal/api/github_provider.go`  
- **Move**: All GitHub-specific types (PRESERVE EXACTLY)
  - `GitHubWebhookPayload` through `GitHubBotUserInfo`
  - `GitHubIssueCommentWebhookPayload`, `GitHubPullRequestReviewCommentWebhookPayload`
  - All GitHub comment/issue/PR/review types
- **Move**: GitHub webhook handlers
  - `GitHubWebhookHandler`
  - `handleGitHubPullRequestEvent`, `handleGitHubIssueCommentEvent`
  - `handleGitHubPullRequestReviewCommentEvent`
- **Verify**: All GitHub types compile independently

### 3.2 Enhance GitHub conversion methods
- **Move**: Existing conversion functions (PRESERVE EXACTLY)
  - `convertGitHubReviewCommentToUnified`
  - `convertGitHubUserToUnified`, `convertGitHubRepoToUnified`
  - `convertGitHubInReplyToIDPtr`
- **Create**: Missing GitHub conversions:
```go
func (g *GitHubProvider) ConvertCommentEvent(payload interface{}) (*UnifiedWebhookEvent, error) 
func (g *GitHubProvider) ConvertReviewerEvent(payload interface{}) (*UnifiedWebhookEvent, error)
func (g *GitHubProvider) convertGitHubIssueCommentToUnified(payload GitHubIssueCommentWebhookPayload) UnifiedComment
func (g *GitHubProvider) convertGitHubPRToUnified(pr GitHubPullRequest) UnifiedMergeRequest
```
- **Extract**: Data access patterns from current functions:
  - `processGitHubReviewerChange` → reviewer change data requirements
  - `checkUnifiedAIResponseWarrant` → comment warrant checking data needs  
  - `buildGitHubContextualResponse` → MR context data requirements
- **Verify**: All GitHub webhook events convert to unified structures

### 3.3 Extract GitHub data fetching & posting
- **Move**: GitHub API operations (PRESERVE EXACTLY)
  - `getFreshGitHubBotUserInfo`
  - `fetchGitHubPRCommits`, `fetchGitHubPRComments`
  - `buildGitHubTimeline`, `extractGitHubCommentContext`
  - `checkIfGitHubCommentIsByBot`
- **Move**: GitHub posting functions (PRESERVE EXACTLY)
  - `postGitHubCommentReaction`, `postGitHubCommentReply`
  - `generateAndPostGitHubResponse` (posting parts)
- **Move**: GitHub utilities
  - `findIntegrationTokenForGitHubRepo`
  - GitHub API helper types (`GitHubCommitInfo`, `GitHubCommentInfo`)
- **Implement**: Provider interface methods:
```go
func (g *GitHubProvider) FetchMRTimeline(mr UnifiedMergeRequest) (*UnifiedTimeline, error)
func (g *GitHubProvider) FetchCodeContext(comment UnifiedComment) (string, error)  
func (g *GitHubProvider) PostCommentReply(mr UnifiedMergeRequest, parentComment *UnifiedComment, response string) error
func (g *GitHubProvider) PostReviewComments(mr UnifiedMergeRequest, comments []UnifiedReviewComment) error
```
- **Verify**: GitHub provider can fetch all data and post responses

## Phase 4: Extract Bitbucket Provider

### 4.1 Create Bitbucket provider file
- **File**: `internal/api/bitbucket_provider.go`
- **Move**: All Bitbucket-specific types (PRESERVE EXACTLY)
  - `BitbucketWebhookPayload` through `BitbucketUserInfo`
  - All Bitbucket comment/PR/repository/branch types
  - `BitbucketReviewerChangeInfo`, `BitbucketBotUserInfo`
- **Move**: Bitbucket webhook handler
  - `BitbucketWebhookHandler`
- **Verify**: All Bitbucket types compile independently

### 4.2 Enhance Bitbucket conversion methods
- **Move**: Existing conversion function (PRESERVE EXACTLY)
  - `convertBitbucketToUnifiedComment`
- **Create**: Missing Bitbucket conversions:
```go
func (b *BitbucketProvider) ConvertCommentEvent(payload interface{}) (*UnifiedWebhookEvent, error)
func (b *BitbucketProvider) ConvertReviewerEvent(payload interface{}) (*UnifiedWebhookEvent, error)
func (b *BitbucketProvider) convertBitbucketPRToUnified(pr BitbucketPullRequest) UnifiedMergeRequest  
func (b *BitbucketProvider) convertBitbucketUserToUnified(user BitbucketUser) UnifiedUser
func (b *BitbucketProvider) convertBitbucketRepoToUnified(repo BitbucketRepository) UnifiedRepository
```
- **Extract**: Data access patterns from current functions:
  - `processBitbucketReviewerChange` → reviewer change data requirements
  - `checkUnifiedAIResponseWarrant` → comment warrant checking data needs
  - `buildBitbucketContextualResponse` → MR context data requirements
- **Verify**: All Bitbucket webhook events convert to unified structures

### 4.3 Extract Bitbucket data fetching & posting  
- **Move**: Bitbucket API operations (PRESERVE EXACTLY)
  - `getFreshBitbucketBotUserInfo`
  - `fetchBitbucketPRCommits`, `fetchBitbucketPRComments`
  - `buildBitbucketTimeline`, `extractBitbucketCommentContext`
  - `checkIfBitbucketCommentIsByBot`
- **Move**: Bitbucket posting functions (PRESERVE EXACTLY)
  - `postBitbucketCommentReply`
  - `generateAndPostBitbucketResponse` (posting parts)
- **Move**: Bitbucket utilities
  - `findIntegrationTokenForBitbucketRepo`
  - Bitbucket API helper types (`BitbucketCommitInfo`, `BitbucketCommentInfo`)
- **Implement**: Provider interface methods:
```go
func (b *BitbucketProvider) FetchMRTimeline(mr UnifiedMergeRequest) (*UnifiedTimeline, error)
func (b *BitbucketProvider) FetchCodeContext(comment UnifiedComment) (string, error)
func (b *BitbucketProvider) PostCommentReply(mr UnifiedMergeRequest, parentComment *UnifiedComment, response string) error  
func (b *BitbucketProvider) PostReviewComments(mr UnifiedMergeRequest, comments []UnifiedReviewComment) error
```
- **Verify**: Bitbucket provider can fetch all data and post responses

## Phase 5: Create Unified Processing Core

### 5.1 Create unified processor (provider-agnostic)
- **File**: `internal/api/unified_processor.go`
- **Move**: Response warrant checking (PRESERVE EXACTLY)
  - `checkUnifiedAIResponseWarrant` 
  - `classifyContentType`, `classifyReplyContentType`
  - `determineResponseType`, `determineReplyResponseType`
- **Move**: Comment reply processing (PRESERVE EXACTLY)
  - `buildContextualAIResponse` logic (make provider-agnostic using UnifiedTimeline)
  - `synthesizeContextualResponse`
  - All `generate*Response` template functions (docs, error, performance, security, design, contextual)
  - `generateAIResponseFromPrompt`, `generateLLMResponse`, `generateStructuredFallbackResponse`
- **Create**: Full review processing (extract from existing `triggerReviewFor*` functions):
```go
func (p *UnifiedProcessor) ProcessFullReview(ctx context.Context, event UnifiedWebhookEvent, timeline *UnifiedTimeline) ([]UnifiedReviewComment, *LearningMetadata, error)
```
- **Verify**: Both comment reply and full review flows work provider-agnostically

### 5.2 Create unified context builder  
- **File**: `internal/api/unified_context.go`
- **Move**: Context building logic (PRESERVE EXACTLY, make provider-agnostic)
  - `buildTimeline` → work with `UnifiedTimeline` instead of provider-specific types
  - `extractCommentContext` → work with unified timeline items  
  - `findTargetComment` logic → work with unified comments
  - Timeline analysis and sorting
- **Move**: Prompt building (PRESERVE EXACTLY)
  - `buildGeminiPromptEnhanced` → unified version using `UnifiedComment`, `UnifiedPosition`
  - All prompt construction logic
  - Context analysis functions
- **Move**: Helper functions
  - `parseTimeBestEffort`, `shortSHA`, `firstNonEmpty`, `min`
  - `analyzeResponseType`
- **Verify**: Context building works with unified data from any provider

### 5.3 Create learning processor (provider-agnostic)
- **File**: `internal/api/learning_processor.go`  
- **Move**: Learning detection (PRESERVE EXACTLY)
  - `augmentResponseWithLearningMetadata`
  - Learning pattern detection logic from comment/response content
  - Metadata extraction patterns
- **Move**: Learning application (PRESERVE EXACTLY)
  - `applyLearningFromReply` 
  - Learning API integration
- **Create**: Provider-agnostic org resolution (generalize `findOrgIDForGitLabInstance`):
```go
func (l *LearningProcessor) FindOrgIDForRepository(repo UnifiedRepository) (int64, error)
```
- **Note**: Learning extraction happens AFTER response generation as part of response processing
- **Verify**: Learning extraction works with unified data from any provider

## Phase 6: Create Orchestrator & Refactor Main Handler

### 6.1 Create webhook orchestrator
- **File**: `internal/api/webhook_orchestrator.go`
- **Create**: Main orchestrator handling both flows:
```go
type WebhookOrchestrator struct {
    providers        map[string]WebhookProvider  // "gitlab", "github", "bitbucket"
    processor        UnifiedProcessor
    learningProcessor LearningProcessor
    db               *sql.DB
    server           *Server                     // For existing DB operations
}

// Main processing flows
func (o *WebhookOrchestrator) ProcessCommentEvent(provider string, payload interface{}) error
func (o *WebhookOrchestrator) ProcessReviewerEvent(provider string, payload interface{}) error
```

- **Implement**: Processing flows (PRESERVE current behavior exactly):

**Comment Reply Flow**:
1. `provider.ConvertCommentEvent(payload)` → `UnifiedWebhookEvent`
2. `provider.GetBotUserInfo(event.Repository)` → `UnifiedBotUserInfo`  
3. `processor.CheckResponseWarrant(event, botInfo)` → `(warranted, scenario)`
4. If warranted: `provider.FetchMRTimeline(event.MergeRequest)` → `UnifiedTimeline`
5. `processor.ProcessCommentReply(event, timeline)` → `(response, learning)`
6. `provider.PostCommentReply(event.MergeRequest, event.Comment, response)`
7. If learning: `learningProcessor.ApplyLearning(learning)`

**Full Review Flow** (when bot assigned as reviewer):
1. `provider.ConvertReviewerEvent(payload)` → `UnifiedWebhookEvent`
2. Check if bot assigned: `event.ReviewerChange.BotAssigned`
3. `provider.FetchMRTimeline(event.MergeRequest)` → `UnifiedTimeline`
4. `processor.ProcessFullReview(event, timeline)` → `(comments, learning)`
5. `provider.PostReviewComments(event.MergeRequest, comments)`
6. Track review in database (same as current `TrackReviewTriggered`)
7. If learning: `learningProcessor.ApplyLearning(learning)`

- **Verify**: Same async processing, error handling, logging as current implementation

### 6.2 Simplify webhook_handler.go
- **Keep**: Main `Server` struct and database operations (UNCHANGED)
- **Keep**: Exact same webhook endpoints (`/api/v1/gitlab/webhook`, etc.)
- **Replace**: Handler implementations with orchestrator calls:
```go
func (s *Server) GitLabWebhookHandler(c echo.Context) error {
    return s.orchestrator.ProcessWebhookEvent("gitlab", c)
}
```
- **Keep**: All existing database functions (`TrackReviewTriggered`, `TrackAICommentFromURL`, etc.)
- **Keep**: `getFirstAIConnector`, `getModelForProvider` (move to orchestrator)
- **Verify**: Same webhook URLs, same response codes, same behavior

### 6.3 Provider initialization & integration
- **Update**: Server initialization to create orchestrator:
```go
func NewServer(db *sql.DB) *Server {
    s := &Server{db: db}
    
    // Initialize providers with server dependency injection
    providers := map[string]WebhookProvider{
        "gitlab":    NewGitLabProvider(s),
        "github":    NewGitHubProvider(s), 
        "bitbucket": NewBitbucketProvider(s),
    }
    
    s.orchestrator = NewWebhookOrchestrator(providers, db, s)
    return s
}
```
- **Ensure**: Providers can access database through server reference (same as current)
- **Maintain**: Same integration token lookup, same AI connector access
- **Keep**: Same async patterns (`go func()` calls), same error handling
- **Verify**: All providers can access needed data and post responses

## Phase 7: Migration Strategy & Validation

### 7.1 Migration approach (no breaking changes)
- **Preserve**: Original webhook_handler.go as webhook_handler_backup.go
- **Phase-by-phase**: Each phase can be implemented and tested independently
- **Backwards compatibility**: All webhook endpoints must work throughout migration
- **Database**: No schema changes - use existing tables and operations
- **Async patterns**: Maintain same `go func()` patterns for review processing
- **Error handling**: Keep same error logging and response patterns

### 7.2 Validation per phase
**Phase 1-2**: GitLab provider
- **Test**: GitLab webhooks (reviewer assignment, comment replies) work identically
- **Verify**: Same database entries, same API calls, same responses
- **Check**: Logging output matches current format

**Phase 3**: GitHub provider  
- **Test**: GitHub webhooks work identically to current behavior
- **Verify**: Conversion to unified structures preserves all data
- **Check**: Response posting matches current formatting

**Phase 4**: Bitbucket provider
- **Test**: Bitbucket webhooks work identically to current behavior  
- **Verify**: All provider-specific quirks (auth, metadata) preserved
- **Check**: Comment threading and reply logic maintained

**Phase 5**: Unified processing
- **Critical**: LLM prompt building produces IDENTICAL prompts to current
- **Test**: Response generation templates produce same outputs
- **Verify**: Learning extraction patterns work across all providers
- **Check**: Context building (timeline, comment context) identical

**Phase 6**: Orchestrator integration
- **Test**: End-to-end flows for all providers work identically
- **Verify**: Database operations same as current (reviews, comments, learning)
- **Check**: Async processing maintains same timing and patterns
- **Validate**: Error scenarios handle gracefully

### 7.3 Success validation
- **Functional**: All webhook endpoints respond identically to current
- **Data**: Same database entries, same API calls, same file operations
- **Performance**: No regression in response times or memory usage
- **Logging**: Same log output for debugging and monitoring
- **Learning**: Learning generation and application works across providers
- **Review comments**: Full review flow produces same review comments as current

## Success Criteria

### Code Organization
- [ ] webhook_handler.go reduced to <500 lines (orchestrator calls only)
- [ ] Provider files each <1500 lines (gitlab_provider.go, github_provider.go, bitbucket_provider.go)
- [ ] Unified processing files each <1000 lines (unified_processor.go, unified_context.go, learning_processor.go)
- [ ] Clear layered architecture: Provider → Unified → Provider
- [ ] No circular dependencies, clean interface boundaries

### Functionality Preservation (CRITICAL)
- [ ] All webhook endpoints respond identically (/api/v1/gitlab/webhook, /api/v1/github/webhook, /api/v1/bitbucket/webhook)
- [ ] Comment reply flow: Same warrant checking, same LLM prompts, same responses, same learning
- [ ] Full review flow: Same reviewer detection, same full review generation, same comment posting
- [ ] Database operations identical: same tables, same queries, same data
- [ ] API calls identical: same external API requests to GitLab/GitHub/Bitbucket
- [ ] Async processing preserved: same goroutine patterns, same timing

### Architecture Improvements  
- [ ] Two clear flow types: Comment Reply Flow + Full Review Flow
- [ ] Provider layer: Platform-specific fetch/convert/post operations only
- [ ] Unified layer: Provider-agnostic LLM processing, context building, learning
- [ ] Data flow: Provider-specific → Unified structures → Provider-specific
- [ ] LLM prompt building completely unified and reusable
- [ ] Learning generation works across all providers uniformly
- [ ] Easy to add new providers by implementing WebhookProvider interface

### Data Structure Coverage
- [ ] UnifiedWebhookEvent covers all current webhook event types
- [ ] UnifiedMergeRequest covers all MR/PR data accessed in current code
- [ ] UnifiedComment covers all comment data accessed (inline, discussion, system)
- [ ] UnifiedTimeline covers all timeline building requirements (commits, comments, changes)
- [ ] UnifiedBotUserInfo covers all bot detection and API requirements
- [ ] Provider conversion methods handle all edge cases from current code

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
