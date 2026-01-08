package gitea

import "time"

// GiteaV2WebhookPayload represents the complete Gitea webhook payload structure
// Reference: https://docs.gitea.com/usage/webhooks
type GiteaV2WebhookPayload struct {
	Secret      string              `json:"secret,omitempty"`
	Ref         string              `json:"ref,omitempty"`
	Before      string              `json:"before,omitempty"`
	After       string              `json:"after,omitempty"`
	Action      string              `json:"action,omitempty"` // opened, closed, reopened, edited, assigned, unassigned, etc.
	Number      int                 `json:"number,omitempty"`
	PullRequest *GiteaV2PullRequest `json:"pull_request,omitempty"`
	Issue       *GiteaV2Issue       `json:"issue,omitempty"`
	Comment     *GiteaV2Comment     `json:"comment,omitempty"`
	Review      *GiteaV2Review      `json:"review,omitempty"`
	Repository  *GiteaV2Repository  `json:"repository,omitempty"`
	Sender      *GiteaV2User        `json:"sender,omitempty"`
}

// GiteaV2PullRequest represents a pull request in Gitea webhooks
type GiteaV2PullRequest struct {
	ID        int64            `json:"id"`
	Number    int64            `json:"number"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	User      *GiteaV2User     `json:"user,omitempty"`
	State     string           `json:"state"` // open, closed
	Merged    bool             `json:"merged"`
	MergedAt  *time.Time       `json:"merged_at,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	ClosedAt  *time.Time       `json:"closed_at,omitempty"`
	HTMLURL   string           `json:"html_url"`
	DiffURL   string           `json:"diff_url"`
	PatchURL  string           `json:"patch_url"`
	Head      *GiteaV2PRBranch `json:"head,omitempty"`
	Base      *GiteaV2PRBranch `json:"base,omitempty"`
	Assignees []*GiteaV2User   `json:"assignees,omitempty"`
	Labels    []*GiteaV2Label  `json:"labels,omitempty"`
}

// GiteaV2Issue represents an issue in Gitea webhooks
// Issues and PRs share similar structure in Gitea
type GiteaV2Issue struct {
	ID          int64           `json:"id"`
	Number      int64           `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	User        *GiteaV2User    `json:"user,omitempty"`
	State       string          `json:"state"` // open, closed
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
	HTMLURL     string          `json:"html_url"`
	PullRequest *GiteaV2PRRef   `json:"pull_request,omitempty"` // Present if issue is actually a PR
	Assignees   []*GiteaV2User  `json:"assignees,omitempty"`
	Labels      []*GiteaV2Label `json:"labels,omitempty"`
}

// GiteaV2PRRef indicates if an issue is a pull request
type GiteaV2PRRef struct {
	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"merged_at,omitempty"`
}

// GiteaV2PRBranch represents the head or base branch of a PR
type GiteaV2PRBranch struct {
	Ref  string             `json:"ref"`
	SHA  string             `json:"sha"`
	Repo *GiteaV2Repository `json:"repo,omitempty"`
}

// GiteaV2Comment represents a comment in Gitea webhooks
// Used for issue comments, PR comments, and review comments
type GiteaV2Comment struct {
	ID             int64        `json:"id"`
	HTMLURL        string       `json:"html_url"`
	PullRequestURL string       `json:"pull_request_url,omitempty"`
	IssueURL       string       `json:"issue_url,omitempty"`
	User           *GiteaV2User `json:"user,omitempty"`
	Body           string       `json:"body"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	// Fields for code review comments (inline comments)
	Path             string `json:"path,omitempty"`
	CommitID         string `json:"commit_id,omitempty"`
	OriginalCommitID string `json:"original_commit_id,omitempty"`
	DiffHunk         string `json:"diff_hunk,omitempty"`
	Position         int    `json:"position,omitempty"`
	OriginalPosition int    `json:"original_position,omitempty"`
	Line             int    `json:"line,omitempty"`
	Side             string `json:"side,omitempty"` // "LEFT" or "RIGHT"
	StartLine        int    `json:"start_line,omitempty"`
	StartSide        string `json:"start_side,omitempty"`
	// Threading
	InReplyTo int64 `json:"in_reply_to,omitempty"`
	// Review context
	ReviewID int64 `json:"review_id,omitempty"`
}

// GiteaV2Review represents a pull request review
type GiteaV2Review struct {
	ID        int64        `json:"id"`
	Type      string       `json:"type"` // APPROVE, REQUEST_CHANGES, COMMENT
	Content   string       `json:"content"`
	User      *GiteaV2User `json:"user,omitempty"`
	State     string       `json:"state"`
	CommitID  string       `json:"commit_id"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	HTMLURL   string       `json:"html_url"`
	Official  bool         `json:"official"`
}

// GiteaV2Repository represents repository information in webhooks
type GiteaV2Repository struct {
	ID            int64        `json:"id"`
	Name          string       `json:"name"`
	FullName      string       `json:"full_name"` // owner/repo
	Owner         *GiteaV2User `json:"owner,omitempty"`
	Private       bool         `json:"private"`
	Fork          bool         `json:"fork"`
	HTMLURL       string       `json:"html_url"`
	CloneURL      string       `json:"clone_url"`
	DefaultBranch string       `json:"default_branch"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// GiteaV2User represents a user in Gitea webhooks
type GiteaV2User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"` // Some webhooks use 'username' instead of 'login'
	HTMLURL   string `json:"html_url,omitempty"`
}

// GiteaV2Label represents a label on an issue or PR
type GiteaV2Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// GiteaV2Commit represents commit information in webhooks
type GiteaV2Commit struct {
	ID        string         `json:"id"`
	Message   string         `json:"message"`
	URL       string         `json:"url"`
	Author    *GiteaV2Author `json:"author,omitempty"`
	Committer *GiteaV2Author `json:"committer,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// GiteaV2Author represents commit author/committer information
type GiteaV2Author struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

// API Response types (different from webhook payloads)

// GiteaIssueComment represents an issue comment from the API
type GiteaIssueComment struct {
	ID             int64        `json:"id"`
	HTMLURL        string       `json:"html_url"`
	PullRequestURL string       `json:"pull_request_url,omitempty"`
	IssueURL       string       `json:"issue_url,omitempty"`
	User           *GiteaV2User `json:"user"`
	Body           string       `json:"body"`
	CreatedAt      string       `json:"created_at"`
	UpdatedAt      string       `json:"updated_at"`
}

// GiteaReview represents a review from the API
type GiteaReview struct {
	ID            int64        `json:"id"`
	User          *GiteaV2User `json:"user"`
	State         string       `json:"state"` // COMMENT, APPROVED, REQUEST_CHANGES
	Body          string       `json:"body"`
	CommitID      string       `json:"commit_id"`
	CommentsCount int          `json:"comments_count"`
	SubmittedAt   string       `json:"submitted_at"`
	UpdatedAt     string       `json:"updated_at"`
	HTMLURL       string       `json:"html_url"`
}

// GiteaReviewComment represents a review comment from the API
type GiteaReviewComment struct {
	ID        int64        `json:"id"`
	HTMLURL   string       `json:"html_url"`
	User      *GiteaV2User `json:"user"`
	Body      string       `json:"body"`
	Path      string       `json:"path"`
	Line      int          `json:"line"`
	Side      string       `json:"side"` // LEFT or RIGHT
	CommitID  string       `json:"commit_id"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
}
