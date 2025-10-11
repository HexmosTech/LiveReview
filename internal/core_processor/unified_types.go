package core_processor

// Phase 1.2: Comprehensive unified types with V2 naming to avoid conflicts
// All types use V2 suffix to prevent redeclaration errors with existing webhook_handler.go

// UnifiedWebhookEventV2 - Top level event container for all webhook types
type UnifiedWebhookEventV2 struct {
	EventType      string // "comment_created", "reviewer_assigned", "mr_updated"
	Provider       string // "gitlab", "github", "bitbucket"
	Timestamp      string
	Repository     UnifiedRepositoryV2
	MergeRequest   *UnifiedMergeRequestV2   // For MR events
	Comment        *UnifiedCommentV2        // For comment events
	ReviewerChange *UnifiedReviewerChangeV2 // For reviewer assignment events
	Actor          UnifiedUserV2            // User who triggered the event
}

// UnifiedMergeRequestV2 - Enhanced MR/PR representation covering all providers
type UnifiedMergeRequestV2 struct {
	ID           string
	Number       int // For display (IID/Number)
	Title        string
	Description  string
	State        string
	Author       UnifiedUserV2
	SourceBranch string
	TargetBranch string
	WebURL       string
	CreatedAt    string
	UpdatedAt    string
	Reviewers    []UnifiedUserV2
	Assignees    []UnifiedUserV2
	Labels       []string
	Metadata     map[string]interface{} // Provider-specific data
}

// UnifiedCommentV2 - Enhanced comment representation covering all comment types
type UnifiedCommentV2 struct {
	ID           string
	Body         string
	Author       UnifiedUserV2
	CreatedAt    string
	UpdatedAt    string
	WebURL       string
	InReplyToID  *string
	Position     *UnifiedPositionV2     // For inline code comments
	DiscussionID *string                // Thread/discussion ID
	System       bool                   // System vs user comment
	Metadata     map[string]interface{} // Provider-specific data
}

// UnifiedUserV2 - User representation across all providers
type UnifiedUserV2 struct {
	ID        string
	Username  string
	Name      string
	Email     string
	AvatarURL string
	WebURL    string
	Metadata  map[string]interface{} // Provider-specific data
}

// UnifiedRepositoryV2 - Repository representation across all providers
type UnifiedRepositoryV2 struct {
	ID            string
	Name          string
	FullName      string
	WebURL        string
	CloneURL      string
	DefaultBranch string
	Owner         UnifiedUserV2
	Metadata      map[string]interface{} // Provider-specific data
}

// UnifiedPositionV2 - Code position for inline comments
type UnifiedPositionV2 struct {
	FilePath   string
	LineNumber int
	LineType   string // "old", "new", "context"
	StartLine  *int
	EndLine    *int
	Metadata   map[string]interface{} // Provider-specific data
}

// UnifiedBotUserInfoV2 - Bot user information for warrant checking
type UnifiedBotUserInfoV2 struct {
	UserID      string
	Username    string
	Name        string
	IsBot       bool
	Permissions []string
	Metadata    map[string]interface{} // Provider-specific data
}

// UnifiedReviewerChangeV2 - Reviewer assignment/removal events
type UnifiedReviewerChangeV2 struct {
	Action            string // "added", "removed"
	CurrentReviewers  []UnifiedUserV2
	PreviousReviewers []UnifiedUserV2
	BotAssigned       bool
	BotRemoved        bool
	ChangedBy         UnifiedUserV2
}

// UnifiedCommitV2 - Commit information for timeline building
type UnifiedCommitV2 struct {
	SHA       string
	Message   string
	Author    UnifiedCommitAuthorV2
	Timestamp string
	WebURL    string
}

// UnifiedCommitAuthorV2 - Commit author information
type UnifiedCommitAuthorV2 struct {
	Name  string
	Email string
}

// UnifiedTimelineV2 - Timeline container for context building
type UnifiedTimelineV2 struct {
	Items []UnifiedTimelineItemV2
}

// UnifiedTimelineItemV2 - Individual timeline item
type UnifiedTimelineItemV2 struct {
	Type         string // "commit", "comment", "review_change"
	Timestamp    string
	Commit       *UnifiedCommitV2
	Comment      *UnifiedCommentV2
	ReviewChange *UnifiedReviewerChangeV2
}

// ResponseScenarioV2 - Response scenarios for warrant checking
type ResponseScenarioV2 struct {
	Type       string
	Reason     string
	Confidence float64
	Metadata   map[string]interface{}
}

// AIConnectorV2 - AI connector configuration
type AIConnectorV2 struct {
	ID          int64
	Name        string
	Provider    string
	Model       string
	APIKey      string
	BaseURL     string
	Temperature float64
	MaxTokens   int
	Active      bool
	CreatedAt   string
	UpdatedAt   string
}

// LearningMetadataV2 - Learning extraction metadata
type LearningMetadataV2 struct {
	Type       string
	Content    string
	Context    string
	Confidence float64
	Tags       []string
	OrgID      int64
	Metadata   map[string]interface{}
}

// CommentContextV2 - Context information for comments
type CommentContextV2 struct {
	MRContext       UnifiedMRContextV2
	Timeline        UnifiedTimelineV2
	CodeContext     string
	RelatedComments []UnifiedCommentV2
	Metadata        map[string]interface{}
}

// UnifiedMRContextV2 - MR context for processing
type UnifiedMRContextV2 struct {
	Repository   UnifiedRepositoryV2
	MergeRequest UnifiedMergeRequestV2
	SourceBranch string
	TargetBranch string
	Changes      []UnifiedFileChangeV2
	Metadata     map[string]interface{}
}

// UnifiedFileChangeV2 - File change information
type UnifiedFileChangeV2 struct {
	FilePath   string
	ChangeType string // "added", "modified", "deleted"
	Additions  int
	Deletions  int
	Patch      string
	Metadata   map[string]interface{}
}

// UnifiedReviewCommentV2 - Review comment for full review flow
type UnifiedReviewCommentV2 struct {
	FilePath   string
	LineNumber int
	Content    string
	Severity   string
	Category   string
	Position   *UnifiedPositionV2
	Metadata   map[string]interface{}
}
