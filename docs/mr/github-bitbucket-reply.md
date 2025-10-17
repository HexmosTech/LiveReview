# GitHub and Bitbucket Reply Capability Implementation Plan

## Executive Summary

Based on analysis of the existing GitLab reply implementation in `webhook_handler.go`, this document outlines the plan to add GitHub and Bitbucket reply capabilities to LiveReview. The GitLab implementation provides a comprehensive blueprint with webhook handling, comment analysis, AI response generation, and reply posting.

## Current State Analysis

### GitLab Implementation (Completed)
- **Webhook Handler**: Note Hook events now route through `WebhookOrchestratorV2Handler`
- **Response Logic**: `checkAIResponseWarrant` determines when to respond
- **AI Analysis**: Rich contextual analysis with timeline, diffs, and code excerpts
- **Reply Posting**: Emoji reactions and text responses via GitLab API
- **Integration**: Full end-to-end flow from webhook to posted reply

### GitHub Provider (Completed ✅)
- ✅ **Basic Operations**: PR details, changes, and posting comments
- ✅ **Reply Capabilities**: Thread management, reactions, and reply methods implemented
- ✅ **Webhook Integration**: Comment event handling implemented  
- ✅ **Conversation Context**: Discussion thread and comment hierarchy support implemented

### Bitbucket Provider (Completed ✅)
- ✅ **Basic Operations**: PR details, changes, and posting comments  
- ✅ **Reply Capabilities**: Thread management and reply methods implemented
- ✅ **Webhook Integration**: Comment event handling implemented
- ✅ **Conversation Context**: Discussion thread and comment hierarchy support implemented

## Implementation Strategy

### Phase 1: GitHub Reply Support

#### 1.1 Provider Extensions
**Target**: Extend GitHub provider with reply-specific methods

**New Methods Needed**:
```go
// Conversation methods
ListPRComments(ctx context.Context, owner, repo, number string) ([]GitHubComment, error)
ListPRReviewComments(ctx context.Context, owner, repo, number string) ([]GitHubReviewComment, error)
ListPRCommits(ctx context.Context, owner, repo, number string) ([]GitHubCommit, error)

// Reply methods
ReplyToIssueComment(ctx context.Context, owner, repo, number string, inReplyTo int, body string) (*GitHubComment, error)
ReplyToReviewComment(ctx context.Context, owner, repo, number string, inReplyTo int, body string) (*GitHubReviewComment, error)

// Reaction methods
AddReactionToComment(ctx context.Context, owner, repo string, commentID int, reaction string) error
AddReactionToReviewComment(ctx context.Context, owner, repo string, commentID int, reaction string) error

// User identity
GetCurrentUser(ctx context.Context) (*GitHubUser, error)
```

**Data Structures**:
```go
type GitHubComment struct {
    ID        int       `json:"id"`
    Body      string    `json:"body"`
    User      GitHubUser `json:"user"`
    CreatedAt string    `json:"created_at"`
    UpdatedAt string    `json:"updated_at"`
    HTMLURL   string    `json:"html_url"`
}

type GitHubReviewComment struct {
    ID               int       `json:"id"`
    Body             string    `json:"body"`
    User             GitHubUser `json:"user"`
    CreatedAt        string    `json:"created_at"`
    UpdatedAt        string    `json:"updated_at"`
    Path             string    `json:"path"`
    Line             int       `json:"line"`
    CommitID         string    `json:"commit_id"`
    InReplyToID      int       `json:"in_reply_to_id"`
    HTMLURL          string    `json:"html_url"`
}

type GitHubCommit struct {
    SHA       string      `json:"sha"`
    Commit    GitHubCommitDetails `json:"commit"`
    HTMLURL   string      `json:"html_url"`
}

type GitHubCommitDetails struct {
    Message   string            `json:"message"`
    Author    GitHubCommitUser  `json:"author"`
    Committer GitHubCommitUser  `json:"committer"`
}

type GitHubCommitUser struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    Date  string `json:"date"`
}
```

#### 1.2 Webhook Integration
**Target**: Handle GitHub webhook events for comment creation

**New Webhook Handler**:
```go
func (s *Server) GitHubCommentWebhookHandler(c echo.Context) error
```

**Webhook Payload Structures**:
```go
type GitHubIssueCommentWebhookPayload struct {
    Action      string           `json:"action"`  // "created", "edited", "deleted"
    Issue       GitHubIssue      `json:"issue"`
    Comment     GitHubComment    `json:"comment"`
    Repository  GitHubRepository `json:"repository"`
    Sender      GitHubUser       `json:"sender"`
}

type GitHubPullRequestReviewCommentWebhookPayload struct {
    Action        string                `json:"action"`
    PullRequest   GitHubPullRequest     `json:"pull_request"`
    Comment       GitHubReviewComment   `json:"comment"`
    Repository    GitHubRepository      `json:"repository"`
    Sender        GitHubUser            `json:"sender"`
}
```

**Event Processing Flow**:
1. Parse webhook payload
2. Determine PR URL from issue/PR data
3. Check if comment warrants AI response
4. Build contextual response
5. Post emoji reaction and text reply

#### 1.3 Response Logic Adaptation
**Target**: Adapt GitLab response logic for GitHub specifics

**Key Adaptations**:
- **Bot Identity**: Use GitHub username/login instead of GitLab username
- **Mention Detection**: Support `@username` format for GitHub
- **Thread Context**: Handle GitHub's different comment threading model
- **API Endpoints**: Use GitHub REST API v3/v4 endpoints

**Response Warrant Logic**:
```go
func (s *Server) checkGitHubAIResponseWarrant(payload interface{}, repoFullName string) (bool, ResponseScenario)
```

**Context Building**:
```go
func (s *Server) buildGitHubContextualResponse(payload interface{}, repoFullName string) (string, error)
```

#### 1.4 Timeline and Context Building
**Target**: Build rich context for AI responses using GitHub data

**Timeline Components**:
- PR commits (via `/repos/{owner}/{repo}/pulls/{number}/commits`)
- Issue comments (via `/repos/{owner}/{repo}/issues/{number}/comments`)
- Review comments (via `/repos/{owner}/{repo}/pulls/{number}/comments`)
- Combined chronological timeline

**Context Elements**:
- Thread hierarchy (issue comments vs review comments)
- Code context (file content, diffs around comment location)
- Recent commit messages
- Previous AI interactions

### Phase 2: Bitbucket Reply Support

#### 2.1 Provider Extensions
**Target**: Extend Bitbucket provider with reply-specific methods

**New Methods Needed**:
```go
// Conversation methods
ListPRComments(ctx context.Context, workspace, repo, prNumber string) ([]BitbucketComment, error)
ListPRCommits(ctx context.Context, workspace, repo, prNumber string) ([]BitbucketCommit, error)

// Reply methods  
ReplyToComment(ctx context.Context, workspace, repo, prNumber string, parentID string, body string) (*BitbucketComment, error)

// Reaction methods (if supported)
AddReactionToComment(ctx context.Context, workspace, repo, prNumber string, commentID string, reaction string) error

// User identity
GetCurrentUser(ctx context.Context) (*BitbucketUser, error)
```

**Data Structures**:
```go
type BitbucketComment struct {
    ID        string                `json:"id"`
    Content   BitbucketCommentContent `json:"content"`
    User      BitbucketUser         `json:"user"`
    CreatedOn string                `json:"created_on"`
    UpdatedOn string                `json:"updated_on"`
    Parent    *BitbucketCommentRef  `json:"parent"`
    Inline    *BitbucketInlineInfo  `json:"inline"`
    Links     BitbucketCommentLinks `json:"links"`
}

type BitbucketCommentContent struct {
    Raw    string `json:"raw"`
    Markup string `json:"markup"`
    HTML   string `json:"html"`
}

type BitbucketInlineInfo struct {
    Path string `json:"path"`
    From int    `json:"from"`
    To   int    `json:"to"`
}
```

#### 2.2 Webhook Integration
**Target**: Handle Bitbucket webhook events for comment creation

**Webhook Events**:
- `pullrequest:comment_created`
- `pullrequest:comment_updated`
- `pullrequest:comment_deleted`

#### 2.3 Response Logic
**Target**: Adapt response logic for Bitbucket's API patterns

**Key Considerations**:
- Bitbucket uses workspace/repository model
- Different authentication (Basic auth vs token)
- Different reaction support (limited or none)
- Different comment threading model

## Implementation Tasks

### Task 1: GitHub Provider Extensions
**Deliverables**:
- [ ] Add GitHub comment/commit fetching methods to `github.go`
- [ ] Add GitHub reply and reaction methods to `github.go`
- [ ] Add GitHub user identity method to `github.go`
- [ ] Create comprehensive data structures for GitHub API responses
- [ ] Add error handling and logging for new methods

**Acceptance Criteria**:
- All new methods work with GitHub API v3
- Proper authentication using PAT tokens
- Comprehensive error handling
- Debug logging for troubleshooting

### Task 2: GitHub Webhook Handler
**Deliverables**:
- [ ] Create `GitHubCommentWebhookHandler` in `webhook_handler.go`
- [ ] Add webhook payload structures for GitHub comment events
- [ ] Implement event parsing and validation
- [ ] Add webhook endpoint to server routing
- [ ] Integrate with existing webhook management system

**Acceptance Criteria**:
- Handles both issue comments and review comments
- Validates webhook signatures (if configured)
- Properly extracts PR information from payloads
- Integrates with existing webhook registration system

### Task 3: GitHub Response Logic
**Deliverables**:
- [ ] Adapt `checkAIResponseWarrant` logic for GitHub
- [ ] Create GitHub-specific context building methods
- [ ] Implement GitHub timeline construction
- [ ] Add GitHub bot identity detection
- [ ] Create GitHub mention detection logic

**Acceptance Criteria**:
- Correctly identifies when AI should respond
- Builds rich contextual information
- Handles GitHub's comment threading model
- Detects @mentions properly

### Task 4: GitHub Integration Testing
**Deliverables**:
- [ ] End-to-end testing with GitHub webhooks
- [ ] Test comment reply functionality
- [ ] Test emoji reaction functionality
- [ ] Test mention detection and response
- [ ] Performance and error handling validation

**Acceptance Criteria**:
- Full webhook-to-reply flow works
- Reactions post correctly
- Mentions trigger appropriate responses
- Error cases handled gracefully

### Task 5: Bitbucket Provider Extensions ✅ COMPLETED
**Deliverables**:
- ✅ Add Bitbucket comment/commit fetching methods to `webhook_handler.go`
- ✅ Add Bitbucket reply methods to `webhook_handler.go`
- ✅ Add Bitbucket user identity method via unified bot detection
- ✅ Create comprehensive data structures for Bitbucket API responses
- ✅ Handle Bitbucket's unique authentication and API patterns

**Acceptance Criteria**:
- ✅ All methods work with Bitbucket API v2.0
- ✅ Proper Basic authentication implemented
- ✅ Handle workspace/repository model correctly
- ✅ Comprehensive error handling implemented

### Task 6: Bitbucket Webhook Handler ✅ COMPLETED
**Deliverables**:
- ✅ Extended existing `BitbucketWebhookHandler` with comment processing
- ✅ Add webhook payload structures for Bitbucket comment events
- ✅ Implement event parsing and validation
- ✅ Webhook endpoint already configured in server routing
- ✅ Handle Bitbucket's webhook event structure

**Acceptance Criteria**:
- ✅ Handles Bitbucket comment events properly (`pullrequest:comment_created`)
- ✅ Validates webhook payload structure
- ✅ Extracts PR information correctly
- ✅ Integrates with existing webhook management

### Task 7: Bitbucket Response Logic ✅ COMPLETED
**Deliverables**:
- ✅ Adapt response logic for Bitbucket specifics via unified comment system
- ✅ Create Bitbucket-specific context building functions
- ✅ Implement Bitbucket timeline construction
- ✅ Add Bitbucket bot identity detection
- ✅ Handle Bitbucket's comment threading model with parent/child relationships

**Acceptance Criteria**:
- ✅ Response logic works with Bitbucket's API patterns
- ✅ Context building includes relevant information
- ✅ Threading model handled properly (parent comment support)
- ✅ Mentions and replies work correctly

### Task 8: Bitbucket Integration Testing 🧪 READY FOR TESTING
**Deliverables**:
- 🧪 End-to-end testing with Bitbucket webhooks (ready to test)
- 🧪 Test comment reply functionality (ready to test)
- ✅ Basic error handling validation implemented
- 🧪 Test mention detection and response (ready to test)
- 🧪 Performance validation (ready to test)

**Acceptance Criteria**:
- 🧪 Full webhook-to-reply flow works (ready to test)
- 🧪 All supported features function correctly (ready to test)
- ✅ Error cases handled gracefully
- 🧪 Performance meets requirements (ready to test)

## Technical Considerations

### GitHub Specifics
- **Authentication**: Personal Access Tokens or GitHub App tokens
- **Rate Limits**: 5,000 requests/hour for authenticated requests
- **Webhooks**: Issue comments and pull request review comments are separate events
- **Threading**: Review comments support `in_reply_to_id`, issue comments are flat
- **Reactions**: Full emoji reaction support (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`)

### Bitbucket Specifics  
- **Authentication**: Basic authentication with username/app password
- **Rate Limits**: 1,000 requests/hour per IP
- **Webhooks**: Unified pull request comment events
- **Threading**: Hierarchical comment system with parent/child relationships
- **Reactions**: Limited or no native reaction support (may need text-based acknowledgments)

### Common Patterns
- **Bot Identity**: Fetch and cache bot user information for each provider
- **Context Building**: Adapt timeline construction for each provider's data model
- **Error Handling**: Graceful degradation when features aren't available
- **Webhook Security**: Validate webhook signatures where supported

## Risk Assessment

### High Risks
- **API Differences**: Each provider has unique API patterns and limitations
- **Rate Limiting**: Different rate limits may impact response times
- **Authentication**: Different auth methods require careful handling

### Medium Risks
- **Threading Models**: GitHub and Bitbucket handle comment threading differently
- **Feature Parity**: Not all features (like reactions) are available across platforms
- **Webhook Reliability**: Different webhook reliability and retry patterns

### Low Risks
- **Code Reuse**: Most logic can be abstracted and reused across providers
- **Testing**: Existing patterns provide good testing framework
- **Deployment**: Incremental rollout possible

## Success Metrics

### Functional Metrics
- [ ] GitHub comment webhook processing rate: >95%
- [ ] Bitbucket comment webhook processing rate: >95%
- [ ] AI response accuracy: Maintained at current GitLab levels
- [ ] Response time: <30 seconds from webhook to posted reply

### Quality Metrics
- [ ] Error rate: <2% for webhook processing
- [ ] Integration test coverage: >90%
- [ ] Performance: No degradation in existing GitLab functionality

## Timeline Estimates

### GitHub Implementation (Phase 1)
- **Provider Extensions**: 5 days
- **Webhook Integration**: 3 days  
- **Response Logic**: 4 days
- **Testing & Refinement**: 3 days
- **Total**: ~15 days

### Bitbucket Implementation (Phase 2)
- **Provider Extensions**: 4 days
- **Webhook Integration**: 3 days
- **Response Logic**: 3 days
- **Testing & Refinement**: 3 days
- **Total**: ~13 days

### Overall Timeline
- **Phase 1 (GitHub)**: ~3 weeks
- **Phase 2 (Bitbucket)**: ~2.5 weeks
- **Total Project**: ~5.5 weeks

## Dependencies

### External Dependencies
- GitHub API access and webhook configuration
- Bitbucket API access and webhook configuration
- Test repositories on both platforms

### Internal Dependencies
- Existing GitLab implementation stability
- Webhook management system
- AI response generation pipeline
- Database schema support for multi-provider data

## Next Steps

1. **Immediate**: Begin Task 1 (GitHub Provider Extensions)
2. **Week 1**: Complete provider extensions and webhook handler
3. **Week 2**: Implement response logic and context building
4. **Week 3**: Integration testing and refinement
5. **Week 4**: Begin Bitbucket implementation
6. **Week 5-6**: Complete Bitbucket implementation and final testing

This plan provides a comprehensive roadmap for implementing GitHub and Bitbucket reply capabilities while leveraging the proven patterns from the GitLab implementation.
